package contact_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
)

func TestCreateContactValidation(t *testing.T) {
	svc := contact.NewService(nil, nil)
	var actorID pgtype.UUID

	_, err := svc.Create(context.Background(), actorID, contact.CreateContactInput{
		FirstName: "",
		LastName:  "Mustermann",
	})
	if err != contact.ErrInvalidContact {
		t.Fatalf("expected ErrInvalidContact for empty first name, got %v", err)
	}

	_, err = svc.Create(context.Background(), actorID, contact.CreateContactInput{
		FirstName: "Max",
		LastName:  "",
	})
	if err != contact.ErrInvalidContact {
		t.Fatalf("expected ErrInvalidContact for empty last name, got %v", err)
	}
}
