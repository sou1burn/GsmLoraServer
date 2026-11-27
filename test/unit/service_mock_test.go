package unit

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"

	telemetryv1 "github.com/sou1burn/GsmLoraServer/api/proto/telemetry/v1"
	"github.com/sou1burn/GsmLoraServer/internal/esp"
	"github.com/sou1burn/GsmLoraServer/internal/handlers"
	"github.com/sou1burn/GsmLoraServer/internal/models"
	"github.com/sou1burn/GsmLoraServer/internal/service"
	mocks "github.com/sou1burn/GsmLoraServer/mocks"
)

func newServiceWithMockRepo(t *testing.T) (*service.DataService, *mocks.MockRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	repo := mocks.NewMockRepository(ctrl)
	svc := service.NewDataService(esp.NewESPConnector())
	svc.Repo = repo
	return svc, repo
}

func TestGetAllSessions_DelegatesToRepo(t *testing.T) {
	svc, repo := newServiceWithMockRepo(t)
	ctx := context.Background()
	sessions := []models.Session{{Id: 1, Name: "s1"}}

	repo.EXPECT().GetAllSessions(ctx).Times(1).Return(sessions, nil)

	got, err := svc.GetAllSessions(ctx)
	if err != nil {
		t.Fatalf("GetAllSessions() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "s1" {
		t.Errorf("GetAllSessions() = %+v, want %+v", got, sessions)
	}
}

func TestGetAllSessions_RepoErrorPropagates(t *testing.T) {
	svc, repo := newServiceWithMockRepo(t)
	wantErr := errors.New("db down")

	repo.EXPECT().GetAllSessions(gomock.Any()).Times(1).Return(nil, wantErr)

	if _, err := svc.GetAllSessions(context.Background()); !errors.Is(err, wantErr) {
		t.Errorf("ожидали ошибку %v, получено %v", wantErr, err)
	}
}

func TestSaveSession_RepoErrorPropagates(t *testing.T) {
	svc, repo := newServiceWithMockRepo(t)
	wantErr := errors.New("insert failed")

	repo.EXPECT().SaveSession(gomock.Any(), gomock.Any()).Times(1).Return(wantErr)

	if err := svc.SaveSession(context.Background(), models.Session{Id: 1}); !errors.Is(err, wantErr) {
		t.Errorf("ожидали ошибку %v, получено %v", wantErr, err)
	}
}

func TestRemoveSession_RepoErrorPropagates(t *testing.T) {
	svc, repo := newServiceWithMockRepo(t)
	wantErr := errors.New("delete failed")

	repo.EXPECT().RemoveSession(gomock.Any(), 7).Times(1).Return(wantErr)

	if err := svc.RemoveSession(context.Background(), 7); !errors.Is(err, wantErr) {
		t.Errorf("ожидали ошибку %v, получено %v", wantErr, err)
	}
}

func TestAddPendingMessage_StoresPerType(t *testing.T) {
	svc, _ := newServiceWithMockRepo(t)

	svc.AddPendingMessage("SET_SETTINGS: SF=7, TX=14, BW=125", handlers.SetSettingsMessage{
		Params: &telemetryv1.SetSettings{Params: &telemetryv1.LoRaParams{Sf: 7, Tx: 14, Bw: 125}},
	})
	svc.AddPendingMessage("START_MEASUREMENT: 3", handlers.StartMeasurementMessage{})

	setSettings := svc.PendingMessages["SET_SETTINGS"]
	if setSettings == nil {
		t.Fatal("ожидали отложенное сообщение SetSettingsMessage")
	}
	got := setSettings.Type.(handlers.SetSettingsMessage).Params.GetParams()
	if got.GetSf() != 7 || got.GetTx() != 14 || got.GetBw() != 125 {
		t.Errorf("канон SetSettings искажён: %+v", got)
	}

	start := svc.PendingMessages["START_MEASUREMENT"]
	if start == nil {
		t.Fatal("ожидали отложенное сообщение StartMeasurementMessage")
	}
	if start.Message != "START_MEASUREMENT: 3" {
		t.Errorf("сообщение StartMeasurement искажено: %+v", start)
	}
}

func TestProcessPacketData_DropsInvalidPacketNum(t *testing.T) {
	svc, _ := newServiceWithMockRepo(t)
	for _, num := range []int{0, -3, 11, 999} {
		if err := svc.ProcessPacketData(models.Packet{MeasurementId: 1, PacketNum: num}); err != nil {
			t.Fatalf("packet_num=%d: err = %v", num, err)
		}
	}
	if err := svc.ProcessPacketData(models.Packet{MeasurementId: 1, PacketNum: 1}); err != nil {
		t.Fatalf("валидный пакет отклонён: %v", err)
	}
}

func TestSetSettingsForSession_WritesToCleanSession(t *testing.T) {
	svc, repo := newServiceWithMockRepo(t)

	repo.EXPECT().
		AddSettingsToSession(gomock.Any(), gomock.Any(), float32(7), float32(14), float32(125)).
		Times(1).
		Return(true, nil)

	if err := svc.SetSettingsForSession(context.Background(), models.Params{Sf: 7, Tx: 14, Bandwidth: 125}); err != nil {
		t.Fatalf("ожидали успех, получили %v", err)
	}
}

func TestSetSettingsForSession_RejectsWhenPacketsExist(t *testing.T) {
	svc, repo := newServiceWithMockRepo(t)

	repo.EXPECT().
		AddSettingsToSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1).
		Return(false, nil)
	repo.EXPECT().
		SessionHasPackets(gomock.Any(), gomock.Any()).
		Times(1).
		Return(true, nil)

	if err := svc.SetSettingsForSession(context.Background(), models.Params{Sf: 7}); err == nil {
		t.Fatal("ожидали ошибку (ALREADY_SET) при наличии пакетов в сессии")
	}
}

func TestSetSettingsForSession_SilentWhenAlreadySet(t *testing.T) {
	svc, repo := newServiceWithMockRepo(t)

	repo.EXPECT().
		AddSettingsToSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1).
		Return(false, nil)
	repo.EXPECT().
		SessionHasPackets(gomock.Any(), gomock.Any()).
		Times(1).
		Return(false, nil)

	if err := svc.SetSettingsForSession(context.Background(), models.Params{Sf: 7}); err != nil {
		t.Fatalf("ожидали тихий успех (настройки уже заданы), получили %v", err)
	}
}
