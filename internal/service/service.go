package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sou1burn/GsmLoraServer/internal/core"
	"github.com/sou1burn/GsmLoraServer/internal/esp"
	"github.com/sou1burn/GsmLoraServer/internal/handlers"
	"github.com/sou1burn/GsmLoraServer/internal/models"
)

const measurementTimeout = 15 * time.Second

type measurementBuffer struct {
	packets  []*models.Packet
	deadline time.Time
}

type PendingMessage struct {
	Type      handlers.Message
	Message   string
	Timestamp time.Time
}

type Repository interface {
	Save(ctx context.Context, packet *models.Packet, sessionId int) error
	SaveSession(ctx context.Context, session models.Session) error
	RemoveSession(ctx context.Context, sessionId int) error
	GetAllSessions(ctx context.Context) ([]models.Session, error)
	FindBySession(ctx context.Context, sessionId int) ([]models.Packet, error)
	SessionHasPackets(ctx context.Context, sessionId int) (bool, error)
	AddSettingsToSession(ctx context.Context, sessionId int, sf, tx, bw float32) (written bool, err error)
}

var pendingOrder = []reflect.Type{
	reflect.TypeOf(handlers.SetSettingsMessage{}),
	reflect.TypeOf(handlers.StartMeasurementMessage{}),
}

type DataService struct {
	ctx                 context.Context
	packetChan          chan *models.Packet
	Repo                Repository
	mu                  sync.RWMutex
	espConnector        *esp.ESPConnector
	clients             map[net.Conn]struct{}
	PendingMessages     map[reflect.Type]*PendingMessage
	pendingMessagesLock sync.Mutex
}

func NewDataService(connector *esp.ESPConnector) *DataService {
	service := &DataService{
		ctx:             context.Background(),
		packetChan:      make(chan *models.Packet, 100),
		Repo:            nil,
		espConnector:    connector,
		clients:         make(map[net.Conn]struct{}),
		PendingMessages: make(map[reflect.Type]*PendingMessage, 5),
	}
	return service
}

func (s *DataService) StartProcessing(ctx context.Context) {
	s.ctx = ctx
	go s.processPackets()
	go s.startPendingProcessing()
}

func (s *DataService) GetChannelStatus() (int, int) {
	return len(s.packetChan), cap(s.packetChan)
}

func (s *DataService) GetChannelUsage() float64 {
	current, capacity := s.GetChannelStatus()
	return float64(current) / float64(capacity) * 100
}

func (s *DataService) ProcessPacketData(packet models.Packet) error {
	if packet.PacketNum < 1 || packet.PacketNum > 10 {
		log.Printf("Invalid packet_num %d for measurement_id %d — packet dropped", packet.PacketNum, packet.MeasurementId)
		return nil
	}
	log.Printf("Processing packet: %v", packet)
	select {
	case s.packetChan <- &packet:
	default:
		fmt.Printf("Channel full, saving data and starting channel drain: %+v\n", packet)
		err := s.Repo.Save(context.WithoutCancel(s.ctx), &packet, s.espConnector.GetCurrentSession())
		if err != nil {
			log.Printf("Unexpected error while saving data: %v", err)
		}
		go func() {
			err := s.DrainDataChannel()
			if err != nil {
				log.Printf("Error during channel drain: %v", err)
			}
		}()
	}
	log.Printf("Data channel usage :%f", s.GetChannelUsage())

	return nil
}

func (s *DataService) DrainDataChannel() error {
	for {
		select {
		case packet := <-s.packetChan:
			drainCtx := context.WithoutCancel(s.ctx)
			err := s.Repo.Save(drainCtx, packet, s.espConnector.GetCurrentSession())
			if err != nil {
				log.Printf("Error saving packet during drain: %v", err)
			}
		default:
			log.Printf("Data channel drain completed")
			return nil
		}
	}
}

func (s *DataService) ProcessInterfaceSettingChange(message string) error {
	cleanedMessage := strings.TrimPrefix(message, "SET_SETTINGS: ")
	parts := strings.Split(cleanedMessage, ", ")

	if s.espConnector.GetConn() == nil || !s.espConnector.IsConnected() {
		return errors.New("ESP_NOT_CONNECTED")
	}

	params := models.Params{}

	for _, part := range parts {
		keyVal := strings.Split(part, "=")
		if len(keyVal) != 2 {
			continue
		}

		key := strings.TrimSpace(keyVal[0])
		value := strings.TrimSpace(keyVal[1])

		val, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %w", key, err)
		}

		switch key {
		case "SF":
			params.Sf = float32(val)
		case "TX":
			params.Tx = float32(val)
		case "BW":
			params.Bandwidth = float32(val)
		}
	}

	log.Printf("Got params: %+v\n", params)
	return s.espConnector.SendParamsToDevice(params)
}

