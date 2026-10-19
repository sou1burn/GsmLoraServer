package configs

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port       string
	DbPort     string
	DbUser     string
	DbPassword string
	DbName     string
	DbHost     string
	Env        string
}

type EspConfig struct {
	EspHost string
	EspPort string
	EspMac  string
	EspIP   string
}

type ClientConfig struct {
	ClientIp   string
	ClientPort string
}

func getenv(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

func LoadConfig() *Config {
	if err := godotenv.Load("db.env"); err != nil {
		log.Println("No .env file found")
	}
	return &Config{
		Port:       getenv("PORT", "9090"),
		DbPort:     getenv("DB_PORT", "5432"),
		DbUser:     getenv("DB_USER", "postgres"),
		DbPassword: getenv("DB_PASSWORD", "xxXX1234"),
		DbName:     getenv("DB_NAME", "server"),
		DbHost:     getenv("DB_HOST", "localhost"),
		Env:        getenv("ENV", "dev"),
	}
}

func LoadEspConfig() *EspConfig {
	if err := godotenv.Load("esp.env"); err != nil {
		log.Println("No esp.env file found")
	}
	return &EspConfig{
		EspHost: getenv("ESP_HOST", "localhost"),
		EspPort: getenv("ESP_PORT", "9090"),
		EspMac:  getenv("ESP_MAC", "FF:FF:FF:FF:FF"),
		EspIP:   getenv("ESP_IP", "127.0.0.1"),
	}
}

func LoadClientConfig() *ClientConfig {
	if err := godotenv.Load("client.env"); err != nil {
		log.Println("No .env file found")
	}
	return &ClientConfig{
		ClientIp:   getenv("CLIENT_IP", "127.0.0.1"),
		ClientPort: getenv("CLIENT_PORT", "9090"),
	}
}
