package auth_test

import (
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
