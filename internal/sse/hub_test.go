package sse_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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

// ---------------------------------------------------------------------------
// shared test helpers
// ---------------------------------------------------------------------------

func waitFor(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

// recordingWriter is a thread-safe http.ResponseWriter + http.Flusher whose
// writes can optionally be made to block forever (a client that never reads).
// SetWriteDeadline is advertised but ignored, so a client that is deliberately
// blocked stays blocked.
type recordingWriter struct {
	mu          sync.Mutex
	header      http.Header
	body        strings.Builder
	flushes     int
	block       chan struct{}
	releaseOnce sync.Once
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{header: make(http.Header)}
}

func newBlockingWriter() *recordingWriter {
	w := newRecordingWriter()
	w.block = make(chan struct{})
	return w
}

func (w *recordingWriter) Header() http.Header { return w.header }
func (w *recordingWriter) WriteHeader(int)     {}
func (w *recordingWriter) Flush() {
	w.mu.Lock()
	w.flushes++
	w.mu.Unlock()
}

func (w *recordingWriter) SetWriteDeadline(time.Time) error { return nil }

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	block := w.block
	w.mu.Unlock()
	if block != nil {
		<-block
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.body.Write(p)
	return len(p), nil
}

func (w *recordingWriter) written() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

func (w *recordingWriter) release() {
	w.releaseOnce.Do(func() {
		w.mu.Lock()
		b := w.block
		w.mu.Unlock()
		if b != nil {
			close(b)
		}
	})
}

func startClient(hub *sse.Hub, w http.ResponseWriter) (context.CancelFunc, chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/events/stream", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		hub.ServeHTTP(w, req)
		close(done)
	}()
	return cancel, done
}

// commentFrame returns the first SSE frame in body whose first line is an SSE
// comment (a line beginning with ':'), or "" if there is none.
func commentFrame(body string) string {
	for _, frame := range strings.Split(body, "\n\n") {
		lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
		if len(lines) > 0 && strings.HasPrefix(lines[0], ":") {
			return frame
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// AC-1-cap-enforced
// ---------------------------------------------------------------------------

func TestAC1CapEnforced(t *testing.T) {
	hub := sse.NewHub()
	hub.MaxClients = 1

	w1 := newRecordingWriter()
	c1, d1 := startClient(hub, w1)
	defer func() { c1(); w1.release(); <-d1 }()

	if !waitFor(func() bool { return hub.ClientCount() == 1 }, 2*time.Second) {
		t.Fatalf("AC-1: first client never registered (count=%d)", hub.ClientCount())
	}

	maxSeen := 0
	sample := func() {
		if n := hub.ClientCount(); n > maxSeen {
			maxSeen = n
		}
	}

	w2 := newRecordingWriter()
	c2, d2 := startClient(hub, w2)
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		sample()
		time.Sleep(time.Millisecond)
	}
	sample()
	c2()
	w2.release()
	<-d2

	if maxSeen > 1 {
		t.Fatalf("AC-1: live registered clients exceeded configured max=1 (observed %d)", maxSeen)
	}
	if n := hub.ClientCount(); n > 1 {
		t.Fatalf("AC-1: stream beyond the cap stayed registered (count=%d)", n)
	}
}

// ---------------------------------------------------------------------------
// AC-2-heartbeat
// ---------------------------------------------------------------------------

func TestAC2Heartbeat(t *testing.T) {
	hub := sse.NewHub()
	hub.MaxClients = 10
	hub.HeartbeatInterval = 20 * time.Millisecond

	w := newRecordingWriter()
	cancel, done := startClient(hub, w)
	defer cancel()

	if !waitFor(func() bool { return hub.ClientCount() == 1 }, time.Second) {
		t.Fatalf("AC-2: client never registered")
	}
	if !waitFor(func() bool { return strings.Contains(w.written(), "connected") }, time.Second) {
		t.Fatalf("AC-2: connected frame never written")
	}

	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	body := w.written()
	frame := commentFrame(body)
	if frame == "" {
		t.Fatalf("AC-2: no keep-alive SSE comment (':' line) received within the injected interval; body=%q", body)
	}
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "data:") {
			t.Fatalf("AC-2: keep-alive comment frame carried an event data line: %q", frame)
		}
	}
}

// ---------------------------------------------------------------------------
// AC-3-write-deadline
// ---------------------------------------------------------------------------

// deadlineWriter simulates a client whose underlying write blocks. It lets the
// first Write through (the initial connected frame) and blocks every later
// write until the deadline set via SetWriteDeadline elapses, then fails. If no
// deadline is ever set the blocked write never returns.
type deadlineWriter struct {
	mu            sync.Mutex
	header        http.Header
	allowed       int
	deadlineSet   bool
	deadline      time.Time
	deadlineCalls int
	unblock       chan struct{}
	unblockOnce   sync.Once
}

