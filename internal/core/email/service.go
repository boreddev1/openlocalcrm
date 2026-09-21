package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/crypto"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/mailclient"
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

// AccountInput describes the fields accepted when creating or updating an
// email account. Password is always plaintext on the way in and is
// encrypted via internal/crypto before it ever reaches the database.
type AccountInput struct {
	Name         string
	EmailAddress string
	Provider     string
	ImapHost     string
	ImapPort     int32
	SmtpHost     string
	SmtpPort     int32
	Username     string
	Password     string
	IsActive     bool
	AccountType  string
	OwnerUserID  pgtype.UUID
}

// CreateAccount creates a new email account with its password encrypted at
// rest.
func (s *Service) CreateAccount(ctx context.Context, input AccountInput) (db.EmailAccount, error) {
	encrypted, err := crypto.EncryptSecret(input.Password)
	if err != nil {
		return db.EmailAccount{}, fmt.Errorf("failed to encrypt password: %w", err)
	}

	accountType := input.AccountType
	if accountType == "" {
		accountType = "personal"
	}

	return s.queries.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name:              input.Name,
		EmailAddress:      input.EmailAddress,
		Provider:          input.Provider,
		ImapHost:          pgtype.Text{String: input.ImapHost, Valid: input.ImapHost != ""},
		ImapPort:          pgtype.Int4{Int32: input.ImapPort, Valid: input.ImapPort != 0},
		SmtpHost:          pgtype.Text{String: input.SmtpHost, Valid: input.SmtpHost != ""},
		SmtpPort:          pgtype.Int4{Int32: input.SmtpPort, Valid: input.SmtpPort != 0},
		Username:          pgtype.Text{String: input.Username, Valid: input.Username != ""},
		PasswordEncrypted: pgtype.Text{String: encrypted, Valid: encrypted != ""},
		IsActive:          true,
		AccountType:       accountType,
		OwnerUserID:       input.OwnerUserID,
	})
}

// ListAccounts returns every email account. Passwords remain encrypted;
// callers must never decrypt them for display.
func (s *Service) ListAccounts(ctx context.Context) ([]db.EmailAccount, error) {
	return s.queries.ListEmailAccounts(ctx)
}

// GetAccount returns a single email account by ID.
func (s *Service) GetAccount(ctx context.Context, id pgtype.UUID) (db.EmailAccount, error) {
	acc, err := s.queries.GetEmailAccountByID(ctx, id)
	if err != nil {
		return db.EmailAccount{}, ErrAccountNotFound
	}
	return acc, nil
}

// UpdateAccount updates an existing email account. If input.Password is
// non-empty, the stored credential is rotated; otherwise the existing
// encrypted password is kept.
func (s *Service) UpdateAccount(ctx context.Context, id pgtype.UUID, input AccountInput) (db.EmailAccount, error) {
	existing, err := s.queries.GetEmailAccountByID(ctx, id)
	if err != nil {
		return db.EmailAccount{}, ErrAccountNotFound
	}

	encrypted := existing.PasswordEncrypted
	if input.Password != "" {
		enc, err := crypto.EncryptSecret(input.Password)
		if err != nil {
			return db.EmailAccount{}, fmt.Errorf("failed to encrypt password: %w", err)
		}
		encrypted = pgtype.Text{String: enc, Valid: enc != ""}
	}

	return s.queries.UpdateEmailAccount(ctx, db.UpdateEmailAccountParams{
		ID:                id,
		Name:              input.Name,
		ImapHost:          pgtype.Text{String: input.ImapHost, Valid: input.ImapHost != ""},
		ImapPort:          pgtype.Int4{Int32: input.ImapPort, Valid: input.ImapPort != 0},
		SmtpHost:          pgtype.Text{String: input.SmtpHost, Valid: input.SmtpHost != ""},
		SmtpPort:          pgtype.Int4{Int32: input.SmtpPort, Valid: input.SmtpPort != 0},
		Username:          pgtype.Text{String: input.Username, Valid: input.Username != ""},
		PasswordEncrypted: encrypted,
		IsActive:          input.IsActive,
	})
}

// DeleteAccount permanently removes an email account.
func (s *Service) DeleteAccount(ctx context.Context, id pgtype.UUID) error {
	return s.queries.DeleteEmailAccount(ctx, id)
}

// TestConnection performs a live IMAP login and SMTP AUTH check against the
// account's configured servers, returning an honest error on any failure —
// never a fabricated success.
func (s *Service) TestConnection(ctx context.Context, id pgtype.UUID) error {
	acc, err := s.queries.GetEmailAccountByID(ctx, id)
	if err != nil {
		return ErrAccountNotFound
	}

	password, err := crypto.DecryptSecret(acc.PasswordEncrypted.String)
	if err != nil {
		return fmt.Errorf("failed to decrypt account password: %w", err)
	}

	if err := mailclient.TestLogin(mailclient.IMAPConfig{
		Host:        acc.ImapHost.String,
		Port:        int(acc.ImapPort.Int32),
		Username:    acc.Username.String,
		Password:    password,
		ImplicitTLS: acc.ImapPort.Int32 == 993,
	}); err != nil {
		return fmt.Errorf("imap connection test failed: %w", err)
	}

	if err := mailclient.TestAuth(mailclient.SMTPConfig{
		Host:        acc.SmtpHost.String,
		Port:        int(acc.SmtpPort.Int32),
		Username:    acc.Username.String,
		Password:    password,
		ImplicitTLS: acc.SmtpPort.Int32 == 465,
	}); err != nil {
		return fmt.Errorf("smtp connection test failed: %w", err)
	}

	return nil
}
