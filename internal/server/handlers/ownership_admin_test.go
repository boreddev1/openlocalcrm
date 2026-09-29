package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/company"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

func TestBulkAssignOnlyTouchesUnownedRows(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	auditSvc := audit.NewService(q)
	contactSvc := contact.NewService(q, auditSvc)
	h := handlers.NewOwnershipAdminHandler(q, auditSvc)

	ctx := context.Background()

	createdAdmin, err := q.CreateUser(ctx, db.CreateUserParams{
		Email:        "admin@example.de",
		PasswordHash: "hash",
		FirstName:    "Admin",
		LastName:     "User",
		Role:         "ADMIN",
	})
	require.NoError(t, err)

	createdSales, err := q.CreateUser(ctx, db.CreateUserParams{
		Email:        "sales@example.de",
		PasswordHash: "hash",
		FirstName:    "Sales",
		LastName:     "User",
		Role:         "VERTRIEB",
	})
	require.NoError(t, err)

	adminID := uuid.UUID(createdAdmin.ID.Bytes)
	salesID := uuid.UUID(createdSales.ID.Bytes)
	otherUserID := uuid.New()

	adminClaims := &auth.AccessClaims{UserID: adminID, Email: "admin@example.de", Role: "ADMIN"}
	salesClaims := &auth.AccessClaims{UserID: salesID, Email: "sales@example.de", Role: "VERTRIEB"}

	// Seed 2 unowned contacts
	unowned1, err := contactSvc.Create(ctx, pgtype.UUID{}, contact.CreateContactInput{
		FirstName: "Unowned",
		LastName:  "One",
	})
	require.NoError(t, err)

	unowned2, err := contactSvc.Create(ctx, pgtype.UUID{}, contact.CreateContactInput{
		FirstName: "Unowned",
		LastName:  "Two",
	})
	require.NoError(t, err)

	// Seed 1 owned contact
	otherUserPG := pgtype.UUID{Bytes: otherUserID, Valid: true}
	ownedContact, err := contactSvc.Create(ctx, otherUserPG, contact.CreateContactInput{
		FirstName: "Owned",
		LastName:  "Three",
	})
	require.NoError(t, err)

	perform := func(claims *auth.AccessClaims, payload any) *httptest.ResponseRecorder {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/bulk-assign", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, claims))
		rec := httptest.NewRecorder()
		h.BulkAssign(rec, req)
		return rec
	}

	// 1. Non-admin is refused (403)
	rec := perform(salesClaims, map[string]any{
		"entity":        "contacts",
		"owner_user_id": salesID.String(),
	})
	require.Equal(t, http.StatusForbidden, rec.Code)

	// 2. Admin successfully bulk-assigns
	rec = perform(adminClaims, map[string]any{
		"entity":        "contacts",
		"owner_user_id": salesID.String(),
		"limit":         500,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Assigned int `json:"assigned"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 2, body.Assigned)

	// 3. Verify unowned contacts are now owned by salesID
	c1, err := q.GetContactByID(ctx, unowned1.ID)
	require.NoError(t, err)
	require.True(t, c1.OwnerUserID.Valid)
	require.Equal(t, salesID.String(), uuid.UUID(c1.OwnerUserID.Bytes).String())

	c2, err := q.GetContactByID(ctx, unowned2.ID)
	require.NoError(t, err)
	require.True(t, c2.OwnerUserID.Valid)
	require.Equal(t, salesID.String(), uuid.UUID(c2.OwnerUserID.Bytes).String())

	// 4. Verify the already-owned contact kept its original owner
	c3, err := q.GetContactByID(ctx, ownedContact.ID)
	require.NoError(t, err)
	require.True(t, c3.OwnerUserID.Valid)
	require.Equal(t, otherUserID.String(), uuid.UUID(c3.OwnerUserID.Bytes).String())
}

func TestBulkAssignCompanies(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	auditSvc := audit.NewService(q)
	compSvc := company.NewService(q, auditSvc)
	h := handlers.NewOwnershipAdminHandler(q, auditSvc)

	ctx := context.Background()

	createdAdmin, err := q.CreateUser(ctx, db.CreateUserParams{
		Email:        "admin@example.de",
		PasswordHash: "hash",
		FirstName:    "Admin",
		LastName:     "User",
		Role:         "ADMIN",
	})
	require.NoError(t, err)

	createdSales, err := q.CreateUser(ctx, db.CreateUserParams{
		Email:        "sales@example.de",
		PasswordHash: "hash",
		FirstName:    "Sales",
		LastName:     "User",
		Role:         "VERTRIEB",
	})
	require.NoError(t, err)

	adminID := uuid.UUID(createdAdmin.ID.Bytes)
	salesID := uuid.UUID(createdSales.ID.Bytes)
	otherUserID := uuid.New()

	adminClaims := &auth.AccessClaims{UserID: adminID, Email: "admin@example.de", Role: "ADMIN"}

	// Seed 2 unowned companies
	comp1, err := compSvc.Create(ctx, pgtype.UUID{}, company.CreateCompanyInput{Name: "Unowned Co 1"})
	require.NoError(t, err)
	comp2, err := compSvc.Create(ctx, pgtype.UUID{}, company.CreateCompanyInput{Name: "Unowned Co 2"})
	require.NoError(t, err)

	// Seed 1 owned company
	otherUserPG := pgtype.UUID{Bytes: otherUserID, Valid: true}
	ownedComp, err := compSvc.Create(ctx, otherUserPG, company.CreateCompanyInput{Name: "Owned Co 3"})
	require.NoError(t, err)

	b, _ := json.Marshal(map[string]any{
		"entity":        "companies",
		"owner_user_id": salesID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/bulk-assign", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec := httptest.NewRecorder()
	h.BulkAssign(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Assigned int `json:"assigned"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 2, body.Assigned)

	// Verify unowned companies got updated
	c1, err := q.GetCompanyByID(ctx, comp1.ID)
	require.NoError(t, err)
	require.True(t, c1.OwnerUserID.Valid)
	require.Equal(t, salesID.String(), uuid.UUID(c1.OwnerUserID.Bytes).String())

	c2, err := q.GetCompanyByID(ctx, comp2.ID)
	require.NoError(t, err)
	require.True(t, c2.OwnerUserID.Valid)
	require.Equal(t, salesID.String(), uuid.UUID(c2.OwnerUserID.Bytes).String())

	// Verify owned company was untouched
	c3, err := q.GetCompanyByID(ctx, ownedComp.ID)
	require.NoError(t, err)
	require.True(t, c3.OwnerUserID.Valid)
	require.Equal(t, otherUserID.String(), uuid.UUID(c3.OwnerUserID.Bytes).String())
}

func TestBulkAssignValidation(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	h := handlers.NewOwnershipAdminHandler(q, nil)

	adminID := uuid.New()
	adminClaims := &auth.AccessClaims{UserID: adminID, Email: "admin@example.de", Role: "ADMIN"}

	// 1. Invalid entity
	b, _ := json.Marshal(map[string]any{
		"entity":        "deals",
		"owner_user_id": adminID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/bulk-assign", bytes.NewReader(b))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec := httptest.NewRecorder()
	h.BulkAssign(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// 2. Non-existent owner_user_id
	b, _ = json.Marshal(map[string]any{
		"entity":        "contacts",
		"owner_user_id": uuid.New().String(),
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/ownership/bulk-assign", bytes.NewReader(b))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminClaims))
	rec = httptest.NewRecorder()
	h.BulkAssign(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
