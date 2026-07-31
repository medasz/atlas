package scan

import (
	"context"
	"testing"
)

func TestPipelineEventChannel(t *testing.T) {
	events := make(chan openPortEvent, 10)
	events <- openPortEvent{IP: "127.0.0.1", Port: 80, Banner: "HTTP/1.1", IsV6: false}
	close(events)

	e, ok := <-events
	if !ok {
		t.Fatalf("expected event, got closed channel")
	}
	if e.IP != "127.0.0.1" || e.Port != 80 {
		t.Errorf("expected 127.0.0.1:80, got %s:%d", e.IP, e.Port)
	}
}

func TestScannerCloseGraceful(t *testing.T) {
	sc := &Scanner{}
	sc.startPipeline(context.Background())
	if sc.cancel == nil {
		t.Fatalf("expected cancel func, got nil")
	}
	sc.Close()
}
