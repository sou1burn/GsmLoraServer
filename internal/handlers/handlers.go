package handlers

import (
	"Lora_Esp_Gsm_Gps_project/internal/models"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type loraResponse struct {
	RequestId int     `json:"id"`
	RSSI      int     `json:"rssi"`
	SNR       float64 `json:"snr"`
	ErrorBits int     `json:"error_bits"`
	Checksum  *string `json:"checksum"`
}

type LoraParser struct {
	Data      *models.LoRaData
	Timestamp time.Time
	RequestId int
	Checksum  *string
}

type PacketParser struct {
	Data      *models.Packet
	Timestamp time.Time
	RequestId int
	Checksum  *string
}

func (p *LoraParser) ParseLoraData(response string) error {
	rawData := loraResponse{}
	if err := json.Unmarshal([]byte(response), &rawData); err != nil {
		return err
	}
	p.RequestId = rawData.RequestId
	p.Checksum = rawData.Checksum
	lora := models.LoRaData{}
	lora.RequestId = rawData.RequestId
	lora.RSSI = rawData.RSSI
	lora.SNR = rawData.SNR
	p.Timestamp = time.Now()
	p.Data = &lora

	return nil
}

func (p *PacketParser) ParsePacketData(response string) (models.Packet, error) {
	var raw struct {
		models.Packet
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(response), &raw); err != nil {
		return models.Packet{}, err
	}
	packet := raw.Packet
	packet.Timestamp = time.Now().UTC().Truncate(time.Millisecond)
	if raw.Timestamp != "" {
		ts, err := time.Parse(time.RFC3339Nano, raw.Timestamp)
		if err != nil {
			log.Printf("Invalid packet timestamp %q, using receive time: %v", raw.Timestamp, err)
		} else {
			packet.Timestamp = ts.UTC()
		}
	}
	return packet, nil
}

type Message interface {
	Type() string
}

type SetSettingsMessage struct {
	Params models.Params
}

type StartMeasurementMessage struct {
	SessionId int
}

type StopMeasurementMessage struct {
	RequestId int
}

type GetMeasurementSessionsMessage struct{}

type AddSessionMessage struct {
	Session models.Session
}

type RemoveSessionMessage struct {
	SessionId int
}

type IncomingMeasurementMessage struct {
	Data      models.Packet
	SessionId int
}

type EspMessage struct{}

type SettingsAckMessage struct {
	Sf float32
	Tx float32
	Bw float32
}

type AckMessage struct{}

type ErrMessage struct {
	Error string
}

type InitialEspConnectionMessage struct{}

type MasterStatusMessage struct {
	Status string
}

type SlaveStatusMessage struct {
	Status string
}
type UnknownMessage struct{}

func (m SetSettingsMessage) Type() string            { return "SET_SETTINGS" }
func (m StartMeasurementMessage) Type() string       { return "START_MEASUREMENT" }
func (m StopMeasurementMessage) Type() string        { return "STOP_MEASUREMENT" }
func (m GetMeasurementSessionsMessage) Type() string { return "GET_MEASUREMENT_SESSIONS" }
func (m AddSessionMessage) Type() string             { return "ADD_SESSION" }
func (m RemoveSessionMessage) Type() string          { return "REMOVE_SESSION" }
func (m UnknownMessage) Type() string                { return "UNKNOWN" }
func (m EspMessage) Type() string                    { return "ESP_IDENTIFY" }
func (m InitialEspConnectionMessage) Type() string   { return "INITIAL_ESP_CONNECTION" }
func (m IncomingMeasurementMessage) Type() string    { return "ESP_MEASUREMENT" }
func (m AckMessage) Type() string                    { return "ACK" }
func (m ErrMessage) Type() string                    { return "ERROR" }
func (m MasterStatusMessage) Type() string           { return "MASTER_STATUS" }
func (m SlaveStatusMessage) Type() string            { return "SLAVE_STATUS" }
func (m SettingsAckMessage) Type() string            { return "SETTINGS_ACK" }

type GetMessage struct {
	What string
}

func ParseClientMessage(message string) (Message, error) {
	if strings.HasPrefix(message, "SET_SETTINGS") {
		par := models.Params{}
		_, err := fmt.Sscanf(message, "SET_SETTINGS: SF=%f, TX=%f, BW=%f", &par.Sf, &par.Tx, &par.Bandwidth)
		if err != nil {
			return nil, fmt.Errorf("error parsing SET_SETTINGS: %w", err)
		}
		return SetSettingsMessage{Params: par}, nil

	} else if strings.HasPrefix(message, "START_MEASUREMENT") {
		cleaned := strings.TrimPrefix(message, "START_MEASUREMENT: ")
		sessionId, err := strconv.Atoi(strings.TrimSpace(cleaned))
		if err != nil {
			return nil, fmt.Errorf("error parsing START_MEASUREMENT: %w", err)
		}
		return StartMeasurementMessage{SessionId: sessionId}, nil

	} else if strings.HasPrefix(message, "GET_MEASUREMENT_SESSIONS") {
		return GetMeasurementSessionsMessage{}, nil

	} else if strings.HasPrefix(message, "ADD_SESSION") {
		sessionStr := strings.TrimPrefix(message, "ADD_SESSION: ")
		session, err := ParseSession(sessionStr)
		if err != nil {
			log.Printf("error parsing ADD_SESSION: %v", err)
			return nil, err
		}
		return AddSessionMessage{Session: session}, nil

	} else if strings.HasPrefix(message, "REMOVE_SESSION") {
		sessionId, err := ParseRemoveSessionMessage(message)
		if err != nil {
			log.Printf("error parsing REMOVE_SESSION: %v", err)
			return nil, err
		}
		return RemoveSessionMessage{SessionId: sessionId}, nil

	} else if strings.HasPrefix(message, "IDENTIFY") {
		return EspMessage{}, nil

	} else if strings.HasPrefix(message, "MEASUREMENT:") {
		return ParseIncomingMeasurement(message)

	} else if strings.HasPrefix(message, "ACK: SSET") {
		re := regexp.MustCompile(`SF([\d.]+),BW([\d.]+),TX([\d.]+)`)
		var sf, tx, bw float64
		var err error
		matches := re.FindStringSubmatch(message)
		if matches == nil || len(matches) < 4 {
			return nil, fmt.Errorf("не удалось найти настройки в строке: %s", message)
		}
		if sf, err = strconv.ParseFloat(matches[1], 32); err != nil {
			return nil, fmt.Errorf("ошибка парсинга SF: %w", err)
		}
		if bw, err = strconv.ParseFloat(matches[2], 32); err != nil {
			return nil, fmt.Errorf("ошибка парсинга BW: %w", err)
		}
		if tx, err = strconv.ParseFloat(matches[3], 32); err != nil {
			return nil, fmt.Errorf("ошибка парсинга TX: %w", err)
		}
		return SettingsAckMessage{Sf: float32(sf), Bw: float32(bw), Tx: float32(tx)}, nil

	} else if strings.HasPrefix(message, "ACK") {
		return AckMessage{}, nil

	} else if strings.Contains(message, "ERROR") {
		return ErrMessage{Error: message}, nil

	} else if strings.Contains(message, "MASTER_STATUS") {
		status := strings.TrimPrefix(message, "MASTER_STATUS ")

		return MasterStatusMessage{status}, nil

	} else if strings.Contains(message, "SLAVE_STATUS") {
		status := strings.TrimPrefix(message, "SLAVE_STATUS ")

		return SlaveStatusMessage{status}, nil
	}

	return UnknownMessage{}, nil
}

func ParseSession(data string) (models.Session, error) {
	cleaned := strings.TrimPrefix(data, "ADD_SESSION: ")
	cleaned = strings.Trim(strings.TrimSpace(cleaned), "[]")

	parts := strings.Split(cleaned, ",")
	if len(parts) != 5 {
		log.Printf("Error parsing session: %v, got len: %v", data, len(parts))
		return models.Session{}, errors.New("invalid session format")
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	sessionID, err := strconv.Atoi(parts[0])
	if err != nil {
		return models.Session{}, fmt.Errorf("invalid session id: %w", err)
	}
	count, err := strconv.Atoi(parts[4])
	if err != nil {
		return models.Session{}, fmt.Errorf("invalid points: %w", err)
	}
	return models.Session{
		Id:        sessionID,
		Name:      parts[1],
		StartTime: parts[2],
		EndTime:   parts[3],
		Count:     count,
	}, nil
}

func ParseRemoveSessionMessage(message string) (int, error) {
	cleaned := strings.TrimPrefix(message, "REMOVE_SESSION: ")

	sessionID, err := strconv.Atoi(strings.TrimSpace(cleaned))
	if err != nil {
		return 0, fmt.Errorf("invalid session ID: %w", err)
	}

	return sessionID, nil
}

func ParseTime(timeStr string) (time.Time, error) {
	parsedTime, err := time.Parse(models.TimeLayout, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time format: %w", err)
	}
	return parsedTime, nil
}

func ParseIncomingMeasurement(message string) (IncomingMeasurementMessage, error) {
	cleaned, ok := strings.CutPrefix(message, "MEASUREMENT:")
	if !ok {
		return IncomingMeasurementMessage{}, errors.New("error parsing incoming measurement: no MEASUREMENT: prefix")
	}
	packetParser := PacketParser{}
	data, err := packetParser.ParsePacketData(cleaned)
	if err != nil {
		return IncomingMeasurementMessage{}, fmt.Errorf("error parsing incoming measurement: %w", err)
	}
	return IncomingMeasurementMessage{Data: data}, nil
}
