package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/backup"
	"github.com/openlocalcrm/openlocalcrm/internal/core/invite"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

func TestUserHandler_ListAndProtection(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	h := handlers.NewUserHandler(querier, invite.NewService(querier, nil))

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
	var issued struct {
		InviteURL string `json:"invite_url"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode invite response: %v (body: %s)", err, rec.Body.String())
	}
	if !strings.Contains(issued.InviteURL, "/accept-invite?token=") {
		t.Fatalf("expected a copyable /accept-invite link, got %q", issued.InviteURL)
	}
	if issued.ExpiresAt == "" {
		t.Fatalf("expected expires_at in the invite response, got %s", rec.Body.String())
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
	h := handlers.NewBackupHandler(backup.NewService(querier, nil, nil, querier))

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

	// 2. Drill as ADMIN in demo mode -> 501 Not Implemented. The demo has no
	// PostgreSQL pool, so a real scratch-schema round trip is impossible and
	// the endpoint must say so instead of reporting a healthy check.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/backup/drill", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec = httptest.NewRecorder()
	h.Drill(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for a demo-mode drill, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "passed") || strings.Contains(rec.Body.String(), "healthy") {
		t.Fatalf("demo drill must not report a successful result: %s", rec.Body.String())
	}

	// 2b. Restore in demo mode -> 501 as well
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/backup/restore",
		bytes.NewReader([]byte(`{"format":"openlocalcrm-backup","version":1,"tables":{}}`)))
	restoreReq = restoreReq.WithContext(context.WithValue(restoreReq.Context(), auth.UserContextKey, adminClaims))
	rec = httptest.NewRecorder()
	h.Restore(rec, restoreReq)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for a demo-mode restore, got %d (body: %s)", rec.Code, rec.Body.String())
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

func TestUserHandler_UpdateRoleAndStatusAuditing(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(querier)
	h := handlers.NewUserHandler(querier, invite.NewService(querier, nil)).WithAudit(auditSvc)

	adminUser, _ := querier.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	targetUser, _ := querier.GetUserByEmail(context.Background(), "vertrieb@openlocalcrm.local")
	adminClaims := &auth.AccessClaims{
		UserID: uuid.UUID(adminUser.ID.Bytes),
		Email:  "admin@openlocalcrm.local",
		Role:   "ADMIN",
	}

	targetUUID := uuid.UUID(targetUser.ID.Bytes)

	// 1. Update role
	rCtx := chi.NewRouteContext()
	rCtx.URLParams.Add("id", targetUUID.String())
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/"+targetUUID.String()+"/role", bytes.NewReader([]byte(`{"role":"BACKOFFICE"}`)))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rCtx))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for role update, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// 2. Update status
	rCtx2 := chi.NewRouteContext()
	rCtx2.URLParams.Add("id", targetUUID.String())
	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/users/"+targetUUID.String()+"/status", bytes.NewReader([]byte(`{"status":"DEACTIVATED"}`)))
	req2 = req2.WithContext(context.WithValue(req2.Context(), chi.RouteCtxKey, rCtx2))
	req2 = req2.WithContext(context.WithValue(req2.Context(), auth.UserContextKey, adminClaims))
	rec2 := httptest.NewRecorder()
	h.UpdateStatus(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for status update, got %d (body: %s)", rec2.Code, rec2.Body.String())
	}

	// 3. Verify audit logs
	logs, err := querier.ListAuditLogsAsc(context.Background())
	if err != nil {
		t.Fatalf("failed listing audit logs: %v", err)
	}

	var foundRoleAudit, foundStatusAudit bool
	for _, l := range logs {
		if l.Action == "USER_ROLE_CHANGED" {
			foundRoleAudit = true
		}
		if l.Action == "USER_STATUS_CHANGED" {
			foundStatusAudit = true
		}
	}

	if !foundRoleAudit {
		t.Errorf("expected USER_ROLE_CHANGED audit log")
	}
	if !foundStatusAudit {
		t.Errorf("expected USER_STATUS_CHANGED audit log")
	}

	// 4. Verify cryptographic hash chain holds
	valid, count, err := auditSvc.VerifyAuditChain(context.Background())
	if err != nil || !valid || count < 2 {
		t.Errorf("expected valid audit chain with at least 2 logs, valid=%v, count=%d, err=%v", valid, count, err)
	}
}
