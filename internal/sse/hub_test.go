package sse_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

// The hardening task requires an exported, settable configuration surface on
// Hub (see the spec: "exported fields or constructor options"). These tests
// pin that contract to three exported fields so the acceptance criteria can be
// driven with small values:
//
//	MaxClients        int
//	HeartbeatInterval time.Duration
//	WriteDeadline     time.Duration
//
// Reflection is used deliberately: against the base Hub the tests then fail on
// the missing configuration surface instead of failing to compile, and the
// failure names the exact contract the implementation must satisfy. Every
// assertion below that follows the surface check exercises the real ServeHTTP
// path.

const (
	hubConfigMaxClientsField        = "MaxClients"
	hubConfigHeartbeatIntervalField = "HeartbeatInterval"
	hubConfigWriteDeadlineField     = "WriteDeadline"
)

func hubIntField(t *testing.T, hub *sse.Hub, name string) int64 {
	t.Helper()
	v := reflect.ValueOf(hub).Elem()
	f := v.FieldByName(name)
	if !f.IsValid() {
		t.Fatalf("sse.Hub has no exported configuration field %q (required settable surface)", name)
	}
	if !f.CanSet() {
		t.Fatalf("sse.Hub field %q must be exported and settable", name)
	}
	if !f.CanInt() {
		t.Fatalf("sse.Hub field %q must have an integer/duration type, got %s", name, f.Kind())
	}
	return f.Int()
}

func setHubIntField(t *testing.T, hub *sse.Hub, name string, value int64) {
	t.Helper()
	v := reflect.ValueOf(hub).Elem()
	f := v.FieldByName(name)
	if !f.IsValid() {
		t.Fatalf("sse.Hub has no exported configuration field %q (required settable surface)", name)
	}
	if !f.CanSet() {
		t.Fatalf("sse.Hub field %q must be exported and settable", name)
	}
	if !f.CanInt() {
		t.Fatalf("sse.Hub field %q must have an integer/duration type, got %s", name, f.Kind())
	}
	f.SetInt(value)
}

func setHubConfig(t *testing.T, hub *sse.Hub, maxClients int, heartbeat, writeDeadline time.Duration) {
	t.Helper()
	setHubIntField(t, hub, hubConfigMaxClientsField, int64(maxClients))
	setHubIntField(t, hub, hubConfigHeartbeatIntervalField, int64(heartbeat))
	setHubIntField(t, hub, hubConfigWriteDeadlineField, int64(writeDeadline))
}

// readFrames parses SSE frames (separated by a blank line) and sends their raw
// text (without the terminating blank line) on frames. It closes frames when
// the stream ends.
func readFrames(r io.Reader, frames chan<- string) {
	defer close(frames)
	br := bufio.NewReader(r)
	var b strings.Builder
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			if line == "\n" || line == "\r\n" {
				frames <- b.String()
				b.Reset()
			} else {
				b.WriteString(line)
			}
		}
		if err != nil {
			return
		}
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s (condition not met within %s)", msg, timeout)
}

func dialStream(t *testing.T, url string) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		t.Fatalf("build stream request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("open stream: %v", err)
	}
	return resp, cancel
}

func tryOpenStream(url string) (*http.Response, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return resp, cancel, nil
}

type liveStream struct {
	resp   *http.Response
	cancel context.CancelFunc
	ended  chan struct{}
}

func trackStream(resp *http.Response, cancel context.CancelFunc) *liveStream {
	s := &liveStream{resp: resp, cancel: cancel, ended: make(chan struct{})}
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		close(s.ended)
	}()
	return s
}

func openLiveStream(t *testing.T, url string) *liveStream {
	t.Helper()
	resp, cancel := dialStream(t, url) //nolint:bodyclose // trackStream drains and closes this body
	return trackStream(resp, cancel)
}

func streamClosed(s *liveStream) bool {
	select {
	case <-s.ended:
		return true
	default:
		return false
	}
}

