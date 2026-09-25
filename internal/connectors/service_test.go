package connectors_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/connectors"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type MockContactQuerier struct {
	db.Querier
}

func (m *MockContactQuerier) CreateContact(ctx context.Context, arg db.CreateContactParams) (db.Contact, error) {
	var id pgtype.UUID
	_ = id.Scan("33333333-3333-3333-3333-333333333333")
	return db.Contact{
		ID:        id,
		FirstName: arg.FirstName,
		LastName:  arg.LastName,
		Email:     arg.Email,
	}, nil
}

func TestConnectorLeadIntake(t *testing.T) {
	mockDB := &MockContactQuerier{}
	contactSvc := contact.NewService(mockDB, nil)
	engine := connectors.NewEngine(contactSvc, nil, "valid-token-123")

	// 1. Invalid Token Test
	_, err := engine.IngestLead(context.Background(), "invalid-token", connectors.LeadIntakePayload{
		LastName: "Mustermann",
	})
	if err != connectors.ErrUnauthorizedConnector {
		t.Fatalf("expected ErrUnauthorizedConnector, got %v", err)
	}

	// 2. Valid Ingest
	res, err := engine.IngestLead(context.Background(), "valid-token-123", connectors.LeadIntakePayload{
		FirstName: "Sabine",
		LastName:  "Mustermann",
		Email:     "sabine@photovoltaik-test.de",
		Street:    "Goethestraße 10",
		City:      "Frankfurt",
		Zip:       "60313",
		Source:    "WEBSITE_CALCULATOR",
	})

	if err != nil {
		t.Fatalf("expected successful ingestion, got: %v", err)
	}

	if res.LastName != "Mustermann" {
		t.Fatalf("expected contact LastName Mustermann, got %s", res.LastName)
	}
}

type MockEventTrigger struct {
	calledWith []struct {
		TriggerType string
		TargetID    string
		TargetType  string
		TargetName  string
	}
	err error
}

func (m *MockEventTrigger) TriggerEvent(ctx context.Context, triggerType, targetID, targetType, targetName string) ([]automation.WorkflowRun, error) {
	m.calledWith = append(m.calledWith, struct {
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
	return nil, m.err
}

func TestConnectorLeadIntake_TriggersNewLeadWorkflow(t *testing.T) {
	mockDB := &MockContactQuerier{}
	contactSvc := contact.NewService(mockDB, nil)
	trigger := &MockEventTrigger{}
	engine := connectors.NewEngine(contactSvc, nil, "valid-token-123", trigger)

	res, err := engine.IngestLead(context.Background(), "valid-token-123", connectors.LeadIntakePayload{
		FirstName: "Max",
		LastName:  "Mustermann",
		Email:     "max@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected ingest error: %v", err)
	}
	if res.LastName != "Mustermann" {
		t.Fatalf("expected Mustermann, got %s", res.LastName)
	}

	if len(trigger.calledWith) != 1 {
		t.Fatalf("expected 1 TriggerEvent call, got %d", len(trigger.calledWith))
	}
	call := trigger.calledWith[0]
	if call.TriggerType != "NEW_LEAD" {
		t.Errorf("expected trigger NEW_LEAD, got %s", call.TriggerType)
	}
	if call.TargetType != "CONTACT" {
		t.Errorf("expected target type CONTACT, got %s", call.TargetType)
	}
	if call.TargetName != "Max Mustermann" {
		t.Errorf("expected target name 'Max Mustermann', got %q", call.TargetName)
	}
	if call.TargetID != "33333333-3333-3333-3333-333333333333" {
		t.Errorf("expected target ID 33333333-3333-3333-3333-333333333333, got %s", call.TargetID)
	}
}

func TestConnectorLeadIntake_TriggerErrorDoesNotFailIngest(t *testing.T) {
	mockDB := &MockContactQuerier{}
	contactSvc := contact.NewService(mockDB, nil)
	trigger := &MockEventTrigger{err: context.DeadlineExceeded}
	engine := connectors.NewEngine(contactSvc, nil, "valid-token-123", trigger)

	res, err := engine.IngestLead(context.Background(), "valid-token-123", connectors.LeadIntakePayload{
		FirstName: "Erika",
		LastName:  "Musterfrau",
		Email:     "erika@example.com",
	})
	if err != nil {
		t.Fatalf("ingest should not fail when trigger fails, got error: %v", err)
	}
	if res.LastName != "Musterfrau" {
		t.Fatalf("expected Musterfrau, got %s", res.LastName)
	}
	if len(trigger.calledWith) != 1 {
		t.Fatalf("expected trigger to be called once")
	}
}
