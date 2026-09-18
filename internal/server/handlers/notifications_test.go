package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/core/notification"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/stretchr/testify/require"
)

func TestNotificationHandler(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	hub := sse.NewHub()
	svc := notification.NewService(q, hub)
	h := handlers.NewNotificationHandler(svc)

	// Create test notification
	notif, err := svc.Create(context.Background(), notification.CreateNotificationInput{
		Type:    "SYSTEM",
		Title:   "System Update",
		Message: "Version 1.0 bereit",
	})
	require.NoError(t, err)

	t.Run("ListUnread", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
		rec := httptest.NewRecorder()

		h.ListUnread(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var items []db.Notification
		err := json.NewDecoder(rec.Body).Decode(&items)
		require.NoError(t, err)
		require.NotEmpty(t, items)
	})

	t.Run("MarkRead invalid uuid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/notifications/invalid-id/read", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "not-a-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()

		h.MarkRead(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("MarkRead valid uuid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/notifications/test/read", nil)
		rctx := chi.NewRouteContext()
		idStr := uuid.UUID(notif.ID.Bytes).String()
		rctx.URLParams.Add("id", idStr)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()

		h.MarkRead(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("MarkAllRead", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/notifications/read-all", nil)
		rec := httptest.NewRecorder()

		h.MarkAllRead(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})
}