// AC-DEFAULTS-FINITE: a Hub created with production defaults exposes a finite
// positive limit and positive heartbeat/deadline durations, never zero or
// unbounded as on the base.
func TestAC_DEFAULTS_FINITE(t *testing.T) {
	hub := sse.NewHub()

	maxClients := hubIntField(t, hub, hubConfigMaxClientsField)
	heartbeat := time.Duration(hubIntField(t, hub, hubConfigHeartbeatIntervalField))
	writeDeadline := time.Duration(hubIntField(t, hub, hubConfigWriteDeadlineField))

	if maxClients <= 0 {
		t.Fatalf("AC-DEFAULTS-FINITE: MaxClients default must be a finite positive integer, got %d", maxClients)
	}
	if heartbeat <= 0 {
		t.Fatalf("AC-DEFAULTS-FINITE: HeartbeatInterval default must be a positive duration, got %s", heartbeat)
	}
	if writeDeadline <= 0 {
		t.Fatalf("AC-DEFAULTS-FINITE: WriteDeadline default must be a positive duration, got %s", writeDeadline)
	}
}

// AC-INJECTABLE: the small values set through the exported surface govern
// behavior: the injected cap holds at the small limit and the injected
// heartbeat interval is the one actually used (the write deadline is driven by
// the same surface in AC-WRITE-DEADLINE-HUNG).
func TestAC_INJECTABLE(t *testing.T) {
	hub := sse.NewHub()
	const limit = 1
	const heartbeat = 20 * time.Millisecond
	const writeDeadline = 100 * time.Millisecond
	setHubConfig(t, hub, limit, heartbeat, writeDeadline)

	if got := hubIntField(t, hub, hubConfigMaxClientsField); got != limit {
		t.Fatalf("AC-INJECTABLE: MaxClients read back as %d, want %d", got, limit)
	}
	if got := time.Duration(hubIntField(t, hub, hubConfigHeartbeatIntervalField)); got != heartbeat {
		t.Fatalf("AC-INJECTABLE: HeartbeatInterval read back as %s, want %s", got, heartbeat)
	}
	if got := time.Duration(hubIntField(t, hub, hubConfigWriteDeadlineField)); got != writeDeadline {
		t.Fatalf("AC-INJECTABLE: WriteDeadline read back as %s, want %s", got, writeDeadline)
	}

	srv := httptest.NewServer(hub)
	defer srv.Close()

	resp, cancel := dialStream(t, srv.URL)
	defer cancel()
	defer resp.Body.Close()
	frames := make(chan string, 256)
	go readFrames(resp.Body, frames)
	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 1 }, "first client registered")

	// The small injected interval governs: a heartbeat arrives quickly.
	gotHeartbeat := false
	heartbeatDeadline := time.After(2 * time.Second)
	for !gotHeartbeat {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Fatalf("AC-INJECTABLE: stream ended before the injected heartbeat interval elapsed")
			}
			if strings.HasPrefix(frame, ":") {
				gotHeartbeat = true
			}
		case <-heartbeatDeadline:
			t.Fatalf("AC-INJECTABLE: no heartbeat within 2s despite injected interval %s", heartbeat)
		}
	}
	go func() {
		for range frames {
		}
	}()

	// The small injected cap governs: a surplus client never raises the count.
	secondResp, secondCancel, err := tryOpenStream(srv.URL) //nolint:bodyclose // trackStream drains and closes this body
	if err != nil {
		t.Fatalf("AC-INJECTABLE: surplus stream request failed: %v", err)
	}
	second := trackStream(secondResp, secondCancel)
	defer second.cancel()

	maxSeen := 0
	stopAt := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(stopAt) {
		if n := hub.ClientCount(); n > maxSeen {
			maxSeen = n
		}
		time.Sleep(2 * time.Millisecond)
	}
	if maxSeen > limit {
		t.Fatalf("AC-INJECTABLE: ClientCount reached %d despite injected limit %d", maxSeen, limit)
	}
}

