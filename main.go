package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sou1burn/GsmLoraServer/cmd/app"
	"github.com/sou1burn/GsmLoraServer/cmd/server"
	"github.com/sou1burn/GsmLoraServer/configs"
	"github.com/sou1burn/GsmLoraServer/internal/esp"
)

var (
	version   = "dev"
	buildTime = time.Now().String()
)

func main() {
	log.Printf("Lora ESP GSM GPS Project v.%s (built %s)\n", version, buildTime)
	logFile, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("Не удалось открыть файл для логов: %v", err)
	} else {
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(multiWriter)
		defer func(logFile *os.File) {
			err := logFile.Close()
			if err != nil {
				log.Printf("Error closing log file: %v", err)
			}
		}(logFile)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	espConnector := esp.NewESPConnector()

	appInstance := app.New()

	dbCfg := configs.LoadConfig()
	appInstance.EspConnector = espConnector
	appInstance.InitService(espConnector)

	err = appInstance.InitDB(dbCfg)
	if err != nil {
		log.Println("Error connecting to database:", err)
		return
	}
	defer func(appInstance *app.App) {
		err := appInstance.Close()
		if err != nil {
			log.Println("Error closing app:", err)
		}
	}(appInstance)

	srv := server.NewServer(appInstance.DataService, espConnector)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	appInstance.DataService.StartProcessing(ctx)
	defer cancel()

	go func(ctx context.Context) {
		if err := srv.StartServer(ctx, dbCfg.Port); err != nil {
			log.Printf("Error starting server: %v", err)
		}
	}(ctx)

	<-ctx.Done()
	srv.Shutdown()
	err = appInstance.DataService.DrainDataChannel()
	if err != nil {
		log.Printf("Error draining data chan: %v", err)
	}
}
