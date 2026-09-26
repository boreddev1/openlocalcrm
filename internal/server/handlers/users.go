package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/invite"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type UserHandler struct {
	queries db.Querier
	invites *invite.Service
}

// NewUserHandler builds the handler. invites may be nil, in which case
// /users/invite reports that the invite flow is unavailable instead of creating
// an INVITED user that could never be activated.
func NewUserHandler(queries db.Querier, invites *invite.Service) *UserHandler {
	return &UserHandler{
		queries: queries,
		invites: invites,
	}
}

type UserResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	TotpEnabled bool   `json:"totp_enabled"`
	LastLoginAt string `json:"last_login_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

func toUserResponse(u db.User) UserResponse {
	resp := UserResponse{
		ID:          uuid.UUID(u.ID.Bytes).String(),
		Email:       u.Email,
		FirstName:   u.FirstName,
		LastName:    u.LastName,
		Role:        u.Role,
		Status:      u.Status,
		TotpEnabled: u.TotpEnabled,
		CreatedAt:   u.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
	if u.LastLoginAt.Valid {
		resp.LastLoginAt = u.LastLoginAt.Time.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.queries.ListUsers(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to list users"}`, http.StatusInternalServerError)
		return
	}

	result := make([]UserResponse, len(users))
	for i, u := range users {
		result[i] = toUserResponse(u)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

type InviteUserRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
}

// InviteUserResponse is the one-time result of issuance. The link is shown once
// and is not recoverable afterwards, because only its SHA-256 hash is stored.
//
// There is deliberately no "email_sent" field: nothing in this codebase can
// deliver mail for an invite, so such a claim would be a lie the operator would
// act on.
type InviteUserResponse struct {
	User      UserResponse `json:"user"`
	InviteURL string       `json:"invite_url"`
	ExpiresAt string       `json:"expires_at"`
	// Delivery states plainly that the admin must hand the link over.
	Delivery string `json:"delivery"`
}

func (h *UserHandler) Invite(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Nur Administratoren können Benutzer einladen"}`, http.StatusForbidden)
		return
	}
	if h.invites == nil {
		writeJSONError(w, http.StatusNotImplemented, "invite_unavailable",
			"Die Einladungsfunktion benötigt einen Datenbankzugriff, der hier nicht verfügbar ist.")
		return
	}

	var req InviteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request","message":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	// The role is validated, not silently corrected: defaulting an unknown role
	// to BENUTZER would hand out an access level the admin never asked for
	// while reporting success.
	role := strings.ToUpper(strings.TrimSpace(req.Role))
	if role == "" {
		role = "BENUTZER"
	}
	if role != "ADMIN" && role != "BENUTZER" && role != "VERTRIEB" && role != "BACKOFFICE" {
		writeJSONError(w, http.StatusBadRequest, "invalid_role",
			"Rolle muss ADMIN, BENUTZER, VERTRIEB oder BACKOFFICE sein")
		return
	}

	var actorID pgtype.UUID
	_ = actorID.Scan(claims.UserID.String())

	ctx := invite.WithRequestMeta(r.Context(), auth.ClientIP(r), r.UserAgent())
	issued, err := h.invites.Issue(ctx, actorID, invite.CreateUserInput{
		Email:     req.Email,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Role:      role,
	}, inviteBaseURL(r))
	if err != nil {
		switch {
		case errors.Is(err, invite.ErrInvalidRequest):
			writeJSONError(w, http.StatusBadRequest, "validation_error",
				"email, first_name, last_name und eine gültige role sind erforderlich")
		case db.IsUniqueViolation(err):
			// Only reachable when the database actually raised SQLSTATE 23505, so
			// the conflict claim is backed by the store. The in-memory demo cannot
			// report one, so it falls through to the honest generic failure.
			writeJSONError(w, http.StatusConflict, "user_exists",
				"Für diese E-Mail-Adresse existiert bereits ein Benutzer.")
		default:
			writeJSONError(w, http.StatusInternalServerError, "invite_failed",
				"Die Einladung konnte nicht erstellt werden: "+err.Error())
		}
		return
	}

	invited, err := h.queries.GetUserByID(r.Context(), issued.Invite.UserID)
	if err != nil {
		// The invite really exists, so this is an internal inconsistency, not a
		// client error. Reporting it as a failure would be more honest than
		// returning a half-populated body.
		writeJSONError(w, http.StatusInternalServerError, "invite_user_unreadable",
			"Die Einladung wurde gespeichert, das Benutzerkonto konnte aber nicht geladen werden.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(InviteUserResponse{
		User:      toUserResponse(invited),
		InviteURL: issued.URL,
		ExpiresAt: issued.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		Delivery:  "link",
	})
}