func newDeadlineWriter(allowed int) *deadlineWriter {
	return &deadlineWriter{header: make(http.Header), allowed: allowed, unblock: make(chan struct{})}
}

func (w *deadlineWriter) Header() http.Header { return w.header }
func (w *deadlineWriter) WriteHeader(int)     {}
func (w *deadlineWriter) Flush()              {}

func (w *deadlineWriter) SetWriteDeadline(t time.Time) error {
	w.mu.Lock()
	w.deadlineSet = true
	w.deadline = t
	w.deadlineCalls++
	w.mu.Unlock()
	return nil
}

func (w *deadlineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.allowed > 0 {
		w.allowed--
		w.mu.Unlock()
		return len(p), nil
	}
	set, d := w.deadlineSet, w.deadline
	w.mu.Unlock()
	if !set {
		<-w.unblock
		return 0, errors.New("write blocked and no write deadline was applied")
	}
	if wait := time.Until(d); wait > 0 {
		time.Sleep(wait)
	}
	return 0, os.ErrDeadlineExceeded
}

func (w *deadlineWriter) unblockAll() {
	w.unblockOnce.Do(func() { close(w.unblock) })
}

func (w *deadlineWriter) deadlineWasSet() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.deadlineSet
}

func TestAC3WriteDeadline(t *testing.T) {
	hub := sse.NewHub()
	hub.MaxClients = 10
	hub.HeartbeatInterval = 20 * time.Millisecond
	hub.WriteDeadline = 50 * time.Millisecond

	w := newDeadlineWriter(1) // allow the initial connected frame, block the rest
	defer w.unblockAll()

	cancel, done := startClient(hub, w)
	defer cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("AC-3: handler blocked on a non-reading client instead of returning at the write deadline")
	}

	if !w.deadlineWasSet() {
		t.Fatalf("AC-3: SetWriteDeadline was never called on the underlying ResponseWriter")
	}
}

// ---------------------------------------------------------------------------
// AC-4-broadcast-nonblocking
// ---------------------------------------------------------------------------

func TestAC4BroadcastNonBlocking(t *testing.T) {
	hub := sse.NewHub()
	hub.MaxClients = 10

	// Slow client: its writes block forever, so its handler never drains the
	// channel and the buffer fills up.
	slow := newBlockingWriter()
	slowCancel, slowDone := startClient(hub, slow)
	defer func() { slow.release(); slowCancel(); <-slowDone }()

	if !waitFor(func() bool { return hub.ClientCount() >= 1 }, time.Second) {
		t.Fatalf("AC-4: slow client never registered")
	}

	fast := newRecordingWriter()
	fastCancel, fastDone := startClient(hub, fast)
	defer func() { fastCancel(); fast.release(); <-fastDone }()

	if !waitFor(func() bool { return hub.ClientCount() >= 2 }, time.Second) {
		t.Fatalf("AC-4: fast client never registered")
	}
	if !waitFor(func() bool { return strings.Contains(fast.written(), "connected") }, time.Second) {
		t.Fatalf("AC-4: fast client never received the connected frame")
	}

	filled := make(chan struct{})
	go func() {
		for i := 0; i < 64; i++ {
			hub.Broadcast(sse.Event{Type: "fill", Data: i})
		}
		close(filled)
	}()
	select {
	case <-filled:
	case <-time.After(2 * time.Second):
		t.Fatalf("AC-4: Broadcast blocked on a slow (non-reading) client")
	}

	time.Sleep(20 * time.Millisecond)
	hub.Broadcast(sse.Event{Type: "final", Data: "payload"})
	if !waitFor(func() bool { return strings.Contains(fast.written(), "event: final") }, time.Second) {
		t.Fatalf("AC-4: other live client did not receive the event; body=%q", fast.written())
	}
}

// ---------------------------------------------------------------------------
// AC-5-wire-format-unchanged
// ---------------------------------------------------------------------------

