package app

import (
	"Lora_Esp_Gsm_Gps_project/cmd/server"
	"Lora_Esp_Gsm_Gps_project/configs"
	"Lora_Esp_Gsm_Gps_project/internal/esp"
	"Lora_Esp_Gsm_Gps_project/internal/postgres"
	"Lora_Esp_Gsm_Gps_project/internal/service"
	"context"
	"database/sql"
	"errors"
)

type App struct {
	DB           *sql.DB
	Server       *server.Server
	DataService  *service.DataService
	EspConnector *esp.ESPConnector
}

func New() *App {
	return &App{}
}

func (a *App) InitDB(cfg *configs.Config) error {
	var db *sql.DB
	var err error
	db, err = postgres.NewPostgresDB(*cfg)
	if err != nil {
		return err
	}

	if err = postgres.Migrate(context.Background(), db); err != nil {
		return errors.Join(err, db.Close())
	}
	repo := postgres.NewPostgresRepo(db)
	if a.DataService == nil {
		a.DataService = service.NewDataService(a.EspConnector)
	}

	a.DB = db
	a.DataService.Repo = repo
	return nil
}

func (a *App) InitServer(cfg *configs.Config) error {
	a.Server = server.NewServer(a.DataService, a.EspConnector)
	go func() {
		err := a.Server.StartServer(cfg.Port)
		if err != nil {
			panic(err)
		}
	}()

	return nil
}

func (a *App) InitService(connector *esp.ESPConnector) {
	a.DataService = service.NewDataService(connector)
}

func (a *App) Close() error {
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}
