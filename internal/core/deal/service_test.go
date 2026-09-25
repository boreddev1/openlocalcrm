package deal_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/stretchr/testify/require"
)

func TestDealService_Lifecycle(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := deal.NewService(querier, auditService)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

	// 1. Validation error on empty title
	_, err := svc.Create(ctx, actorID, deal.CreateDealInput{Title: ""})
	if err != deal.ErrInvalidDeal {
		t.Fatalf("expected ErrInvalidDeal, got %v", err)
	}

	// 2. Create valid deal
	created, err := svc.Create(ctx, actorID, deal.CreateDealInput{
		Title:       "10 kWp Photovoltaik Anlage Weber",
		Currency:    "EUR",
		Stage:       "OFFER_SENT",
		Probability: 60,
	})
	if err != nil {
		t.Fatalf("failed to create deal: %v", err)
	}
	if created.Title != "10 kWp Photovoltaik Anlage Weber" {
		t.Fatalf("unexpected title: %s", created.Title)
	}

	// 3. Get deal
	fetched, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to get deal: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected ID %v, got %v", created.ID, fetched.ID)
	}

	// 4. Update stage to WON
	updated, err := svc.UpdateStage(ctx, actorID, created.ID, "WON", 100)
	if err != nil {
		t.Fatalf("failed to update stage: %v", err)
	}
	if updated.Stage != "WON" || updated.Probability != 100 {
		t.Fatalf("unexpected stage or probability: %s / %d", updated.Stage, updated.Probability)
	}
	if !updated.ClosedAt.Valid {
		t.Fatalf("expected closed_at to be valid for WON deal")
	}

	// 5. List deals
	deals, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to list deals: %v", err)
	}
	if len(deals) == 0 {
		t.Fatalf("expected at least 1 deal")
	}

	// 6. Delete deal
	if err := svc.Delete(ctx, actorID, created.ID); err != nil {
		t.Fatalf("failed to delete deal: %v", err)
	}
}

func TestDealService_Revoke(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(querier)
	svc := deal.NewService(querier, auditSvc)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	d, err := svc.Create(ctx, actorID, deal.CreateDealInput{
		Title:       "Photovoltaikanlage 10 kWp",
		Stage:       "WON",
		Probability: 100,
	})
	require.NoError(t, err)

	revokedAt := time.Now().UTC()
	reason := "Kunde widerruft Haustürgeschäft fristgerecht (§355 BGB)"
	revoked, err := svc.Revoke(ctx, actorID, d.ID, revokedAt, reason)
	require.NoError(t, err)
	require.Equal(t, "WIDERRUFEN", revoked.Stage)
	require.Equal(t, int32(0), revoked.Probability)
	require.True(t, revoked.WiderrufenAt.Valid)
	require.Equal(t, reason, revoked.WiderrufGrund.String)
}
