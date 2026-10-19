package models

import "time"

const TimeLayout = "2006-01-02 15:04:05.999999999 -0700 MST"

type Destination int

const (
	Unknown Destination = iota
	Lora
	Client
)

func (d Destination) String() string {
	switch d {
	case Lora:
		return "Lora"
	case Client:
		return "Client"
	default:
		return "Unknown"
	}
}

type Coordinate struct {
	Latitude  float64 `json:"latitude,string"`
	Longitude float64 `json:"longitude,string"`
}

type LoRaData struct {
	RequestId int     `json:"id"`
	RSSI      int     `json:"rssi"`
	SNR       float64 `json:"snr"`
	Timestamp string  `json:"timestamp"`
}

type Packet struct {
	MeasurementId int        `json:"id"`
	PacketNum     int        `json:"packet_number"`
	RSSI          float64    `json:"rssi"`
	SNRL          float64    `json:"snr"`
	Coordinate    Coordinate `json:"coordinate"`
	Hdop          float32    `json:"hdop"`
	Timestamp     time.Time  `json:"timestamp"`
}

type Params struct {
	Bandwidth float32 `json:"bandwidth"`
	Sf        float32 `json:"sf"`
	Tx        float32 `json:"tx"`
}

type Session struct {
	Id        int     `json:"id"`
	Name      string  `json:"name"`
	StartTime string  `json:"start_time"`
	EndTime   string  `json:"end_time"`
	Count     int     `json:"count"`
	Sf        float32 `json:"sf"`
	Tx        float32 `json:"tx"`
	Bandwidth float32 `json:"bandwidth"`
}
