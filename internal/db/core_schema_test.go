package db_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

func TestCoreEntityModels(t *testing.T) {
	contactID := uuid.New()
	companyID := uuid.New()

	var pgContactID pgtype.UUID
	_ = pgContactID.Scan(contactID.String())

	var pgCompanyID pgtype.UUID
	_ = pgCompanyID.Scan(companyID.String())

	contact := db.Contact{
		ID:        pgContactID,
		CompanyID: pgCompanyID,
		FirstName: "Max",
		LastName:  "Mustermann",
		Email:     pgtype.Text{String: "max@muster-energie.de", Valid: true},
	}

	if contact.FirstName != "Max" || contact.LastName != "Mustermann" {
		t.Fatalf("contact model field mismatch")
	}

	deal := db.Deal{
		Title: "100 kWp Solar Installation",
		Stage: "OFFER_SENT",
	}
	if deal.Stage != "OFFER_SENT" {
		t.Fatalf("deal model field mismatch")
	}

	todo := db.Todo{
		Title:    "Follow up on solar offer",
		Status:   "OPEN",
		Priority: "HIGH",
	}
	if todo.Status != "OPEN" || todo.Priority != "HIGH" {
		t.Fatalf("todo model field mismatch")
	}
}
