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
}
