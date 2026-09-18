package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

func TestBackupHandler_Drill(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	h := handlers.NewBackupHandler(q)

	t.Run("forbidden for non-admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/backup/drill", nil)
		rec := httptest.NewRecorder()
		h.Drill(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("success for admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/backup/drill", nil)
		ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AccessClaims{
			Role: "ADMIN",
		})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		h.Drill(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var res handlers.DrillResponse
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		require.Equal(t, "healthy", res.Status)
		require.Equal(t, "passed", res.IntegrityCheck)
	})
}

func TestBackupHandler_Export(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	h := handlers.NewBackupHandler(q)

	t.Run("forbidden without admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/backup/export", nil)
		ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AccessClaims{
			Role: "SALES",
		})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		h.Export(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("success for admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/backup/export", nil)
		ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AccessClaims{
			Role: "ADMIN",
		})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		h.Export(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var payload handlers.ExportPayload
		err := json.NewDecoder(rec.Body).Decode(&payload)
		require.NoError(t, err)
		require.NotEmpty(t, payload.ExportedAt)
	})
}
