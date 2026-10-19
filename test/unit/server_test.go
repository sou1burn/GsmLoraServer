//go:build ignore

package unit

import (
	server2 "Lora_Esp_Gsm_Gps_project/cmd/server"
	"Lora_Esp_Gsm_Gps_project/internal/models"
	"Lora_Esp_Gsm_Gps_project/mocks"
	"errors"
	"github.com/golang/mock/gomock"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type MockConn struct {
	readBuff  *strings.Reader
	writeBuff *strings.Builder
	mux       sync.RWMutex
	isClosed  bool
}

func (m *MockConn) Close() error {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.isClosed = true
	return nil
}

func NewMockConn() *MockConn {
	return &MockConn{
		readBuff:  strings.NewReader(""),
		writeBuff: &strings.Builder{},
		isClosed:  false,
	}
}

func (m *MockConn) SetReadData(data string) {
	m.readBuff = strings.NewReader(data)
}

func (m *MockConn) GetWrittenData() string {
	return m.writeBuff.String()
}

func (m *MockConn) Read(b []byte) (n int, err error) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	if m.isClosed {
		return 0, errors.New("connection closed")
	}
	return m.readBuff.Read(b)
}

func (m *MockConn) Write(b []byte) (n int, err error) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	if m.isClosed {
		return 0, errors.New("connection closed")
	}
	return m.writeBuff.Write(b)
}

func (m *MockConn) LocalAddr() net.Addr                { return nil }
func (m *MockConn) RemoteAddr() net.Addr               { return nil }
func (m *MockConn) SetDeadline(t time.Time) error      { return nil }
func (m *MockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *MockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestServer_WithMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := core.NewMockMeasurementHandler(ctrl)
	mockConnector := core.NewMockConnector(ctrl)

	server := server2.NewServer(mockHandler, mockConnector)

	t.Run("HandleAddSession_Success", func(t *testing.T) {
		mockConn := NewMockConn()
		session := models.Session{
			Id:        1,
			Name:      "Test Session",
			StartTime: "2023-01-01 10:00:00",
			EndTime:   "2023-01-01 11:00:00",
			Count:     10,
		}

		mockHandler.EXPECT().
			SaveSession(session).
			Times(1).
			Return(nil)

		server.HandleAddSession(mockConn, session)
	})

	t.Run("HandleAddSession_Error", func(t *testing.T) {
		mockConn := NewMockConn()
		session := models.Session{Id: 1, Name: "Test Session"}
		expectedError := errors.New("database error")

		mockHandler.EXPECT().
			SaveSession(session).
			Times(1).
			Return(expectedError)

		server.HandleAddSession(mockConn, session)

		writtenData := mockConn.GetWrittenData()
		if !strings.Contains(writtenData, "ERROR") {
			t.Error("Error response should be sent when save fails")
		}
		if !strings.Contains(writtenData, expectedError.Error()) {
			t.Error("Error message should be included in response")
		}
	})

	t.Run("HandleRemoveSession_Success", func(t *testing.T) {
		mockConn := NewMockConn()
		sessionId := 1

		mockHandler.EXPECT().
			RemoveSession(sessionId).
			Times(1).
			Return(nil)

		server.HandleRemoveSession(mockConn, sessionId)
	})

	t.Run("HandleGetMeasurementSessions_Success", func(t *testing.T) {
		mockConn := NewMockConn()
		expectedSessions := []models.Session{
			{Id: 1, Name: "Session 1", Count: 10},
			{Id: 2, Name: "Session 2", Count: 20},
		}

		mockHandler.EXPECT().
			GetAllSessions().
			Times(1).
			Return(expectedSessions, nil)

		mockHandler.EXPECT().
			SendAllSessions(mockConn, gomock.Any()).
			Times(1).
			DoAndReturn(func(conn net.Conn, data string) error {
				if !strings.Contains(data, "Session 1") || !strings.Contains(data, "Session 2") {
					t.Error("Session data not properly formatted")
				}
				return nil
			})

		server.HandleGetMeasurementSessions(mockConn)
	})

	//t.Run("HandleGetMeasurementSessions_Error", func(t *testing.T) {
	//	mockConn := NewMockConn()
	//	expectedError := errors.New("database unavailable")
	//
	//	mockHandler.EXPECT().
	//		GetAllSessions().
	//		Times(1).
	//		Return(nil, expectedError)
	//
	//	mockHandler.EXPECT().
	//		SendAllSessions(gomock.Any(), gomock.Any()).
	//		Times(0)
	//
	//	server.HandleGetMeasurementSessions(mockConn)
	//
	//	writtenData := mockConn.GetWrittenData()
	//	if !strings.Contains(writtenData, "ERROR") {
	//		t.Error("Error response should be sent when get sessions fails")
	//	}
	//})

	t.Run("HandleSetSettings_Success", func(t *testing.T) {
		mockConn := NewMockConn()
		message := "SET_SETTINGS param1=value1"

		mockHandler.EXPECT().
			ProcessInterfaceSettingChange(mockConn, message).
			Times(1).
			Return(nil)

		server.HandleSetSettings(mockConn, message)
	})

	t.Run("HandleConnection_MessageParsing", func(t *testing.T) {
		mockConn := NewMockConn()

		testCases := []struct {
			name      string
			message   string
			setupMock func()
		}{
			{
				name:    "StartMeasurement",
				message: "START_MEASUREMENT: 1\n",
				setupMock: func() {
					mockConnector.EXPECT().IsConnected().Return(true)
					mockConnector.EXPECT().SendCommand("START_MEASUREMENT: ", 1).Return(nil)
				},
			},
			{
				name:    "GetSessions",
				message: "GET_SESSIONS\n",
				setupMock: func() {
					mockHandler.EXPECT().
						GetAllSessions().
						Return([]models.Session{}, nil)
					mockHandler.EXPECT().
						SendAllSessions(mockConn, gomock.Any()).
						Return(nil)
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tc.setupMock()
				mockConn.SetReadData(tc.message)

				go func() {
					_ = server.HandleConnection(mockConn)
				}()

				time.Sleep(100 * time.Millisecond)
			})
		}
	})
}

func TestServer_IntegrationScenarios(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := core.NewMockMeasurementHandler(ctrl)
	mockConnector := core.NewMockConnector(ctrl)
	server := server2.NewServer(mockHandler, mockConnector)

	t.Run("MultipleClients", func(t *testing.T) {
		mockConn1 := NewMockConn()
		mockConn2 := NewMockConn()

		gomock.InOrder(
			mockHandler.EXPECT().
				GetAllSessions().
				Return([]models.Session{{Id: 1, Name: "Session 1"}}, nil),
			mockHandler.EXPECT().
				SendAllSessions(mockConn1, gomock.Any()).
				Return(nil),
			mockHandler.EXPECT().
				GetAllSessions().
				Return([]models.Session{{Id: 1, Name: "Session 1"}}, nil),
			mockHandler.EXPECT().
				SendAllSessions(mockConn2, gomock.Any()).
				Return(nil),
		)

		server.HandleGetMeasurementSessions(mockConn1)
		server.HandleGetMeasurementSessions(mockConn2)
	})

	t.Run("ConcurrentOperations", func(t *testing.T) {
		var wg sync.WaitGroup
		mockConn := NewMockConn()

		mockHandler.EXPECT().
			GetAllSessions().
			MinTimes(5).
			Return([]models.Session{}, nil)
		mockHandler.EXPECT().
			SendAllSessions(mockConn, gomock.Any()).
			MinTimes(5).
			Return(nil)

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				server.HandleGetMeasurementSessions(mockConn)
			}()
		}

		wg.Wait()
	})
}