func (s *DataService) processPackets() {
	buffers := make(map[int]*measurementBuffer)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	// Отмена контекста останавливает приём пакетов, но не запись уже
	// принятых (AUDIT.md №6): Save не должен падать на context canceled.
	saveCtx := context.WithoutCancel(s.ctx)
	for {
		select {
		case <-s.ctx.Done():
			return
		case packet, ok := <-s.packetChan:
			if !ok {
				return
			}
			err := s.Repo.Save(saveCtx, packet, s.espConnector.GetCurrentSession())
			if err != nil {
				log.Printf("Error saving packet during processing: %v", err)
			}
			buf, exists := buffers[packet.MeasurementId]
			if !exists {
				buf = &measurementBuffer{packets: make([]*models.Packet, 10)}
				buffers[packet.MeasurementId] = buf
			}
			buf.packets[packet.PacketNum-1] = packet
			buf.deadline = time.Now().Add(measurementTimeout)
			if isBufferFull(buf.packets) {
				s.sendAverage(buf.packets)
				delete(buffers, packet.MeasurementId)
			}
		case now := <-ticker.C:
			for id, buf := range buffers {
				if now.After(buf.deadline) {
					s.sendAverage(buf.packets)
					delete(buffers, id)
				}
			}
		}
	}
}

func (s *DataService) sendAverage(packets []*models.Packet) {
	if avg := calculateAverage(packets); avg != nil {
		s.SendPacketToClients(*avg)
	}
}

func isBufferFull(buffer []*models.Packet) bool {
	for i := 0; i < 10; i++ {
		if buffer[i] == nil {
			return false
		}
	}
	return true
}

func calculateAverage(packets []*models.Packet) *models.Packet {
	var sumRSSI, sumLat, sumLon, sumSNRL float64
	var sumHdop float32
	var last *models.Packet
	count := 0

	for _, p := range packets {
		if p == nil {
			continue
		}
		count++
		last = p
		sumRSSI += p.RSSI
		sumSNRL += p.SNRL
		sumLat += p.Coordinate.Latitude
		sumLon += p.Coordinate.Longitude
		sumHdop += p.Hdop
	}
	if count == 0 {
		return nil
	}

	return &models.Packet{
		MeasurementId: last.MeasurementId,
		PacketNum:     0,
		RSSI:          sumRSSI / float64(count),
		SNRL:          sumSNRL / float64(count),
		Coordinate: models.Coordinate{
			Latitude:  sumLat / float64(count),
			Longitude: sumLon / float64(count),
		},
		Hdop:      sumHdop / float32(count),
		Timestamp: last.Timestamp,
	}
}

func (s *DataService) processPendingMessages() {
	s.pendingMessagesLock.Lock()
	defer s.pendingMessagesLock.Unlock()
	for _, typ := range pendingOrder {
		pending, ok := s.PendingMessages[typ]
		if !ok {
			continue
		}
		var err error
		switch pending.Type.(type) {
		case handlers.SetSettingsMessage:
			err = s.ProcessInterfaceSettingChange(pending.Message)
		case handlers.StartMeasurementMessage:
			var msg handlers.Message
			msg, err = handlers.ParseClientMessage(pending.Message)
			if start, ok := msg.(handlers.StartMeasurementMessage); err == nil && ok {
				err = s.espConnector.SendCommand("START_MEASUREMENT", start.SessionId)
			}
		}
		if err != nil {
			log.Printf("Error processing pending %s: %v", pending.Type.Type(), err)
			continue
		}
		delete(s.PendingMessages, typ)
		log.Printf("Pending %s processed successfully", pending.Type.Type())
	}
}

func (s *DataService) SendAllSessions(conn net.Conn, data string) error {
	var err error

	resp := fmt.Sprintf("SESSIONS: %s\n", data)
	log.Printf("Sent message: %s", resp)
	_, err = conn.Write([]byte(resp))
	if err != nil {
		log.Println(fmt.Errorf("error sending message to client: %w", err))
		return err
	}
	log.Printf("Sent all sessions data")
	return nil
}

