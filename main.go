package main

import (
	"Lora_Esp_Gsm_Gps_project/cmd/app"
	"Lora_Esp_Gsm_Gps_project/cmd/server"
	"Lora_Esp_Gsm_Gps_project/configs"
	"Lora_Esp_Gsm_Gps_project/internal/esp"
	"context"
	"io"
	"log"
	"os"
	"time"
)

var (
	version   = "dev"
	buildTime = time.Now().String()
)

func main() {
	log.Printf("Lora ESP GSM GPS Project v.%s (built %s)\n", version, buildTime)
	logFile, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Не удалось открыть файл для логов: %v", err)
	} else {
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(multiWriter)
	}

	defer func(logFile *os.File) {
		err := logFile.Close()
		if err != nil {
			log.Printf("Error closing log file: %v", err)
		}
	}(logFile)

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
	appInstance.DataService.StartProcessing(context.Background())

	go func() {
		if err := srv.StartServer(dbCfg.Port); err != nil {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	select {}
}
