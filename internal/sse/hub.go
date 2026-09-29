package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Event represents a Server-Sent Event message
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Hub manages active SSE client streams and broadcasting
type Hub struct {
	mu      sync.RWMutex
	clients map[chan Event]uuid.UUID
}

// NewHub creates a new SSE Hub instance
func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan Event]uuid.UUID),
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

// SendToUser sends an event only to the clients registered with the given userID
func (h *Hub) SendToUser(userID uuid.UUID, evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch, owner := range h.clients {
		if owner == userID {
			select {
			case ch <- evt:
			default:
				// Non-blocking drop if client is saturated
			}
		}
	}
}

// ClientCount returns the number of active connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// register records a subscriber's channel against its user id and returns the
// channel, so a test can observe the close.
func (h *Hub) register(userID uuid.UUID) chan Event {
	ch := make(chan Event, 16)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = userID
	return ch
}

// RevokeUser closes every stream belonging to userID. A role or status change
// must terminate a long-lived stream immediately, not at token expiry.
func (h *Hub) RevokeUser(userID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, owner := range h.clients {
		if owner == userID {
			close(ch)
			delete(h.clients, ch)
		}
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.ServeHTTPFor(w, r, uuid.Nil)
}

func (h *Hub) ServeHTTPFor(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"Streaming unsupported"}`, http.StatusInternalServerError)
		return
	}

	// Disable HTTP server write deadline for this long-lived SSE stream
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.register(userID)

	defer func() {
		h.mu.Lock()
		if _, ok := h.clients[ch]; ok {
			delete(h.clients, ch)
			close(ch)
		}
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
