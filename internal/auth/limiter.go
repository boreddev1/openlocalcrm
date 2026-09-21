package auth

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxLimiterEntries = 10000
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
	maxEntries  int
	stopChan    chan struct{}
}

func NewRateLimiter(maxAttempts int, windowDur, lockoutDur time.Duration) *RateLimiter {
	return NewRateLimiterWithCapacity(maxAttempts, windowDur, lockoutDur, DefaultMaxLimiterEntries)
}

func NewRateLimiterWithCapacity(maxAttempts int, windowDur, lockoutDur time.Duration, maxEntries int) *RateLimiter {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxLimiterEntries
	}
	rl := &RateLimiter{
		attempts:    make(map[string]*attemptRecord),
		maxAttempts: maxAttempts,
		lockoutDur:  lockoutDur,
		windowDur:   windowDur,
		maxEntries:  maxEntries,
		stopChan:    make(chan struct{}),
	}

	// Finding #34: Background cleanup ticker every 5 minutes to prevent unbounded memory growth
	go rl.cleanupLoop(5 * time.Minute)

	return rl
}

func (rl *RateLimiter) Stop() {
	select {
	case <-rl.stopChan:
		// already closed
	default:
		close(rl.stopChan)
	}
}

func (rl *RateLimiter) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopChan:
			return
		case <-ticker.C:
			rl.CleanupExpired()
		}
	}
}

func (rl *RateLimiter) CleanupExpired() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, rec := range rl.attempts {
		if !rec.lockedAt.IsZero() {
			if now.Sub(rec.lockedAt) > rl.lockoutDur {
				delete(rl.attempts, key)
			}
		} else if now.Sub(rec.updatedAt) > rl.windowDur {
			delete(rl.attempts, key)
		}
	}
}

func (rl *RateLimiter) ensureCapacityLocked(now time.Time) {
	if len(rl.attempts) < rl.maxEntries {
		return
	}

	// 1. Remove expired
	for key, rec := range rl.attempts {
		if !rec.lockedAt.IsZero() {
			if now.Sub(rec.lockedAt) > rl.lockoutDur {
				delete(rl.attempts, key)
			}
		} else if now.Sub(rec.updatedAt) > rl.windowDur {
			delete(rl.attempts, key)
		}
	}

	// 2. If still at or over capacity, evict oldest entries
	if len(rl.attempts) >= rl.maxEntries {
		countToEvict := rl.maxEntries / 10
		if countToEvict < 1 {
			countToEvict = 1
		}
		for key := range rl.attempts {
			delete(rl.attempts, key)
			countToEvict--
			if countToEvict <= 0 {
				break
			}
		}
	}
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
		rl.ensureCapacityLocked(now)
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

func (rl *RateLimiter) Allow(key string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rec, ok := rl.attempts[key]
	if !ok || now.Sub(rec.updatedAt) > rl.windowDur {
		rl.ensureCapacityLocked(now)
		rl.attempts[key] = &attemptRecord{
			failures:  1,
			updatedAt: now,
		}
		return true, 0
	}

	if !rec.lockedAt.IsZero() {
		remaining := rec.lockedAt.Add(rl.lockoutDur).Sub(now)
		if remaining > 0 {
			return false, remaining
		}
		rec.failures = 1
		rec.lockedAt = time.Time{}
		rec.updatedAt = now
		return true, 0
	}

	rec.failures++
	rec.updatedAt = now

	if rec.failures > rl.maxAttempts {
		rec.lockedAt = now
		return false, rl.lockoutDur
	}

	return true, 0
}

func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, key)
}

// RateLimitMiddleware creates an HTTP middleware that limits requests per IP
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				parts := strings.Split(forwarded, ",")
				ip = strings.TrimSpace(parts[0])
			} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				ip = realIP
			}
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}

			allowed, remaining := limiter.Allow(ip)
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(remaining.Seconds())+1))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"too_many_requests","message":"Zu viele Anfragen. Bitte versuchen Sie es später erneut."}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
