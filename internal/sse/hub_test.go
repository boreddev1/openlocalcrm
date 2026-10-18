package sse_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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
// review-sse-hub-hardening-5 / harden-sse-hub — acceptance criteria AC-1..AC-8
//
// The spec leaves the injection surface to the implementer ("constructor options
// or settable fields"). These tests pin the minimal *settable-field* surface that
// makes the criteria observable from outside the implementation:
//
//	*sse.Hub exported settable fields, initialised by sse.NewHub() to finite,
//	non-zero production defaults:
//	    MaxStreams        int            // global cap on concurrent streams
//	    HeartbeatInterval time.Duration  // keep-alive comment-frame interval
//	    WriteTimeout      time.Duration  // per-write deadline
//
// The lookup below also accepts aliases (MaxClients/MaxConcurrentStreams,
// Heartbeat/HeartbeatEvery, WriteDeadline) so an equivalent naming satisfies the
// criteria just as well. It fails with an explicit assertion — never a compile
// error — so the red run shows the missing capability on the real path.
//
// A ResponseWriter that cannot carry a write deadline is out of scope for AC-3:
// the test double implements http.ResponseWriter, http.Flusher and
// SetWriteDeadline, which is exactly the capability http.ResponseController
// looks for.
// ---------------------------------------------------------------------------

var (
	capFieldNames       = []string{"MaxStreams", "MaxClients", "MaxConcurrentStreams"}
	heartbeatFieldNames = []string{"HeartbeatInterval", "HeartbeatEvery", "Heartbeat"}
	deadlineFieldNames  = []string{"WriteTimeout", "WriteDeadline"}
)

// ---------------------------------------------------------------------------
// test double: a client connection that can hang on write and records the write
// deadline the stream loop installs.
// ---------------------------------------------------------------------------

type fakeWriter struct {
	mu         sync.Mutex
	header     http.Header
	buf        bytes.Buffer
	writes     int
	hangAfter  int // writes 1..hangAfter succeed; later writes hang until the deadline
	deadline   time.Time
	deadlineOK bool
	deadlines  []time.Time

	blockedOnce sync.Once
	blocked     chan struct{} // closed when Write() starts hanging

	stopOnce sync.Once
	stopped  chan struct{}
}

func newFakeWriter(hangAfter int) *fakeWriter {
	return &fakeWriter{
		header:    make(http.Header),
		hangAfter: hangAfter,
		blocked:   make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

func (w *fakeWriter) Header() http.Header        { return w.header }
func (w *fakeWriter) WriteHeader(statusCode int) {}
func (w *fakeWriter) Flush()                     {}
func (w *fakeWriter) stop()                      { w.stopOnce.Do(func() { close(w.stopped) }) }

func (w *fakeWriter) snapshot() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *fakeWriter) deadlineSnapshots() []time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]time.Time, len(w.deadlines))
	copy(out, w.deadlines)
	return out
}

// SetWriteDeadline is the capability http.ResponseController.SetWriteDeadline
// dispatches to. It lets AC-3 observe that the loop installs a deadline.
func (w *fakeWriter) SetWriteDeadline(t time.Time) error {
	w.mu.Lock()
	w.deadline = t
	w.deadlineOK = true
	w.deadlines = append(w.deadlines, t)
	w.mu.Unlock()
	return nil
}

// Write succeeds until hangAfter writes have completed, then behaves like a
// socket with a write deadline: it blocks until the installed deadline has
// passed and then fails with os.ErrDeadlineExceeded.
func (w *fakeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes++
	hang := w.writes > w.hangAfter
	if !hang {
		w.buf.Write(p)
		w.mu.Unlock()
		return len(p), nil
	}
	w.blockedOnce.Do(func() { close(w.blocked) })
	w.mu.Unlock()

	for {
		w.mu.Lock()
		set := w.deadlineOK
		dl := w.deadline
		w.mu.Unlock()

		if set && !dl.IsZero() {
			wait := time.Until(dl)
			if wait <= 0 {
				return 0, os.ErrDeadlineExceeded
			}
			select {
			case <-time.After(wait):
				return 0, os.ErrDeadlineExceeded
			case <-w.stopped:
				return 0, os.ErrDeadlineExceeded
			}
		}

		select {
		case <-time.After(5 * time.Millisecond):
		case <-w.stopped:
			return 0, os.ErrDeadlineExceeded
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// hubField locates a settable exported configuration field by any of the
// accepted names. It fails with an explicit assertion rather than a compile
// error so the red run shows the missing capability on the real path.
func hubField(t *testing.T, hub *sse.Hub, names []string, what string) reflect.Value {
	t.Helper()
	v := reflect.ValueOf(hub)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		t.Fatalf("sse.NewHub() returned a non-pointer; cannot inspect the %s", what)
	}
	v = v.Elem()
	for _, name := range names {
		f := v.FieldByName(name)
		if f.IsValid() && f.CanSet() {
			return f
		}
	}
	t.Fatalf("Hub exposes no settable field for the %s (tried %v); AC-1, AC-2, AC-3 and AC-7 require the cap, heartbeat interval and write deadline to be injectable", what, names)
	return reflect.Value{}
}

func intField(t *testing.T, hub *sse.Hub, names []string, what string) int64 {
	t.Helper()
	f := hubField(t, hub, names, what)
	switch f.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return f.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(f.Uint())
	}
	t.Fatalf("Hub field for the %s has kind %s, want an integer", what, f.Kind())
	return 0
}

func durationField(t *testing.T, hub *sse.Hub, names []string, what string) time.Duration {
	t.Helper()
	f := hubField(t, hub, names, what)
	if f.Kind() != reflect.Int64 {
		t.Fatalf("Hub field for the %s has kind %s, want time.Duration (int64)", what, f.Kind())
	}
	return time.Duration(f.Int())
}

func setIntField(t *testing.T, hub *sse.Hub, want int, names []string, what string) {
	t.Helper()
	f := hubField(t, hub, names, what)
	switch f.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.SetInt(int64(want))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		f.SetUint(uint64(want))
	default:
		t.Fatalf("Hub field for the %s has kind %s, want an integer", what, f.Kind())
	}
}

