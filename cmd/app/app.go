package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/sou1burn/GsmLoraServer/configs"
	"github.com/sou1burn/GsmLoraServer/internal/esp"
	"github.com/sou1burn/GsmLoraServer/internal/postgres"
	"github.com/sou1burn/GsmLoraServer/internal/service"
)

type App struct {
	DB           *sql.DB
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

func (a *App) InitService(connector *esp.ESPConnector) {
	a.DataService = service.NewDataService(connector)
}

func (a *App) Close() error {
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}
