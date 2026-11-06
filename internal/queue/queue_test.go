package queue_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/queue"
)

func uuidStr(id pgtype.UUID) string {
	return uuid.UUID(id.Bytes).String()
}

// fakeInserter captures every job that would have been enqueued, so tests
// can assert on fan-out behavior without a real River client/Postgres.
type fakeInserter struct {
	inserted []river.JobArgs
}

func (f *fakeInserter) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.inserted = append(f.inserted, args)
	return &rivertype.JobInsertResult{}, nil
}

// fakeSyncer stands in for the real IMAP-backed email.Service.SyncAccount.
type fakeSyncer struct {
	messages []db.EmailMessage
	err      error
	called   []pgtype.UUID
}

func (f *fakeSyncer) SyncAccount(ctx context.Context, id pgtype.UUID) ([]db.EmailMessage, error) {
	f.called = append(f.called, id)
	return f.messages, f.err
}

// fakeTriager stands in for the real ai.TriageService, avoiding any
// dependency on a live AI backend for the job-orchestration tests.
type fakeTriager struct {
	result *ai.TriageResult
	err    error
}

func (f *fakeTriager) TriageEmail(ctx context.Context, sender, subject, body string) (*ai.TriageResult, error) {
	return f.result, f.err
}

func newTestUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}

func TestJobKinds(t *testing.T) {
	syncArgs := queue.EmailSyncArgs{AccountID: "acc-123"}
	if syncArgs.Kind() != "email_sync" {
		t.Fatalf("expected email_sync, got %s", syncArgs.Kind())
	}

	triageArgs := queue.EmailTriageArgs{MessageID: "msg-456"}
	if triageArgs.Kind() != "email_triage" {
		t.Fatalf("expected email_triage, got %s", triageArgs.Kind())
	}

	syncAllArgs := queue.EmailSyncAllArgs{}
	if syncAllArgs.Kind() != "email_sync_all" {
		t.Fatalf("expected email_sync_all, got %s", syncAllArgs.Kind())
	}

	workers := river.NewWorkers()
	queue.RegisterWorkers(workers, queue.Deps{})
	// Registration succeeded without panics
}

func TestEmailSyncWorker_EnqueuesTriageForEachNewMessage(t *testing.T) {
	accountID := newTestUUID(t, "11111111-1111-1111-1111-111111111111")
	msgID1 := newTestUUID(t, "22222222-2222-2222-2222-222222222222")
	msgID2 := newTestUUID(t, "33333333-3333-3333-3333-333333333333")

	syncer := &fakeSyncer{messages: []db.EmailMessage{{ID: msgID1}, {ID: msgID2}}}
	inserter := &fakeInserter{}

	worker := queue.NewEmailSyncWorkerForTest(queue.Deps{EmailSvc: syncer, Inserter: inserter})

	err := worker.Work(context.Background(), &river.Job[queue.EmailSyncArgs]{
		Args: queue.EmailSyncArgs{AccountID: uuidStr(accountID)},
	})
	if err != nil {
		t.Fatalf("Work failed: %v", err)
	}

	if len(syncer.called) != 1 {
		t.Fatalf("expected SyncAccount to be called once, got %d", len(syncer.called))
	}
	if len(inserter.inserted) != 2 {
		t.Fatalf("expected 2 triage jobs enqueued, got %d", len(inserter.inserted))
	}
	for _, args := range inserter.inserted {
		if _, ok := args.(queue.EmailTriageArgs); !ok {
			t.Fatalf("expected EmailTriageArgs, got %T", args)
		}
	}
}

func TestEmailSyncWorker_RequiresAccountID(t *testing.T) {
	worker := queue.NewEmailSyncWorkerForTest(queue.Deps{})
	err := worker.Work(context.Background(), &river.Job[queue.EmailSyncArgs]{Args: queue.EmailSyncArgs{}})
	if err == nil {
		t.Fatal("expected error for missing account_id")
	}
}

func TestEmailSyncWorker_PropagatesSyncError(t *testing.T) {
	syncer := &fakeSyncer{err: context.DeadlineExceeded}
	worker := queue.NewEmailSyncWorkerForTest(queue.Deps{EmailSvc: syncer})

	accountID := newTestUUID(t, "11111111-1111-1111-1111-111111111111")
	err := worker.Work(context.Background(), &river.Job[queue.EmailSyncArgs]{
		Args: queue.EmailSyncArgs{AccountID: uuidStr(accountID)},
	})
	if err == nil {
		t.Fatal("expected sync error to propagate")
	}
}

