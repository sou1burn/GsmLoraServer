package unit

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	telemetryv1 "github.com/sou1burn/GsmLoraServer/api/proto/telemetry/v1"
	"github.com/sou1burn/GsmLoraServer/cmd/server"
	"github.com/sou1burn/GsmLoraServer/internal/models"
	mocks "github.com/sou1burn/GsmLoraServer/mocks"
)

// mockConn — детерминированный net.Conn в памяти: данные на запись
// аккумулируются и доступны для проверок.
type mockConn struct {
	readBuff  *strings.Reader
	writeBuff *strings.Builder
	mux       sync.RWMutex
	isClosed  bool
}

func newMockConn() *mockConn {
	return &mockConn{
		readBuff:  strings.NewReader(""),
		writeBuff: &strings.Builder{},
		isClosed:  false,
	}
}

func (m *mockConn) SetReadData(data string) { m.readBuff = strings.NewReader(data) }
func (m *mockConn) GetWrittenData() string  { return m.writeBuff.String() }

func (m *mockConn) Close() error {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.isClosed = true
	return nil
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	if m.isClosed {
		return 0, errors.New("connection closed")
	}
	return m.readBuff.Read(b)
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	if m.isClosed {
		return 0, errors.New("connection closed")
	}
	return m.writeBuff.Write(b)
}

func (m *mockConn) LocalAddr() net.Addr { return nil }
func (m *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321}
}

func (m *mockConn) SetDeadline(time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(time.Time) error { return nil }

func newServerWithMocks(t *testing.T) (*server.Server, *mocks.MockMeasurementHandler, *mocks.MockConnector) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	handler := mocks.NewMockMeasurementHandler(ctrl)
	connector := mocks.NewMockConnector(ctrl)
	return server.NewServer(handler, connector), handler, connector
}

func TestHandleAddSession_Success(t *testing.T) {
	srv, handler, _ := newServerWithMocks(t)
	conn := newMockConn()
	session := models.Session{
		Id:        1,
		Name:      "Test Session",
		StartTime: "2023-01-01 10:00:00",
		EndTime:   "2023-01-01 11:00:00",
		Count:     10,
	}

	handler.EXPECT().
		SaveSession(gomock.Any(), session).
		Times(1).
		Return(nil)

	srv.HandleAddSession(context.Background(), conn, session)
}

func TestHandleAddSession_Error(t *testing.T) {
	srv, handler, connector := newServerWithMocks(t)
	conn := newMockConn()
	session := models.Session{Id: 1, Name: "Test Session"}
	wantErr := errors.New("database error")

	// replyError отвечает клиенту, только если это не соединение с ESP.
	connector.EXPECT().GetConn().Return(nil).AnyTimes()
	handler.EXPECT().
		SaveSession(gomock.Any(), session).
		Times(1).
		Return(wantErr)

	srv.HandleAddSession(context.Background(), conn, session)

	written := conn.GetWrittenData()
	if !strings.Contains(written, "ERROR") {
		t.Error("при ошибке сохранения клиенту должен отправляться ERROR")
	}
	if !strings.Contains(written, wantErr.Error()) {
		t.Errorf("текст ошибки должен попадать в ответ, записано: %q", written)
	}
}

func TestHandleRemoveSession_Success(t *testing.T) {
	srv, handler, _ := newServerWithMocks(t)
	conn := newMockConn()

	handler.EXPECT().
		RemoveSession(gomock.Any(), 1).
		Times(1).
		Return(nil)

	srv.HandleRemoveSession(context.Background(), conn, 1)
}

func TestHandleGetMeasurementSessions(t *testing.T) {
	srv, handler, _ := newServerWithMocks(t)
	conn := newMockConn()
	sessions := []models.Session{
		{Id: 1, Name: "Session 1", Count: 10},
		{Id: 2, Name: "Session 2", Count: 20},
	}

	handler.EXPECT().
		GetAllSessions(gomock.Any()).
		Times(1).
		Return(sessions, nil)

	done := make(chan string, 1)
	handler.EXPECT().
		SendAllSessions(conn, gomock.Any()).
		Times(1).
		DoAndReturn(func(_ net.Conn, data string) error {
			done <- data
			return nil
		})

	srv.HandleGetMeasurementSessions(context.Background(), conn)

	select {
	case data := <-done:
		if !strings.Contains(data, "Session 1") || !strings.Contains(data, "Session 2") {
			t.Errorf("сессии неправильно отформатированы: %q", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendAllSessions не вызван за 2 секунды")
	}
}

func TestSendDeviceStatus_WireText(t *testing.T) {
	cases := []struct {
		device telemetryv1.DeviceKind
		status telemetryv1.DeviceStatus
		want   string
	}{
		{telemetryv1.DeviceKind_DEVICE_KIND_MASTER, telemetryv1.DeviceStatus_DEVICE_STATUS_CONNECTED, "MASTER_STATUS CONNECTED"},
		{telemetryv1.DeviceKind_DEVICE_KIND_MASTER, telemetryv1.DeviceStatus_DEVICE_STATUS_DISCONNECTED, "MASTER_STATUS DISCONNECTED"},
		{telemetryv1.DeviceKind_DEVICE_KIND_SLAVE, telemetryv1.DeviceStatus_DEVICE_STATUS_DISCONNECTED, "SLAVE_STATUS DISCONNECTED"},
	}
	for _, c := range cases {
		srv, handler, _ := newServerWithMocks(t)
		handler.EXPECT().Broadcast(c.want).Times(1).Return(nil)
		if err := srv.SendDeviceStatus(c.device, c.status); err != nil {
			t.Errorf("SendDeviceStatus(%v, %v): %v", c.device, c.status, err)
		}
	}
}

func TestHandleSetSettings_Success(t *testing.T) {
	srv, handler, _ := newServerWithMocks(t)
	conn := newMockConn()
	params := &telemetryv1.LoRaParams{Sf: 7, Tx: 14, Bw: 125}

	handler.EXPECT().
		ProcessInterfaceSettingChange(params).
		Times(1).
		Return(nil)

	srv.HandleSetSettings(conn, params)
}

func TestHandleSetSettings_Error(t *testing.T) {
	srv, handler, connector := newServerWithMocks(t)
	conn := newMockConn()
	params := &telemetryv1.LoRaParams{Sf: 7, Tx: 14, Bw: 125}

	connector.EXPECT().GetConn().Return(nil).AnyTimes()
	handler.EXPECT().
		ProcessInterfaceSettingChange(params).
		Times(1).
		Return(errors.New("ESP_NOT_CONNECTED"))

	srv.HandleSetSettings(conn, params)

	if written := conn.GetWrittenData(); !strings.Contains(written, "ERROR") {
		t.Errorf("при ошибке клиенту должен отправляться ERROR, записано: %q", written)
	}
}

func TestHandleEspConnection(t *testing.T) {
	srv, _, connector := newServerWithMocks(t)
	conn := newMockConn()

	connector.EXPECT().SetConn(conn).Times(1)
	connector.EXPECT().SetConnected(true).Times(1)

	srv.HandleEspConnection(conn)
}
