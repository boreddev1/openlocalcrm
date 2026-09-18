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
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

func TestContactHandler_Endpoints(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := contact.NewService(querier, auditService)
	h := handlers.NewContactHandler(svc)

	adminUUID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: adminUUID,
		Role:   "ADMIN",
	})

	// 1. Create contact
	createBody, _ := json.Marshal(handlers.CreateContactRequest{
		FirstName:   "Klaus",
		LastName:    "Schneider",
		Email:       "klaus.schneider@firma.de",
		Phone:       "+49 69 555123",
		AddressCity: "Frankfurt",
	})
	reqCreate := httptest.NewRequest("POST", "/api/v1/contacts", bytes.NewReader(createBody)).WithContext(ctx)
	recCreate := httptest.NewRecorder()
	h.Create(recCreate, reqCreate)

	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var created db.Contact
	_ = json.NewDecoder(recCreate.Body).Decode(&created)
	contactIDStr := uuid.UUID(created.ID.Bytes).String()

	// 2. List contacts
	reqList := httptest.NewRequest("GET", "/api/v1/contacts", nil).WithContext(ctx)
	recList := httptest.NewRecorder()
	h.List(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recList.Code)
	}

	// 3. Search contacts
	reqSearch := httptest.NewRequest("GET", "/api/v1/contacts?q=Schneider", nil).WithContext(ctx)
	recSearch := httptest.NewRecorder()
	h.List(recSearch, reqSearch)
	if recSearch.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on search, got %d", recSearch.Code)
	}

	// 4. Get contact by ID
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", contactIDStr)
	reqGet := httptest.NewRequest("GET", "/api/v1/contacts/"+contactIDStr, nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recGet := httptest.NewRecorder()
	h.Get(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on get, got %d", recGet.Code)
	}

	// 5. Update contact
	updateBody, _ := json.Marshal(handlers.CreateContactRequest{
		FirstName: "Klaus",
		LastName:  "Schneider-Müller",
		Email:     "k.schneider@firma.de",
	})
	reqUpdate := httptest.NewRequest("PUT", "/api/v1/contacts/"+contactIDStr, bytes.NewReader(updateBody)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recUpdate := httptest.NewRecorder()
	h.Update(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on update, got %d: %s", recUpdate.Code, recUpdate.Body.String())
	}

	// 6. Delete contact with Admin claims
	reqDelete := httptest.NewRequest("DELETE", "/api/v1/contacts/"+contactIDStr, nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recDelete := httptest.NewRecorder()
	h.Delete(recDelete, reqDelete)
	if recDelete.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on delete, got %d: %s", recDelete.Code, recDelete.Body.String())
	}
}
