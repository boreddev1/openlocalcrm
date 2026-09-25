package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

func TestDealHandler_Endpoints(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := deal.NewService(querier, auditService)
	h := handlers.NewDealHandler(svc)

	adminUUID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: adminUUID,
		Role:   "ADMIN",
	})

	// 1. Create deal
	createBody, _ := json.Marshal(handlers.CreateDealRequest{
		Title:       "15 kWp PV Anlage Test",
		Stage:       "QUALIFIED",
		Currency:    "EUR",
		Probability: 40,
	})
	reqCreate := httptest.NewRequest("POST", "/api/v1/deals", bytes.NewReader(createBody)).WithContext(ctx)
	recCreate := httptest.NewRecorder()
	h.Create(recCreate, reqCreate)

	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var created db.Deal
	_ = json.NewDecoder(recCreate.Body).Decode(&created)
	dealIDStr := uuid.UUID(created.ID.Bytes).String()

	// 2. List deals
	reqList := httptest.NewRequest("GET", "/api/v1/deals", nil).WithContext(ctx)
	recList := httptest.NewRecorder()
	h.List(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recList.Code)
	}

	// 3. Get deal by ID
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", dealIDStr)
	reqGet := httptest.NewRequest("GET", "/api/v1/deals/"+dealIDStr, nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recGet := httptest.NewRecorder()
	h.Get(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recGet.Code)
	}

	// 4. Update deal
	updateBody, _ := json.Marshal(map[string]any{
		"title": "15 kWp PV Anlage Weber - Verhandelt",
		"stage": "NEGOTIATION",
	})
	reqUpdate := httptest.NewRequest("PUT", "/api/v1/deals/"+dealIDStr, bytes.NewReader(updateBody)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recUpdate := httptest.NewRecorder()
	h.Update(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", recUpdate.Code, recUpdate.Body.String())
	}

	// 5. Attach solar calculation
	solarBody, _ := json.Marshal(map[string]any{
		"kwp":          15.0,
		"storage_kwh":  10.0,
		"price_gross":  28500,
		"yearly_yield": 14500,
	})
	reqSolar := httptest.NewRequest("POST", "/api/v1/deals/"+dealIDStr+"/solar-calculation", bytes.NewReader(solarBody)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recSolar := httptest.NewRecorder()
	h.AttachSolarCalculation(recSolar, reqSolar)
	if recSolar.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on attach solar calc, got %d: %s", recSolar.Code, recSolar.Body.String())
	}

	// 6. Delete deal
	reqDelete := httptest.NewRequest("DELETE", "/api/v1/deals/"+dealIDStr, nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recDelete := httptest.NewRecorder()
	h.Delete(recDelete, reqDelete)
	if recDelete.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on delete, got %d: %s", recDelete.Code, recDelete.Body.String())
	}
}

type MockDealTrigger struct {
	calls []struct {
		TriggerType string
		TargetID    string
		TargetType  string
		TargetName  string
	}
}

func (m *MockDealTrigger) TriggerEvent(ctx context.Context, triggerType, targetID, targetType, targetName string) ([]automation.WorkflowRun, error) {
	m.calls = append(m.calls, struct {
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
	return nil, nil
}

func TestDealHandler_TriggersDealWonWorkflow(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := deal.NewService(querier, auditService)
	trigger := &MockDealTrigger{}
	h := handlers.NewDealHandler(svc, trigger)

	adminUUID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: adminUUID,
		Role:   "ADMIN",
	})

	// Create deal in QUALIFIED stage
	created, err := svc.Create(ctx, pgtype.UUID{Bytes: adminUUID, Valid: true}, deal.CreateDealInput{
		Title:       "Gewerbe PV 100 kWp",
		Stage:       "QUALIFIED",
		Currency:    "EUR",
		Probability: 40,
	})
	if err != nil {
		t.Fatalf("failed to create deal: %v", err)
	}
	dealIDStr := uuid.UUID(created.ID.Bytes).String()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", dealIDStr)

	// 1. Update stage to WON -> must trigger DEAL_WON
	updateBody, _ := json.Marshal(map[string]any{
		"stage": "WON",
	})
	reqUpdate := httptest.NewRequest("PUT", "/api/v1/deals/"+dealIDStr, bytes.NewReader(updateBody)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recUpdate := httptest.NewRecorder()
	h.Update(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", recUpdate.Code, recUpdate.Body.String())
	}

	if len(trigger.calls) != 1 {
		t.Fatalf("expected 1 trigger call on WON, got %d", len(trigger.calls))
	}
	if trigger.calls[0].TriggerType != "DEAL_WON" {
		t.Errorf("expected DEAL_WON, got %s", trigger.calls[0].TriggerType)
	}
	if trigger.calls[0].TargetType != "DEAL" {
		t.Errorf("expected DEAL, got %s", trigger.calls[0].TargetType)
	}
	if trigger.calls[0].TargetID != dealIDStr {
		t.Errorf("expected target ID %s, got %s", dealIDStr, trigger.calls[0].TargetID)
	}

	// 2. Updating title when stage is ALREADY WON -> must NOT re-trigger DEAL_WON
	updateBody2, _ := json.Marshal(map[string]any{
		"title": "Gewerbe PV 100 kWp - Final",
	})
	reqUpdate2 := httptest.NewRequest("PUT", "/api/v1/deals/"+dealIDStr, bytes.NewReader(updateBody2)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recUpdate2 := httptest.NewRecorder()
	h.Update(recUpdate2, reqUpdate2)
	if recUpdate2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recUpdate2.Code)
	}

	if len(trigger.calls) != 1 {
		t.Fatalf("expected still 1 trigger call (no duplicate on non-stage change), got %d", len(trigger.calls))
	}
}