func TestAC5WireFormatUnchanged(t *testing.T) {
	hub := sse.NewHub()
	hub.MaxClients = 10
	hub.HeartbeatInterval = time.Hour // ensure no keep-alive before the assertion

	w := newRecordingWriter()
	cancel, done := startClient(hub, w)
	defer cancel()

	if !waitFor(func() bool { return hub.ClientCount() == 1 }, time.Second) {
		t.Fatalf("AC-5: client never registered")
	}
	if !waitFor(func() bool { return strings.Contains(w.written(), "connected") }, time.Second) {
		t.Fatalf("AC-5: connected frame never written")
	}

	hub.Broadcast(sse.Event{Type: "deal_updated", Data: map[string]string{"id": "d-1"}})

	if !waitFor(func() bool { return strings.Contains(w.written(), "deal_updated") }, time.Second) {
		t.Fatalf("AC-5: event never written to the client")
	}
	cancel()
	<-done

	body := w.written()
	want := "event: deal_updated\ndata: {\"id\":\"d-1\"}\n\n"
	if !strings.Contains(body, want) {
		t.Fatalf("AC-5: expected exact SSE frame %q, got body=%q", want, body)
	}
	for _, frame := range strings.Split(body, "\n\n") {
		if strings.Contains(frame, "deal_updated") {
			if !strings.Contains(frame, `data: {"id":"d-1"}`) {
				t.Fatalf("AC-5: event frame malformed/framing inserted: %q", frame)
			}
			if strings.Contains(frame, ": ") {
				t.Fatalf("AC-5: keep-alive comment inserted into event frame: %q", frame)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// AC-6-unregister-once
// ---------------------------------------------------------------------------

func TestAC6UnregisterOnce(t *testing.T) {
	hub := sse.NewHub()
	hub.MaxClients = 10

	w := newRecordingWriter()
	cancel, done := startClient(hub, w)

	if !waitFor(func() bool { return hub.ClientCount() == 1 }, time.Second) {
		t.Fatalf("AC-6: client never registered")
	}
	before := hub.ClientCount()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("AC-6: handler did not return after context cancellation")
	}

	after := hub.ClientCount()
	if before != 1 || after != 0 {
		t.Fatalf("AC-6: live client count did not decrease by exactly one (before=%d after=%d)", before, after)
	}

	// A double close or a send on the closed channel would panic here.
	hub.Broadcast(sse.Event{Type: "after_unregister", Data: "x"})
	if n := hub.ClientCount(); n != 0 {
		t.Fatalf("AC-6: client count changed after unregister: %d", n)
	}
}

// ---------------------------------------------------------------------------
// AC-7-finite-defaults
// ---------------------------------------------------------------------------

func TestAC7FiniteDefaults(t *testing.T) {
	hub := sse.NewHub()

	if hub.MaxClients <= 0 {
		t.Fatalf("AC-7: MaxClients production default is not finite non-zero: %d", hub.MaxClients)
	}
	if hub.HeartbeatInterval <= 0 {
		t.Fatalf("AC-7: HeartbeatInterval production default is not finite non-zero: %v", hub.HeartbeatInterval)
	}
	if hub.WriteDeadline <= 0 {
		t.Fatalf("AC-7: WriteDeadline production default is not finite non-zero: %v", hub.WriteDeadline)
	}

	// A subscriber is served using those defaults.
	w := newRecordingWriter()
	cancel, done := startClient(hub, w)
	defer cancel()

	if !waitFor(func() bool { return hub.ClientCount() == 1 }, time.Second) {
		t.Fatalf("AC-7: subscriber was not served with the default configuration")
	}
	if !waitFor(func() bool { return strings.Contains(w.written(), "connected") }, time.Second) {
		t.Fatalf("AC-7: subscriber did not receive the connected frame with default configuration")
	}
	cancel()
	<-done
}

// ---------------------------------------------------------------------------
// AC-8-concurrent-register-safe
// ---------------------------------------------------------------------------

func TestAC8ConcurrentRegisterSafe(t *testing.T) {
	const (
		k = 2 // configured cap
		m = 4 // extra concurrent registrations
	)
	hub := sse.NewHub()
	hub.MaxClients = k

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		mu    sync.Mutex
		maxN  int
		stops []func()
		dones []chan struct{}
	)

	for i := 0; i < k+m; i++ {
		w := newRecordingWriter()
		cancel, done := startClient(hub, w)
		stops = append(stops, cancel)
		dones = append(dones, done)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
		}()
	}

	sample := func() {
		n := hub.ClientCount()
		mu.Lock()
		if n > maxN {
			maxN = n
		}
		mu.Unlock()
	}

	close(start)
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		sample()
		time.Sleep(time.Millisecond)
	}
	sample()

	for _, s := range stops {
		s()
	}
	for _, d := range dones {
		<-d
	}
	wg.Wait()

	mu.Lock()
	observed := maxN
	mu.Unlock()
	if observed > k {
		t.Fatalf("AC-8: live client count exceeded cap K=%d under concurrent registration (observed %d)", k, observed)
	}
}
