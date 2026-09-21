package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/core/email"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type EmailHandler struct {
	service  *email.Service
	demoMode bool
}

func NewEmailHandler(service *email.Service, demoMode ...bool) *EmailHandler {
	dm := os.Getenv("DEMO_MODE") == "true" || os.Getenv("DEMO_MODE") == "1"
	if len(demoMode) > 0 {
		dm = demoMode[0]
	}
	return &EmailHandler{service: service, demoMode: dm}
}

func (h *EmailHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	messages, err := h.service.ListMessages(r.Context(), int32(limit), int32(offset))
	if err != nil {
		http.Error(w, `{"error":"failed to list email messages"}`, http.StatusInternalServerError)
		return
	}
	if messages == nil {
		messages = make([]db.EmailMessage, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(messages)
}

func (h *EmailHandler) GetThread(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "threadID")
	if threadID == "" {
		http.Error(w, `{"error":"missing thread_id"}`, http.StatusBadRequest)
		return
	}

	messages, err := h.service.GetThread(r.Context(), threadID)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch thread"}`, http.StatusInternalServerError)
		return
	}
	if messages == nil {
		messages = make([]db.EmailMessage, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(messages)
}

type IngestDemoEmailRequest struct {
	SenderEmail string   `json:"sender_email"`
	SenderName  string   `json:"sender_name"`
	Subject     string   `json:"subject"`
	BodyText    string   `json:"body_text"`
	Recipients  []string `json:"recipients"`
}

func (h *EmailHandler) IngestDemo(w http.ResponseWriter, r *http.Request) {
	if !h.demoMode && os.Getenv("DEMO_MODE") != "true" && os.Getenv("DEMO_MODE") != "1" {
		http.Error(w, `{"error":"forbidden","message":"Demo-E-Mail-Ingest ist in der Produktionsumgebung deaktiviert"}`, http.StatusForbidden)
		return
	}

	var req IngestDemoEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	msg, err := h.service.IngestMessage(r.Context(), email.IngestEmailInput{
		MessageID:       "msg-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@inbox.local",
		SenderEmail:     req.SenderEmail,
		SenderName:      req.SenderName,
		RecipientEmails: req.Recipients,
		Subject:         req.Subject,
		BodyText:        req.BodyText,
		ReceivedAt:      time.Now(),
	})

	if err != nil {
		http.Error(w, `{"error":"failed to ingest email"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(msg)
}
