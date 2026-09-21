package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// EmailSyncInterval matches the UI's "Auto-Sync alle 60 Sekunden" claim.
const EmailSyncInterval = 60 * time.Second

// NewClient initializes a new River client with the registered workers and
// the periodic email-sync fan-out job.
func NewClient(ctx context.Context, dbPool *pgxpool.Pool, deps Deps) (*river.Client[pgx.Tx], error) {
	// Auto-apply River schema migrations
	migrator, err := rivermigrate.New(riverpgxv5.New(dbPool), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{}); err != nil {
		return nil, fmt.Errorf("failed to apply river migrations: %w", err)
	}

	holder := &ClientHolder{}
	deps.Inserter = holder

	workers := river.NewWorkers()
	RegisterWorkers(workers, deps)

	client, err := river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(EmailSyncInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return EmailSyncAllArgs{}, nil
				},
				&river.PeriodicJobOpts{ID: "email-sync-all", RunOnStart: true},
			),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create river client: %w", err)
	}

	holder.Client = client
	return client, nil
}
