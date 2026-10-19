package core

import (
	"Lora_Esp_Gsm_Gps_project/internal/models"
	"context"
	"database/sql"
	"net"
)

type Message interface {
	Type() string
}

type Repository interface {
	FindById(ctx context.Context, id int) (interface{}, error)
}

type Service interface {
	ProcessData(data interface{}) error
}

type App interface {
	GetDB() *sql.DB
	GetRepo() Repository
	GetService() Service
	Close() error
}

type MeasurementHandler interface {
	ProcessPacketData(packet models.Packet) error
	ProcessInterfaceSettingChange(msg string) error
	GetMeasurements(ctx context.Context, sessionId int) ([]models.Packet, error)
	SendMeasurementsToClient(ctx context.Context, conn net.Conn, command string) error
	GetAllSessions(ctx context.Context) ([]models.Session, error)
	SendAllSessions(conn net.Conn, data string) error
	SaveSession(ctx context.Context, session models.Session) error
	RemoveSession(ctx context.Context, sessionId int) error
	AddPendingMessage(message string, dest models.Destination, typ Message)
	GetAllSessionMeasurements(ctx context.Context, sessionId int) ([]models.Packet, error)
	SendSessionPackets(sessionId int, packets []models.Packet) error
	SetSettingsForSession(ctx context.Context, params models.Params) error
	AddClient(conn net.Conn)
	RemoveClient(conn net.Conn)
	Broadcast(msg string) error
}
