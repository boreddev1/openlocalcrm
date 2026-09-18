package contact_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestContactService_Lifecycle(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := contact.NewService(querier, auditService)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

	// 1. Validation errors
	_, err := svc.Create(ctx, actorID, contact.CreateContactInput{
		FirstName: "",
		LastName:  "Mustermann",
	})
	if err != contact.ErrInvalidContact {
		t.Fatalf("expected ErrInvalidContact for empty first name, got %v", err)
	}

	_, err = svc.Create(ctx, actorID, contact.CreateContactInput{
		FirstName: "Max",
		LastName:  "",
	})
	if err != contact.ErrInvalidContact {
		t.Fatalf("expected ErrInvalidContact for empty last name, got %v", err)
	}

	// 2. Create valid contact
	lat := 50.1109
	lon := 8.6821
	created, err := svc.Create(ctx, actorID, contact.CreateContactInput{
		FirstName:     "Dr. Michael",
		LastName:      "Weber",
		Email:         "michael.weber@weber-solar.de",
		Phone:         "+49 69 987654",
		Mobile:        "+49 171 1234567",
		Position:      "Geschäftsführer",
		LeadSource:    "Messe Frankfurt",
		AddressStreet: "Westhafen Tower 1",
		AddressZip:    "60327",
		AddressCity:   "Frankfurt",
		Latitude:      &lat,
		Longitude:     &lon,
	})
	if err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}
	if created.FirstName != "Dr. Michael" || created.LastName != "Weber" {
		t.Fatalf("unexpected name: %s %s", created.FirstName, created.LastName)
	}

	// 3. Get contact by ID
	fetched, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to get contact: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected ID %v, got %v", created.ID, fetched.ID)
	}

	// 4. Update contact
	updated, err := svc.Update(ctx, actorID, contact.UpdateContactInput{
		ID:        created.ID,
		FirstName: "Dr. Michael",
		LastName:  "Weber-Schulz",
		Email:     "m.weber@weber-solar.de",
	})
	if err != nil {
		t.Fatalf("failed to update contact: %v", err)
	}
	if updated.LastName != "Weber-Schulz" {
		t.Fatalf("unexpected updated last name: %s", updated.LastName)
	}

	// 5. List contacts
	contacts, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to list contacts: %v", err)
	}
	if len(contacts) == 0 {
		t.Fatalf("expected at least 1 contact")
	}

	// 6. Delete contact
	if err := svc.Delete(ctx, actorID, created.ID); err != nil {
		t.Fatalf("failed to delete contact: %v", err)
	}
}
