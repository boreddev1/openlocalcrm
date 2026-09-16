package email_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/email"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
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