func TestEmailSyncAllWorker_OnlyEnqueuesActiveAccounts(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	ctx := context.Background()

	active, err := q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name: "Aktiv", EmailAddress: "aktiv@openlocalcrm.local", Provider: "IMAP", IsActive: true,
	})
	if err != nil {
		t.Fatalf("CreateEmailAccount failed: %v", err)
	}
	inactive, err := q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name: "Inaktiv", EmailAddress: "inaktiv@openlocalcrm.local", Provider: "IMAP", IsActive: true,
	})
	if err != nil {
		t.Fatalf("CreateEmailAccount failed: %v", err)
	}
	if _, err := q.UpdateEmailAccount(ctx, db.UpdateEmailAccountParams{ID: inactive.ID, Name: inactive.Name, IsActive: false}); err != nil {
		t.Fatalf("UpdateEmailAccount failed: %v", err)
	}

	inserter := &fakeInserter{}
	worker := queue.NewEmailSyncAllWorkerForTest(queue.Deps{Queries: q, Inserter: inserter})

	if err := worker.Work(ctx, &river.Job[queue.EmailSyncAllArgs]{}); err != nil {
		t.Fatalf("Work failed: %v", err)
	}

	if len(inserter.inserted) != 1 {
		t.Fatalf("expected exactly 1 sync job enqueued for the active account, got %d", len(inserter.inserted))
	}
	got, ok := inserter.inserted[0].(queue.EmailSyncArgs)
	if !ok {
		t.Fatalf("expected EmailSyncArgs, got %T", inserter.inserted[0])
	}
	if got.AccountID != uuidStr(active.ID) {
		t.Fatalf("expected sync job for active account %s, got %s", uuidStr(active.ID), got.AccountID)
	}
}

func TestEmailTriageWorker_PersistsTagsFromRealTriageResult(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	ctx := context.Background()

	acc, err := q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{Name: "Postfach", EmailAddress: "postfach@openlocalcrm.local", Provider: "IMAP", IsActive: true})
	if err != nil {
		t.Fatalf("CreateEmailAccount failed: %v", err)
	}
	msg, err := q.CreateEmailMessage(ctx, db.CreateEmailMessageParams{
		AccountID:       acc.ID,
		ThreadID:        "thread-1",
		MessageID:       "<msg-1@example.com>",
		Direction:       "INBOUND",
		SenderEmail:     "kunde@example.com",
		RecipientEmails: []byte("[]"),
		Subject:         "Anfrage PV-Anlage",
		BodyText:        pgtype.Text{String: "Bitte Angebot schicken.", Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateEmailMessage failed: %v", err)
	}

	triager := &fakeTriager{result: &ai.TriageResult{Category: "ANFRAGE", Priority: "HIGH"}}
	worker := queue.NewEmailTriageWorkerForTest(queue.Deps{Queries: q, Triager: triager})

	err = worker.Work(ctx, &river.Job[queue.EmailTriageArgs]{
		Args: queue.EmailTriageArgs{MessageID: uuidStr(msg.ID)},
	})
	if err != nil {
		t.Fatalf("Work failed: %v", err)
	}

	updated, err := q.GetEmailMessageByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetEmailMessageByID failed: %v", err)
	}
	var tags []string
	if err := json.Unmarshal(updated.Tags, &tags); err != nil {
		t.Fatalf("failed to unmarshal persisted tags: %v", err)
	}
	if len(tags) != 2 || tags[0] != "ANFRAGE" || tags[1] != "HIGH" {
		t.Fatalf("expected tags [ANFRAGE HIGH], got %v", tags)
	}
}

func TestEmailTriageWorker_RequiresMessageID(t *testing.T) {
	worker := queue.NewEmailTriageWorkerForTest(queue.Deps{})
	err := worker.Work(context.Background(), &river.Job[queue.EmailTriageArgs]{Args: queue.EmailTriageArgs{}})
	if err == nil {
		t.Fatal("expected error for missing message_id")
	}
}

