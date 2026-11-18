package core

import (
	"context"
	"net"

	"github.com/sou1burn/GsmLoraServer/internal/models"
)

type Message interface {
	Type() string
}

type MeasurementHandler interface {
	ProcessPacketData(packet models.Packet) error
	ProcessInterfaceSettingChange(msg string) error
	GetAllSessions(ctx context.Context) ([]models.Session, error)
	SendAllSessions(conn net.Conn, data string) error
	SaveSession(ctx context.Context, session models.Session) error
	RemoveSession(ctx context.Context, sessionId int) error
	AddPendingMessage(message string, typ Message)
	GetAllSessionMeasurements(ctx context.Context, sessionId int) ([]models.Packet, error)
	SendSessionPackets(sessionId int, packets []models.Packet) error
	SetSettingsForSession(ctx context.Context, params models.Params) error
	AddClient(conn net.Conn)
	RemoveClient(conn net.Conn)
	Broadcast(msg string) error
}