// AC-CAP-ENFORCED: with L idle streams open, the next client either gets a
// non-2xx response or an existing stream is closed, and the number of
// concurrently held-open streams never exceeds L.
func TestAC_CAP_ENFORCED(t *testing.T) {
	hub := sse.NewHub()
	const limit = 2
	setHubConfig(t, hub, limit, time.Hour, 5*time.Second)

	srv := httptest.NewServer(hub)
	defer srv.Close()

	first := openLiveStream(t, srv.URL)
	defer first.cancel()
	second := openLiveStream(t, srv.URL)
	defer second.cancel()
	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == limit }, "two idle streams registered")

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	maxSeen := 0
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			n := hub.ClientCount()
			mu.Lock()
			if n > maxSeen {
				maxSeen = n
			}
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
		}
	}()

	thirdResp, thirdCancel, err := tryOpenStream(srv.URL) //nolint:bodyclose // trackStream drains and closes this body
	if err != nil {
		close(stop)
		wg.Wait()
		t.Fatalf("AC-CAP-ENFORCED: surplus stream request failed: %v", err)
	}
	third := trackStream(thirdResp, thirdCancel)
	defer third.cancel()

	thirdAccepted := thirdResp.StatusCode >= 200 && thirdResp.StatusCode < 300
	waitFor(t, 2*time.Second, func() bool {
		if !thirdAccepted {
			return true
		}
		return streamClosed(first) || streamClosed(second)
	}, "AC-CAP-ENFORCED: surplus stream was neither declined nor backed by an evicted stream")

	close(stop)
	wg.Wait()

	waitFor(t, 2*time.Second, func() bool {
		open := 0
		if !streamClosed(first) {
			open++
		}
		if !streamClosed(second) {
			open++
		}
		if thirdAccepted && !streamClosed(third) {
			open++
		}
		return open <= limit
	}, "AC-CAP-ENFORCED: more than the configured limit of streams stayed open")

	if maxSeen > limit {
		t.Fatalf("AC-CAP-ENFORCED: ClientCount reached %d, exceeding the configured limit %d", maxSeen, limit)
	}
}

// AC-HEARTBEAT-IDLE: with no Broadcast in flight, the client receives periodic
// SSE comment frames (first byte ':') that carry no event:/data: fields.
func TestAC_HEARTBEAT_IDLE(t *testing.T) {
	hub := sse.NewHub()
	setHubConfig(t, hub, 8, 20*time.Millisecond, time.Second)

	srv := httptest.NewServer(hub)
	defer srv.Close()

	resp, cancel := dialStream(t, srv.URL)
	defer cancel()
	defer resp.Body.Close()
	frames := make(chan string, 256)
	go readFrames(resp.Body, frames)
	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 1 }, "client registered")

	beats := 0
	deadline := time.After(2 * time.Second)
	for beats < 2 {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Fatalf("AC-HEARTBEAT-IDLE: stream ended after %d heartbeats", beats)
			}
			if !strings.HasPrefix(frame, ":") {
				continue
			}
			if strings.Contains(frame, "event:") || strings.Contains(frame, "data:") {
				t.Fatalf("AC-HEARTBEAT-IDLE: heartbeat comment frame must carry no event:/data: fields, got %q", frame)
			}
			beats++
		case <-deadline:
			t.Fatalf("AC-HEARTBEAT-IDLE: expected recurring comment frames, saw %d in 2s", beats)
		}
	}
	go func() {
		for range frames {
		}
	}()
}

