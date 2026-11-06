package automation_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/core/notification"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/stretchr/testify/require"
)

func TestAutomationService_ListDefaultWorkflows_NoFallbackOnEmpty(t *testing.T) {
	ctx := context.Background()

	// 1. With nil querier -> returns honestly empty slice, NO hardcoded fallback
	nilSvc := automation.NewService(nil)
	wfs, err := nilSvc.ListDefaultWorkflows(ctx)
	require.NoError(t, err)
	require.Empty(t, wfs, "nil querier must return honestly empty workflows list, no fallback")
}

func TestAutomationService_ListAndCreate(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := automation.NewService(querier)

	// 1. List default seeded workflows from querier
	wfs, err := svc.ListDefaultWorkflows(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, wfs)

	// 2. Validation error on empty name
	_, err = svc.CreateWorkflow(ctx, automation.Workflow{Name: ""})
	require.Error(t, err)

	// 3. Create custom workflow
	custom := automation.Workflow{
		Name:        "Automatischer Willkommens-Call",
		Description: "Erstellt Anruf-Todo nach Registrierung",
		TriggerType: "USER_REGISTERED",
		TargetType:  "CONTACT",
		IsActive:    true,
		Steps: []automation.StepDefinition{
			{
				StepNumber: 1,
				Title:      "Begrüßungsanruf terminieren",
				ActionType: "CREATE_TASK",
				Payload:    map[string]any{"priority": "HIGH"},
			},
		},
	}

	created, err := svc.CreateWorkflow(ctx, custom)
	require.NoError(t, err)
	require.Equal(t, custom.Name, created.Name)
}

func TestAutomationService_TriggerEvent_StepFailure_MarksRunFailed(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := automation.NewService(querier)

	// Create a workflow with an action that fails during execution (e.g., webhook with invalid/unreachable URL)
	wf, err := svc.CreateWorkflow(ctx, automation.Workflow{
		Name:        "Failing Webhook Workflow",
		TriggerType: "FAIL_TRIGGER",
		TargetType:  "CONTACT",
		IsActive:    true,
		Steps: []automation.StepDefinition{
			{
				StepNumber: 1,
				Title:      "Call unreachable webhook",
				ActionType: "WEBHOOK",
				Payload:    map[string]any{"url": "http://127.0.0.1:0/unreachable"},
			},
		},
	})
	require.NoError(t, err)

	runs, err := svc.TriggerEvent(ctx, "FAIL_TRIGGER", "44444444-4444-4444-4444-444444444441", "CONTACT", "Dr. Michael Weber")
	require.NoError(t, err)
	require.Len(t, runs, 1)

	run := runs[0]
	require.Equal(t, "FAILED", run.Status, "Step failure must mark run as FAILED, never disguised as WAITING_APPROVAL")
	require.NotNil(t, run.CompletedAt, "Failed run must have completed_at set")

	// Verify run in DB has FAILED status and snapshot with error cause
	dbRun, err := querier.GetWorkflowRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, "FAILED", dbRun.Status)
	require.NotEmpty(t, dbRun.SnapshotJson)

	var snapshot map[string]any
	err = json.Unmarshal(dbRun.SnapshotJson, &snapshot)
	require.NoError(t, err)
	require.Contains(t, snapshot, "error", "Snapshot must contain honest error explanation")
	_ = wf
}

