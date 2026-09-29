package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/notification"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type NotificationHandler struct {
	service *notification.Service
}

func NewNotificationHandler(service *notification.Service) *NotificationHandler {
	return &NotificationHandler{service: service}
}

func (h *NotificationHandler) ListUnread(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	uid := pgtype.UUID{Bytes: claims.UserID, Valid: true}
	actorID := &uid

	items, err := h.service.ListUnread(r.Context(), actorID, 20)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch notifications"}`, http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = make([]db.Notification, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	uid := pgtype.UUID{Bytes: claims.UserID, Valid: true}
	actorID := &uid

	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid notification id"}`, http.StatusBadRequest)
		return
	}

	if err := h.service.MarkAsRead(r.Context(), id, actorID); err != nil {
		if strings.Contains(err.Error(), "forbidden") {
			http.Error(w, `{"error":"forbidden","message":"Sie können nur Ihre eigenen Benachrichtigungen als gelesen markieren"}`, http.StatusForbidden)
			return
		}
		http.Error(w, `{"error":"failed to mark notification as read"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFrom(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}
	uid := pgtype.UUID{Bytes: claims.UserID, Valid: true}
	actorID := &uid

	if err := h.service.MarkAllAsRead(r.Context(), actorID); err != nil {
		http.Error(w, `{"error":"failed to mark all as read"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
