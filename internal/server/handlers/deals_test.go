package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	"github.com/stretchr/testify/require"
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

func TestDealHandler_Create_RejectsWiderrufenStage(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := deal.NewService(querier, auditService)
	h := handlers.NewDealHandler(svc)

	adminUUID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: adminUUID,
		Role:   "ADMIN",
	})

	body, _ := json.Marshal(handlers.CreateDealRequest{
		Title: "Direct Widerruf Test",
		Stage: "WIDERRUFEN",
	})
	req := httptest.NewRequest("POST", "/api/v1/deals", bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "WIDERRUFEN")
}

func TestDealHandler_Update_RejectsWiderrufenStage(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := deal.NewService(querier, auditService)
	h := handlers.NewDealHandler(svc)

	adminUUID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: adminUUID,
		Role:   "ADMIN",
	})

	d, err := svc.Create(ctx, pgtype.UUID{Bytes: adminUUID, Valid: true}, deal.CreateDealInput{
		Title: "Direct Widerruf Update Test",
		Stage: "LEAD",
	})
	require.NoError(t, err)
	dealIDStr := uuid.UUID(d.ID.Bytes).String()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", dealIDStr)

	body, _ := json.Marshal(map[string]any{
		"stage": "WIDERRUFEN",
	})
	req := httptest.NewRequest("PUT", "/api/v1/deals/"+dealIDStr, bytes.NewReader(body)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "WIDERRUFEN")
}

func TestDealHandler_Revoke(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	dealService := deal.NewService(querier, auditService)
	dealH := handlers.NewDealHandler(dealService)

	adminUUID := uuid.New()
	benutzerUUID := uuid.New()

	// Seed deal
	d, err := dealService.Create(context.Background(), pgtype.UUID{Bytes: adminUUID, Valid: true}, deal.CreateDealInput{
		Title:       "PV Anlage Widerruf Test",
		Stage:       "WON",
		Probability: 100,
	})
	require.NoError(t, err)
	dealIDStr := uuid.UUID(d.ID.Bytes).String()

	r := chi.NewRouter()
	r.Route("/deals", func(dr chi.Router) {
		dr.With(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE")).Post("/{id}/revoke", dealH.Revoke)
	})

	revokeBody, _ := json.Marshal(handlers.RevokeDealRequest{
		WiderrufGrund: "Fristgerechter Widerruf nach Haustürgeschäft",
	})

	// 1. BENUTZER role receives 403 Forbidden
	reqBenutzer := httptest.NewRequest("POST", "/deals/"+dealIDStr+"/revoke", bytes.NewReader(revokeBody))
	reqBenutzer = reqBenutzer.WithContext(context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: benutzerUUID,
		Role:   "BENUTZER",
	}))
	recBenutzer := httptest.NewRecorder()
	r.ServeHTTP(recBenutzer, reqBenutzer)
	require.Equal(t, http.StatusForbidden, recBenutzer.Code)

	// 2. ADMIN role succeeds with 200 OK
	reqAdmin := httptest.NewRequest("POST", "/deals/"+dealIDStr+"/revoke", bytes.NewReader(revokeBody))
	reqAdmin = reqAdmin.WithContext(context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: adminUUID,
		Role:   "ADMIN",
	}))
	recAdmin := httptest.NewRecorder()
	r.ServeHTTP(recAdmin, reqAdmin)
	require.Equal(t, http.StatusOK, recAdmin.Code)

	var revoked db.Deal
	_ = json.NewDecoder(recAdmin.Body).Decode(&revoked)
	require.Equal(t, "WIDERRUFEN", revoked.Stage)
	require.Equal(t, int32(0), revoked.Probability)
	require.True(t, revoked.WiderrufenAt.Valid)
}

type failingRevokeQuerier struct {
	db.Querier
	deal db.Deal
}

func (q *failingRevokeQuerier) GetDealByID(context.Context, pgtype.UUID) (db.Deal, error) {
	return q.deal, nil
}

func (q *failingRevokeQuerier) RevokeDeal(context.Context, db.RevokeDealParams) (db.Deal, error) {
	return db.Deal{}, errors.New("database secret: connection reset")
}

func TestDealHandler_RevokeErrorSemantics(t *testing.T) {
	claims := &auth.AccessClaims{UserID: uuid.New(), Role: "ADMIN"}
	t.Run("missing", func(t *testing.T) {
		querier := demo.NewInMemoryQuerier()
		h := handlers.NewDealHandler(deal.NewService(querier, nil))
		id := uuid.New().String()
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", id)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/deals/"+id+"/revoke", bytes.NewBufferString(`{}`))
		ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, auth.UserContextKey, claims)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		h.Revoke(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.NotContains(t, rec.Body.String(), "record not found")
	})

	t.Run("unexpected", func(t *testing.T) {
		id := uuid.New()
		querier := &failingRevokeQuerier{deal: db.Deal{ID: pgtype.UUID{Bytes: id, Valid: true}, Stage: "WON"}}
		h := handlers.NewDealHandler(deal.NewService(querier, nil))
		idString := id.String()
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", idString)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/deals/"+idString+"/revoke", bytes.NewBufferString(`{}`))
		ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, auth.UserContextKey, claims)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		h.Revoke(rec, req)
		require.Equal(t, http.StatusInternalServerError, rec.Code)
		require.NotContains(t, rec.Body.String(), "database secret")
		require.NotContains(t, rec.Body.String(), "connection reset")
	})
}

func TestDealHandler_RevokedDealsReturnConflict(t *testing.T) {
	for _, operation := range []string{"update", "attach", "delete"} {
		t.Run(operation, func(t *testing.T) {
			querier := demo.NewInMemoryQuerier()
			svc := deal.NewService(querier, audit.NewService(querier))
			h := handlers.NewDealHandler(svc)
			adminID := uuid.New()
			created, err := svc.Create(context.Background(), pgtype.UUID{Bytes: adminID, Valid: true}, deal.CreateDealInput{
				Title:       "Terminal handler test",
				Stage:       "WON",
				Probability: 100,
			})
			require.NoError(t, err)
			_, err = svc.Revoke(context.Background(), pgtype.UUID{Bytes: adminID, Valid: true}, created.ID, time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC), "Original")
			require.NoError(t, err)

			id := uuid.UUID(created.ID.Bytes).String()
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", id)
			ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, auth.UserContextKey, &auth.AccessClaims{UserID: adminID, Role: "ADMIN"})
			var req *http.Request
			switch operation {
			case "update":
				req = httptest.NewRequest(http.MethodPut, "/api/v1/deals/"+id, bytes.NewBufferString(`{"stage":"LEAD"}`)).WithContext(ctx)
			case "attach":
				req = httptest.NewRequest(http.MethodPost, "/api/v1/deals/"+id+"/solar-calculation", bytes.NewBufferString(`{}`)).WithContext(ctx)
			case "delete":
				req = httptest.NewRequest(http.MethodDelete, "/api/v1/deals/"+id, nil).WithContext(ctx)
			}
			rec := httptest.NewRecorder()
			switch operation {
			case "update":
				h.Update(rec, req)
			case "attach":
				h.AttachSolarCalculation(rec, req)
			case "delete":
				h.Delete(rec, req)
			}
			require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		})
	}
}
