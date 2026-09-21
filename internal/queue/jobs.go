package queue

import (
	"context"
	"fmt"
	"log"

	"github.com/riverqueue/river"
)

// EmailSyncArgs defines the payload for an email sync job
type EmailSyncArgs struct {
	AccountID string `json:"account_id"`
}

func (EmailSyncArgs) Kind() string { return "email_sync" }

// EmailSyncWorker processes email synchronization for a given account
type EmailSyncWorker struct {
	river.WorkerDefaults[EmailSyncArgs]
}

func (w *EmailSyncWorker) Work(ctx context.Context, job *river.Job[EmailSyncArgs]) error {
	if ctx.Err() != nil {
		return fmt.Errorf("email sync cancelled: %w", ctx.Err())
	}
	attempt := 1
	if job.JobRow != nil {
		attempt = job.Attempt
	}
	log.Printf("[worker] syncing email account: %s (attempt: %d)", job.Args.AccountID, attempt)
	// In demo/sync mode this is a no-op; real IMAP sync would connect and fetch here.
	// The synchronous fallback ensures the job doesn't silently fail.
	if job.Args.AccountID == "" {
		return fmt.Errorf("email sync: account_id is required")
	}
	log.Printf("[worker] email sync completed for account: %s", job.Args.AccountID)
	return nil
}

// EmailTriageArgs defines the payload for AI email triage
type EmailTriageArgs struct {
	MessageID string `json:"message_id"`
}

func (EmailTriageArgs) Kind() string { return "email_triage" }

// EmailTriageWorker processes AI email classification and draft generation
type EmailTriageWorker struct {
	river.WorkerDefaults[EmailTriageArgs]
}

func (w *EmailTriageWorker) Work(ctx context.Context, job *river.Job[EmailTriageArgs]) error {
	if ctx.Err() != nil {
		return fmt.Errorf("email triage cancelled: %w", ctx.Err())
	}
	attempt := 1
	if job.JobRow != nil {
		attempt = job.Attempt
	}
	log.Printf("[worker] triaging email message: %s (attempt: %d)", job.Args.MessageID, attempt)
	if job.Args.MessageID == "" {
		return fmt.Errorf("email triage: message_id is required")
	}
	// Real triage would call the AI gateway here; in demo mode this logs the action.
	log.Printf("[worker] email triage completed for message: %s", job.Args.MessageID)
	return nil
}

// RegisterWorkers registers all job workers into the River worker pool
func RegisterWorkers(workers *river.Workers) {
	river.AddWorker(workers, &EmailSyncWorker{})
	river.AddWorker(workers, &EmailTriageWorker{})
}