type UpdateRoleRequest struct {
	Role string `json:"role"`
}

func (h *UserHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Nur Administratoren können Rollen ändern"}`, http.StatusForbidden)
		return
	}

	idStr := chi.URLParam(r, "id")
	targetUUID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}
	targetID := pgtype.UUID{Bytes: targetUUID, Valid: true}

	var req UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	newRole := strings.ToUpper(req.Role)
	if newRole != "ADMIN" && newRole != "BENUTZER" && newRole != "VERTRIEB" && newRole != "BACKOFFICE" {
		http.Error(w, `{"error":"invalid_role","message":"Rolle muss ADMIN, BENUTZER, VERTRIEB oder BACKOFFICE sein"}`, http.StatusBadRequest)
		return
	}

	targetUser, err := h.queries.GetUserByID(r.Context(), targetID)
	if err != nil {
		http.Error(w, `{"error":"user_not_found"}`, http.StatusNotFound)
		return
	}

	// Last Admin Protection
	if targetUser.Role == "ADMIN" && newRole != "ADMIN" {
		allUsers, _ := h.queries.ListUsers(r.Context())
		activeAdmins := 0
		for _, u := range allUsers {
			if u.Role == "ADMIN" && u.Status == "ACTIVE" && u.ID.Bytes != targetID.Bytes {
				activeAdmins++
			}
		}
		if activeAdmins == 0 {
			http.Error(w, `{"error":"last_admin_protection","message":"Der letzte aktive Administrator kann nicht herabgestuft werden"}`, http.StatusBadRequest)
			return
		}
	}

	err = h.queries.UpdateUserRole(r.Context(), db.UpdateUserRoleParams{
		ID:   targetID,
		Role: newRole,
	})
	if err != nil {
		http.Error(w, `{"error":"update_failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

func (h *UserHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Nur Administratoren können Status ändern"}`, http.StatusForbidden)
		return
	}

	idStr := chi.URLParam(r, "id")
	targetUUID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}
	targetID := pgtype.UUID{Bytes: targetUUID, Valid: true}

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	newStatus := strings.ToUpper(req.Status)
	if newStatus != "ACTIVE" && newStatus != "DEACTIVATED" && newStatus != "INVITED" {
		http.Error(w, `{"error":"invalid_status","message":"Status muss ACTIVE oder DEACTIVATED sein"}`, http.StatusBadRequest)
		return
	}

	targetUser, err := h.queries.GetUserByID(r.Context(), targetID)
	if err != nil {
		http.Error(w, `{"error":"user_not_found"}`, http.StatusNotFound)
		return
	}

	// Last Admin Protection
	if targetUser.Role == "ADMIN" && newStatus == "DEACTIVATED" {
		allUsers, _ := h.queries.ListUsers(r.Context())
		activeAdmins := 0
		for _, u := range allUsers {
			if u.Role == "ADMIN" && u.Status == "ACTIVE" && u.ID.Bytes != targetID.Bytes {
				activeAdmins++
			}
		}
		if activeAdmins == 0 {
			http.Error(w, `{"error":"last_admin_protection","message":"Der letzte aktive Administrator kann nicht deaktiviert werden"}`, http.StatusBadRequest)
			return
		}
	}

	err = h.queries.UpdateUserStatus(r.Context(), db.UpdateUserStatusParams{
		ID:     targetID,
		Status: newStatus,
	})
	if err != nil {
		http.Error(w, `{"error":"update_failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
