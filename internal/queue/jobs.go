package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// EmailSyncer is the subset of internal/core/email's Service needed to sync
// an account's inbox. A narrow interface so job tests can inject a fake
// instead of a real IMAP connection.
type EmailSyncer interface {
	SyncAccount(ctx context.Context, id pgtype.UUID) ([]db.EmailMessage, error)
}

// EmailTriager is the subset of internal/ai's TriageService needed to
// classify a message. A narrow interface so job tests can inject a fake
// instead of requiring a live AI backend.
type EmailTriager interface {
	TriageEmail(ctx context.Context, sender, subject, body string) (*ai.TriageResult, error)
}

// JobInserter is the subset of *river.Client[pgx.Tx] the workers need to
// enqueue follow-up jobs.
type JobInserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// ClientHolder lazily hands workers the real River client once it exists.
// river.NewClient needs Workers to be built first, but the workers
// themselves need the client (to enqueue follow-up jobs) — this indirection
// breaks that construction-order cycle. Set Client after river.NewClient
// returns.
type ClientHolder struct {
	Client JobInserter
}

func (h *ClientHolder) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if h.Client == nil {
		return nil, fmt.Errorf("queue: river client not yet initialized")
	}
	return h.Client.Insert(ctx, args, opts)
}

// Deps bundles the real dependencies job workers need. RegisterWorkers
// injects these instead of the workers reaching for global state, so
// cmd/worker can wire real services and tests can wire fakes.
type Deps struct {
	Queries  db.Querier
	EmailSvc EmailSyncer
	Triager  EmailTriager
	Inserter JobInserter
}

func uuidStr(id pgtype.UUID) string {
	return uuid.UUID(id.Bytes).String()
}

// EmailSyncAllArgs triggers a fan-out sync across every active email
// account. Enqueued on a 60s periodic schedule (see client.go) to match the
// UI's "Auto-Sync alle 60 Sekunden".
type EmailSyncAllArgs struct{}

func (EmailSyncAllArgs) Kind() string { return "email_sync_all" }

type EmailSyncAllWorker struct {
	river.WorkerDefaults[EmailSyncAllArgs]
	deps Deps
}

func (w *EmailSyncAllWorker) Work(ctx context.Context, job *river.Job[EmailSyncAllArgs]) error {
	accounts, err := w.deps.Queries.ListEmailAccounts(ctx)
	if err != nil {
		return fmt.Errorf("email sync fan-out: failed to list accounts: %w", err)
	}

	for _, acc := range accounts {
		if !acc.IsActive {
			continue
		}
		if _, err := w.deps.Inserter.Insert(ctx, EmailSyncArgs{AccountID: uuidStr(acc.ID)}, nil); err != nil {
			log.Printf("[worker] failed to enqueue sync for account %s: %v", acc.EmailAddress, err)
		}
	}
	return nil
}

// EmailSyncArgs defines the payload for an email sync job
type EmailSyncArgs struct {
	AccountID string `json:"account_id"`
}

func (EmailSyncArgs) Kind() string { return "email_sync" }

// EmailSyncWorker processes a real, incremental IMAP sync for one account
// and enqueues AI triage for every newly ingested message.
type EmailSyncWorker struct {
	river.WorkerDefaults[EmailSyncArgs]
	deps Deps
}

func (w *EmailSyncWorker) Work(ctx context.Context, job *river.Job[EmailSyncArgs]) error {
	if job.Args.AccountID == "" {
		return fmt.Errorf("email sync: account_id is required")
	}
	var accountID pgtype.UUID
	if err := accountID.Scan(job.Args.AccountID); err != nil {
		return fmt.Errorf("email sync: invalid account_id %q: %w", job.Args.AccountID, err)
	}

	messages, err := w.deps.EmailSvc.SyncAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("email sync: %w", err)
	}

	for _, msg := range messages {
		if _, err := w.deps.Inserter.Insert(ctx, EmailTriageArgs{MessageID: uuidStr(msg.ID)}, nil); err != nil {
			log.Printf("[worker] failed to enqueue triage for message %q: %v", msg.Subject, err)
		}
	}

	log.Printf("[worker] email sync completed for account %s: %d new message(s)", job.Args.AccountID, len(messages))
	return nil
}

// EmailTriageArgs defines the payload for AI email triage
type EmailTriageArgs struct {
	MessageID string `json:"message_id"`
}

func (EmailTriageArgs) Kind() string { return "email_triage" }

// EmailTriageWorker classifies an inbound message via the real AI gateway
// (PII-guarded inside the gateway itself) and persists category/priority as
// tags on the message.
type EmailTriageWorker struct {
	river.WorkerDefaults[EmailTriageArgs]
	deps Deps
}

func (w *EmailTriageWorker) Work(ctx context.Context, job *river.Job[EmailTriageArgs]) error {
	if job.Args.MessageID == "" {
		return fmt.Errorf("email triage: message_id is required")
	}
	var msgID pgtype.UUID
	if err := msgID.Scan(job.Args.MessageID); err != nil {
		return fmt.Errorf("email triage: invalid message_id %q: %w", job.Args.MessageID, err)
	}

	msg, err := w.deps.Queries.GetEmailMessageByID(ctx, msgID)
	if err != nil {
		return fmt.Errorf("email triage: message %s not found: %w", job.Args.MessageID, err)
	}

	result, err := w.deps.Triager.TriageEmail(ctx, msg.SenderEmail, msg.Subject, msg.BodyText.String)
	if err != nil {
		return fmt.Errorf("email triage: ai gateway failed: %w", err)
	}

	tagsJSON, err := json.Marshal([]string{result.Category, result.Priority})
	if err != nil {
		return fmt.Errorf("email triage: failed to encode tags: %w", err)
	}
	if _, err := w.deps.Queries.UpdateEmailMessageTags(ctx, db.UpdateEmailMessageTagsParams{ID: msgID, Tags: tagsJSON}); err != nil {
		return fmt.Errorf("email triage: failed to persist tags: %w", err)
	}

	log.Printf("[worker] email triage completed for message %s: category=%s priority=%s", job.Args.MessageID, result.Category, result.Priority)
	return nil
}

// RegisterWorkers registers all job workers into the River worker pool
func RegisterWorkers(workers *river.Workers, deps Deps) {
	river.AddWorker(workers, &EmailSyncAllWorker{deps: deps})
	river.AddWorker(workers, &EmailSyncWorker{deps: deps})
	river.AddWorker(workers, &EmailTriageWorker{deps: deps})
}

// NewEmailSyncAllWorkerForTest constructs an EmailSyncAllWorker with
// injected deps, for use by tests outside this package.
func NewEmailSyncAllWorkerForTest(deps Deps) *EmailSyncAllWorker {
	return &EmailSyncAllWorker{deps: deps}
}

// NewEmailSyncWorkerForTest constructs an EmailSyncWorker with injected
// deps, for use by tests outside this package.
func NewEmailSyncWorkerForTest(deps Deps) *EmailSyncWorker {
	return &EmailSyncWorker{deps: deps}
}

// NewEmailTriageWorkerForTest constructs an EmailTriageWorker with injected
// deps, for use by tests outside this package.
func NewEmailTriageWorkerForTest(deps Deps) *EmailTriageWorker {
	return &EmailTriageWorker{deps: deps}
}
