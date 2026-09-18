package company_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/company"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestCompanyService_Lifecycle(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := company.NewService(querier, auditService)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

	// 1. Validation error on empty name
	_, err := svc.Create(ctx, actorID, company.CreateCompanyInput{Name: ""})
	if err != company.ErrInvalidCompany {
		t.Fatalf("expected ErrInvalidCompany, got %v", err)
	}

	// 2. Create valid company
	created, err := svc.Create(ctx, actorID, company.CreateCompanyInput{
		Name:          "Müller & Söhne Logistik GmbH",
		Domain:        "mueller-logistik.de",
		Phone:         "+49 69 123456",
		Email:         "kontakt@mueller-logistik.de",
		AddressStreet: "Speicherstraße 12",
		AddressZip:    "60327",
		AddressCity:   "Frankfurt am Main",
	})
	if err != nil {
		t.Fatalf("failed to create company: %v", err)
	}
	if created.Name != "Müller & Söhne Logistik GmbH" {
		t.Fatalf("unexpected name: %s", created.Name)
	}

	// 3. Get company
	fetched, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to get company: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected ID %v, got %v", created.ID, fetched.ID)
	}

	// 4. Update company
	updated, err := svc.Update(ctx, actorID, company.UpdateCompanyInput{
		ID:    created.ID,
		Name:  "Müller Logistik International GmbH",
		Email: "info@mueller-logistik.de",
	})
	if err != nil {
		t.Fatalf("failed to update company: %v", err)
	}
	if updated.Name != "Müller Logistik International GmbH" {
		t.Fatalf("unexpected updated name: %s", updated.Name)
	}

	// 5. List companies
	companies, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to list companies: %v", err)
	}
	if len(companies) == 0 {
		t.Fatalf("expected at least 1 company")
	}

	// 6. Delete company
	if err := svc.Delete(ctx, actorID, created.ID); err != nil {
		t.Fatalf("failed to delete company: %v", err)
	}
}
