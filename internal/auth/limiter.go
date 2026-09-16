package auth

import (
	"sync"
	"time"
)

type attemptRecord struct {
	failures  int
	lockedAt  time.Time
	updatedAt time.Time
}

type RateLimiter struct {
	mu          sync.RWMutex
	attempts    map[string]*attemptRecord
	maxAttempts int
	lockoutDur  time.Duration
	windowDur   time.Duration
}

func NewRateLimiter(maxAttempts int, windowDur, lockoutDur time.Duration) *RateLimiter {
	rl := &RateLimiter{
		attempts:    make(map[string]*attemptRecord),
		maxAttempts: maxAttempts,
		lockoutDur:  lockoutDur,
		windowDur:   windowDur,
	}
	return rl
}

func (rl *RateLimiter) IsLocked(key string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rec, ok := rl.attempts[key]
	if !ok {
		return false, 0
	}

	now := time.Now()
	if !rec.lockedAt.IsZero() {
		remaining := rec.lockedAt.Add(rl.lockoutDur).Sub(now)
		if remaining > 0 {
			return true, remaining
		}
		// Lock expired, reset
		delete(rl.attempts, key)
		return false, 0
	}

	// Window expired without lock
	if now.Sub(rec.updatedAt) > rl.windowDur {
		delete(rl.attempts, key)
		return false, 0
	}

	return false, 0
}

func (rl *RateLimiter) RecordFailure(key string) (locked bool, remaining time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rec, ok := rl.attempts[key]
	if !ok || now.Sub(rec.updatedAt) > rl.windowDur {
		rec = &attemptRecord{
			failures:  1,
			updatedAt: now,
		}
		rl.attempts[key] = rec
		return false, 0
	}

	rec.failures++
	rec.updatedAt = now

	if rec.failures >= rl.maxAttempts {
		rec.lockedAt = now
		return true, rl.lockoutDur
	}

	return false, 0
}

func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, key)
}
