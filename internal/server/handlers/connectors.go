package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/openlocalcrm/openlocalcrm/internal/connectors"
)

type ConnectorHandler struct {
	engine *connectors.Engine
}

func NewConnectorHandler(engine *connectors.Engine) *ConnectorHandler {
	return &ConnectorHandler{engine: engine}
}

func (h *ConnectorHandler) LeadIntake(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, `{"error":"unauthorized","message":"Authorization: Bearer <token> header required"}`, http.StatusUnauthorized)
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		http.Error(w, `{"error":"unauthorized","message":"Authorization: Bearer <token> cannot be empty"}`, http.StatusUnauthorized)
		return
	}

	var payload connectors.LeadIntakePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json payload"}`, http.StatusBadRequest)
		return
	}

	contact, err := h.engine.IngestLead(r.Context(), token, payload)
	if err != nil {
		if err == connectors.ErrUnauthorizedConnector {
			http.Error(w, `{"error":"unauthorized connector token"}`, http.StatusUnauthorized)
			return
		}
		http.Error(w, `{"error":"failed to ingest lead: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "success",
		"contact_id": contact.ID,
		"message":    "Lead erfolgreich importiert",
	})
}
