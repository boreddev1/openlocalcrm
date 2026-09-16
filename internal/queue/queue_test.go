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
}
