package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/core/note"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestNoteHandlerEndpoints(t *testing.T) {
	hub := sse.NewHub()
	svc := note.NewService(hub)
	h := handlers.NewNoteHandler(svc, nil)

	r := chi.NewRouter()
	r.Get("/notes", h.List)
	r.Post("/notes", h.Create)
	r.Get("/notes/{id}", h.Get)
	r.Put("/notes/{id}", h.Update)
	r.Delete("/notes/{id}", h.Delete)
	r.Post("/ai/synthesize-notes", h.Synthesize)

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

	// 6. Synthesize
	req = httptest.NewRequest(http.MethodPost, "/ai/synthesize-notes", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
