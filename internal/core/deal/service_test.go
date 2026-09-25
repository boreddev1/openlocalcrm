package deal_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
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

func TestDealService_WiderrufenIsTerminalBelowHandlers(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := deal.NewService(querier, audit.NewService(querier))
	actorID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	_, err := svc.Create(ctx, actorID, deal.CreateDealInput{Title: "Invalid", Stage: "WIDERRUFEN"})
	require.Error(t, err)

	created, err := svc.Create(ctx, actorID, deal.CreateDealInput{
		Title:       "Terminal test",
		Stage:       "WON",
		Probability: 100,
	})
	require.NoError(t, err)
	_, err = svc.UpdateStage(ctx, actorID, created.ID, "WIDERRUFEN", 0)
	require.Error(t, err)

	revokedAt := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	revoked, err := svc.Revoke(ctx, actorID, created.ID, revokedAt, "Original reason")
	require.NoError(t, err)

	_, err = svc.Update(ctx, actorID, deal.UpdateDealInput{
		ID:          created.ID,
		Title:       "Reopened",
		Stage:       "LEAD",
		Probability: 20,
		Currency:    "EUR",
		Value:       revoked.Value,
	})
	require.Error(t, err)
	_, err = svc.UpdateStage(ctx, actorID, created.ID, "WON", 100)
	require.ErrorIs(t, err, deal.ErrDealRevoked)
	_, err = svc.UpdateStage(ctx, actorID, created.ID, "WIDERRUFEN", 0)
	require.ErrorIs(t, err, deal.ErrDealRevoked)
	err = svc.Delete(ctx, actorID, created.ID)
	require.Error(t, err)

	stored, err := querier.GetDealByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "WIDERRUFEN", stored.Stage)
	require.Equal(t, int32(0), stored.Probability)
	require.Equal(t, revokedAt, stored.WiderrufenAt.Time)
	require.Equal(t, "Original reason", stored.WiderrufGrund.String)
}

func TestDealService_RepeatedRevokePreservesOriginalEvidence(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := deal.NewService(querier, audit.NewService(querier))
	actorID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	created, err := svc.Create(ctx, actorID, deal.CreateDealInput{Title: "Idempotent revoke", Stage: "WON", Probability: 100})
	require.NoError(t, err)

	originalAt := time.Date(2026, 9, 20, 9, 30, 0, 0, time.UTC)
	originalReason := "Original legal reason"
	first, err := svc.Revoke(ctx, actorID, created.ID, originalAt, originalReason)
	require.NoError(t, err)
	second, err := svc.Revoke(ctx, actorID, created.ID, originalAt.Add(48*time.Hour), "Replacement reason")
	require.NoError(t, err)
	require.Equal(t, first.WiderrufenAt, second.WiderrufenAt)
	require.Equal(t, first.WiderrufGrund, second.WiderrufGrund)
	require.Equal(t, originalAt, second.WiderrufenAt.Time)
	require.Equal(t, originalReason, second.WiderrufGrund.String)
	logs, err := querier.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "DEAL",
		EntityID:   created.ID,
		Limit:      10,
	})
	require.NoError(t, err)
	revokeAudits := 0
	for _, entry := range logs {
		if entry.Action == "REVOKE" {
			revokeAudits++
		}
	}
	require.Equal(t, 1, revokeAudits)
}