func (s *DataService) GetAllSessions(ctx context.Context) ([]models.Session, error) {
	sessions, err := s.Repo.GetAllSessions(ctx)
	if err != nil {
		log.Printf("Error getting all sessions: %v", err)
		return nil, err
	}
	return sessions, nil
}

func (s *DataService) SaveSession(ctx context.Context, session models.Session) error {
	err := s.Repo.SaveSession(ctx, session)
	if err != nil {
		log.Printf("Error saving session: %v", err)
		return err
	}
	return nil
}

func (s *DataService) RemoveSession(ctx context.Context, sessionId int) error {
	err := s.Repo.RemoveSession(ctx, sessionId)
	if err != nil {
		log.Printf("Error removing session: %v", err)
	}
	return err
}

func (s *DataService) AddClient(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.clients[conn]; exists {
		return
	}
	s.clients[conn] = struct{}{}
	log.Printf("Client added: %s. Total clients: %d", conn.RemoteAddr(), len(s.clients))
}

func (s *DataService) RemoveClient(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, conn)
}

func (s *DataService) Broadcast(msg string) error {
	s.mu.RLock()
	conns := make([]net.Conn, 0, len(s.clients))
	for conn := range s.clients {
		conns = append(conns, conn)
	}
	s.mu.RUnlock()

	var errs []error
	for _, conn := range conns {
		if _, err := conn.Write([]byte(msg + "\n")); err != nil {
			errs = append(errs, fmt.Errorf("write to %s: %w", conn.RemoteAddr(), err))
		}
	}
	return errors.Join(errs...)
}

func (s *DataService) SendPacketToClients(packet models.Packet) {
	message := fmt.Sprintf("MEASUREMENT: [%s, %f, %f, %f, %f, %f]", packet.Timestamp.Format(models.TimeLayout), packet.RSSI, packet.SNRL, packet.Hdop, packet.Coordinate.Latitude, packet.Coordinate.Longitude)
	if err := s.Broadcast(message); err != nil {
		log.Printf("Error sending message to client: %v", err)
		return
	}
	log.Printf("Message %s sent to clients", message)
}

func (s *DataService) AddPendingMessage(message string, messageType core.Message) {
	s.pendingMessagesLock.Lock()
	defer s.pendingMessagesLock.Unlock()

	msg := &PendingMessage{
		Type:      messageType,
		Message:   message,
		Timestamp: time.Now(),
	}

	msgType := reflect.TypeOf(messageType)

	if existing, exists := s.PendingMessages[msgType]; !exists ||
		msg.Timestamp.After(existing.Timestamp) {
		s.PendingMessages[msgType] = msg
		log.Printf("Added new pending message: %v", msg)
		return
	}
	log.Printf("Havent added pending message: %v", msg)
}

func (s *DataService) startPendingProcessing() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.PendingTick()
		}
	}
}

func (s *DataService) PendingTick() {
	if !s.espConnector.IsConnected() {
		return
	}
	s.processPendingMessages()
}

func (s *DataService) GetAllSessionMeasurements(ctx context.Context, sessionId int) ([]models.Packet, error) {
	packets, err := s.Repo.FindBySession(ctx, sessionId)
	if err != nil {
		log.Printf("Error getting all measurements for session: %v", err)
		return nil, err
	}

	return packets, nil
}

func (s *DataService) SendSessionPackets(sessionId int, packets []models.Packet) error {
	for _, packet := range packets {
		s.SendPacketToClients(packet)
	}
	return nil
}

func (s *DataService) SetSettingsForSession(ctx context.Context, params models.Params) error {
	session := s.espConnector.GetCurrentSession()

	written, err := s.Repo.AddSettingsToSession(ctx, session, params.Sf, params.Tx, params.Bandwidth)
	if err != nil {
		log.Printf("Error adding settings to session: %v", err)
		return err
	}
	if written {
		log.Printf("Added settings: %+v to session: %+v", params, session)
		return nil
	}

	hasPackets, err := s.Repo.SessionHasPackets(ctx, session)
	if err != nil {
		return err
	}
	if hasPackets {
		return errors.New("already set or already have packets for session")
	}
	return nil
}
