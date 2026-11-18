package unit

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"

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

	svc.AddPendingMessage("SET_SETTINGS: SF=7, TX=14, BW=125", handlers.SetSettingsMessage{})
	svc.AddPendingMessage("START_MEASUREMENT: 3", handlers.StartMeasurementMessage{})

	setSettings := svc.PendingMessages[reflect.TypeOf(handlers.SetSettingsMessage{})]
	if setSettings == nil {
		t.Fatal("ожидали отложенное сообщение SetSettingsMessage")
	}
	if setSettings.Message != "SET_SETTINGS: SF=7, TX=14, BW=125" {
		t.Errorf("сообщение SetSettings искажено: %+v", setSettings)
	}

	start := svc.PendingMessages[reflect.TypeOf(handlers.StartMeasurementMessage{})]
	if start == nil {
		t.Fatal("ожидали отложенное сообщение StartMeasurementMessage")
	}
	if start.Message != "START_MEASUREMENT: 3" {
		t.Errorf("сообщение StartMeasurement искажено: %+v", start)
	}
}
