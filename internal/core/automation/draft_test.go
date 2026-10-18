package automation_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) Generate(ctx context.Context, prompt, systemInstruction string) (string, error) {
	return m.response, m.err
}

func TestAIDrafter_DraftEmail_ContactSuccess(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	ctx := context.Background()

	// 1. Seed active email account
	acc, _ := q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name:         "Sales Inbox",
		EmailAddress: "vertrieb@openlocalcrm.local",
		IsActive:     true,
	})

	// 2. Seed contact with email
	c, _ := q.CreateContact(ctx, db.CreateContactParams{
		FirstName: "Sabine",
		LastName:  "Muster",
		Email:     pgtype.Text{String: "sabine@example.de", Valid: true},
	})
	contactIDStr := uuid.UUID(c.ID.Bytes).String()

	llm := &mockLLMClient{
		response: "Betreff: Erstkontakt Solaranlage\n\nGuten Tag Frau Muster,\nvielen Dank für Ihr Interesse an einer PV-Anlage.",
	}
	drafter := automation.NewAIDrafter(q, llm)

	err := drafter.DraftEmail(ctx, "Erstkontakt Photovoltaik", "CONTACT", contactIDStr, "Sabine Muster")
	if err != nil {
		t.Fatalf("DraftEmail failed: %v", err)
	}

	// Verify email message was persisted as draft
	msgs, _ := q.ListEmailMessages(ctx, db.ListEmailMessagesParams{Limit: 10, Offset: 0})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 email message created, got %d", len(msgs))
	}
	draft := msgs[0]
	if !draft.IsDraft {
		t.Errorf("expected draft.IsDraft == true")
	}
	if draft.IsRead {
		t.Errorf("expected draft.IsRead == false")
	}
	if draft.Direction != "OUTBOUND" {
		t.Errorf("expected direction OUTBOUND, got %s", draft.Direction)
	}
	if draft.Subject != "Erstkontakt Solaranlage" {
		t.Errorf("expected subject 'Erstkontakt Solaranlage', got %q", draft.Subject)
	}
	if !strings.Contains(draft.BodyText.String, "Frau Muster") {
		t.Errorf("expected body to contain 'Frau Muster', got %q", draft.BodyText.String)
	}
	if draft.AccountID != acc.ID {
		t.Errorf("expected account ID %v, got %v", acc.ID, draft.AccountID)
	}
}

func TestAIDrafter_DraftEmail_DealSuccess(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	ctx := context.Background()

	_, _ = q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name:         "Sales",
		EmailAddress: "vertrieb@openlocalcrm.local",
		IsActive:     true,
	})
	c, _ := q.CreateContact(ctx, db.CreateContactParams{
		FirstName: "Klaus",
		LastName:  "Meier",
		Email:     pgtype.Text{String: "klaus@example.de", Valid: true},
	})
	dl, _ := q.CreateDeal(ctx, db.CreateDealParams{
		Title:     "Solar Projekt Meier",
		Stage:     "QUALIFIED",
		ContactID: c.ID,
	})
	dealIDStr := uuid.UUID(dl.ID.Bytes).String()

	llm := &mockLLMClient{
		response: "Betreff: Ihr PV-Angebot\n\nHallo Herr Meier, anbei das Angebot.",
	}
	drafter := automation.NewAIDrafter(q, llm)

	err := drafter.DraftEmail(ctx, "Angebot", "DEAL", dealIDStr, "Solar Projekt Meier")
	if err != nil {
		t.Fatalf("DraftEmail on deal failed: %v", err)
	}

	msgs, _ := q.ListEmailMessages(ctx, db.ListEmailMessagesParams{Limit: 10, Offset: 0})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message created, got %d", len(msgs))
	}
	if msgs[0].Subject != "Ihr PV-Angebot" {
		t.Errorf("expected subject 'Ihr PV-Angebot', got %q", msgs[0].Subject)
	}
	if msgs[0].DealID != dl.ID {
		t.Errorf("expected deal ID %v, got %v", dl.ID, msgs[0].DealID)
	}
}

func TestAIDrafter_DraftEmail_HonestErrorOnMissingAccount(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	c, _ := q.CreateContact(context.Background(), db.CreateContactParams{
		FirstName: "Max",
		LastName:  "Mustermann",
		Email:     pgtype.Text{String: "max@example.de", Valid: true},
	})
	contactIDStr := uuid.UUID(c.ID.Bytes).String()

	llm := &mockLLMClient{response: "Test"}
	drafter := automation.NewAIDrafter(q, llm)

	err := drafter.DraftEmail(context.Background(), "template", "CONTACT", contactIDStr, "Max Mustermann")
	if err == nil {
		t.Fatal("expected error when no email account configured, got nil")
	}
	if !strings.Contains(err.Error(), "kein aktives E-Mail-Konto") {
		t.Errorf("expected error message to mention missing account, got: %v", err)
	}
}

func TestAIDrafter_DraftEmail_HonestErrorOnAIFailure(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	ctx := context.Background()
	_, _ = q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name:         "Sales",
		EmailAddress: "vertrieb@openlocalcrm.local",
		IsActive:     true,
	})
	c, _ := q.CreateContact(ctx, db.CreateContactParams{
		FirstName: "Max",
		LastName:  "Mustermann",
		Email:     pgtype.Text{String: "max@example.de", Valid: true},
	})
	contactIDStr := uuid.UUID(c.ID.Bytes).String()

	llm := &mockLLMClient{err: errors.New("upstream AI unavailable")}
	drafter := automation.NewAIDrafter(q, llm)

	err := drafter.DraftEmail(ctx, "template", "CONTACT", contactIDStr, "Max Mustermann")
	if err == nil {
		t.Fatal("expected error on AI failure, got nil")
	}
	if !strings.Contains(err.Error(), "upstream AI unavailable") {
		t.Errorf("expected error to wrap AI error, got: %v", err)
	}
}

func TestAIDrafter_DraftEmail_HonestErrorOnContactWithoutEmail(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	ctx := context.Background()
	_, _ = q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name:         "Sales",
		EmailAddress: "vertrieb@openlocalcrm.local",
		IsActive:     true,
	})
	c, _ := q.CreateContact(ctx, db.CreateContactParams{
		FirstName: "Keine",
		LastName:  "Mail",
		Email:     pgtype.Text{String: "", Valid: false},
	})
	contactIDStr := uuid.UUID(c.ID.Bytes).String()

	llm := &mockLLMClient{response: "Test"}
	drafter := automation.NewAIDrafter(q, llm)

	err := drafter.DraftEmail(ctx, "template", "CONTACT", contactIDStr, "Keine Mail")
	if err == nil {
		t.Fatal("expected error for contact without email, got nil")
	}
	if !strings.Contains(err.Error(), "keine gültige E-Mail-Adresse") {
		t.Errorf("expected error about missing email, got: %v", err)
	}
}