func TestAutomationService_ExecuteStep_SetTag_AppendsRealTagsDedup(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := automation.NewService(querier)

	contactIDStr := "44444444-4444-4444-4444-444444444441"
	contactUUID := pgtype.UUID{Bytes: uuid.MustParse(contactIDStr), Valid: true}

	dealIDStr := "55555555-5555-5555-5555-555555555551"
	dealUUID := pgtype.UUID{Bytes: uuid.MustParse(dealIDStr), Valid: true}

	// 1. Tag Contact via SET_TAG workflow
	_, err := svc.CreateWorkflow(ctx, automation.Workflow{
		Name:        "Tag Contact Workflow",
		TriggerType: "TAG_CONTACT_TEST",
		TargetType:  "CONTACT",
		IsActive:    true,
		Steps: []automation.StepDefinition{
			{
				StepNumber: 1,
				Title:      "Add PV-Interessent Tag",
				ActionType: "SET_TAG",
				Payload:    map[string]any{"tag": "PV-Interessent"},
			},
		},
	})
	require.NoError(t, err)

	_, err = svc.TriggerEvent(ctx, "TAG_CONTACT_TEST", contactIDStr, "CONTACT", "Dr. Michael Weber")
	require.NoError(t, err)

	// Verify contact in DB has the tag
	c, err := querier.GetContactByID(ctx, contactUUID)
	require.NoError(t, err)
	var contactTags []string
	require.NoError(t, json.Unmarshal(c.Tags, &contactTags))
	require.Contains(t, contactTags, "PV-Interessent")

	// Trigger again: deduplication ensures tag appears only once
	_, err = svc.TriggerEvent(ctx, "TAG_CONTACT_TEST", contactIDStr, "CONTACT", "Dr. Michael Weber")
	require.NoError(t, err)

	c, err = querier.GetContactByID(ctx, contactUUID)
	require.NoError(t, err)
	var contactTagsAfter []string
	require.NoError(t, json.Unmarshal(c.Tags, &contactTagsAfter))
	require.Len(t, contactTagsAfter, 1, "Tags must be deduplicated on repeated execution")

	// 2. Tag Deal via SET_TAG workflow
	_, err = svc.CreateWorkflow(ctx, automation.Workflow{
		Name:        "Tag Deal Workflow",
		TriggerType: "TAG_DEAL_TEST",
		TargetType:  "DEAL",
		IsActive:    true,
		Steps: []automation.StepDefinition{
			{
				StepNumber: 1,
				Title:      "Add VIP Deal Tag",
				ActionType: "SET_TAG",
				Payload:    map[string]any{"tag": "VIP"},
			},
		},
	})
	require.NoError(t, err)

	_, err = svc.TriggerEvent(ctx, "TAG_DEAL_TEST", dealIDStr, "DEAL", "PV-Gewerbedach")
	require.NoError(t, err)

	d, err := querier.GetDealByID(ctx, dealUUID)
	require.NoError(t, err)
	var dealTags []string
	require.NoError(t, json.Unmarshal(d.Tags, &dealTags))
	require.Contains(t, dealTags, "VIP")

	// Verify TAG_APPLIED audit log exists
	logs, err := querier.ListRecentAuditLogs(ctx, db.ListRecentAuditLogsParams{Limit: 10, Offset: 0})
	require.NoError(t, err)
	var foundAudit bool
	for _, l := range logs {
		if l.Action == "TAG_APPLIED" {
			foundAudit = true
			break
		}
	}
	require.True(t, foundAudit, "Audit log with action TAG_APPLIED must be created")
}

func TestAutomationService_ExecuteStep_NotifyUser_DealAssignee(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	notifSvc := notification.NewService(querier, nil)
	svc := automation.NewService(querier, notifSvc)

	// Deal 55555555-5555-5555-5555-555555555551 is assigned to adminID (11111111-1111-1111-1111-111111111111)
	dealIDStr := "55555555-5555-5555-5555-555555555551"
	expectedAdminID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	_, err := svc.CreateWorkflow(ctx, automation.Workflow{
		Name:        "Notify Deal Assignee Workflow",
		TriggerType: "NOTIFY_DEAL_TEST",
		TargetType:  "DEAL",
		IsActive:    true,
		Steps: []automation.StepDefinition{
			{
				StepNumber: 1,
				Title:      "Deal gewonnen!",
				ActionType: "NOTIFY_USER",
				Payload:    map[string]any{"message": "Herzlichen Glückwunsch zum Abschluss"},
			},
		},
	})
	require.NoError(t, err)

	runs, err := svc.TriggerEvent(ctx, "NOTIFY_DEAL_TEST", dealIDStr, "DEAL", "PV-Gewerbedach")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "COMPLETED", runs[0].Status)

	// Verify notification in DB has the real deals.assigned_to as user_id, NOT deal UUID!
	notifications, err := querier.ListAllNotifications(ctx, db.ListAllNotificationsParams{
		UserID: pgtype.UUID{Bytes: expectedAdminID, Valid: true},
		Limit:  10,
	})
	require.NoError(t, err)
	var found bool
	for _, n := range notifications {
		if n.Title == "Deal gewonnen!" {
			found = true
			require.Equal(t, expectedAdminID, uuid.UUID(n.UserID.Bytes), "Notification user_id must match deals.assigned_to")
			break
		}
	}
	require.True(t, found, "Notification must be sent to the deal assignee")
}

