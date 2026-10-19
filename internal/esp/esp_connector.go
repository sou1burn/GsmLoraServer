package esp

import (
	"Lora_Esp_Gsm_Gps_project/internal/models"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
)

type ESPConnector struct {
	IP             string
	Port           string
	Conn           net.Conn
	isConnected    bool
	mutex          sync.RWMutex
	CurrentSession int
}

//type Message interface {
//	Type() string
//}
//
//type MeasurementMessage struct {
//	Data      models.Packet
//	sessionId int
//}
//
//type NoSignalErrorMessage struct {
//	Error string
//}
//
//type UnknownMessage struct{}
//
//func (m MeasurementMessage) Type() string   { return "MEASUREMENT" }
//func (m NoSignalErrorMessage) Type() string { return "NO_SIGNAL" }
//func (m UnknownMessage) Type() string       { return "UNKNOWN" }
//
//type GetMessage struct {
//	What string
//}

func (e *ESPConnector) GetIP() string {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.IP
}

func (e *ESPConnector) GetPort() string {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.Port
}

func (e *ESPConnector) GetConn() net.Conn {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.Conn
}
func (e *ESPConnector) SetIP(ip string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.IP = ip
}

func (e *ESPConnector) SetPort(port string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.Port = port
}

func (e *ESPConnector) SetConn(conn net.Conn) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.Conn = conn
}

func (e *ESPConnector) SetConnected(connected bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.isConnected = connected
}

func (e *ESPConnector) GetCurrentSession() int {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.CurrentSession
}

func (e *ESPConnector) SetCurrentSession(session int) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.CurrentSession = session
}

func NewESPConnector() *ESPConnector {
	return &ESPConnector{
		isConnected: false,
	}
}

//func (e *ESPConnector) Connect(ip, port string) error {
//	e.mutex.Lock()
//	defer e.mutex.Unlock()
//	maxAttempts := 3
//	retryDelay := 5 * time.Second
//	var err error
//
//	for attempt := 1; attempt <= maxAttempts; attempt++ {
//		log.Printf("Attempting to connect to ESP32 at %s (attempt %d/%d)", ip, attempt, maxAttempts)
//
//		Conn, dialErr := net.Dial("tcp", ip)
//		if dialErr == nil {
//			e.IP = ip
//			e.Port = port
//			e.Conn = Conn
//			e.isConnected = true
//
//			log.Printf("Successfully connected to ESP32 at %s on attempt %d", ip, attempt)
//			return nil
//		}
//
//		err = dialErr
//
//		if attempt < maxAttempts {
//			log.Printf("Connection attempt %d failed: %v. Retrying in %v...", ip, dialErr, retryDelay)
//			time.Sleep(retryDelay)
//			retryDelay = time.Duration(float64(retryDelay) * 1.5)
//		}
//	}
//	e.Conn = nil
//	e.isConnected = false
//	return fmt.Errorf("failed to connect to ESP32 at %s after %d attempts: %v", ip, maxAttempts, err)
//}

func (e *ESPConnector) SendCommand(command string, sessionId int) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if !e.isConnected || e.Conn == nil {
		log.Printf("not Connected to ESP32")
		return errors.New("ESP_NOT_CONNECTED")
	}
	var message string
	if sessionId == 0 {
		message = command
	} else {
		e.CurrentSession = sessionId
		message = fmt.Sprintf("%s SESSION_ID=%d", command, sessionId)
	}
	_, err := e.Conn.Write([]byte(message + "\n"))

	if err != nil {
		e.isConnected = false
		if e.Conn != nil {
			e.Conn.Close()
			e.Conn = nil
		}
		return fmt.Errorf("failed to send command to ESP32: %v", err)
	}

	log.Printf("Command sent to ESP32: %s", message)
	return nil
}

func (e *ESPConnector) SendParamsToDevice(params models.Params) error {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	if !e.isConnected || e.Conn == nil {
		log.Printf("not Connected to ESP32")
		return errors.New("ESP_NOT_CONNECTED")
	}

	message := fmt.Sprintf("SET_SETTINGS: SF=%.1f, TX=%.1f, BW=%.1f", params.Sf, params.Tx, params.Bandwidth)

	_, err := e.Conn.Write([]byte(message + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send params to ESP32: %v", err)
	}

	log.Printf("Params sent to ESP32: %s", message)
	return nil
}

func (e *ESPConnector) ProcessInterfaceSettingChange(message string) error {
	cleanedMessage := strings.TrimPrefix(message, "SET_SETTINGS: ")
	parts := strings.Split(cleanedMessage, ", ")

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

	log.Printf("Parsed params: %+v", params)
	return e.SendParamsToDevice(params)
}

func (e *ESPConnector) IsConnected() bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.isConnected
}

func (e *ESPConnector) Close() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.Conn != nil {
		err := e.Conn.Close()
		e.Conn = nil
		e.isConnected = false
		log.Printf("Closing connection with esp on teardown")
		return err
	}
	return nil
}
