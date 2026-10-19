package unit

import (
	"Lora_Esp_Gsm_Gps_project/internal/handlers"
	"Lora_Esp_Gsm_Gps_project/internal/models"
	"encoding/json"
	"testing"
	"time"
)

func TestLoraParser_ParseLoraData(t *testing.T) {
	validJSON := `{"id":7,"rssi":-85,"snr":9.5,"error_bits":0,"checksum":"xyz"}`

	tests := []struct {
		name    string
		input   string
		wantErr bool
		want    *models.LoRaData
	}{
		{
			name:    "valid LoRa data",
			input:   validJSON,
			wantErr: false,
			want: &models.LoRaData{
				RequestId: 7,
				RSSI:      -85,
				SNR:       9.5,
			},
		},
		{
			name:    "invalid JSON",
			input:   `{bad`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := &handlers.LoraParser{}
			err := parser.ParseLoraData(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLoraData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if parser.RequestId != 7 {
					t.Errorf("RequestId = %d, want 7", parser.RequestId)
				}
				if parser.Checksum == nil || *parser.Checksum != "xyz" {
					t.Errorf("Checksum = %v, want xyz", parser.Checksum)
				}
				if parser.Data.RequestId != tt.want.RequestId ||
					parser.Data.RSSI != tt.want.RSSI ||
					parser.Data.SNR != tt.want.SNR {
					t.Errorf("LoRaData = %+v, want %+v", parser.Data, tt.want)
				}
			}
		})
	}
}

func TestPacketParser_ParsePacketData(t *testing.T) {
	samplePacket := models.Packet{
		MeasurementId: 123,
		PacketNum:     1,
		RSSI:          -75.5,
		SNRL:          8.2,
		Coordinate: models.Coordinate{
			Latitude:  48.1173,
			Longitude: 11.5167,
		},
		Hdop:      0.9,
		Timestamp: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	jsonData, _ := json.Marshal(samplePacket)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid packet",
			input:   string(jsonData),
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   "{bad",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := &handlers.PacketParser{}
			got, err := parser.ParsePacketData(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePacketData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				var original models.Packet
				if err := json.Unmarshal([]byte(tt.input), &original); err != nil {
					t.Fatalf("Failed to unmarshal test input: %v", err)
				}
				// Now compare all fields.
				if got != original {
					t.Errorf("Packet mismatch:\n got %+v\nwant %+v", got, original)
				}
			}
		})
	}
}