func setDurationField(t *testing.T, hub *sse.Hub, want time.Duration, names []string, what string) {
	t.Helper()
	f := hubField(t, hub, names, what)
	if f.Kind() != reflect.Int64 {
		t.Fatalf("Hub field for the %s has kind %s, want time.Duration (int64)", what, f.Kind())
	}
	f.SetInt(int64(want))
}

func configureHub(t *testing.T, hub *sse.Hub, maxStreams int, heartbeat, writeTimeout time.Duration) {
	t.Helper()
	setIntField(t, hub, maxStreams, capFieldNames, "maximum concurrent streams")
	setDurationField(t, hub, heartbeat, heartbeatFieldNames, "heartbeat interval")
	setDurationField(t, hub, writeTimeout, deadlineFieldNames, "write deadline")
}

func startStream(hub *sse.Hub, w http.ResponseWriter) (context.CancelFunc, <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/events/stream", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.ServeHTTP(w, req)
	}()
	return cancel, done
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

func waitDone(t *testing.T, done <-chan struct{}, timeout time.Duration, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %s", what, timeout)
	}
}

// countCommentFrames counts SSE frames whose first non-blank line begins with
// ':' — i.e. keep-alive comment frames (AC-2 and AC-8).
func countCommentFrames(body string) int {
	n := 0
	for _, frame := range strings.Split(body, "\n\n") {
		for _, line := range strings.Split(frame, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			if strings.HasPrefix(line, ":") {
				n++
			}
			break
		}
	}
	return n
}

func assertNoBroadcastPanic(t *testing.T, hub *sse.Hub) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Broadcast after the stream was unregistered panicked (a closed channel was written): %v", r)
		}
	}()
	hub.Broadcast(sse.Event{Type: "after_unregister", Data: map[string]string{"id": "a"}})
	hub.Broadcast(sse.Event{Type: "after_unregister", Data: map[string]string{"id": "b"}})
}

// ---------------------------------------------------------------------------
// AC-1: the concurrent-stream cap is never exceeded and the extra attempt
// resolves (declined or oldest closed) without wedging the hub.
// ---------------------------------------------------------------------------

