package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/core/emailtemplate"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// EmailTemplateHandler exposes admin-only template CRUD plus the real generator.
// It owns status codes and the JSON projection only; persistence and generation
// live in internal/core/emailtemplate.
type EmailTemplateHandler struct {
	service *emailtemplate.Service
}

func NewEmailTemplateHandler(service *emailtemplate.Service) *EmailTemplateHandler {
	return &EmailTemplateHandler{service: service}
}

// TemplateResponse is the safe projection of a persisted template. It is a
// distinct type so a future column on db.EmailTemplate is never serialized by
// accident.
type TemplateResponse struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Category  string `json:"category"`
	Locale    string `json:"locale"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func toTemplateResponse(t db.EmailTemplate) TemplateResponse {
	createdAt, updatedAt := emailtemplate.Stamp(t)
	return TemplateResponse{
		Key:       t.Key,
		Name:      t.Name,
		Subject:   t.Subject,
		Body:      t.Body,
		Category:  t.Category,
		Locale:    t.Locale,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

func (h *EmailTemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	templates, err := h.service.List(r.Context())
	if err != nil {
		writeTemplateFailure(w, err)
		return
	}
	if templates == nil {
		templates = []db.EmailTemplate{}
	}
	out := make([]TemplateResponse, 0, len(templates))
	for _, t := range templates {
		out = append(out, toTemplateResponse(t))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

type templateRequest struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Category string `json:"category"`
	Locale   string `json:"locale"`
}

func (h *EmailTemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req templateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTemplateBytes)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Der Anfragetext konnte nicht gelesen werden.")
		return
	}
	created, err := h.service.Create(r.Context(), req.input())
	if err != nil {
		writeTemplateFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toTemplateResponse(created))
}

func (h *EmailTemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chi.URLParam(r, "key"))
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Der Vorlagenschlüssel fehlt.")
		return
	}
	var req templateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTemplateBytes)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Der Anfragetext konnte nicht gelesen werden.")
		return
	}
	updated, err := h.service.Update(r.Context(), key, req.input())
	if err != nil {
		writeTemplateFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toTemplateResponse(updated))
}

func (h *EmailTemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chi.URLParam(r, "key"))
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Der Vorlagenschlüssel fehlt.")
		return
	}
	if err := h.service.Delete(r.Context(), key); err != nil {
		writeTemplateFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"key": key, "deleted": true})
}

type generateRequest struct {
	Prompt string `json:"prompt"`
}

// Generate returns a real model draft. It never persists anything: the client
// must save the draft through the ordinary create or update call, so a generated
// template can never appear "saved" without a row behind it.
func (h *EmailTemplateHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTemplateBytes)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Der Anfragetext konnte nicht gelesen werden.")
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Bitte beschreiben Sie die gewünschte Vorlage.")
		return
	}
	generated, err := h.service.Generate(r.Context(), req.Prompt)
	if err != nil {
		writeTemplateFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"generated": generated,
		"persisted": false,
	})
}

// maxTemplateBytes caps a single template body. A template is prose, not a file.
const maxTemplateBytes = 256 << 10

func (req templateRequest) input() emailtemplate.Input {
	return emailtemplate.Input{
		Key:      req.Key,
		Name:     req.Name,
		Subject:  req.Subject,
		Body:     req.Body,
		Category: req.Category,
		Locale:   req.Locale,
	}
}

// writeTemplateFailure maps a service error to a status. The 409 is only
// reachable when the store reported a real unique violation; the in-memory demo
// cannot, so demo mode answers with its generic 500 and the real message instead
// of claiming a conflict it never detected.
func writeTemplateFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, emailtemplate.ErrNoGateway):
		writeJSONError(w, http.StatusNotImplemented, "email_template_unavailable",
			"Die Vorlagenverwaltung benötigt einen Datenbankzugriff, der hier nicht verfügbar ist.")
	case errors.Is(err, emailtemplate.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "email_template_not_found", "Diese Vorlage existiert nicht.")
	case errors.Is(err, emailtemplate.ErrInvalidInput):
		writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, emailtemplate.ErrGenerationFailed):
		writeJSONError(w, http.StatusBadGateway, "template_generation_failed",
			"Die KI-Dienststelle konnte keine Vorlage liefern: "+err.Error())
	case db.IsUniqueViolation(err):
		writeJSONError(w, http.StatusConflict, "email_template_conflict",
			"Eine Vorlage mit diesem Schlüssel existiert bereits.")
	default:
		writeJSONError(w, http.StatusInternalServerError, "email_template_failed", err.Error())
	}
}