func TestAutomationService_ExecuteStep_NotifyUser_Contact_Skipped(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := automation.NewService(querier)

	contactIDStr := "44444444-4444-4444-4444-444444444441"
	contactUUID := uuid.MustParse(contactIDStr)

	_, err := svc.CreateWorkflow(ctx, automation.Workflow{
		Name:        "Notify Contact Workflow",
		TriggerType: "NOTIFY_CONTACT_TEST",
		TargetType:  "CONTACT",
		IsActive:    true,
		Steps: []automation.StepDefinition{
			{
				StepNumber: 1,
				Title:      "Lead zugewiesen",
				ActionType: "NOTIFY_USER",
				Payload:    map[string]any{"message": "Neuer Lead eingegangen"},
			},
		},
	})
	require.NoError(t, err)

	runs, err := svc.TriggerEvent(ctx, "NOTIFY_CONTACT_TEST", contactIDStr, "CONTACT", "Dr. Michael Weber")
	require.NoError(t, err)
	require.Len(t, runs, 1)

	// The run completes (or succeeds), but step status in snapshot must be SKIPPED with reason:
	// „Kein zuständiger Benutzer ermittelt"
	dbRun, err := querier.GetWorkflowRunByID(ctx, runs[0].ID)
	require.NoError(t, err)

	var snapshot map[string]any
	err = json.Unmarshal(dbRun.SnapshotJson, &snapshot)
	require.NoError(t, err)
	require.Contains(t, snapshot, "step_1_status", "Snapshot must record step 1 status")
	require.Equal(t, "SKIPPED", snapshot["step_1_status"])
	require.Equal(t, "Kein zuständiger Benutzer ermittelt", snapshot["step_1_reason"])

	// CRITICAL INVARIANT: NEVER write contact UUID as notification user_id!
	notifs, _ := querier.ListAllNotifications(ctx, db.ListAllNotificationsParams{
		UserID: pgtype.UUID{Bytes: contactUUID, Valid: true},
		Limit:  10,
	})
	require.Empty(t, notifs, "NEVER create notification with contact UUID as user_id")
}

type mockEmailDrafter struct {
	called bool
}

func (m *mockEmailDrafter) DraftEmail(ctx context.Context, template string, targetType, targetID, targetName string) error {
	m.called = true
	return nil
}

func TestAutomationService_ApproveStep_SuccessAndErrors(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := automation.NewService(querier)

	drafter := &mockEmailDrafter{}
	svc.SetEmailDrafter(drafter)

	// 1. Trigger wf-1 (NEW_LEAD) on contact
	runs, err := svc.TriggerEvent(ctx, "NEW_LEAD", "44444444-4444-4444-4444-444444444441", "CONTACT", "Dr. Michael Weber")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "WAITING_APPROVAL", runs[0].Status)
	require.Equal(t, 2, runs[0].CurrentStep)

	// 2. Approve step 2
	approvedRun, err := svc.ApproveStep(ctx, runs[0].ID)
	require.NoError(t, err)
	require.Equal(t, "COMPLETED", approvedRun.Status)
	require.True(t, drafter.called, "Email drafter should be called on approval")

	// 3. Trying to approve already COMPLETED run returns error
	_, err = svc.ApproveStep(ctx, runs[0].ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is not waiting for approval")

	// 4. Trying to approve non-existent run returns error
	_, err = svc.ApproveStep(ctx, "non-existent-run")
	require.Error(t, err)

	// 5. Test with nil querier returns error
	nilSvc := automation.NewService(nil)
	_, err = nilSvc.ApproveStep(ctx, runs[0].ID)
	require.Error(t, err)

	// 6. Test approval failure when step fails
	// Create run with step that fails on approval
	_, err = svc.CreateWorkflow(ctx, automation.Workflow{
		Name:        "Failing Approval WF",
		TriggerType: "FAIL_APPROVE_TRIGGER",
		TargetType:  "CONTACT",
		IsActive:    true,
		Steps: []automation.StepDefinition{
			{
				StepNumber: 1,
				Title:      "Draft Email",
				ActionType: "DRAFT_EMAIL",
			},
			{
				StepNumber: 2,
				Title:      "Unreachable Webhook",
				ActionType: "WEBHOOK",
				Payload:    map[string]any{"url": "http://127.0.0.1:0/fail"},
			},
		},
	})
	require.NoError(t, err)

	failRuns, err := svc.TriggerEvent(ctx, "FAIL_APPROVE_TRIGGER", "44444444-4444-4444-4444-444444444441", "CONTACT", "Dr. Michael Weber")
	require.NoError(t, err)
	require.Equal(t, "WAITING_APPROVAL", failRuns[0].Status)

	_, err = svc.ApproveStep(ctx, failRuns[0].ID)
	require.Error(t, err)

	failedRunInDB, err := querier.GetWorkflowRunByID(ctx, failRuns[0].ID)
	require.NoError(t, err)
	require.Equal(t, "FAILED", failedRunInDB.Status)
}

