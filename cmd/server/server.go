package server

import (
	"Lora_Esp_Gsm_Gps_project/internal/core"
	"Lora_Esp_Gsm_Gps_project/internal/esp"
	"Lora_Esp_Gsm_Gps_project/internal/handlers"
	"Lora_Esp_Gsm_Gps_project/internal/models"
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const statusTimeout = 120 * time.Second

type DeviceStatus struct {
	LastUpdated time.Time
	Status      string
}

type Server struct {
	mutex                sync.Mutex
	measurementHandler   core.MeasurementHandler
	connector            esp.Connector
	waitList             []net.Conn
	waitListMutex        sync.RWMutex
	SlaveState           DeviceStatus
	MasterState          DeviceStatus
	isMeasurementStarted atomic.Bool
	monitorOnce          sync.Once
}

func NewServer(h core.MeasurementHandler, connector esp.Connector) *Server {
	return &Server{
		measurementHandler: h,
		connector:          connector,
		SlaveState:         DeviceStatus{},
		MasterState:        DeviceStatus{},
	}
}

func (s *Server) StartServer(port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", port, err)
	}
	return s.Serve(ln)
}

func (s *Server) Serve(ln net.Listener) error {
	defer ln.Close()
	log.Printf("Server started listening on %v", ln.Addr())
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			log.Printf("error accepting connection: %v", err)
			continue
		}
		go func() {
			if err := s.HandleConnection(conn); err != nil {
				log.Printf("Error handling connection: %v", err)
			}
		}()
	}
}

func (s *Server) HandleConnection(conn net.Conn) error {
	log.Printf("Client connected from %s", conn.RemoteAddr())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		s.measurementHandler.RemoveClient(conn)
		if s.connector.GetConn() == conn {
			s.connector.SetConnected(false)
			s.connector.SetConn(nil)
			log.Printf("ESP disconnected: %s", conn.RemoteAddr())
		}
		_ = conn.Close()
	}()
	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		message := scanner.Text()
		log.Printf("Got message: %s", message)
		parsedMsg, err := handlers.ParseClientMessage(message)

		if err != nil {
			log.Printf("Error parsing message: %v", err)
			s.replyError(conn, err)
			continue
		}

		log.Printf("Parsed message type: %T", parsedMsg)
		switch msg := parsedMsg.(type) {
		case handlers.SetSettingsMessage:
			s.isMeasurementStarted.Store(false)
			s.measurementHandler.AddClient(conn)
			if s.connector.IsConnected() {
				s.HandleSetSettings(conn, message)
			} else {
				s.addWaitingClient(conn)
				log.Printf("Client %s waiting for esp connection", conn.RemoteAddr())
				s.measurementHandler.AddPendingMessage(message, models.Lora, handlers.SetSettingsMessage{})
				go s.waitForEspConnectionLogged(conn)
			}

		case handlers.StartMeasurementMessage:
			s.isMeasurementStarted.Store(true)
			s.measurementHandler.AddClient(conn)
			if s.connector.IsConnected() {
				s.handleStartMeasurement(ctx, conn, msg.SessionId)
			} else {
				s.addWaitingClient(conn)
				log.Printf("Client %s waiting for esp connection", conn.RemoteAddr())
				s.measurementHandler.AddPendingMessage(message, models.Lora, handlers.StartMeasurementMessage{})
				go s.waitForEspConnectionLogged(conn)
			}

		case handlers.StopMeasurementMessage: // command unused
			return nil
		case handlers.GetMeasurementSessionsMessage: //esp not used here
			s.measurementHandler.AddClient(conn)
			s.HandleGetMeasurementSessions(ctx, conn)
			s.mutex.Lock()
			master, slave := s.MasterState.Status, s.SlaveState.Status
			s.mutex.Unlock()
			if master != "" {
				s.write(conn, "MASTER_STATUS "+master)
			}
			if slave != "" {
				s.write(conn, "SLAVE_STATUS "+slave)
			}

		case handlers.AddSessionMessage: //esp not used here
			s.measurementHandler.AddClient(conn)
			s.HandleAddSession(ctx, conn, msg.Session)

		case handlers.RemoveSessionMessage: // esp not used here
			s.measurementHandler.AddClient(conn)
			s.HandleRemoveSession(ctx, conn, msg.SessionId)

		case handlers.EspMessage:
			log.Printf("Esp message received: %v", msg)
			s.HandleEspConnection(conn)
			if err := s.SendMasterStatus("ISALIVE"); err != nil {
				log.Printf("Error sending master status: %v", err)
			}
			s.StartStatusMonitor()
		case handlers.IncomingMeasurementMessage:
			log.Printf("Received incoming measurement: %v", msg)
			if !s.isMeasurementStarted.Load() {
				log.Printf("Measurement has not started, skip packet")
				break
			}
			if err := s.measurementHandler.ProcessPacketData(msg.Data); err != nil {
				log.Printf("Error processing packet: %v", err)
			}
		case handlers.SettingsAckMessage:
			log.Printf("Received settings ack: %v", msg)
			if err := s.SendStatus(fmt.Sprintf(msg.Type()+": SF: %f, BW: %f, TX: %f", msg.Sf, msg.Bw, msg.Tx)); err != nil {
				log.Printf("Error sending settings ack: %v", err)
			}
			s.SetSettingsForSession(ctx, models.Params{Sf: msg.Sf, Tx: msg.Tx, Bandwidth: msg.Bw})
		case handlers.AckMessage:
			log.Printf("Received ack message: %v", msg)
			if err := s.SendMasterStatus("ISALIVE"); err != nil {
				log.Printf("Error sending master status: %v", err)
			}
		case handlers.ErrMessage:
			log.Printf("Received err message from master: %v", msg)
		case handlers.UnknownMessage:
			log.Printf("Unknown message type: %v", message)
			s.replyError(conn, fmt.Errorf("unknown command: %s", message))
		case handlers.MasterStatusMessage:
			log.Printf("New status for master: %v", msg.Status)
			if err := s.SendMasterStatus(msg.Status); err != nil {
				log.Printf("Error sending master status: %v", err)
			}
		case handlers.SlaveStatusMessage:
			log.Printf("New status for slave: %v", msg.Status)
			if err := s.SendSlaveStatus(msg.Status); err != nil {
				log.Printf("Error sending slave status: %v", err)
			}
		default:
			log.Printf("Unhandled message type: %T", msg)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Println("Error reading:", err.Error())
		return err
	}
	return nil
}