// AC-WRITE-DEADLINE-HUNG: when a client stops draining its buffer, a write to
// it fails within the injected deadline instead of blocking forever, and the
// stream's goroutine returns and unregisters the client.
func TestAC_WRITE_DEADLINE_HUNG(t *testing.T) {
	hub := sse.NewHub()
	setHubConfig(t, hub, 4, time.Hour, 100*time.Millisecond)

	srv := httptest.NewServer(hub)
	defer srv.Close()

	// The client never reads its body, so the server's write to it blocks.
	resp, cancel := dialStream(t, srv.URL) //nolint:bodyclose // closed by the defer below; the linter cannot follow the helper
	defer cancel()
	defer resp.Body.Close()
	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 1 }, "client registered")

	blob := strings.Repeat("x", 8<<20)
	start := time.Now()
	hub.Broadcast(sse.Event{Type: "blob", Data: blob})

	waitFor(t, 5*time.Second, func() bool { return hub.ClientCount() == 0 },
		"AC-WRITE-DEADLINE-HUNG: blocked client was not unregistered within the write deadline window")
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("AC-WRITE-DEADLINE-HUNG: write took %s before the client was unregistered", elapsed)
	}

	// Once the handler gives up, the server closes the stream and the client
	// body reaches EOF instead of staying blocked.
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("AC-WRITE-DEADLINE-HUNG: stream did not terminate after the write deadline")
	}
}

// AC-BROADCAST-NONBLOCKING: a stalled client (handler blocked on a write, its
// buffered channel full) must not delay Broadcast or delivery to a healthy
// client.
func TestAC_BROADCAST_NONBLOCKING(t *testing.T) {
	hub := sse.NewHub()
	// A long write deadline keeps the stalled handler blocked for the duration
	// of this test.
	setHubConfig(t, hub, 8, time.Hour, time.Hour)

	srv := httptest.NewServer(hub)
	defer srv.Close()

	// Healthy client: continuously drain and watch for the marker event.
	healthyResp, healthyCancel := dialStream(t, srv.URL) //nolint:bodyclose // closed by the defer below; the linter cannot follow the helper
	defer healthyCancel()
	defer healthyResp.Body.Close()
	markerSeen := make(chan struct{})
	var markerOnce sync.Once
	go func() {
		br := bufio.NewReader(healthyResp.Body)
		for {
			line, err := br.ReadString('\n')
			if strings.Contains(line, "event: marker") {
				markerOnce.Do(func() { close(markerSeen) })
			}
			if err != nil {
				return
			}
		}
	}()

	// Stalled client: connected but never draining its body.
	stalledResp, stalledCancel := dialStream(t, srv.URL)
	defer stalledCancel()
	defer stalledResp.Body.Close()

	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 2 }, "both clients registered")

	// Make the stalled handler block on a socket write, then fill its buffered
	// channel so a blocking Broadcast would hang there.
	hub.Broadcast(sse.Event{Type: "blob", Data: strings.Repeat("x", 8<<20)})
	time.Sleep(300 * time.Millisecond)

	result := make(chan time.Duration, 1)
	go func() {
		for i := 0; i < 128; i++ {
			hub.Broadcast(sse.Event{Type: "flood", Data: i})
		}
		time.Sleep(50 * time.Millisecond)
		start := time.Now()
		for i := 0; i < 5; i++ {
			hub.Broadcast(sse.Event{Type: "marker", Data: map[string]string{"id": "m1"}})
		}
		result <- time.Since(start)
	}()

	select {
	case elapsed := <-result:
		if elapsed > 250*time.Millisecond {
			t.Fatalf("AC-BROADCAST-NONBLOCKING: Broadcast took %s while a client was stalled", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("AC-BROADCAST-NONBLOCKING: Broadcast blocked on a stalled client")
	}

	select {
	case <-markerSeen:
	case <-time.After(2 * time.Second):
		t.Fatalf("AC-BROADCAST-NONBLOCKING: healthy client did not receive the event while another client was stalled")
	}
}

// AC-BROADCAST-EMPTY: Broadcast on an empty Hub returns immediately without
// panicking, and a client connecting afterwards still receives later events.
func TestAC_BROADCAST_EMPTY(t *testing.T) {
	hub := sse.NewHub()
	setHubConfig(t, hub, 8, time.Hour, time.Second)

	srv := httptest.NewServer(hub)
	defer srv.Close()

	start := time.Now()
	hub.Broadcast(sse.Event{Type: "early", Data: "x"})
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("AC-BROADCAST-EMPTY: Broadcast with no clients took %s", elapsed)
	}

	resp, cancel := dialStream(t, srv.URL)
	defer cancel()
	defer resp.Body.Close()
	frames := make(chan string, 64)
	go readFrames(resp.Body, frames)
	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 1 }, "client registered after empty broadcast")

	hub.Broadcast(sse.Event{Type: "after", Data: "y"})
	deadline := time.After(2 * time.Second)
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Fatalf("AC-BROADCAST-EMPTY: stream ended before the post-connection event arrived")
			}
			if strings.Contains(frame, "event: after") {
				return
			}
		case <-deadline:
			t.Fatalf("AC-BROADCAST-EMPTY: client did not receive an event broadcast after it connected")
		}
	}
}

