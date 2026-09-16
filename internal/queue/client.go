package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// NewClient initializes a new River client with the registered workers
func NewClient(ctx context.Context, dbPool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	// Auto-apply River schema migrations
	migrator, err := rivermigrate.New(riverpgxv5.New(dbPool), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{}); err != nil {
		return nil, fmt.Errorf("failed to apply river migrations: %w", err)
	}

	workers := river.NewWorkers()
	RegisterWorkers(workers)

	client, err := river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create river client: %w", err)
	}

	return client, nil
}
