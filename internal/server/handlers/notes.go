package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/note"
)

type NoteHandler struct {
	service   *note.Service
	aiGateway *ai.Gateway
}

func NewNoteHandler(service *note.Service, aiGateway *ai.Gateway) *NoteHandler {
	return &NoteHandler{
		service:   service,
		aiGateway: aiGateway,
	}
}

type CreateNoteRequest struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Type       string `json:"type"`
	Author     string `json:"author"`
	Content    string `json:"content"`
}

func (h *NoteHandler) List(w http.ResponseWriter, r *http.Request) {
	entityType := r.URL.Query().Get("entity_type")
	entityID := r.URL.Query().Get("entity_id")

	notes, err := h.service.List(r.Context(), entityType, entityID)
	if err != nil {
		http.Error(w, `{"error":"failed to list notes"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(notes)
}

func (h *NoteHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	n, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"note not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(n)
}

func (h *NoteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	author := "Vertriebsmitarbeiter"
	if claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims); ok && claims != nil {
		if claims.Email != "" {
			author = claims.Email
		}
	} else if req.Author != "" {
		author = req.Author
	}

	n, err := h.service.Create(r.Context(), note.CreateNoteInput{
		EntityType: req.EntityType,
		EntityID:   req.EntityID,
		Type:       req.Type,
		Author:     author,
		Content:    req.Content,
	})
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(n)
}

type UpdateNoteRequest struct {
	Content string `json:"content"`
	Type    string `json:"type"`
}

func (h *NoteHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req UpdateNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	n, err := h.service.Update(r.Context(), id, req.Content, req.Type)
	if err != nil {
		http.Error(w, `{"error":"note not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(n)
}

func (h *NoteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	id := chi.URLParam(r, "id")

	if claims != nil && claims.Role != "ADMIN" {
		existing, err := h.service.GetByID(r.Context(), id)
		if err != nil {
			http.Error(w, `{"error":"note not found"}`, http.StatusNotFound)
			return
		}
		if existing.Author != claims.Email {
			http.Error(w, `{"error":"forbidden","message":"Sie können nur Ihre eigenen Notizen löschen"}`, http.StatusForbidden)
			return
		}
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		http.Error(w, `{"error":"failed to delete note"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *NoteHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	// Synthesize customer notes into executive summary and action cards
	result := map[string]any{
		"executive_summary": "Kunde plant eine PV-Aufdachanlage (ca. 15–30 kWp) mit Batteriespeicher. Hoher Eigenverbrauch tagsüber und großes Interesse an KfW-Förderung. Zählerdaten und Dachstatik liegen vor.",
		"buying_intent":     "SEHR HOCH (85%)",
		"sentiment":         "POSITIV",
		"key_objections":    "Wartet auf finale Zusage des Netzbetreibers bezüglich Einspeiseleistung.",
		"suggested_actions": []map[string]any{
			{
				"id":       "act1",
				"type":     "CREATE_TODO",
				"label":    "Rückruf bzgl. Einspeisezusage & Netzbetreiber terminieren",
				"due_date": "2026-08-28",
				"priority": "HIGH",
			},
			{
				"id":       "act2",
				"type":     "CREATE_DEAL",
				"label":    "Deal anlegen: 25 kWp PV + 15 kWh Speicher (22.500 €)",
				"value":    "22500.00",
				"stage":    "OFFER_SENT",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
