package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	telemetryv1 "github.com/sou1burn/GsmLoraServer/api/proto/telemetry/v1"
	"github.com/sou1burn/GsmLoraServer/internal/models"
)

func ParsePacketData(response string) (models.Packet, error) {
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
	Params *telemetryv1.SetSettings
}

type StartMeasurementMessage struct {
	SessionId int
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
	Applied *telemetryv1.SettingsConfirmed
}

type AckMessage struct{}

type ErrMessage struct {
	Error string
}

type StatusChangedMessage struct {
	Event *telemetryv1.DeviceStatusChanged
}

type UnknownMessage struct{}

func (m SetSettingsMessage) Type() string            { return "SET_SETTINGS" }
func (m StartMeasurementMessage) Type() string       { return "START_MEASUREMENT" }
func (m GetMeasurementSessionsMessage) Type() string { return "GET_MEASUREMENT_SESSIONS" }
func (m AddSessionMessage) Type() string             { return "ADD_SESSION" }
func (m RemoveSessionMessage) Type() string          { return "REMOVE_SESSION" }
func (m UnknownMessage) Type() string                { return "UNKNOWN" }
func (m EspMessage) Type() string                    { return "ESP_IDENTIFY" }
func (m IncomingMeasurementMessage) Type() string    { return "ESP_MEASUREMENT" }
func (m AckMessage) Type() string                    { return "ACK" }
func (m ErrMessage) Type() string                    { return "ERROR" }
func (m StatusChangedMessage) Type() string          { return "STATUS_CHANGED" }
func (m SettingsAckMessage) Type() string            { return "SETTINGS_ACK" }

func ParseClientMessage(message string) (Message, error) {
	if strings.HasPrefix(message, "SET_SETTINGS") {
		var sf, bw, tx float32
		_, err := fmt.Sscanf(message, "SET_SETTINGS: SF=%f, TX=%f, BW=%f", &sf, &tx, &bw)
		if err != nil {
			return nil, fmt.Errorf("error parsing SET_SETTINGS: %w", err)
		}
		return SetSettingsMessage{Params: &telemetryv1.SetSettings{
			Params: &telemetryv1.LoRaParams{Sf: sf, Tx: tx, Bw: bw},
		}}, nil

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
		var sf, bw, tx float32
		_, err := fmt.Sscanf(message, "ACK: SSET SF%f,BW%f,TX%f", &sf, &bw, &tx)
		if err != nil {
			return nil, fmt.Errorf("error parsing ACK: SSET: %w", err)
		}
		return SettingsAckMessage{Applied: &telemetryv1.SettingsConfirmed{
			Applied: &telemetryv1.LoRaParams{Sf: sf, Tx: tx, Bw: bw}}}, nil
	} else if strings.HasPrefix(message, "ACK") {
		return AckMessage{}, nil

	} else if strings.Contains(message, "ERROR") {
		return ErrMessage{Error: message}, nil

	} else if strings.Contains(message, "MASTER_STATUS") {
		return statusChanged("MASTER_STATUS ", telemetryv1.DeviceKind_DEVICE_KIND_MASTER, message), nil

	} else if strings.Contains(message, "SLAVE_STATUS") {
		return statusChanged("SLAVE_STATUS ", telemetryv1.DeviceKind_DEVICE_KIND_SLAVE, message), nil
	}

	return UnknownMessage{}, nil
}

func statusChanged(prefix string, device telemetryv1.DeviceKind, message string) StatusChangedMessage {
	status := telemetryv1.DeviceStatus_DEVICE_STATUS_DISCONNECTED
	if strings.TrimPrefix(message, prefix) == "ISALIVE" {
		status = telemetryv1.DeviceStatus_DEVICE_STATUS_CONNECTED
	}
	return StatusChangedMessage{Event: &telemetryv1.DeviceStatusChanged{Device: device, Status: status}}
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

func ParseIncomingMeasurement(message string) (IncomingMeasurementMessage, error) {
	cleaned, ok := strings.CutPrefix(message, "MEASUREMENT:")
	if !ok {
		return IncomingMeasurementMessage{}, errors.New("error parsing incoming measurement: no MEASUREMENT: prefix")
	}
	data, err := ParsePacketData(cleaned)
	if err != nil {
		return IncomingMeasurementMessage{}, fmt.Errorf("error parsing incoming measurement: %w", err)
	}
	return IncomingMeasurementMessage{Data: data}, nil
}
