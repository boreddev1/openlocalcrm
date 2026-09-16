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
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

func TestUserHandler_ListAndProtection(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	h := handlers.NewUserHandler(querier)

	// 1. List Users
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var users []handlers.UserResponse
	if err := json.NewDecoder(rec.Body).Decode(&users); err != nil {
		t.Fatalf("failed decoding user list: %v", err)
	}
	if len(users) < 2 {
		t.Fatalf("expected at least 2 users, got %d", len(users))
	}

	// 2. Invite user as non-admin -> should fail
	benutzerClaims := &auth.AccessClaims{
		UserID: uuid.New(),
		Email:  "user@openlocalcrm.local",
		Role:   "BENUTZER",
	}
	invitePayload := []byte(`{"email":"new@openlocalcrm.local","first_name":"Hans","last_name":"Neu","role":"BENUTZER"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(invitePayload))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, benutzerClaims))
	rec = httptest.NewRecorder()
	h.Invite(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-admin invite, got %d", rec.Code)
	}

	// 3. Invite user as ADMIN -> should succeed
	adminUser, _ := querier.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	adminUUID := uuid.UUID(adminUser.ID.Bytes)
	adminClaims := &auth.AccessClaims{
		UserID: adminUUID,
		Email:  "admin@openlocalcrm.local",
		Role:   "ADMIN",
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users/invite", bytes.NewReader(invitePayload))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec = httptest.NewRecorder()
	h.Invite(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for admin invite, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// 4. Last Admin Protection: Try to demote the only admin
	rCtx := chi.NewRouteContext()
	rCtx.URLParams.Add("id", adminUUID.String())
	req = httptest.NewRequest(http.MethodPut, "/api/v1/users/"+adminUUID.String()+"/role", bytes.NewReader([]byte(`{"role":"BENUTZER"}`)))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rCtx))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec = httptest.NewRecorder()
	h.UpdateRole(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when demoting last admin, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestBackupHandler_DrillAndExport(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	h := handlers.NewBackupHandler(querier)

	adminClaims := &auth.AccessClaims{
		UserID: uuid.New(),
		Email:  "admin@openlocalcrm.local",
		Role:   "ADMIN",
	}
	benutzerClaims := &auth.AccessClaims{
		UserID: uuid.New(),
		Email:  "benutzer@openlocalcrm.local",
		Role:   "BENUTZER",
	}

	// 1. Drill as non-admin -> 403 Forbidden
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/drill", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, benutzerClaims))
	rec := httptest.NewRecorder()
	h.Drill(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin drill, got %d", rec.Code)
	}

	// 2. Drill as ADMIN -> 200 OK
	req = httptest.NewRequest(http.MethodPost, "/api/v1/backup/drill", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec = httptest.NewRecorder()
	h.Drill(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin drill, got %d", rec.Code)
	}
	var drill handlers.DrillResponse
	if err := json.NewDecoder(rec.Body).Decode(&drill); err != nil {
		t.Fatalf("failed decoding drill response: %v", err)
	}
	if drill.IntegrityCheck != "passed" || drill.UserCount < 2 {
		t.Fatalf("unexpected drill stats: %+v", drill)
	}

	// 3. Export as ADMIN -> 200 OK with json attachment
	req = httptest.NewRequest(http.MethodGet, "/api/v1/backup/export", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec = httptest.NewRecorder()
	h.Export(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin export, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Disposition") == "" {
		t.Fatalf("expected Content-Disposition header in export")
	}
}
