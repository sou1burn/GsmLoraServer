package esp

import "net"

type Connector interface {
	IsConnected() bool
	SendCommand(command string, sessionId int) error
	GetConn() net.Conn
	SetConn(conn net.Conn)
	SetConnected(connected bool)
}

var _ Connector = (*ESPConnector)(nil)