func TestParseClientMessage(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    handlers.Message
		wantErr bool
	}{
		{
			name:  "SET_SETTINGS",
			input: "SET_SETTINGS: SF=12, TX=20, BW=125",
			want: handlers.SetSettingsMessage{
				Params: models.Params{Sf: 12, Tx: 20, Bandwidth: 125},
			},
		},
		{
			name:  "START_MEASUREMENT",
			input: "START_MEASUREMENT: 5",
			want:  handlers.StartMeasurementMessage{SessionId: 5},
		},
		{
			name:  "GET_MEASUREMENT_SESSIONS",
			input: "GET_MEASUREMENT_SESSIONS",
			want:  handlers.GetMeasurementSessionsMessage{},
		},
		{
			name:  "ADD_SESSION valid",
			input: "ADD_SESSION: [1, test, 2023-01-01T00:00:00Z, 2023-01-01T01:00:00Z, 10]",
			want: handlers.AddSessionMessage{
				Session: models.Session{
					Id:        1,
					Name:      "test",
					StartTime: "2023-01-01T00:00:00Z",
					EndTime:   "2023-01-01T01:00:00Z",
					Count:     10,
				},
			},
		},
		{
			name:    "ADD_SESSION invalid format",
			input:   "ADD_SESSION: [1, test, 2023-01-01T00:00:00Z, 2023-01-01T01:00:00Z]",
			wantErr: true,
		},
		{
			name:  "REMOVE_SESSION",
			input: "REMOVE_SESSION: 42",
			want:  handlers.RemoveSessionMessage{SessionId: 42},
		},
		{
			name:    "REMOVE_SESSION invalid ID",
			input:   "REMOVE_SESSION: abc",
			wantErr: true,
		},
		{
			name:  "IDENTIFY",
			input: "IDENTIFY",
			want:  handlers.EspMessage{},
		},
		{
			name:  "MEASUREMENT with valid JSON",
			input: `MEASUREMENT: {"id":1,"packet_number":1,"rssi":-75.5,"snr":8.2,"coordinate":{"latitude":"48.1173","longitude":"11.5167"},"hdop":0.9,"timestamp":"2023-01-01T00:00:00Z"}`,
			want: handlers.IncomingMeasurementMessage{
				Data: models.Packet{
					MeasurementId: 1,
					PacketNum:     1,
					RSSI:          -75.5,
					SNRL:          8.2,
					Coordinate: models.Coordinate{
						Latitude:  48.1173,
						Longitude: 11.5167,
					},
					Hdop:      0.9,
					Timestamp: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				// sessionId is unexported, so we cannot set or compare it.
			},
		},
		{
			name:    "MEASUREMENT with invalid JSON",
			input:   `MEASUREMENT: {bad`,
			wantErr: true,
		},
		{
			name:  "ACK",
			input: "ACK something",
			want:  handlers.AckMessage{},
		},
		{
			name:  "ERROR",
			input: "ERROR: something went wrong",
			want:  handlers.ErrMessage{}, // error string is unexported, we only check type
		},
		{
			name:  "MASTER_STATUS",
			input: "MASTER_STATUS idle",
			want:  handlers.MasterStatusMessage{Status: "idle"},
		},
		{
			name:  "SLAVE_STATUS",
			input: "SLAVE_STATUS busy",
			want:  handlers.SlaveStatusMessage{Status: "busy"},
		},
		{
			name:  "unknown message",
			input: "UNKNOWN_COMMAND",
			want:  handlers.UnknownMessage{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := handlers.ParseClientMessage(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseClientMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Compare messages by type and exported fields.
			switch want := tt.want.(type) {
			case handlers.SetSettingsMessage:
				g, ok := got.(handlers.SetSettingsMessage)
				if !ok || g.Params != want.Params {
					t.Errorf("got %+v, want %+v", g, want)
				}
			case handlers.StartMeasurementMessage:
				g, ok := got.(handlers.StartMeasurementMessage)
				if !ok || g.SessionId != want.SessionId {
					t.Errorf("got %+v, want %+v", g, want)
				}
			case handlers.GetMeasurementSessionsMessage:
				_, ok := got.(handlers.GetMeasurementSessionsMessage)
				if !ok {
					t.Errorf("got %T, want GetMeasurementSessionsMessage", got)
				}
			case handlers.AddSessionMessage:
				g, ok := got.(handlers.AddSessionMessage)
				if !ok || g.Session != want.Session {
					t.Errorf("got %+v, want %+v", g, want)
				}
			case handlers.RemoveSessionMessage:
				g, ok := got.(handlers.RemoveSessionMessage)
				if !ok || g.SessionId != want.SessionId {
					t.Errorf("got %+v, want %+v", g, want)
				}
			case handlers.EspMessage:
				_, ok := got.(handlers.EspMessage)
				if !ok {
					t.Errorf("got %T, want EspMessage", got)
				}
			case handlers.IncomingMeasurementMessage:
				g, ok := got.(handlers.IncomingMeasurementMessage)
				if !ok {
					t.Errorf("got %T, want IncomingMeasurementMessage", got)
					return
				}
				// Compare exported Data field.
				if g.Data != want.Data {
					t.Errorf("Data mismatch:\n got %+v\nwant %+v", g.Data, want.Data)
				}
				// sessionId is unexported, cannot compare.
			case handlers.AckMessage:
				_, ok := got.(handlers.AckMessage)
				if !ok {
					t.Errorf("got %T, want AckMessage", got)
				}
			case handlers.ErrMessage:
				_, ok := got.(handlers.ErrMessage)
				if !ok {
					t.Errorf("got %T, want ErrMessage", got)
				}
				// error string is unexported; we don't verify its content.
			case handlers.MasterStatusMessage:
				g, ok := got.(handlers.MasterStatusMessage)
				if !ok || g.Status != want.Status {
					t.Errorf("got %+v, want %+v", g, want)
				}
			case handlers.SlaveStatusMessage:
				g, ok := got.(handlers.SlaveStatusMessage)
				if !ok || g.Status != want.Status {
					t.Errorf("got %+v, want %+v", g, want)
				}
			case handlers.UnknownMessage:
				_, ok := got.(handlers.UnknownMessage)
				if !ok {
					t.Errorf("got %T, want UnknownMessage", got)
				}
			default:
				t.Errorf("unexpected want type %T", want)
			}
		})
	}
}

func TestParseSession(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    models.Session
		wantErr bool
	}{
		{
			name:  "valid session",
			input: "ADD_SESSION: [1, session1, 2023-01-01T00:00:00Z, 2023-01-01T01:00:00Z, 10]",
			want: models.Session{
				Id:        1,
				Name:      "session1",
				StartTime: "2023-01-01T00:00:00Z",
				EndTime:   "2023-01-01T01:00:00Z",
				Count:     10,
			},
		},
		{
			name:    "invalid number of fields",
			input:   "ADD_SESSION: [1, session1, 2023-01-01T00:00:00Z, 2023-01-01T01:00:00Z]",
			wantErr: true,
		},
		{
			name:    "non-numeric ID",
			input:   "ADD_SESSION: [abc, session1, 2023-01-01T00:00:00Z, 2023-01-01T01:00:00Z, 10]",
			wantErr: true,
		},
		{
			name:    "non-numeric count",
			input:   "ADD_SESSION: [1, session1, 2023-01-01T00:00:00Z, 2023-01-01T01:00:00Z, ten]",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := handlers.ParseSession(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseSession() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseRemoveSessionMessage(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:  "valid",
			input: "REMOVE_SESSION: 123",
			want:  123,
		},
		{
			name:    "invalid number",
			input:   "REMOVE_SESSION: abc",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "REMOVE_SESSION: ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := handlers.ParseRemoveSessionMessage(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRemoveSessionMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseRemoveSessionMessage() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	expected := time.Date(2023, 1, 1, 12, 0, 0, 123000000, time.UTC)
	validTimeStr := expected.String()

	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "valid format",
			input: validTimeStr,
			want:  expected,
		},
		{
			name:    "invalid format",
			input:   "2023-01-01T12:00:00Z",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := handlers.ParseTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("ParseTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseIncomingMeasurement(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    handlers.IncomingMeasurementMessage
		wantErr bool
	}{
		{
			name:  "valid JSON",
			input: `MEASUREMENT: {"id":1,"packet_number":1,"rssi":-75.5,"snr":8.2,"coordinate":{"latitude":"48.1173","longitude":"11.5167"},"hdop":0.9,"timestamp":"2023-01-01T00:00:00Z"}`,
			want: handlers.IncomingMeasurementMessage{
				Data: models.Packet{
					MeasurementId: 1,
					PacketNum:     1,
					RSSI:          -75.5,
					SNRL:          8.2,
					Coordinate: models.Coordinate{
						Latitude:  48.1173,
						Longitude: 11.5167,
					},
					Hdop:      0.9,
					Timestamp: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},
		{
			name:    "invalid JSON",
			input:   `MEASUREMENT: {bad`,
			wantErr: true,
		},
		{
			name:    "missing prefix",
			input:   `{"id":1}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := handlers.ParseIncomingMeasurement(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseIncomingMeasurement() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Data != tt.want.Data {
					t.Errorf("Data mismatch:\n got %+v\nwant %+v", got.Data, tt.want.Data)
				}
			}
		})
	}
}
