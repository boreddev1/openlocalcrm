package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/email"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/mailclient"
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

// EmailAccountResponse mirrors db.EmailAccount but never exposes the
// encrypted credential, only whether one is set.
type EmailAccountResponse struct {
	ID           pgtype.UUID `json:"id"`
	Name         string      `json:"name"`
	EmailAddress string      `json:"email_address"`
	Provider     string      `json:"provider"`
	ImapHost     string      `json:"imap_host"`
	ImapPort     int32       `json:"imap_port"`
	SmtpHost     string      `json:"smtp_host"`
	SmtpPort     int32       `json:"smtp_port"`
	Username     string      `json:"username"`
	HasPassword  bool        `json:"has_password"`
	IsActive     bool        `json:"is_active"`
	AccountType  string      `json:"account_type"`
	LastSyncAt   *time.Time  `json:"last_sync_at,omitempty"`
}

func toAccountResponse(acc db.EmailAccount) EmailAccountResponse {
	resp := EmailAccountResponse{
		ID:           acc.ID,
		Name:         acc.Name,
		EmailAddress: acc.EmailAddress,
		Provider:     acc.Provider,
		ImapHost:     acc.ImapHost.String,
		ImapPort:     acc.ImapPort.Int32,
		SmtpHost:     acc.SmtpHost.String,
		SmtpPort:     acc.SmtpPort.Int32,
		Username:     acc.Username.String,
		HasPassword:  acc.PasswordEncrypted.Valid && acc.PasswordEncrypted.String != "",
		IsActive:     acc.IsActive,
		AccountType:  acc.AccountType,
	}
	if acc.LastSyncAt.Valid {
		resp.LastSyncAt = &acc.LastSyncAt.Time
	}
	return resp
}

type EmailAccountRequest struct {
	Name         string `json:"name"`
	EmailAddress string `json:"email_address"`
	Provider     string `json:"provider"`
	ImapHost     string `json:"imap_host"`
	ImapPort     int32  `json:"imap_port"`
	SmtpHost     string `json:"smtp_host"`
	SmtpPort     int32  `json:"smtp_port"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	IsActive     bool   `json:"is_active"`
	AccountType  string `json:"account_type"`
}

func (h *EmailHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req EmailAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}
	if req.EmailAddress == "" {
		http.Error(w, `{"error":"email_address is required"}`, http.StatusBadRequest)
		return
	}

	acc, err := h.service.CreateAccount(r.Context(), email.AccountInput{
		Name:         req.Name,
		EmailAddress: req.EmailAddress,
		Provider:     req.Provider,
		ImapHost:     req.ImapHost,
		ImapPort:     req.ImapPort,
		SmtpHost:     req.SmtpHost,
		SmtpPort:     req.SmtpPort,
		Username:     req.Username,
		Password:     req.Password,
		AccountType:  req.AccountType,
	})
	if err != nil {
		http.Error(w, `{"error":"failed to create email account: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toAccountResponse(acc))
}

func (h *EmailHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.service.ListAccounts(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to list email accounts"}`, http.StatusInternalServerError)
		return
	}

	resp := make([]EmailAccountResponse, 0, len(accounts))
	for _, acc := range accounts {
		resp = append(resp, toAccountResponse(acc))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func parseAccountID(r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		return pgtype.UUID{}, false
	}
	return id, true
}

func (h *EmailHandler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAccountID(r)
	if !ok {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	var req EmailAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	acc, err := h.service.UpdateAccount(r.Context(), id, email.AccountInput{
		Name:     req.Name,
		ImapHost: req.ImapHost,
		ImapPort: req.ImapPort,
		SmtpHost: req.SmtpHost,
		SmtpPort: req.SmtpPort,
		Username: req.Username,
		Password: req.Password,
		IsActive: req.IsActive,
	})
	if err != nil {
		if err == email.ErrAccountNotFound {
			http.Error(w, `{"error":"email account not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to update email account: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toAccountResponse(acc))
}

func (h *EmailHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAccountID(r)
	if !ok {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteAccount(r.Context(), id); err != nil {
		http.Error(w, `{"error":"failed to delete email account"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *EmailHandler) TestAccountConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAccountID(r)
	if !ok {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	if err := h.service.TestConnection(r.Context(), id); err != nil {
		if err == email.ErrAccountNotFound {
			http.Error(w, `{"error":"email account not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "connection_failed", "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *EmailHandler) SyncAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := parseAccountID(r)
	if !ok {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	messages, err := h.service.SyncAccount(r.Context(), id)
	if err != nil {
		if err == email.ErrAccountNotFound {
			http.Error(w, `{"error":"email account not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "sync_failed", "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "new_messages": len(messages)})
}

type SendEmailRequest struct {
	AccountID string   `json:"account_id"`
	From      string   `json:"from"`
	To        []string `json:"to"`
	Cc        []string `json:"cc"`
	Subject   string   `json:"subject"`
	TextBody  string   `json:"body_text"`
	HTMLBody  string   `json:"body_html"`
	InReplyTo string   `json:"in_reply_to"`
}

func (h *EmailHandler) SendEmail(w http.ResponseWriter, r *http.Request) {
	var req SendEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}
	var accountID pgtype.UUID
	if err := accountID.Scan(req.AccountID); err != nil {
		http.Error(w, `{"error":"invalid account_id"}`, http.StatusBadRequest)
		return
	}
	if len(req.To) == 0 {
		http.Error(w, `{"error":"at least one recipient (to) is required"}`, http.StatusBadRequest)
		return
	}

	msg, err := h.service.SendMessage(r.Context(), accountID, mailclient.OutgoingMessage{
		From:      req.From,
		To:        req.To,
		Cc:        req.Cc,
		Subject:   req.Subject,
		TextBody:  req.TextBody,
		HTMLBody:  req.HTMLBody,
		InReplyTo: req.InReplyTo,
	})
	if err != nil {
		if err == email.ErrAccountNotFound {
			http.Error(w, `{"error":"email account not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "send_failed", "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(msg)
}

type TagMessageRequest struct {
	Tags []string `json:"tags"`
}

func (h *EmailHandler) TagMessage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	var req TagMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	msg, err := h.service.SetTags(r.Context(), id, req.Tags)
	if err != nil {
		if err == email.ErrMessageNotFound {
			http.Error(w, `{"error":"email message not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to update tags"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(msg)
}
