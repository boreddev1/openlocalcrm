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

	// 1. Create contact with consent and energy data
	strom := 4200.0
	gas := 15000.0
	createBody, _ := json.Marshal(map[string]any{
		"first_name":         "Klaus",
		"last_name":          "Schneider",
		"email":              "klaus.schneider@firma.de",
		"phone":              "+49 69 555123",
		"address_city":       "Frankfurt",
		"consent_phone":      true,
		"consent_email":      true,
		"stromverbrauch_kwh": strom,
		"gasverbrauch_kwh":   gas,
		"zaehlernummer":      "1EMH123456",
		"eigentuemer_status": "EIGENTUEMER",
	})
	reqCreate := httptest.NewRequest("POST", "/api/v1/contacts", bytes.NewReader(createBody)).WithContext(ctx)
	recCreate := httptest.NewRecorder()
	h.Create(recCreate, reqCreate)

	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var created db.Contact
	_ = json.NewDecoder(recCreate.Body).Decode(&created)
	if !created.ConsentPhone || !created.ConsentEmail {
		t.Fatalf("expected consent flags true, got phone=%v, email=%v", created.ConsentPhone, created.ConsentEmail)
	}
	if !created.ConsentUpdatedAt.Valid {
		t.Fatalf("expected valid consent_updated_at")
	}
	if created.Zaehlernummer.String != "1EMH123456" {
		t.Fatalf("expected zaehlernummer 1EMH123456, got %s", created.Zaehlernummer.String)
	}
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
	updateBody, _ := json.Marshal(map[string]any{
		"first_name": "Klaus",
		"last_name":  "Schneider-Müller",
		"email":      "k.schneider@firma.de",
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

func TestContactHandler_UpdatePreservesOmittedConsentAndNullableValues(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	svc := contact.NewService(querier, audit.NewService(querier))
	h := handlers.NewContactHandler(svc)
	actorID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	strom := 4200.0
	gas := 12000.0
	created, err := svc.Create(context.Background(), actorID, contact.CreateContactInput{
		FirstName:         "Klaus",
		LastName:          "Schneider",
		Email:             "klaus@example.de",
		ConsentPhone:      contact.OptionalBool{Value: true, Present: true},
		ConsentEmail:      contact.OptionalBool{Value: true, Present: true},
		StromverbrauchKwh: contact.OptionalFloat64{Value: strom, Present: true},
		GasverbrauchKwh:   contact.OptionalFloat64{Value: gas, Present: true},
		EigentuemerStatus: contact.OptionalString{Value: "Eigentümer", Present: true},
	})
	if err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}

	id := uuid.UUID(created.ID.Bytes).String()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/contacts/"+id, bytes.NewBufferString(`{"first_name":"Klaus","last_name":"Schneider"}`))
	reqCtx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	reqCtx = context.WithValue(reqCtx, auth.UserContextKey, &auth.AccessClaims{
		UserID: uuid.UUID(actorID.Bytes),
		Role:   "ADMIN",
	})
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var updated db.Contact
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("failed to decode updated contact: %v", err)
	}
	if !updated.ConsentPhone || !updated.ConsentEmail {
		t.Errorf("omitted consent was changed: phone=%v email=%v", updated.ConsentPhone, updated.ConsentEmail)
	}
	if !updated.ConsentUpdatedAt.Valid || !updated.ConsentUpdatedAt.Time.Equal(created.ConsentUpdatedAt.Time) {
		t.Errorf("consent timestamp changed: got=%v want=%v", updated.ConsentUpdatedAt, created.ConsentUpdatedAt)
	}
	if !updated.StromverbrauchKwh.Valid || !updated.GasverbrauchKwh.Valid || !updated.EigentuemerStatus.Valid {
		t.Errorf("omitted nullable values were changed: %+v", updated)
	}
}

func TestContactHandler_UpdateClearsExplicitNulls(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	svc := contact.NewService(querier, audit.NewService(querier))
	h := handlers.NewContactHandler(svc)
	actorID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	strom := 4200.0
	created, err := svc.Create(context.Background(), actorID, contact.CreateContactInput{
		FirstName:         "Klaus",
		LastName:          "Schneider",
		StromverbrauchKwh: contact.OptionalFloat64{Value: strom, Present: true},
		GasverbrauchKwh:   contact.OptionalFloat64{Value: strom, Present: true},
		EigentuemerStatus: contact.OptionalString{Value: "Mieter", Present: true},
	})
	if err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}

	id := uuid.UUID(created.ID.Bytes).String()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	body := `{"first_name":"Klaus","last_name":"Schneider","stromverbrauch_kwh":null,"gasverbrauch_kwh":null,"eigentuemer_status":null}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/contacts/"+id, bytes.NewBufferString(body))
	reqCtx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	reqCtx = context.WithValue(reqCtx, auth.UserContextKey, &auth.AccessClaims{
		UserID: uuid.UUID(actorID.Bytes),
		Role:   "ADMIN",
	})
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var updated db.Contact
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("failed to decode updated contact: %v", err)
	}
	if updated.StromverbrauchKwh.Valid || updated.GasverbrauchKwh.Valid || updated.EigentuemerStatus.Valid {
		t.Errorf("explicit nulls did not clear values: %+v", updated)
	}
}

func TestContactHandler_UpdateRejectsNullConsent(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	svc := contact.NewService(querier, audit.NewService(querier))
	h := handlers.NewContactHandler(svc)
	actorID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	created, err := svc.Create(context.Background(), actorID, contact.CreateContactInput{
		FirstName: "Klaus",
		LastName:  "Schneider",
	})
	if err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}

	id := uuid.UUID(created.ID.Bytes).String()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	body := `{"first_name":"Klaus","last_name":"Schneider","consent_email":null}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/contacts/"+id, bytes.NewBufferString(body))
	reqCtx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	reqCtx = context.WithValue(reqCtx, auth.UserContextKey, &auth.AccessClaims{
		UserID: uuid.UUID(actorID.Bytes),
		Role:   "ADMIN",
	})
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestContactHandler_RejectsInvalidEnergy(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "fractional", body: `{"first_name":"Klaus","last_name":"Schneider","stromverbrauch_kwh":1.5}`},
		{name: "negative", body: `{"first_name":"Klaus","last_name":"Schneider","gasverbrauch_kwh":-1}`},
		{name: "above range", body: `{"first_name":"Klaus","last_name":"Schneider","stromverbrauch_kwh":100000000}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			querier := demo.NewInMemoryQuerier()
			svc := contact.NewService(querier, audit.NewService(querier))
			h := handlers.NewContactHandler(svc)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", bytes.NewBufferString(tt.body))
			req = req.WithContext(context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
				UserID: uuid.New(),
				Role:   "ADMIN",
			}))
			rec := httptest.NewRecorder()
			h.Create(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
