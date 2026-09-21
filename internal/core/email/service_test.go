package email_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/email"
	"github.com/openlocalcrm/openlocalcrm/internal/crypto"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

// MockQuerier implements db.Querier for testing email ingestion
type MockEmailQuerier struct {
	db.Querier
	createdMessages []db.CreateEmailMessageParams
}

func (m *MockEmailQuerier) CreateEmailMessage(ctx context.Context, arg db.CreateEmailMessageParams) (db.EmailMessage, error) {
	m.createdMessages = append(m.createdMessages, arg)
	var id pgtype.UUID
	_ = id.Scan("22222222-2222-2222-2222-222222222222")
	return db.EmailMessage{
		ID:          id,
		Subject:     arg.Subject,
		SenderEmail: arg.SenderEmail,
		ReceivedAt:  arg.ReceivedAt,
	}, nil
}

func (m *MockEmailQuerier) SearchContacts(ctx context.Context, arg db.SearchContactsParams) ([]db.Contact, error) {
	return []db.Contact{}, nil
}

func TestIngestEmailMessage(t *testing.T) {
	mockDB := &MockEmailQuerier{}
	hub := sse.NewHub()
	svc := email.NewService(mockDB, nil, nil, hub)

	var accountID pgtype.UUID
	_ = accountID.Scan("11111111-1111-1111-1111-111111111111")

	msg, err := svc.IngestMessage(context.Background(), email.IngestEmailInput{
		AccountID:       accountID,
		MessageID:       "<msg-001@client.de>",
		SenderEmail:     "klaus.meier@energie-kunden.de",
		SenderName:      "Klaus Meier",
		RecipientEmails: []string{"vertrieb@openlocalcrm.local"},
		Subject:         "Anfrage PV-Anlage 15 kWp",
		BodyText:        "Guten Tag, wir interessieren uns für eine Solaranlage.",
		ReceivedAt:      time.Now(),
	})

	if err != nil {
		t.Fatalf("expected successful ingestion, got: %v", err)
	}

	if msg.Subject != "Anfrage PV-Anlage 15 kWp" {
		t.Fatalf("expected subject mismatch, got %s", msg.Subject)
	}

	if len(mockDB.createdMessages) != 1 {
		t.Fatalf("expected 1 message stored in database")
	}
}

