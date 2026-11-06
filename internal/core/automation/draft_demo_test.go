package automation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

// The E2E HITL approval executes AIDrafter.DraftEmail through the real gateway.
// In demo mode that call chain must complete with zero network access and the
// persisted draft must carry the unmistakable DEMO marker, so nobody can pass
// the stub off as a real model.
func TestAIDrafterDemoGatewayPersistsMarkedDraftWithoutNetwork(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	ctx := context.Background()

	if _, err := q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name:         "E2E HITL Absendekonto",
		EmailAddress: "e2e-hitl@openlocalcrm.local",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("seed email account: %v", err)
	}
	c, err := q.CreateContact(ctx, db.CreateContactParams{
		FirstName: "Michael",
		LastName:  "Weber",
		Email:     pgtype.Text{String: "m.weber@weber-maschinenbau.de", Valid: true},
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	gw := ai.NewGateway(ai.GatewayConfig{DemoMode: true})
	drafter := automation.NewAIDrafter(q, gw)

	if draftErr := drafter.DraftEmail(ctx, "Erstkontakt & Qualifizierung", "CONTACT", uuid.UUID(c.ID.Bytes).String(), "Dr. Michael Weber"); draftErr != nil {
		t.Fatalf("demo DraftEmail must succeed without any provider: %v", draftErr)
	}

	msgs, err := q.ListEmailMessages(ctx, db.ListEmailMessagesParams{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 persisted draft, got %d", len(msgs))
	}
	draft := msgs[0]
	if !draft.IsDraft {
		t.Error("expected IsDraft == true")
	}
	if strings.TrimSpace(draft.Subject) == "" {
		t.Error("expected the demo stub to produce a parseable 'Betreff:' subject")
	}
	if !strings.Contains(draft.BodyText.String, ai.DemoStubMarker) {
		t.Errorf("persisted draft body must carry the DEMO marker, got: %q", draft.BodyText.String)
	}
}
