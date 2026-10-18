package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	"github.com/stretchr/testify/require"
)

var (
	testUserID  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	otherUserID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

func newOwnershipRouter(querier db.Querier, callerID uuid.UUID, callerRole string) http.Handler {
	contactSvc := contact.NewService(querier, audit.NewService(querier))
	contactH := handlers.NewContactHandler(contactSvc)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := &auth.AccessClaims{
				UserID: callerID,
				Email:  callerID.String() + "@example.de",
				Role:   callerRole,
			}
			ctx := context.WithValue(r.Context(), auth.UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Post("/api/v1/contacts", contactH.Create)
	r.Put("/api/v1/contacts/{id}", contactH.Update)
	r.Get("/api/v1/contacts/{id}", contactH.Get)
	return r
}

func performHTTPJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeContact(t *testing.T, rec *httptest.ResponseRecorder) db.Contact {
	t.Helper()
	var c db.Contact
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &c))
	return c
}

// TestCreateIgnoresAnOwnerInTheBody: the owner is the creator, full stop. A body
// field that sets the owner lets a caller assign a record to someone else.
func TestCreateIgnoresAnOwnerInTheBody(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	h := newOwnershipRouter(q, testUserID, "VERTRIEB")
	rec := performHTTPJSON(t, h, http.MethodPost, "/api/v1/contacts", map[string]any{
		"first_name":    "Max",
		"last_name":     "Mustermann",
		"owner_user_id": otherUserID.String(),
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	contact := decodeContact(t, rec)
	require.True(t, contact.OwnerUserID.Valid)
	require.Equal(t, testUserID.String(), uuid.UUID(contact.OwnerUserID.Bytes).String(),
		"the creator owns the record regardless of the request body")
}

// TestOnlyOwnerOrAdminMayReassign
func TestOnlyOwnerOrAdminMayReassign(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	ownerRouter := newOwnershipRouter(q, testUserID, "VERTRIEB")
	otherRouter := newOwnershipRouter(q, otherUserID, "VERTRIEB")

	// 1. Create a contact as testUserID
	rec := performHTTPJSON(t, ownerRouter, http.MethodPost, "/api/v1/contacts", map[string]any{
		"first_name": "Anna",
		"last_name":  "Schmidt",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	contact := decodeContact(t, rec)
	contactID := uuid.UUID(contact.ID.Bytes).String()

	// 2. Third party colleague (otherUserID) cannot reassign it away
	rec = performHTTPJSON(t, otherRouter, http.MethodPut, "/api/v1/contacts/"+contactID, map[string]any{
		"first_name":    "Anna",
		"last_name":     "Schmidt",
		"owner_user_id": otherUserID.String(),
	})
	require.Equal(t, http.StatusForbidden, rec.Code)

	// 3. The current owner may reassign it
	rec = performHTTPJSON(t, ownerRouter, http.MethodPut, "/api/v1/contacts/"+contactID, map[string]any{
		"first_name":    "Anna",
		"last_name":     "Schmidt",
		"owner_user_id": otherUserID.String(),
	})
	require.Equal(t, http.StatusOK, rec.Code)
}