func TestCreateAccount_EncryptsPasswordAtRest(t *testing.T) {
	crypto.SetMasterKeyForTest([]byte("0123456789abcdef0123456789abcdef"))
	t.Cleanup(func() { crypto.SetMasterKeyForTest(nil) })

	q := demo.NewEmptyInMemoryQuerier()
	svc := email.NewService(q, nil, nil, nil)

	acc, err := svc.CreateAccount(context.Background(), email.AccountInput{
		Name:         "Vertrieb Postfach",
		EmailAddress: "vertrieb@openlocalcrm.local",
		Provider:     "IMAP",
		ImapHost:     "imap.example.com",
		ImapPort:     993,
		SmtpHost:     "smtp.example.com",
		SmtpPort:     587,
		Username:     "vertrieb@openlocalcrm.local",
		Password:     "hunter2-super-secret",
		AccountType:  "team",
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	if acc.PasswordEncrypted.String == "hunter2-super-secret" {
		t.Fatal("password must not be stored in plaintext")
	}
	if !strings.Contains(acc.PasswordEncrypted.String, "") || acc.PasswordEncrypted.String == "" {
		t.Fatal("expected an encrypted password to be stored")
	}

	decrypted, err := crypto.DecryptSecret(acc.PasswordEncrypted.String)
	if err != nil {
		t.Fatalf("failed to decrypt stored password: %v", err)
	}
	if decrypted != "hunter2-super-secret" {
		t.Fatalf("decrypted password = %q, want hunter2-super-secret", decrypted)
	}
}

func TestUpdateAccount_NotFound(t *testing.T) {
	crypto.SetMasterKeyForTest([]byte("0123456789abcdef0123456789abcdef"))
	t.Cleanup(func() { crypto.SetMasterKeyForTest(nil) })

	q := demo.NewEmptyInMemoryQuerier()
	svc := email.NewService(q, nil, nil, nil)

	var missing pgtype.UUID
	_ = missing.Scan("99999999-9999-9999-9999-999999999999")

	if _, err := svc.UpdateAccount(context.Background(), missing, email.AccountInput{Name: "x"}); err != email.ErrAccountNotFound {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestUpdateAndDeleteAccount(t *testing.T) {
	crypto.SetMasterKeyForTest([]byte("0123456789abcdef0123456789abcdef"))
	t.Cleanup(func() { crypto.SetMasterKeyForTest(nil) })

	q := demo.NewEmptyInMemoryQuerier()
	svc := email.NewService(q, nil, nil, nil)

	acc, err := svc.CreateAccount(context.Background(), email.AccountInput{
		Name:         "Postfach",
		EmailAddress: "postfach@openlocalcrm.local",
		Provider:     "IMAP",
		ImapHost:     "imap.example.com",
		ImapPort:     993,
		SmtpHost:     "smtp.example.com",
		SmtpPort:     587,
		Username:     "postfach@openlocalcrm.local",
		Password:     "initial-pw",
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	updated, err := svc.UpdateAccount(context.Background(), acc.ID, email.AccountInput{
		Name:     "Postfach (umbenannt)",
		ImapHost: "imap2.example.com",
		ImapPort: 993,
		SmtpHost: "smtp2.example.com",
		SmtpPort: 587,
		Username: "postfach@openlocalcrm.local",
		Password: "rotated-pw",
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("UpdateAccount failed: %v", err)
	}
	if updated.Name != "Postfach (umbenannt)" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}
	decrypted, err := crypto.DecryptSecret(updated.PasswordEncrypted.String)
	if err != nil || decrypted != "rotated-pw" {
		t.Fatalf("expected rotated password to be persisted encrypted, got decrypted=%q err=%v", decrypted, err)
	}

	if err := svc.DeleteAccount(context.Background(), acc.ID); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}
	if _, err := svc.GetAccount(context.Background(), acc.ID); err == nil {
		t.Fatal("expected account to be gone after delete")
	}
}

func TestListAccounts_DoesNotLeakPlaintextPasswords(t *testing.T) {
	crypto.SetMasterKeyForTest([]byte("0123456789abcdef0123456789abcdef"))
	t.Cleanup(func() { crypto.SetMasterKeyForTest(nil) })

	q := demo.NewEmptyInMemoryQuerier()
	svc := email.NewService(q, nil, nil, nil)

	if _, err := svc.CreateAccount(context.Background(), email.AccountInput{
		Name:         "Postfach",
		EmailAddress: "postfach@openlocalcrm.local",
		Provider:     "IMAP",
		Username:     "postfach@openlocalcrm.local",
		Password:     "top-secret",
	}); err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	accounts, err := svc.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].PasswordEncrypted.String == "top-secret" {
		t.Fatal("ListAccounts must never return plaintext passwords")
	}
}

func TestGetAccount_ReturnsCreatedAccount(t *testing.T) {
	crypto.SetMasterKeyForTest([]byte("0123456789abcdef0123456789abcdef"))
	t.Cleanup(func() { crypto.SetMasterKeyForTest(nil) })

	q := demo.NewEmptyInMemoryQuerier()
	svc := email.NewService(q, nil, nil, nil)

	acc, err := svc.CreateAccount(context.Background(), email.AccountInput{
		Name:         "Postfach",
		EmailAddress: "postfach@openlocalcrm.local",
		Provider:     "IMAP",
		Username:     "postfach@openlocalcrm.local",
		Password:     "pw",
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	got, err := svc.GetAccount(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if got.EmailAddress != "postfach@openlocalcrm.local" {
		t.Fatalf("unexpected account returned: %+v", got)
	}
}

func TestTestConnection_HonestFailureOnUnreachableHost(t *testing.T) {
	crypto.SetMasterKeyForTest([]byte("0123456789abcdef0123456789abcdef"))
	t.Cleanup(func() { crypto.SetMasterKeyForTest(nil) })

	q := demo.NewEmptyInMemoryQuerier()
	svc := email.NewService(q, nil, nil, nil)

	acc, err := svc.CreateAccount(context.Background(), email.AccountInput{
		Name:         "Unreachable",
		EmailAddress: "unreachable@openlocalcrm.local",
		Provider:     "IMAP",
		ImapHost:     "127.0.0.1",
		ImapPort:     1, // nothing listens here
		SmtpHost:     "127.0.0.1",
		SmtpPort:     1,
		Username:     "unreachable@openlocalcrm.local",
		Password:     "does-not-matter",
	})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	if err := svc.TestConnection(context.Background(), acc.ID); err == nil {
		t.Fatal("expected TestConnection to fail honestly against an unreachable host, got nil error")
	}
}
