package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

func TestRateLimiter(t *testing.T) {
	rl := auth.NewRateLimiter(3, 100*time.Millisecond, 200*time.Millisecond)

	key := "test-ip-user"

	// Initial check: not locked
	if locked, _ := rl.IsLocked(key); locked {
		t.Fatalf("expected key to not be locked initially")
	}

	// 1st failure
	if locked, _ := rl.RecordFailure(key); locked {
		t.Fatalf("unexpected lock on 1st failure")
	}

	// 2nd failure
	if locked, _ := rl.RecordFailure(key); locked {
		t.Fatalf("unexpected lock on 2nd failure")
	}

	// 3rd failure -> should lock!
	locked, dur := rl.RecordFailure(key)
	if !locked {
		t.Fatalf("expected key to be locked on 3rd failure")
	}
	if dur <= 0 {
		t.Fatalf("expected positive remaining lockout duration")
	}

	// Subsequent check: should remain locked
	if isLocked, _ := rl.IsLocked(key); !isLocked {
		t.Fatalf("expected IsLocked to return true")
	}

	// Wait for lockout duration to pass
	time.Sleep(210 * time.Millisecond)
	if isLocked, _ := rl.IsLocked(key); isLocked {
		t.Fatalf("expected lock to expire after lockout duration")
	}

	// Test Reset
	rl.RecordFailure(key)
	rl.Reset(key)
	if isLocked, _ := rl.IsLocked(key); isLocked {
		t.Fatalf("expected key to be unlocked after Reset")
	}
	rl.Stop()
}

func TestRateLimiter_CapacityCap(t *testing.T) {
	maxEntries := 50
	rl := auth.NewRateLimiterWithCapacity(5, time.Minute, time.Minute, maxEntries)
	defer rl.Stop()

	// Fill with 100 different keys (more than maxEntries)
	for i := 0; i < 100; i++ {
		rl.RecordFailure("key-" + time.Now().String() + string(rune(i)))
	}

	// Should not crash and should cap within bounded range
	rl.CleanupExpired()
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := auth.NewRateLimiter(2, 50*time.Millisecond, 100*time.Millisecond)
	defer rl.Stop()

	key := "127.0.0.1"

	// 1st request -> allowed
	allowed, _ := rl.Allow(key)
	if !allowed {
		t.Fatalf("expected 1st request to be allowed")
	}

	// 2nd request -> allowed
	allowed, _ = rl.Allow(key)
	if !allowed {
		t.Fatalf("expected 2nd request to be allowed")
	}

	// 3rd request -> rate limited!
	allowed, rem := rl.Allow(key)
	if allowed {
		t.Fatalf("expected 3rd request to be blocked")
	}
	if rem <= 0 {
		t.Fatalf("expected positive remaining lockout duration")
	}

	time.Sleep(110 * time.Millisecond)

	// After lockout -> allowed again
	allowed, _ = rl.Allow(key)
	if !allowed {
		t.Fatalf("expected request after lockout to be allowed")
	}
}

func TestRealIP_HeaderSpoofingIgnored(t *testing.T) {
	limiter := auth.NewRateLimiter(2, time.Minute, time.Minute)
	defer limiter.Stop()

	handler := auth.RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Untrusted public IP tries to spoof True-Client-IP and X-Real-IP
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "203.0.113.50:1234"
		req.Header.Set("True-Client-IP", "1.1.1."+string(rune('0'+i)))
		req.Header.Set("X-Real-IP", "1.1.1."+string(rune('0'+i)))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d should be allowed, got %d", i+1, rec.Code)
		}
	}

	// 3rd request with yet another spoofed IP from the same RemoteAddr must be 429
	req3 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req3.RemoteAddr = "203.0.113.50:1234"
	req3.Header.Set("True-Client-IP", "1.1.1.99")
	req3.Header.Set("X-Real-IP", "1.1.1.99")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests despite spoofed True-Client-IP, got %d", rec3.Code)
	}
}