func TestAC1_ConcurrentStreamCapNeverExceeded(t *testing.T) {
	hub := sse.NewHub()
	configureHub(t, hub, 2, time.Hour, time.Hour)

	w1 := newFakeWriter(1 << 30)
	w2 := newFakeWriter(1 << 30)
	w3 := newFakeWriter(1 << 30)
	defer w1.stop()
	defer w2.stop()
	defer w3.stop()

	cancel1, done1 := startStream(hub, w1)
	cancel2, done2 := startStream(hub, w2)
	defer cancel1()
	defer cancel2()

	waitFor(t, 2*time.Second, "2 registered streams at the configured cap", func() bool {
		return hub.ClientCount() == 2
	})

	var maxSeen atomic.Int64
	maxSeen.Store(int64(hub.ClientCount()))
	stopMonitor := make(chan struct{})
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		for {
			if n := int64(hub.ClientCount()); n > maxSeen.Load() {
				maxSeen.Store(n)
			}
			select {
			case <-stopMonitor:
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()

	cancel3, done3 := startStream(hub, w3)
	defer cancel3()

	resolved := false
	timeout := time.After(2 * time.Second)
	for !resolved {
		select {
		case <-done3: // the extra client was declined
			resolved = true
		case <-done1: // the oldest stream was closed to make room
			resolved = true
		case <-done2:
			resolved = true
		case <-timeout:
			close(stopMonitor)
			<-monitorDone
			t.Fatalf("AC-1: a third stream at the configured cap of 2 was neither declined nor evicted an old stream within 2s (ClientCount=%d)", hub.ClientCount())
		}
	}

	close(stopMonitor)
	<-monitorDone

	if got := hub.ClientCount(); got > 2 {
		t.Fatalf("AC-1: ClientCount=%d exceeds the configured maximum of 2", got)
	}
	if got := maxSeen.Load(); got > 2 {
		t.Fatalf("AC-1: observed %d concurrently live streams, configured maximum is 2 (a stream beyond the cap was admitted)", got)
	}
}

// ---------------------------------------------------------------------------
// AC-2: an idle client receives periodic keep-alive comment frames at the
// configured interval.
// ---------------------------------------------------------------------------

func TestAC2_IdleStreamReceivesHeartbeatCommentFrame(t *testing.T) {
	hub := sse.NewHub()
	configureHub(t, hub, 10, 25*time.Millisecond, 2*time.Second)

	w := newFakeWriter(1 << 30)
	defer w.stop()

	cancel, _ := startStream(hub, w)
	defer cancel()

	waitFor(t, 2*time.Second, "a keep-alive SSE comment frame (a line beginning with ':') without any broadcast", func() bool {
		return countCommentFrames(w.snapshot()) >= 1
	})

	// The frames are periodic at the injected interval, not a single one-off.
	before := countCommentFrames(w.snapshot())
	waitFor(t, 2*time.Second, "a further keep-alive frame after the first", func() bool {
		return countCommentFrames(w.snapshot()) > before
	})
}

// ---------------------------------------------------------------------------
// AC-3: a hung write is bounded by the write deadline installed through
// http.ResponseController; the loop unblocks and unregisters the client.
// ---------------------------------------------------------------------------

func TestAC3_WriteDeadlineUnblocksHungClient(t *testing.T) {
	hub := sse.NewHub()
	configureHub(t, hub, 10, 20*time.Millisecond, 100*time.Millisecond)

	// The connected frame succeeds; the next write (the heartbeat) hangs.
	w := newFakeWriter(1)
	defer w.stop()

	start := time.Now()
	cancel, done := startStream(hub, w)
	defer cancel()

	select {
	case <-w.blocked:
	case <-time.After(2 * time.Second):
		t.Fatalf("AC-3: the stream loop never attempted a second write to the hung client, so the write deadline could not be observed")
	}

	waitDone(t, done, 2*time.Second, "AC-3 write loop against a hung client")
	elapsed := time.Since(start)

	deadlines := w.deadlineSnapshots()
	if len(deadlines) == 0 {
		t.Fatalf("AC-3: no write deadline was applied before writing to the hung client; the write loop can be pinned indefinitely")
	}
	nearTerm := false
	for _, dl := range deadlines {
		if !dl.IsZero() && dl.After(start) && dl.Sub(start) <= 3*time.Second {
			nearTerm = true
			break
		}
	}
	if !nearTerm {
		t.Fatalf("AC-3: write deadlines %v are not finite near-term deadlines derived from the configured 100ms WriteTimeout", deadlines)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("AC-3: the write loop took %s to unblock against a hung client, configured deadline is 100ms", elapsed)
	}
	if got := hub.ClientCount(); got != 0 {
		t.Fatalf("AC-3: hung client was not unregistered after the deadline expired; ClientCount=%d", got)
	}
	assertNoBroadcastPanic(t, hub)
}

// ---------------------------------------------------------------------------
// AC-4: Broadcast stays non-blocking with a saturated client and a healthy
// client still receives the event.
// ---------------------------------------------------------------------------

func TestAC4_BroadcastNonBlockingWithSaturatedClient(t *testing.T) {
	hub := sse.NewHub()

	stalled := newFakeWriter(1) // drains the connected frame, then hangs on write
	healthy := newFakeWriter(1 << 30)
	defer stalled.stop()
	defer healthy.stop()

	cancelStalled, doneStalled := startStream(hub, stalled)
	cancelHealthy, doneHealthy := startStream(hub, healthy)
	defer cancelStalled()
	defer cancelHealthy()
	defer func() {
		select {
		case <-doneStalled:
		case <-time.After(500 * time.Millisecond):
		}
		select {
		case <-doneHealthy:
		case <-time.After(500 * time.Millisecond):
		}
	}()

	waitFor(t, 2*time.Second, "both clients registered", func() bool {
		return hub.ClientCount() == 2
	})

	// Enough events to fill the stalled client's buffered channel and then some.
	// Broadcast must drop for the saturated client, never block.
	worst := time.Duration(0)
	for i := 0; i < 64; i++ {
		begin := time.Now()
		hub.Broadcast(sse.Event{Type: "flood", Data: map[string]int{"i": i}})
		if d := time.Since(begin); d > worst {
			worst = d
		}
	}
	if worst > time.Second {
		t.Fatalf("AC-4: Broadcast blocked for %s with one saturated client; it must stay non-blocking", worst)
	}

	waitFor(t, 2*time.Second, "the healthy client to receive flood traffic", func() bool {
		return strings.Contains(healthy.snapshot(), "event: flood")
	})
	time.Sleep(50 * time.Millisecond)

	begin := time.Now()
	hub.Broadcast(sse.Event{Type: "healthy_probe", Data: map[string]string{"id": "probe-1"}})
	if d := time.Since(begin); d > time.Second {
		t.Fatalf("AC-4: Broadcast took %s with a saturated client present", d)
	}
	waitFor(t, 2*time.Second, "the healthy client to receive the event", func() bool {
		body := healthy.snapshot()
		return strings.Contains(body, "event: healthy_probe") && strings.Contains(body, `"probe-1"`)
	})
}

// ---------------------------------------------------------------------------
// AC-5: with zero admitted streams Broadcast is a no-op that returns promptly
// and does not panic.
// ---------------------------------------------------------------------------

func TestAC5_BroadcastWithNoStreamsIsSafe(t *testing.T) {
	hub := sse.NewHub()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 16; i++ {
			hub.Broadcast(sse.Event{Type: "noop", Data: map[string]int{"i": i}})
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("AC-5: Broadcast blocked with zero admitted streams")
	}
	if got := hub.ClientCount(); got != 0 {
		t.Fatalf("AC-5: Broadcast with zero streams changed ClientCount to %d", got)
	}
}

// ---------------------------------------------------------------------------
// AC-6: the deferred cleanup unregisters and closes the stream on unwind; a
// later Broadcast neither panics nor writes to a closed channel.
// ---------------------------------------------------------------------------

func TestAC6_DisconnectUnregistersStream(t *testing.T) {
	hub := sse.NewHub()
	w := newFakeWriter(1 << 30)
	defer w.stop()

	cancel, done := startStream(hub, w)
	waitFor(t, 2*time.Second, "client registration", func() bool { return hub.ClientCount() == 1 })

	cancel()
	waitDone(t, done, 2*time.Second, "the handler after its request context was cancelled")

	if got := hub.ClientCount(); got != 0 {
		t.Fatalf("AC-6: client still registered after its context was cancelled; ClientCount=%d", got)
	}
	assertNoBroadcastPanic(t, hub)
}

// ---------------------------------------------------------------------------
// AC-7: the production defaults (no overrides) are finite and non-zero, and the
// same fields accept a small injected value (exercised by AC-1..AC-3).
// ---------------------------------------------------------------------------

func TestAC7_ProductionDefaultsAreFiniteAndNonZero(t *testing.T) {
	hub := sse.NewHub()

	cap := intField(t, hub, capFieldNames, "maximum concurrent streams")
	heartbeat := durationField(t, hub, heartbeatFieldNames, "heartbeat interval")
	deadline := durationField(t, hub, deadlineFieldNames, "write deadline")

	if cap <= 0 {
		t.Fatalf("AC-7: default maximum concurrent streams must be finite and non-zero, got %d", cap)
	}
	if heartbeat <= 0 {
		t.Fatalf("AC-7: default heartbeat interval must be finite and non-zero, got %s", heartbeat)
	}
	if deadline <= 0 {
		t.Fatalf("AC-7: default write deadline must be finite and non-zero, got %s", deadline)
	}
}

// ---------------------------------------------------------------------------
// AC-8: existing event framing is byte-identical to the pre-change wire format;
// only new keep-alive comment frames may be added.
// ---------------------------------------------------------------------------

func TestAC8_ExistingEventWireFormatUnchanged(t *testing.T) {
	hub := sse.NewHub()

	w := newFakeWriter(1 << 30)
	defer w.stop()

	cancel, done := startStream(hub, w)
	defer cancel()

	waitFor(t, 2*time.Second, "client registration", func() bool { return hub.ClientCount() == 1 })

	hub.Broadcast(sse.Event{
		Type: "email_received",
		Data: map[string]string{"id": "mail-1"},
	})
	waitFor(t, 2*time.Second, "the broadcast to reach the client", func() bool {
		return strings.Contains(w.snapshot(), "event: email_received")
	})

	cancel()
	waitDone(t, done, 2*time.Second, "stream after client disconnect")
	body := w.snapshot()

	for _, want := range []string{
		"event: connected\ndata: {\"status\":\"connected\"}\n\n",
		"event: email_received\ndata: {\"id\":\"mail-1\"}\n\n",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("AC-8: pre-change SSE wire format drifted; want substring %q in stream output %q", want, body)
		}
	}
}
