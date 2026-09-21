package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/core/note"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/stretchr/testify/require"
)

func TestNoteHandlerEndpoints(t *testing.T) {
	hub := sse.NewHub()
	svc := note.NewService(nil, hub)
	h := handlers.NewNoteHandler(svc, nil)

	r := chi.NewRouter()
	r.Get("/notes", h.List)
	r.Post("/notes", h.Create)
	r.Get("/notes/{id}", h.Get)
	r.Put("/notes/{id}", h.Update)
	r.Delete("/notes/{id}", h.Delete)

	// 1. List
	req := httptest.NewRequest(http.MethodGet, "/notes", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// 2. Create
	createPayload := []byte(`{"entity_type":"contact","entity_id":"c1","type":"CALL","author":"Max","content":"Test note"}`)
	req = httptest.NewRequest(http.MethodPost, "/notes", bytes.NewReader(createPayload))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rec.Code)
	}

	var created note.Note
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed decoding created note: %v", err)
	}

	// 3. Get
	req = httptest.NewRequest(http.MethodGet, "/notes/"+created.ID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// 4. Update
	updatePayload := []byte(`{"type":"MEETING","content":"Updated note content"}`)
	req = httptest.NewRequest(http.MethodPut, "/notes/"+created.ID, bytes.NewReader(updatePayload))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// 5. Delete
	req = httptest.NewRequest(http.MethodDelete, "/notes/"+created.ID, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestNoteHandlerSynthesize(t *testing.T) {
	hub := sse.NewHub()
	svc := note.NewService(nil, hub)
	_, err := svc.Create(context.Background(), note.CreateNoteInput{
		EntityType: "contact",
		EntityID:   "c1",
		Type:       "CALL",
		Author:     "Max",
		Content:    "Kunde interessiert sich für eine PV-Anlage mit Speicher.",
	})
	require.NoError(t, err)

	requestBody := []byte(`{"entity_type":"contact","entity_id":"c1"}`)

	t.Run("nil gateway returns 502", func(t *testing.T) {
		h := handlers.NewNoteHandler(svc, nil)
		req := httptest.NewRequest(http.MethodPost, "/ai/synthesize-notes", bytes.NewReader(requestBody))
		rec := httptest.NewRecorder()

		h.Synthesize(rec, req)
		require.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("gateway failure returns 502", func(t *testing.T) {
		h := handlers.NewNoteHandler(svc, newFailingAIGateway(t, http.StatusInternalServerError))
		req := httptest.NewRequest(http.MethodPost, "/ai/synthesize-notes", bytes.NewReader(requestBody))
		rec := httptest.NewRecorder()

		h.Synthesize(rec, req)
		require.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("invalid json body returns 400", func(t *testing.T) {
		h := handlers.NewNoteHandler(svc, newFakeAIGateway(t, "{}"))
		req := httptest.NewRequest(http.MethodPost, "/ai/synthesize-notes", bytes.NewReader([]byte("{bad-json")))
		rec := httptest.NewRecorder()

		h.Synthesize(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns real model analysis", func(t *testing.T) {
		analysis := `{"executive_summary":"Modell-Zusammenfassung","buying_intent":"MITTEL","sentiment":"NEUTRAL","key_objections":"Preis","suggested_actions":[]}`
		h := handlers.NewNoteHandler(svc, newFakeAIGateway(t, analysis))
		req := httptest.NewRequest(http.MethodPost, "/ai/synthesize-notes", bytes.NewReader(requestBody))
		rec := httptest.NewRecorder()

		h.Synthesize(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var res map[string]any
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&res))
		require.Equal(t, "Modell-Zusammenfassung", res["executive_summary"])
		require.Equal(t, "MITTEL", res["buying_intent"])
		require.NotContains(t, rec.Body.String(), "22.500")
		require.NotContains(t, rec.Body.String(), "85%")
	})

	t.Run("no matching notes returns 400", func(t *testing.T) {
		h := handlers.NewNoteHandler(svc, newFakeAIGateway(t, "{}"))
		req := httptest.NewRequest(http.MethodPost, "/ai/synthesize-notes", bytes.NewReader([]byte(`{"entity_type":"contact","entity_id":"unknown"}`)))
		rec := httptest.NewRecorder()

		h.Synthesize(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
