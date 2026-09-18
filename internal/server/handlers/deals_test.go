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
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
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
