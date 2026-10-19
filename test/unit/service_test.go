package unit

import (
	"Lora_Esp_Gsm_Gps_project/internal/esp"
	"Lora_Esp_Gsm_Gps_project/internal/service"
	"testing"
	"time"
)

func TestPendingTickDoesNotDeadlock(t *testing.T) {
	connector := esp.NewESPConnector()
	connector.SetConnected(true)
	s := service.NewDataService(connector)

	done := make(chan struct{})
	go func() {
		s.PendingTick()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pendingTick завис: pendingMessagesLock захватывается повторно")
	}
}