func (s *Server) write(conn net.Conn, msg string) {
	if _, err := conn.Write([]byte(msg + "\n")); err != nil {
		log.Printf("Error writing to %s: %v", conn.RemoteAddr(), err)
	}
}

func (s *Server) replyError(conn net.Conn, err error) {
	if conn == s.connector.GetConn() {
		return
	}
	s.write(conn, "ERROR: "+err.Error())
}

func (s *Server) waitForEspConnectionLogged(conn net.Conn) {
	if err := s.waitForEspConnection(conn); err != nil {
		log.Printf("Error waiting for esp connection: %v", err)
	}
}

func (s *Server) waitForEspConnection(conn net.Conn) error {
	timeout := time.After(30 * time.Second)
	log.Printf("Waiting for esp connection to establish")
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			s.removeWaitingClient(conn)
			log.Printf("Client %s not waiting for esp connection to establish, timeout", conn.RemoteAddr())
			return errors.New("ESP connection timeout")
		case <-ticker.C:
			if s.connector.IsConnected() {
				s.removeWaitingClient(conn)
				s.notifyWaitingClients()
				return nil
			}
		}
	}
}

func (s *Server) addWaitingClient(conn net.Conn) {
	s.waitListMutex.Lock()
	s.waitList = append(s.waitList, conn)
	log.Printf("Adding waiting client: %s", conn.RemoteAddr())
	s.waitListMutex.Unlock()
}

func (s *Server) removeWaitingClient(conn net.Conn) {
	s.waitListMutex.Lock()
	defer s.waitListMutex.Unlock()
	for i, connection := range s.waitList {
		if connection == conn {
			s.waitList = append(s.waitList[:i], s.waitList[i+1:]...)
			break
		}
	}
}

func (s *Server) notifyWaitingClients() {
	s.waitListMutex.Lock()
	defer s.waitListMutex.Unlock()
	s.waitList = nil
}

func (s *Server) HandleSetSettings(conn net.Conn, message string) {
	if err := s.measurementHandler.ProcessInterfaceSettingChange(message); err != nil {
		log.Printf("Error sending interface request: %v", err)
		s.replyError(conn, err)
	}
}

func (s *Server) handleStartMeasurement(ctx context.Context, conn net.Conn, sessionId int) {
	if err := s.connector.SendCommand("START_MEASUREMENT", sessionId); err != nil {
		log.Printf("Error processing interface request: %v", err)
		s.replyError(conn, err)
	}
	go func() {
		packets, err := s.measurementHandler.GetAllSessionMeasurements(ctx, sessionId)
		if err != nil {
			log.Printf("Error getting session measurements: %v", err)
		}
		if packets != nil {
			err = s.measurementHandler.SendSessionPackets(sessionId, packets)
			if err != nil {
				log.Printf("Error sending session packets: %v", err)
			}
		}
	}()
}

