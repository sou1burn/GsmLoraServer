package unit

import (
	"testing"
	"time"

	"github.com/sou1burn/GsmLoraServer/internal/esp"
	"github.com/sou1burn/GsmLoraServer/internal/service"
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
