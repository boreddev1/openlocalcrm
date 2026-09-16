package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Event represents a Server-Sent Event message
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Hub manages active SSE client streams and broadcasting
type Hub struct {
	mu      sync.RWMutex
	clients map[chan Event]bool
}

// NewHub creates a new SSE Hub instance
func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan Event]bool),
	}
}

// Broadcast sends an event to all connected SSE clients
func (h *Hub) Broadcast(evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.clients {
		select {
		case ch <- evt:
		default:
			// Non-blocking drop if client is saturated
		}
	}
}

// ClientCount returns the number of active connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"Streaming unsupported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan Event, 16)
	h.mu.Lock()
	h.clients[ch] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		close(ch)
		h.mu.Unlock()
	}()

	// Send initial connection event
	initData, _ := json.Marshal(map[string]string{"status": "connected"})
	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", string(initData))
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(evt.Data)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, string(data))
			flusher.Flush()
		}
	}
}
