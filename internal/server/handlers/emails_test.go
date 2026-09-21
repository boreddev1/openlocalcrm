package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/core/email"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/stretchr/testify/require"
)

func TestEmailHandler(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	hub := sse.NewHub()
	svc := email.NewService(q, nil, nil, hub)
	h := handlers.NewEmailHandler(svc, true)

	t.Run("IngestDemo valid", func(t *testing.T) {
		reqBody := handlers.IngestDemoEmailRequest{
			SenderEmail: "kunde@solar-example.de",
			SenderName:  "Herr Solar",
			Subject:     "Anfrage zu 10 kWp Anlage",
			BodyText:    "Bitte um ein unverbindliches Angebot für unser Einfamilienhaus.",
			Recipients:  []string{"inbox@openlocalcrm.local"},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/emails/demo-ingest", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.IngestDemo(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var msg db.EmailMessage
		err := json.NewDecoder(rec.Body).Decode(&msg)
		require.NoError(t, err)
		require.Equal(t, reqBody.Subject, msg.Subject)
	})

	t.Run("IngestDemo invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/emails/demo-ingest", bytes.NewReader([]byte("{invalid-json")))
		rec := httptest.NewRecorder()

		h.IngestDemo(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("IngestDemo forbidden in production mode", func(t *testing.T) {
		hProd := handlers.NewEmailHandler(svc, false)
		req := httptest.NewRequest(http.MethodPost, "/api/emails/demo-ingest", bytes.NewReader([]byte("{}")))
		rec := httptest.NewRecorder()

		hProd.IngestDemo(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("ListMessages", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/emails?limit=10&offset=0", nil)
		rec := httptest.NewRecorder()

		h.ListMessages(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var messages []db.EmailMessage
		err := json.NewDecoder(rec.Body).Decode(&messages)
		require.NoError(t, err)
		require.NotEmpty(t, messages)
	})

	t.Run("GetThread missing param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/emails/threads/", nil)
		rec := httptest.NewRecorder()

		h.GetThread(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("GetThread with threadID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/emails/threads/test-thread", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("threadID", "test-thread")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()

		h.GetThread(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})
}
