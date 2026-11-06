package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

type SessionHandler struct {
	Auth *auth.AuthService
	Hub  *sse.Hub
}

func NewSessionHandler(authSvc *auth.AuthService, hub *sse.Hub) *SessionHandler {
	return &SessionHandler{
		Auth: authSvc,
		Hub:  hub,
	}
}

func (h *SessionHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": "Nicht angemeldet"})
		return
	}
	userID := pgtype.UUID{Bytes: claims.UserID, Valid: true}
	if err := h.Auth.RevokeAllSessions(r.Context(), userID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "revoke_failed", "message": "Sitzungen konnten nicht widerrufen werden"})
		return
	}
	if h.Hub != nil {
		h.Hub.RevokeUser(claims.UserID)
	}
	ClearAuthCookies(w, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "Alle Sitzungen beendet"})
}