func TestEmailTriageWorker_MessageNotFound(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	worker := queue.NewEmailTriageWorkerForTest(queue.Deps{Queries: q})

	missing := newTestUUID(t, "99999999-9999-9999-9999-999999999999")
	err := worker.Work(context.Background(), &river.Job[queue.EmailTriageArgs]{
		Args: queue.EmailTriageArgs{MessageID: uuidStr(missing)},
	})
	if err == nil {
		t.Fatal("expected error for missing message")
	}
}

type fakeAutomationTrigger struct {
	calls []struct {
		TriggerType string
		TargetID    string
		TargetType  string
		TargetName  string
	}
	err error
}

func (f *fakeAutomationTrigger) TriggerEvent(ctx context.Context, triggerType, targetID, targetType, targetName string) ([]automation.WorkflowRun, error) {
	f.calls = append(f.calls, struct {
		TriggerType string
		TargetID    string
		TargetType  string
		TargetName  string
	}{
		TriggerType: triggerType,
		TargetID:    targetID,
		TargetType:  targetType,
		TargetName:  targetName,
	})
	return nil, f.err
}

type mockInactivityQuerier struct {
	db.Querier
	contacts []db.Contact
	err      error
}

func (m *mockInactivityQuerier) ListInactiveContacts(ctx context.Context, arg db.ListInactiveContactsParams) ([]db.Contact, error) {
	return m.contacts, m.err
}

func TestInactivityScanWorker_TriggersInactivityTimeout(t *testing.T) {
	ctx := context.Background()
	c1ID := newTestUUID(t, "11111111-1111-1111-1111-111111111111")
	c2ID := newTestUUID(t, "22222222-2222-2222-2222-222222222222")

	q := &mockInactivityQuerier{
		contacts: []db.Contact{
			{
				ID:        c1ID,
				FirstName: "Hans",
				LastName:  "Inaktiv",
			},
			{
				ID:        c2ID,
				FirstName: "Greta",
				LastName:  "Alt",
			},
		},
	}

	trigger := &fakeAutomationTrigger{}
	worker := queue.NewInactivityScanWorkerForTest(queue.Deps{
		Queries:       q,
		AutomationSvc: trigger,
	})

	err := worker.Work(ctx, &river.Job[queue.InactivityScanArgs]{
		Args: queue.InactivityScanArgs{},
	})
	if err != nil {
		t.Fatalf("InactivityScanWorker failed: %v", err)
	}

	if len(trigger.calls) != 2 {
		t.Fatalf("expected 2 trigger calls, got %d", len(trigger.calls))
	}

	call1 := trigger.calls[0]
	if call1.TriggerType != "INACTIVITY_TIMEOUT" {
		t.Errorf("expected INACTIVITY_TIMEOUT, got %s", call1.TriggerType)
	}
	if call1.TargetType != "CONTACT" {
		t.Errorf("expected CONTACT, got %s", call1.TargetType)
	}
	if call1.TargetID != uuidStr(c1ID) {
		t.Errorf("expected %s, got %s", uuidStr(c1ID), call1.TargetID)
	}
	if call1.TargetName != "Hans Inaktiv" {
		t.Errorf("expected 'Hans Inaktiv', got %q", call1.TargetName)
	}

	call2 := trigger.calls[1]
	if call2.TargetID != uuidStr(c2ID) || call2.TargetName != "Greta Alt" {
		t.Errorf("unexpected call2: %+v", call2)
	}
}

func TestInactivityScanWorker_DatabaseError(t *testing.T) {
	q := &mockInactivityQuerier{
		err: context.DeadlineExceeded,
	}
	trigger := &fakeAutomationTrigger{}
	worker := queue.NewInactivityScanWorkerForTest(queue.Deps{
		Queries:       q,
		AutomationSvc: trigger,
	})

	err := worker.Work(context.Background(), &river.Job[queue.InactivityScanArgs]{
		Args: queue.InactivityScanArgs{},
	})
	if err == nil {
		t.Fatal("expected error from InactivityScanWorker when query fails")
	}
}

func TestInactivityScanArgs_Kind(t *testing.T) {
	args := queue.InactivityScanArgs{}
	if args.Kind() != "inactivity_scan" {
		t.Fatalf("expected inactivity_scan, got %s", args.Kind())
	}
}
