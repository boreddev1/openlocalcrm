package queue

import (
	"context"
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
	log.Printf("[worker] syncing email account: %s", job.Args.AccountID)
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
	log.Printf("[worker] triaging email message: %s", job.Args.MessageID)
	return nil
}

// RegisterWorkers registers all job workers into the River worker pool
func RegisterWorkers(workers *river.Workers) {
	river.AddWorker(workers, &EmailSyncWorker{})
	river.AddWorker(workers, &EmailTriageWorker{})
}
