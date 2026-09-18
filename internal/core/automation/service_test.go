package automation_test

import (
	"context"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestAutomationService_ListAndCreate(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := automation.NewService(querier)

	// 1. List default workflows
	wfs, err := svc.ListDefaultWorkflows(ctx)
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}
	if len(wfs) == 0 {
		t.Fatalf("expected at least one default workflow")
	}

	// 2. Validation error on empty name
	_, err = svc.CreateWorkflow(ctx, automation.Workflow{Name: ""})
	if err == nil {
		t.Fatalf("expected error on empty workflow name")
	}

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
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	if created.Name != custom.Name {
		t.Fatalf("unexpected workflow name: %s", created.Name)
	}

	// 4. Test with nil querier fallback
	fallbackSvc := automation.NewService(nil)
	fallbackWfs, err := fallbackSvc.ListDefaultWorkflows(ctx)
	if err != nil || len(fallbackWfs) == 0 {
		t.Fatalf("expected fallback workflows, got err: %v", err)
	}
}
