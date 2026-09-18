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
	"github.com/openlocalcrm/openlocalcrm/internal/core/company"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

func TestCompanyHandler_Endpoints(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := company.NewService(querier, auditService)
	h := handlers.NewCompanyHandler(svc)

	adminUUID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: adminUUID,
		Role:   "ADMIN",
	})

	// 1. Create company
	createBody, _ := json.Marshal(handlers.CreateCompanyRequest{
		Name:          "Solarsysteme Rhein-Main GmbH",
		Domain:        "solarsysteme-rm.de",
		Phone:         "+49 69 778899",
		Email:         "kontakt@solarsysteme-rm.de",
		AddressStreet: "Mainzer Landstraße 100",
		AddressZip:    "60329",
		AddressCity:   "Frankfurt",
	})
	reqCreate := httptest.NewRequest("POST", "/api/v1/companies", bytes.NewReader(createBody)).WithContext(ctx)
	recCreate := httptest.NewRecorder()
	h.Create(recCreate, reqCreate)

	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var created db.Company
	_ = json.NewDecoder(recCreate.Body).Decode(&created)
	compIDStr := uuid.UUID(created.ID.Bytes).String()

	// 2. List companies
	reqList := httptest.NewRequest("GET", "/api/v1/companies", nil).WithContext(ctx)
	recList := httptest.NewRecorder()
	h.List(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recList.Code)
	}

	// 3. Get company by ID
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", compIDStr)
	reqGet := httptest.NewRequest("GET", "/api/v1/companies/"+compIDStr, nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recGet := httptest.NewRecorder()
	h.Get(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on get, got %d", recGet.Code)
	}

	// 4. Update company
	updateBody, _ := json.Marshal(handlers.CreateCompanyRequest{
		Name:  "Solarsysteme Rhein-Main Holding GmbH",
		Email: "info@solarsysteme-rm.de",
	})
	reqUpdate := httptest.NewRequest("PUT", "/api/v1/companies/"+compIDStr, bytes.NewReader(updateBody)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recUpdate := httptest.NewRecorder()
	h.Update(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on update, got %d: %s", recUpdate.Code, recUpdate.Body.String())
	}

	// 5. Delete company
	reqDelete := httptest.NewRequest("DELETE", "/api/v1/companies/"+compIDStr, nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recDelete := httptest.NewRecorder()
	h.Delete(recDelete, reqDelete)
	if recDelete.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on delete, got %d: %s", recDelete.Code, recDelete.Body.String())
	}
}
