package email

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
	"github.com/openlocalcrm/openlocalcrm/internal/storage"
)

var (
	ErrAccountNotFound = errors.New("email account not found")
	ErrMessageNotFound = errors.New("email message not found")
)

type Service struct {
	queries db.Querier
	audit   *audit.Service
	storage storage.StorageService
	sseHub  *sse.Hub
}

func NewService(queries db.Querier, audit *audit.Service, storage storage.StorageService, sseHub *sse.Hub) *Service {
	return &Service{
		queries: queries,
		audit:   audit,
		storage: storage,
		sseHub:  sseHub,
	}
}

func (s *Service) EnsureDefaultAccount(ctx context.Context) (pgtype.UUID, error) {
	accounts, err := s.queries.ListEmailAccounts(ctx)
	if err == nil && len(accounts) > 0 {
		return accounts[0].ID, nil
	}

	acc, err := s.queries.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name:              "Default Demo Inbox",
		EmailAddress:      "inbox@openlocalcrm.local",
		Provider:          "IMAP",
		ImapHost:          pgtype.Text{String: "localhost", Valid: true},
		ImapPort:          pgtype.Int4{Int32: 993, Valid: true},
		SmtpHost:          pgtype.Text{String: "localhost", Valid: true},
		SmtpPort:          pgtype.Int4{Int32: 587, Valid: true},
		Username:          pgtype.Text{String: "inbox@openlocalcrm.local", Valid: true},
		PasswordEncrypted: pgtype.Text{String: "", Valid: true},
		IsActive:          true,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return acc.ID, nil
}

type IngestEmailInput struct {
	AccountID       pgtype.UUID
	MessageID       string
	InReplyTo       string
	SenderEmail     string
	SenderName      string
	RecipientEmails []string
	Subject         string
	BodyText        string
	BodyHTML        string
	ReceivedAt      time.Time
}

// IngestMessage processes an incoming email message, associates it with a contact, and triggers live SSE notification
func (s *Service) IngestMessage(ctx context.Context, input IngestEmailInput) (db.EmailMessage, error) {
	if !input.AccountID.Valid {
		defaultAccID, err := s.EnsureDefaultAccount(ctx)
		if err == nil {
			input.AccountID = defaultAccID
		}
	}

	// Auto-generate thread ID if not replied to
	threadID := input.InReplyTo
	if threadID == "" {
		threadID = input.MessageID
	}

	recipientsJSON, _ := json.Marshal(input.RecipientEmails)

	// Try auto-matching to contact by sender email
	var matchedContactID pgtype.UUID
	contacts, err := s.queries.SearchContacts(ctx, db.SearchContactsParams{
		Column1: pgtype.Text{String: strings.TrimSpace(input.SenderEmail), Valid: true},
		Limit:   1,
	})
	if err == nil && len(contacts) > 0 {
		matchedContactID = contacts[0].ID
	}

	receivedAt := input.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	msg, err := s.queries.CreateEmailMessage(ctx, db.CreateEmailMessageParams{
		AccountID:       input.AccountID,
		ThreadID:        threadID,
		MessageID:       input.MessageID,
		InReplyTo:       pgtype.Text{String: input.InReplyTo, Valid: input.InReplyTo != ""},
		Direction:       "INBOUND",
		SenderEmail:     input.SenderEmail,
		SenderName:      pgtype.Text{String: input.SenderName, Valid: input.SenderName != ""},
		RecipientEmails: recipientsJSON,
		Subject:         input.Subject,
		BodyText:        pgtype.Text{String: input.BodyText, Valid: input.BodyText != ""},
		BodyHtml:        pgtype.Text{String: input.BodyHTML, Valid: input.BodyHTML != ""},
		ReceivedAt:      pgtype.Timestamptz{Time: receivedAt, Valid: true},
		IsRead:          false,
		ContactID:       matchedContactID,
	})

	if err != nil {
		return db.EmailMessage{}, err
	}

	// Broadcast live SSE event to all connected clients
	if s.sseHub != nil {
		s.sseHub.Broadcast(sse.Event{
			Type: "email_received",
			Data: map[string]any{
				"id":           msg.ID,
				"subject":      msg.Subject,
				"sender_email": msg.SenderEmail,
				"sender_name":  msg.SenderName.String,
				"received_at":  msg.ReceivedAt.Time,
				"contact_id":   msg.ContactID,
			},
		})
	}

	return msg, nil
}

func (s *Service) ListMessages(ctx context.Context, limit, offset int32) ([]db.EmailMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.queries.ListEmailMessages(ctx, db.ListEmailMessagesParams{
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Service) GetThread(ctx context.Context, threadID string) ([]db.EmailMessage, error) {
	return s.queries.ListEmailMessagesByThread(ctx, threadID)
}

func (s *Service) MarkAsRead(ctx context.Context, id pgtype.UUID) error {
	return s.queries.MarkEmailMessageRead(ctx, id)
}