func (s *Server) HandleGetMeasurementSessions(ctx context.Context, conn net.Conn) {
	go func() {
		sessions, err := s.measurementHandler.GetAllSessions(ctx)
		if err != nil {
			log.Printf("Error getting sessions: %v", err)
			s.replyError(conn, err)
			return
		}
		var parts []string
		for _, session := range sessions {
			var str string
			if session.Bandwidth == 0 || session.Sf == 0 {
				str = fmt.Sprintf("[%d, %s, %s, %s, %d, -]", session.Id, session.Name, session.StartTime, session.EndTime, session.Count)
			} else {
				str = fmt.Sprintf("[%d, %s, %s, %s, %d, %f, %f, %f]", session.Id, session.Name, session.StartTime, session.EndTime, session.Count, session.Sf, session.Bandwidth, session.Tx)
			}
			parts = append(parts, str)
		}
		err = s.measurementHandler.SendAllSessions(conn, strings.Join(parts, ", "))
		if err != nil {
			log.Printf("Error sending sessions to client: %v", err)
			return
		}
	}()
}

func (s *Server) HandleAddSession(ctx context.Context, conn net.Conn, session models.Session) {
	err := s.measurementHandler.SaveSession(ctx, session)
	if err != nil {
		log.Printf("Error saving session: %v", err)
		s.replyError(conn, err)
		return
	}
	log.Printf("Session saved: %v", session)
}

func (s *Server) HandleRemoveSession(ctx context.Context, conn net.Conn, sessionId int) {
	err := s.measurementHandler.RemoveSession(ctx, sessionId)
	if err != nil {
		log.Printf("Error removing session: %v", err)
		s.replyError(conn, err)
		return
	}
	log.Printf("Session removed: %v", sessionId)
}

func (s *Server) HandleEspConnection(conn net.Conn) {
	s.connector.SetIP(conn.RemoteAddr().String())
	s.connector.SetConn(conn)
	s.connector.SetConnected(true)
	log.Printf("Connection from esp: %v", conn.RemoteAddr().String())
	log.Printf("ESP_OK")
}

func (s *Server) SendMasterStatus(status string) error {
	return s.setStatus(&s.MasterState, "MASTER_STATUS", status)
}

func (s *Server) SendSlaveStatus(status string) error {
	return s.setStatus(&s.SlaveState, "SLAVE_STATUS", status)
}

func (s *Server) setStatus(state *DeviceStatus, prefix, status string) error {
	s.mutex.Lock()
	state.Status = "DISCONNECTED"
	if status == "ISALIVE" {
		state.Status = "CONNECTED"
	}
	state.LastUpdated = time.Now()
	msg := prefix + " " + state.Status
	s.mutex.Unlock()

	log.Printf("Sending status: %s", msg)
	return s.SendStatus(msg)
}

func (s *Server) SendStatus(status string) error {
	return s.measurementHandler.Broadcast(status)
}

func (s *Server) StartStatusMonitor() {
	s.monitorOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()

			for range ticker.C {
				s.checkAndUpdateStatus()
			}
		}()
	})
}

func isStale(state DeviceStatus) bool {
	return !state.LastUpdated.IsZero() && time.Since(state.LastUpdated) > statusTimeout && state.Status != "DISCONNECTED"
}

func (s *Server) checkAndUpdateStatus() {
	s.mutex.Lock()
	masterStale, slaveStale := isStale(s.MasterState), isStale(s.SlaveState)
	s.mutex.Unlock()

	if masterStale {
		log.Printf("Master state did not update for over 2 mins, change to DISCONNECTED")
		if err := s.SendMasterStatus("DISCONNECTED"); err != nil {
			log.Printf("Error sending master status: %v", err)
		}
	}
	if slaveStale {
		log.Printf("Slave state did not update for over 2 mins, change to DISCONNECTED")
		if err := s.SendSlaveStatus("DISCONNECTED"); err != nil {
			log.Printf("Error sending slave status: %v", err)
		}
	}
}

func (s *Server) SetSettingsForSession(ctx context.Context, params models.Params) {
	go func() {
		err := s.measurementHandler.SetSettingsForSession(ctx, params)
		if err != nil {
			log.Printf("Error setting settings for session: %v", err)
			if err := s.SendStatus("ALREADY_SET"); err != nil {
				log.Printf("Error sending ALREADY_SET: %v", err)
			}
		}
	}()
}