// AC-UNREGISTER-ON-DISCONNECT: when the handler returns on disconnect, the Hub
// unregisters the client exactly once (a double close would panic), the active
// count returns to its pre-connection baseline, and the stream goroutine ends.
func TestAC_UNREGISTER_ON_DISCONNECT(t *testing.T) {
	hub := sse.NewHub()
	setHubConfig(t, hub, 8, time.Hour, time.Second)

	srv := httptest.NewServer(hub)
	defer srv.Close()

	resp, cancel := dialStream(t, srv.URL)
	defer cancel()
	defer resp.Body.Close()
	frames := make(chan string, 64)
	go readFrames(resp.Body, frames)
	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 1 }, "client registered")

	cancel()
	_ = resp.Body.Close()

	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 0 },
		"AC-UNREGISTER-ON-DISCONNECT: active count did not return to its pre-connection baseline")

	// The handler returned: the frame reader reaches EOF and closes the channel.
	drainUntilClosed := func() bool {
		for {
			select {
			case _, ok := <-frames:
				if !ok {
					return true
				}
			default:
				return false
			}
		}
	}
	waitFor(t, 2*time.Second, drainUntilClosed,
		"AC-UNREGISTER-ON-DISCONNECT: stream handler did not return after the client disconnected")

	// A second connection proves the hub still serves cleanly afterwards.
	resp2, cancel2 := dialStream(t, srv.URL)
	defer cancel2()
	defer resp2.Body.Close()
	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 1 }, "hub did not accept a later client")
	cancel2()
	_ = resp2.Body.Close()
	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 0 }, "later client was not unregistered")
}

// AC-WIRE-FORMAT-UNCHANGED: broadcast events keep the exact SSE framing
// `event: <name>\ndata: <payload>\n\n`; comment frames are the only additional
// frame type.
func TestAC_WIRE_FORMAT_UNCHANGED(t *testing.T) {
	hub := sse.NewHub()
	setHubConfig(t, hub, 8, time.Hour, time.Second)

	srv := httptest.NewServer(hub)
	defer srv.Close()

	resp, cancel := dialStream(t, srv.URL)
	defer cancel()
	defer resp.Body.Close()
	frames := make(chan string, 64)
	go readFrames(resp.Body, frames)
	waitFor(t, 2*time.Second, func() bool { return hub.ClientCount() == 1 }, "client registered")

	hub.Broadcast(sse.Event{
		Type: "email_received",
		Data: map[string]string{"id": "mail-1"},
	})

	want := "event: email_received\ndata: {\"id\":\"mail-1\"}\n"
	deadline := time.After(2 * time.Second)
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Fatalf("AC-WIRE-FORMAT-UNCHANGED: stream ended before the event arrived")
			}
			if !strings.HasPrefix(frame, "event:") && !strings.HasPrefix(frame, ":") {
				t.Fatalf("AC-WIRE-FORMAT-UNCHANGED: unexpected non-event/non-comment frame %q", frame)
			}
			if strings.Contains(frame, "event: email_received") {
				if frame != want {
					t.Fatalf("AC-WIRE-FORMAT-UNCHANGED: got %q, want %q", frame, want)
				}
				return
			}
		case <-deadline:
			t.Fatalf("AC-WIRE-FORMAT-UNCHANGED: expected event frame %q was never delivered", want)
		}
	}
}

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
