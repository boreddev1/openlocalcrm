package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type UserHandler struct {
	queries db.Querier
}

func NewUserHandler(queries db.Querier) *UserHandler {
	return &UserHandler{
		queries: queries,
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

func (h *UserHandler) Invite(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil || claims.Role != "ADMIN" {
		http.Error(w, `{"error":"forbidden","message":"Nur Administratoren können Benutzer einladen"}`, http.StatusForbidden)
		return
	}

	var req InviteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request","message":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.FirstName == "" || req.LastName == "" {
		http.Error(w, `{"error":"validation_error","message":"email, first_name and last_name are required"}`, http.StatusBadRequest)
		return
	}

	role := strings.ToUpper(req.Role)
	if role != "ADMIN" && role != "BENUTZER" && role != "VERTRIEB" && role != "BACKOFFICE" {
		role = "BENUTZER"
	}

	// Generate random initial password
	rnd := make([]byte, 12)
	_, _ = rand.Read(rnd)
	initPass := hex.EncodeToString(rnd)
	hash, err := auth.HashPassword(initPass)
	if err != nil {
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
		return
	}

	u, err := h.queries.CreateUser(r.Context(), db.CreateUserParams{
		Email:        req.Email,
		PasswordHash: hash,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Role:         role,
		Status:       "INVITED",
	})
	if err != nil {
		http.Error(w, `{"error":"user_creation_failed","message":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toUserResponse(u))
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
