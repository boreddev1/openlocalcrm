package db

import (
	"context"
	"testing"
	"time"
)

func TestRedactURL(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    "postgres://crm_user:secret_pass@localhost:5432/crm_db?sslmode=disable",
			expected: "postgres://crm_user:******@localhost:5432/crm_db?sslmode=disable",
		},
		{
			input:    "postgres://postgres:crm_pass@db:5432/openlocalcrm",
			expected: "postgres://postgres:******@db:5432/openlocalcrm",
		},
		{
			input:    "invalid-url",
			expected: "invalid-url",
		},
	}

	for _, c := range cases {
		actual := redactURL(c.input)
		if actual != c.expected {
			t.Errorf("expected %q, got %q", c.expected, actual)
		}
	}
}

func TestConnectWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	pool, err := ConnectWithRetry(ctx, "postgres://invalid:invalid@127.0.0.1:5432/nonexistent", 3, 10*time.Millisecond)
	if err == nil {
		t.Errorf("expected error with cancelled context, got nil")
	}
	if pool != nil {
		t.Errorf("expected nil pool, got %v", pool)
	}
}

func TestConnectWithRetry_FailsGracefully(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// An unreachable port
	pool, err := ConnectWithRetry(ctx, "postgres://user:pass@127.0.0.1:59999/testdb", 2, 20*time.Millisecond)
	if err == nil {
		t.Errorf("expected error connecting to unreachable port, got nil")
	}
	if pool != nil {
		t.Errorf("expected nil pool, got %v", pool)
	}
}
