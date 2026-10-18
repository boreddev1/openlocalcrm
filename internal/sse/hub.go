package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	// DefaultMaxClients bounds the number of concurrent SSE streams. Without a
	// bound a flood of connections can exhaust goroutines and sockets.
	DefaultMaxClients = 1000
	// DefaultHeartbeatInterval is how often an idle stream receives a comment
	// frame so proxies and NAT keep the connection alive.
	DefaultHeartbeatInterval = 25 * time.Second
	// DefaultWriteDeadline bounds a single write to a client, so a hung TCP
	// connection cannot pin the stream goroutine indefinitely.
	DefaultWriteDeadline = 10 * time.Second
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

	// MaxClients is the maximum number of concurrent streams. Streams beyond
	// the limit are declined with 503 Service Unavailable. A non-positive
	// value disables the cap.
	MaxClients int
	// HeartbeatInterval is the period of the SSE comment keep-alive frames.
	// A non-positive value disables heartbeats.
	HeartbeatInterval time.Duration
	// WriteDeadline bounds each write to a client. A non-positive value
	// disables the per-write deadline.
	WriteDeadline time.Duration
}

// NewHub creates a new SSE Hub instance with finite production defaults.
func NewHub() *Hub {
	return &Hub{
		clients:           make(map[chan Event]bool),
		MaxClients:        DefaultMaxClients,
		HeartbeatInterval: DefaultHeartbeatInterval,
		WriteDeadline:     DefaultWriteDeadline,
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
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, `{"error":"Streaming unsupported"}`, http.StatusInternalServerError)
		return
	}

	ch := make(chan Event, 16)

	h.mu.Lock()
	if h.MaxClients > 0 && len(h.clients) >= h.MaxClients {
		h.mu.Unlock()
		http.Error(w, `{"error":"Too many concurrent streams"}`, http.StatusServiceUnavailable)
		return
	}
	h.clients[ch] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		if _, ok := h.clients[ch]; ok {
			delete(h.clients, ch)
			close(ch)
		}
		h.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)
	heartbeat := h.HeartbeatInterval
	var ticker *time.Ticker
	var ticks <-chan time.Time
	if heartbeat > 0 {
		ticker = time.NewTicker(heartbeat)
		defer ticker.Stop()
		ticks = ticker.C
	}

	// Send initial connection event
	initData, _ := json.Marshal(map[string]string{"status": "connected"})
	if !h.writeFrame(w, rc, fmt.Sprintf("event: connected\ndata: %s\n\n", string(initData))) {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticks:
			if !h.writeFrame(w, rc, ": heartbeat\n\n") {
				return
			}
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(evt.Data)
			if err != nil {
				continue
			}
			if !h.writeFrame(w, rc, fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Type, string(data))) {
				return
			}
		}
	}
}

// writeFrame applies the per-write deadline through the http.ResponseController
// and reports whether the frame was written and flushed successfully.
func (h *Hub) writeFrame(w http.ResponseWriter, rc *http.ResponseController, frame string) bool {
	if h.WriteDeadline > 0 {
		// ErrNotSupported is expected for ResponseWriters without a connection
		// (for example httptest.ResponseRecorder); the write is still attempted.
		_ = rc.SetWriteDeadline(time.Now().Add(h.WriteDeadline))
	}
	if _, err := fmt.Fprint(w, frame); err != nil {
		return false
	}
	if err := rc.Flush(); err != nil {
		return false
	}
	return true
}
