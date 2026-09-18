package queue_test

import (
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/queue"
	"github.com/riverqueue/river"
)

func TestJobKinds(t *testing.T) {
	syncArgs := queue.EmailSyncArgs{AccountID: "acc-123"}
	if syncArgs.Kind() != "email_sync" {
		t.Fatalf("expected email_sync, got %s", syncArgs.Kind())
	}

	triageArgs := queue.EmailTriageArgs{MessageID: "msg-456"}
	if triageArgs.Kind() != "email_triage" {
		t.Fatalf("expected email_triage, got %s", triageArgs.Kind())
	}

	workers := river.NewWorkers()
	queue.RegisterWorkers(workers)
	// Registration succeeded without panics

	syncWorker := &queue.EmailSyncWorker{}
	if err := syncWorker.Work(t.Context(), &river.Job[queue.EmailSyncArgs]{Args: syncArgs}); err != nil {
		t.Fatalf("syncWorker failed: %v", err)
	}

	triageWorker := &queue.EmailTriageWorker{}
	if err := triageWorker.Work(t.Context(), &river.Job[queue.EmailTriageArgs]{Args: triageArgs}); err != nil {
		t.Fatalf("triageWorker failed: %v", err)
	}
}
