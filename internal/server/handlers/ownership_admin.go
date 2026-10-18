package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type OwnershipAdminHandler struct {
	queries db.Querier
	audit   *audit.Service
}

func NewOwnershipAdminHandler(queries db.Querier, audit *audit.Service) *OwnershipAdminHandler {
	return &OwnershipAdminHandler{
		queries: queries,
		audit:   audit,
	}
}

type BulkAssignRequest struct {
	Entity      string `json:"entity"`
	OwnerUserID string `json:"owner_user_id"`
	Limit       int    `json:"limit"`
}

type BulkAssignResponse struct {
	Assigned int `json:"assigned"`
}

func (h *OwnershipAdminHandler) BulkAssign(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	if claims.Role != "ADMIN" {
		writeJSONError(w, http.StatusForbidden, "forbidden", "Nur Administratoren dürfen Datensätze massenweise zuweisen")
		return
	}

	var req BulkAssignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Ungültiger Anfragekörper")
		return
	}

	entity := strings.ToLower(strings.TrimSpace(req.Entity))
	if entity != "contacts" && entity != "companies" {
		writeJSONError(w, http.StatusBadRequest, "invalid_entity", "Entity muss 'contacts' oder 'companies' sein")
		return
	}

	ownerUUID, err := uuid.Parse(req.OwnerUserID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_owner_user_id", "Ungültige owner_user_id")
		return
	}
	ownerPG := pgtype.UUID{Bytes: ownerUUID, Valid: true}

	user, err := h.queries.GetUserByID(r.Context(), ownerPG)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "user_not_found", "Zielbenutzer existiert nicht")
		return
	}
	if user.Status != "ACTIVE" {
		writeJSONError(w, http.StatusBadRequest, "user_inactive", "Zielbenutzer ist inaktiv")
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}

	actorID := pgtype.UUID{Bytes: claims.UserID, Valid: true}
	assignedCount := 0

	switch entity {
	case "contacts":
		unowned, err := h.queries.ListUnownedContacts(r.Context())
		if err != nil {
			http.Error(w, `{"error":"failed to list unowned contacts"}`, http.StatusInternalServerError)
			return
		}
		for i, c := range unowned {
			if i >= limit {
				break
			}
			updated, err := h.queries.SetContactOwner(r.Context(), db.SetContactOwnerParams{
				ID:          c.ID,
				OwnerUserID: ownerPG,
			})
			if err == nil {
				assignedCount++
				if h.audit != nil {
					_ = h.audit.Log(r.Context(), actorID, "CONTACT", updated.ID, "BULK_ASSIGN_OWNER", updated, "", "")
				}
			}
		}
	case "companies":
		unowned, err := h.queries.ListUnownedCompanies(r.Context())
		if err != nil {
			http.Error(w, `{"error":"failed to list unowned companies"}`, http.StatusInternalServerError)
			return
		}
		for i, comp := range unowned {
			if i >= limit {
				break
			}
			updated, err := h.queries.SetCompanyOwner(r.Context(), db.SetCompanyOwnerParams{
				ID:          comp.ID,
				OwnerUserID: ownerPG,
			})
			if err == nil {
				assignedCount++
				if h.audit != nil {
					_ = h.audit.Log(r.Context(), actorID, "COMPANY", updated.ID, "BULK_ASSIGN_OWNER", updated, "", "")
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(BulkAssignResponse{Assigned: assignedCount})
}
