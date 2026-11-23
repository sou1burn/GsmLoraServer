package esp

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/sou1burn/GsmLoraServer/internal/models"
)

type ESPConnector struct {
	Conn           net.Conn
	isConnected    bool
	mutex          sync.RWMutex
	CurrentSession int
}

func (e *ESPConnector) GetConn() net.Conn {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.Conn
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

func NewESPConnector() *ESPConnector {
	return &ESPConnector{
		isConnected: false,
	}
}

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
		return fmt.Errorf("failed to send command to ESP32: %w", err)
	}

	log.Printf("Command sent to ESP32: %s", message)
	return nil
}

func (e *ESPConnector) SendParamsToDevice(params models.Params) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if !e.isConnected || e.Conn == nil {
		log.Printf("not Connected to ESP32")
		return errors.New("ESP_NOT_CONNECTED")
	}

	message := fmt.Sprintf("SET_SETTINGS: SF=%.1f, TX=%.1f, BW=%.1f", params.Sf, params.Tx, params.Bandwidth)

	_, err := e.Conn.Write([]byte(message + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send params to ESP32: %w", err)
	}

	log.Printf("Params sent to ESP32: %s", message)
	return nil
}

func (e *ESPConnector) IsConnected() bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.isConnected
}