func TestAutomationService_GetRunByID_And_ListRuns(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := automation.NewService(querier)

	// List runs (demo seeds has run-101 and run-102)
	runs, err := svc.ListRuns(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, runs)

	// GetRunByID existing
	r, err := svc.GetRunByID(ctx, "run-101")
	require.NoError(t, err)
	require.Equal(t, "run-101", r.ID)

	// GetRunByID non-existent
	_, err = svc.GetRunByID(ctx, "does-not-exist")
	require.Error(t, err)

	// With nil querier
	nilSvc := automation.NewService(nil)
	nilRuns, err := nilSvc.ListRuns(ctx)
	require.NoError(t, err)
	require.Empty(t, nilRuns)

	_, err = nilSvc.GetRunByID(ctx, "run-101")
	require.Error(t, err)
}

func TestAutomationService_CreateWorkflow_Validation(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := automation.NewService(querier)

	// Invalid action type
	_, err := svc.CreateWorkflow(ctx, automation.Workflow{
		Name: "Invalid Action WF",
		Steps: []automation.StepDefinition{
			{
				StepNumber: 1,
				Title:      "Bogus action",
				ActionType: "BOGUS_UNKNOWN_ACTION",
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid action type")
}

func TestAutomationService_ExecuteStep_CreateTask(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := automation.NewService(querier)

	dealIDStr := "55555555-5555-5555-5555-555555555551"

	_, err := svc.CreateWorkflow(ctx, automation.Workflow{
		Name:        "Create Task Workflow",
		TriggerType: "TASK_TEST_TRIGGER",
		TargetType:  "DEAL",
		IsActive:    true,
		Steps: []automation.StepDefinition{
			{
				StepNumber: 1,
				Title:      "Follow up on quote",
				ActionType: "CREATE_TASK",
				Payload:    map[string]any{"priority": "URGENT", "title": "Custom Task Title"},
			},
		},
	})
	require.NoError(t, err)

	runs, err := svc.TriggerEvent(ctx, "TASK_TEST_TRIGGER", dealIDStr, "DEAL", "PV-Gewerbedach")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "COMPLETED", runs[0].Status)
}

func TestAutomation_TestRunWorkflow_PausesAtDraftEmail(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := automation.NewService(q)

	// Use real seeded contact: 44444444-4444-4444-4444-444444444441 (Dr. Michael Weber)
	run, err := svc.TestRunWorkflow(context.Background(), "wf-1", automation.TargetInput{
		TargetID:   "44444444-4444-4444-4444-444444444441",
		TargetType: "CONTACT",
		TargetName: "Dr. Michael Weber",
	})
	if err != nil {
		t.Fatalf("TestRunWorkflow failed: %v", err)
	}

	if run.WorkflowID != "wf-1" {
		t.Errorf("expected workflow_id wf-1, got %s", run.WorkflowID)
	}
	if run.Status != "WAITING_APPROVAL" {
		t.Errorf("expected status WAITING_APPROVAL, got %s", run.Status)
	}
	if run.CurrentStep != 2 {
		t.Errorf("expected paused at step 2 (DRAFT_EMAIL), got %d", run.CurrentStep)
	}
}

func TestAutomation_TestRunWorkflow_RequiresTarget(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := automation.NewService(q)

	_, err := svc.TestRunWorkflow(context.Background(), "wf-1", automation.TargetInput{
		TargetID: "",
	})
	if err == nil {
		t.Fatal("expected error when target_id is empty, got nil")
	}
}

func TestAutomation_ListRuns_ExposesSnapshotAndError(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := automation.NewService(q)

	// Seed a failed run with snapshot containing an error
	_, err := q.CreateWorkflowRun(context.Background(), db.CreateWorkflowRunParams{
		ID:           "run-failed-test",
		WorkflowID:   "wf-1",
		TargetID:     "44444444-4444-4444-4444-444444444441",
		TargetType:   "CONTACT",
		TargetName:   "Dr. Michael Weber",
		Status:       "FAILED",
		CurrentStep:  1,
		SnapshotJson: []byte(`{"error":"Schritt 1 fehlgeschlagen"}`),
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun failed: %v", err)
	}

	runs, err := svc.ListRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	var found *automation.WorkflowRun
	for i := range runs {
		if runs[i].ID == "run-failed-test" {
			found = &runs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected to find run-failed-test in ListRuns")
	}
	if found.Error != "Schritt 1 fehlgeschlagen" {
		t.Errorf("expected run.Error to be %q, got %q", "Schritt 1 fehlgeschlagen", found.Error)
	}
	if found.Snapshot == nil {
		t.Error("expected run.Snapshot to be populated, got nil")
	}
}
