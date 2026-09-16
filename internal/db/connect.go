package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectWithRetry attempts to establish a pgxpool.Pool and ping PostgreSQL.
// If the connection fails or the database is starting up, it retries with
// exponential backoff up to maxRetries attempts.
func ConnectWithRetry(ctx context.Context, dbURL string, maxRetries int, initialDelay time.Duration) (*pgxpool.Pool, error) {
	if maxRetries <= 0 {
		maxRetries = 1
	}
	if initialDelay <= 0 {
		initialDelay = 500 * time.Millisecond
	}

	var pool *pgxpool.Pool
	var lastErr error
	delay := initialDelay

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		pool, lastErr = pgxpool.New(ctx, dbURL)
		if lastErr == nil {
			pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
			lastErr = pool.Ping(pingCtx)
			pingCancel()

			if lastErr == nil {
				log.Printf("[Database] Successfully connected to PostgreSQL at %s (attempt %d/%d)", redactURL(dbURL), attempt, maxRetries)
				return pool, nil
			}
			pool.Close()
		}

		if attempt < maxRetries {
			log.Printf("[Database] Connection attempt %d/%d failed: %v. Retrying in %v...", attempt, maxRetries, lastErr, delay)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}

			delay = time.Duration(float64(delay) * 1.5)
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
		}
	}

	return nil, fmt.Errorf("failed connecting to PostgreSQL after %d attempts: %w", maxRetries, lastErr)
}

// redactURL masks credentials in database connection strings for safe logging
func redactURL(u string) string {
	// Simple redactor to avoid printing passwords in log files
	parts := fmt.Sprintf("%v", u)
	if atIdx := len(parts); atIdx > 0 {
		// If URL has user:pass@host format, redact password
		var userPassEnd = -1
		for i := 0; i < len(parts); i++ {
			if parts[i] == '@' {
				userPassEnd = i
				break
			}
		}
		if userPassEnd != -1 {
			colonIdx := -1
			for i := 0; i < userPassEnd; i++ {
				if parts[i] == ':' && i > 8 { // after postgres://
					colonIdx = i
					break
				}
			}
			if colonIdx != -1 {
				return parts[:colonIdx+1] + "******" + parts[userPassEnd:]
			}
		}
	}
	return parts
}
