package connectors_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/connectors"
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
