package sse_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestSSEHub(t *testing.T) {
	hub := sse.NewHub()
	if hub.ClientCount() != 0 {
		t.Fatalf("expected 0 clients initially, got %d", hub.ClientCount())
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/events/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan bool)
	go func() {
		hub.ServeHTTP(rec, req)
		done <- true
	}()

	time.Sleep(50 * time.Millisecond)
	if hub.ClientCount() != 1 {
		t.Fatalf("expected 1 active client, got %d", hub.ClientCount())
	}

	hub.Broadcast(sse.Event{
		Type: "email_received",
		Data: map[string]string{"id": "mail-1"},
	})

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if hub.ClientCount() != 0 {
		t.Fatalf("expected 0 clients after disconnect, got %d", hub.ClientCount())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: connected") {
		t.Fatalf("expected connected event in stream output")
	}
	if !strings.Contains(body, "event: email_received") {
		t.Fatalf("expected email_received event in stream output")
	}
}