func TestDealQueryBoundaryRejectsRevokedMutations(t *testing.T) {
	newRevoked := func(t *testing.T) (*demo.InMemoryQuerier, db.Deal) {
		t.Helper()
		querier := demo.NewInMemoryQuerier()
		svc := deal.NewService(querier, nil)
		created, err := svc.Create(context.Background(), pgtype.UUID{}, deal.CreateDealInput{
			Title:       "Query boundary",
			Stage:       "WON",
			Probability: 100,
		})
		require.NoError(t, err)
		_, err = svc.Revoke(context.Background(), pgtype.UUID{}, created.ID, time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC), "Original")
		require.NoError(t, err)
		return querier, created
	}

	t.Run("update", func(t *testing.T) {
		querier, created := newRevoked(t)
		_, err := querier.UpdateDeal(context.Background(), db.UpdateDealParams{
			ID:          created.ID,
			Title:       "Reopened",
			Stage:       "LEAD",
			Probability: 20,
			Currency:    "EUR",
			Value:       created.Value,
		})
		require.Error(t, err)
	})
	t.Run("stage", func(t *testing.T) {
		querier, created := newRevoked(t)
		_, err := querier.UpdateDealStage(context.Background(), db.UpdateDealStageParams{ID: created.ID, Stage: "WON", Probability: 100})
		require.Error(t, err)
	})
	t.Run("tags", func(t *testing.T) {
		querier, created := newRevoked(t)
		_, err := querier.UpdateDealTags(context.Background(), db.UpdateDealTagsParams{ID: created.ID, Tags: []byte(`["late"]`)})
		require.Error(t, err)
	})
	t.Run("delete", func(t *testing.T) {
		querier, created := newRevoked(t)
		_, err := querier.DeleteDeal(context.Background(), created.ID)
		require.Error(t, err)
	})
}

func TestDealService_UpdateAuditContainsRealDiff(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(querier)
	svc := deal.NewService(querier, auditSvc)
	actorID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	created, err := svc.Create(ctx, actorID, deal.CreateDealInput{Title: "Original", Stage: "WON", Probability: 100})
	require.NoError(t, err)

	_, err = svc.Update(ctx, actorID, deal.UpdateDealInput{
		ID:          created.ID,
		Title:       "Updated",
		Stage:       "WON",
		Probability: 90,
		Currency:    "EUR",
		Value:       created.Value,
	})
	require.NoError(t, err)

	logs, err := querier.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "DEAL",
		EntityID:   created.ID,
		Limit:      10,
	})
	require.NoError(t, err)
	var entry db.AuditLog
	found := false
	for _, logEntry := range logs {
		if logEntry.Action == "UPDATE" {
			entry = logEntry
			found = true
			break
		}
	}
	require.True(t, found)
	var oldValues, newValues map[string]any
	require.NoError(t, json.Unmarshal(entry.OldValues, &oldValues))
	require.NoError(t, json.Unmarshal(entry.NewValues, &newValues))
	require.Equal(t, "Original", oldValues["title"])
	require.Equal(t, "Updated", newValues["title"])
	var changes struct {
		ChangedKeys []string `json:"changed_keys"`
	}
	require.NoError(t, json.Unmarshal(entry.Changes, &changes))
	require.Contains(t, changes.ChangedKeys, "title")
	require.Contains(t, changes.ChangedKeys, "probability")
	require.NotContains(t, changes.ChangedKeys, "currency")
}

func TestDealService_StageAndRevokeAuditContainRealDiffs(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := deal.NewService(querier, audit.NewService(querier))
	actorID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	created, err := svc.Create(ctx, actorID, deal.CreateDealInput{Title: "Audit stages", Stage: "LEAD", Probability: 20})
	require.NoError(t, err)
	_, err = svc.UpdateStage(ctx, actorID, created.ID, "WON", 100)
	require.NoError(t, err)
	_, err = svc.Revoke(ctx, actorID, created.ID, time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC), "Revoked")
	require.NoError(t, err)

	logs, err := querier.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "DEAL",
		EntityID:   created.ID,
		Limit:      10,
	})
	require.NoError(t, err)
	actions := make(map[string]db.AuditLog)
	for _, logEntry := range logs {
		actions[logEntry.Action] = logEntry
	}
	for _, action := range []string{"UPDATE_STAGE", "REVOKE"} {
		entry, ok := actions[action]
		require.True(t, ok, "missing %s audit", action)
		var oldValues, newValues map[string]any
		require.NoError(t, json.Unmarshal(entry.OldValues, &oldValues))
		require.NoError(t, json.Unmarshal(entry.NewValues, &newValues))
		require.NotEmpty(t, oldValues)
		require.NotEmpty(t, newValues)
		require.Contains(t, newValues, "stage")
	}
}
