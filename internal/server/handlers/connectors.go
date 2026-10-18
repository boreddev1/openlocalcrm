package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/connectors"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
)

type ConnectorHandler struct {
	engine   *connectors.Engine
	auditSvc *audit.Service
}

func NewConnectorHandler(engine *connectors.Engine, auditSvc ...*audit.Service) *ConnectorHandler {
	h := &ConnectorHandler{engine: engine}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

func (h *ConnectorHandler) logAudit(ctx context.Context, action, details, ip, ua string) {
	if h.auditSvc == nil {
		return
	}
	var systemActor pgtype.UUID
	_ = systemActor.Scan("00000000-0000-0000-0000-000000000001")
	var entityID pgtype.UUID
	_ = h.auditSvc.Log(ctx, systemActor, "CONNECTOR", entityID, action, map[string]string{"details": details}, ip, ua)
}

func (h *ConnectorHandler) LeadIntake(w http.ResponseWriter, r *http.Request) {
	ip := auth.ClientIP(r)
	ua := r.UserAgent()

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		h.logAudit(r.Context(), "LEAD_INTAKE_FAILED", "missing_bearer_header", ip, ua)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Authorization: Bearer <token> header required")
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		h.logAudit(r.Context(), "LEAD_INTAKE_FAILED", "empty_token", ip, ua)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Authorization: Bearer <token> cannot be empty")
		return
	}

	var payload connectors.LeadIntakePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.logAudit(r.Context(), "LEAD_INTAKE_FAILED", "invalid_json", ip, ua)
		writeJSONError(w, http.StatusBadRequest, "invalid_json", "Ungültiger JSON-Payload")
		return
	}

	contact, err := h.engine.IngestLead(r.Context(), token, payload)
	if err != nil {
		if err == connectors.ErrUnauthorizedConnector {
			h.logAudit(r.Context(), "LEAD_INTAKE_UNAUTHORIZED", "invalid_token", ip, ua)
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Ungültiges Connector-Token")
			return
		}
		h.logAudit(r.Context(), "LEAD_INTAKE_FAILED", "intake_error", ip, ua)
		writeJSONError(w, http.StatusBadRequest, "lead_intake_failed", "Lead-Import fehlgeschlagen")
		return
	}

	h.logAudit(r.Context(), "LEAD_INTAKE_SUCCESS", "imported", ip, ua)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "success",
		"contact_id": contact.ID,
		"message":    "Lead erfolgreich importiert",
	})
}
