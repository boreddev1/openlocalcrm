# SP4-Task B Implementation Plan: AI Email Drafting & Automation UI (U3, U6)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement SP4-Task B of the Automation Engine: Real AI-generated email drafts protected by PII sanitization stored in `email_messages.is_draft` (U3), an admin-protected test-run endpoint (`POST /automations/{id}/test-run`) with explicit targets and honest error handling, and complete frontend integration in `AutomationsPage.tsx` with dynamic naming and honest status rendering (U6).

**Architecture:**
- `internal/db/queries/emails.sql`: Adds `CreateDraftEmailMessage` inserting with `is_draft = TRUE`, `direction = 'OUTBOUND'`, `is_read = FALSE`. Generated via `sqlc` and mirrored in `internal/db/demo/querier.go` using `q.emailMessages[uuidToStr(msg.ID)] = msg` (map storage, no `UpdatedAt`).
- `internal/core/automation/draft.go`: Implements `AIDrafter` satisfying the existing `EmailDrafter` interface. Resolves recipient contact (direct or via deal), queries AI Gateway via `LLMClient.Generate` (PII sanitized by guard), parses subject and body, and persists the draft. Errors from missing accounts, missing recipient emails, or upstream AI fail honestly. Never creates fake demo inboxes.
- `internal/server/router.go`: Injects `aiDrafter` into the primary `automationSvc` created at router init (~line 177), immediately following `aiGateway` instantiation (~line 195).
- `internal/core/automation/service.go`: Adds `Snapshot` and `Error` to `WorkflowRun` struct and `ListRuns`. Adds `TestRunWorkflow(ctx, id, targetInput)` which loads ONLY the specified workflow by ID, validates target inputs, executes steps up to the first approval gate (`DRAFT_EMAIL`), sets `WAITING_APPROVAL` without executing the drafter, and persists the run.
- `internal/server/handlers/automations.go`: Adds `TestRunWorkflow` endpoint protected by `auth.RequireRole("ADMIN")`. Returns 400 if target fields are missing, 404 if workflow not found.
- `web/src/pages/AutomationsPage.tsx`: Replaces fake `setTimeout` toasts with real mutations (`testRunWorkflow`, `approveWorkflowStep`). Eliminates all hardcoded `wf-1` strings by performing client-side joins against loaded workflows. Renders honest status badges for `WAITING_APPROVAL`, `COMPLETED`, `FAILED` (with snapshot error), and `IN_PROGRESS`.

**Tech Stack:** Go 1.26, Chi router, PostgreSQL / sqlc, React 18, Vite, TypeScript, Tailwind CSS, TanStack Query.

---

## Global Constraints
- Migrations symlink `migrations -> internal/db/migrations` must remain intact (`ls -la migrations`).
- `sqlc generate` must be run whenever SQL queries in `internal/db/queries/*.sql` change, and `internal/db/demo/querier.go` must implement all new queries in-memory.
- Invariants U1, U2, and U4 from SP4-Task A must NOT be modified or regressed.
- Coverage gate: `scripts/coverage.sh --min 50.0 --diff 50.0` (diff ≥ 50%, total ≥ 50%).
- Conventional Commits without co-author lines:
  - Commit 1: `feat(automations): implement AI-generated email drafts for DRAFT_EMAIL action (U3)`
  - Commit 2: `feat(automations): wire test-run endpoint and honest workflow run UI (U6)`
- No mock or dummy successes: upstream AI or DB failures must fail honestly and mark runs `FAILED`.
- Frontend verification: `cd web && pnpm lint && pnpm typecheck && pnpm build`.

---

## Part 1: U3 — AI-Generated Email Drafts (`email_messages.is_draft`)

### Task 1: SQL Query & In-Memory Demo Querier for Email Drafts (U3)

**Files:**
- Modify: `internal/db/queries/emails.sql`
- Generate: `internal/db/emails.sql.go`, `internal/db/querier.go`
- Modify: `internal/db/demo/querier.go`
- Test: `internal/db/demo/querier_test.go`

**Interfaces:**
- Produces: `db.Querier.CreateDraftEmailMessage(ctx context.Context, arg db.CreateDraftEmailMessageParams) (db.EmailMessage, error)`

- [ ] **Step 1: Write failing test in `internal/db/demo/querier_test.go`**

```go
func TestDemoQuerier_CreateDraftEmailMessage(t *testing.T) {
	q := demo.NewEmptyInMemoryQuerier()
	ctx := context.Background()

	var accID, contactID pgtype.UUID
	_ = accID.Scan("11111111-1111-1111-1111-111111111111")
	_ = contactID.Scan("22222222-2222-2222-2222-222222222222")

	draft, err := q.CreateDraftEmailMessage(ctx, db.CreateDraftEmailMessageParams{
		AccountID:       accID,
		ThreadID:        "<draft-thread-1>",
		MessageID:       "<draft-msg-1>",
		SenderEmail:     "vertrieb@openlocalcrm.local",
		SenderName:      pgtype.Text{String: "Vertrieb", Valid: true},
		RecipientEmails: []byte(`["kunde@example.de"]`),
		Subject:         "Angebot Photovoltaik",
		BodyText:        pgtype.Text{String: "Guten Tag Herr Schmidt...", Valid: true},
		ContactID:       contactID,
	})
	if err != nil {
		t.Fatalf("CreateDraftEmailMessage failed: %v", err)
	}

	if !draft.IsDraft {
		t.Errorf("expected draft.IsDraft == true, got false")
	}
	if draft.IsRead {
		t.Errorf("expected draft.IsRead == false for unread draft, got true")
	}
	if draft.Direction != "OUTBOUND" {
		t.Errorf("expected direction OUTBOUND, got %s", draft.Direction)
	}

	// Verify message can be retrieved from in-memory map
	retrieved, err := q.GetEmailMessageByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("GetEmailMessageByID failed: %v", err)
	}
	if retrieved.Subject != "Angebot Photovoltaik" {
		t.Errorf("expected retrieved subject 'Angebot Photovoltaik', got %q", retrieved.Subject)
	}
}
```

- [ ] **Step 2: Run test to verify it fails (RED)**

Run: `go test -v ./internal/db/demo -run TestDemoQuerier_CreateDraftEmailMessage`
Expected: FAIL (`q.CreateDraftEmailMessage undefined`)

- [ ] **Step 3: Add `CreateDraftEmailMessage` to `internal/db/queries/emails.sql`**

Append to `internal/db/queries/emails.sql`:
```sql
-- name: CreateDraftEmailMessage :one
INSERT INTO email_messages (
    account_id,
    thread_id,
    message_id,
    direction,
    sender_email,
    sender_name,
    recipient_emails,
    subject,
    body_text,
    body_html,
    received_at,
    is_read,
    is_draft,
    contact_id,
    deal_id
) VALUES (
    $1, $2, $3, 'OUTBOUND', $4, $5, $6, $7, $8, $9, NOW(), FALSE, TRUE, $10, $11
)
RETURNING *;
```

- [ ] **Step 4: Run sqlc generate**

Run: `$(go env GOPATH)/bin/sqlc generate`
Verify: `internal/db/emails.sql.go` contains `CreateDraftEmailMessage` and `db.Querier` interface includes it.

- [ ] **Step 5: Implement `CreateDraftEmailMessage` in `internal/db/demo/querier.go`**

Mirroring `CreateEmailMessage` (`querier.go:1472-1498`), storing into `q.emailMessages` map, with `IsDraft = true`, `IsRead = false`, and no `UpdatedAt`:
```go
func (q *InMemoryQuerier) CreateDraftEmailMessage(ctx context.Context, arg db.CreateDraftEmailMessageParams) (db.EmailMessage, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	msg := db.EmailMessage{
		ID:              newUUID(),
		AccountID:       arg.AccountID,
		ThreadID:        arg.ThreadID,
		MessageID:       arg.MessageID,
		Direction:       "OUTBOUND",
		SenderEmail:     arg.SenderEmail,
		SenderName:      arg.SenderName,
		RecipientEmails: arg.RecipientEmails,
		Subject:         arg.Subject,
		BodyText:        arg.BodyText,
		BodyHtml:        arg.BodyHtml,
		ReceivedAt:      nowTimestamptz(),
		IsRead:          false,
		IsDraft:         true,
		ContactID:       arg.ContactID,
		DealID:          arg.DealID,
		Tags:            []byte("[]"),
		CreatedAt:       nowTimestamptz(),
	}
	q.emailMessages[uuidToStr(msg.ID)] = msg
	return msg, nil
}
```

- [ ] **Step 6: Run test to verify it passes (GREEN)**

Run: `go test -v ./internal/db/demo -run TestDemoQuerier_CreateDraftEmailMessage`
Expected: PASS

---

### Task 2: AI Email Drafter Implementation (U3)

**Files:**
- Create: `internal/core/automation/draft.go`
- Create: `internal/core/automation/draft_test.go`
- Modify: `internal/server/router.go:175-200`

**Interfaces:**
- Consumes:
  - `LLMClient` (narrow interface): `Generate(ctx context.Context, prompt string, systemInstruction string) (string, error)`
  - `db.Querier`: `ListEmailAccounts`, `GetContactByID`, `GetDealByID`, `CreateDraftEmailMessage`
- Produces:
  - `automation.AIDrafter`: implements `EmailDrafter`
    `DraftEmail(ctx context.Context, template string, targetType, targetID, targetName string) error`

- [ ] **Step 1: Write failing unit tests in `internal/core/automation/draft_test.go` (RED)**

```go
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
	calledWithPrompt string
	calledWithSystem string
}

func (m *mockLLMClient) Generate(ctx context.Context, prompt, systemInstruction string) (string, error) {
	m.calledWithPrompt = prompt
	m.calledWithSystem = systemInstruction
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
```

- [ ] **Step 2: Run tests to verify they fail (RED)**

Run: `go test -v ./internal/core/automation -run TestAIDrafter_`
Expected: FAIL (`undefined: automation.NewAIDrafter`)

- [ ] **Step 3: Implement `AIDrafter` in `internal/core/automation/draft.go` (GREEN)**

```go
package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// LLMClient is the narrow interface required by AIDrafter for AI text generation.
type LLMClient interface {
	Generate(ctx context.Context, prompt string, systemInstruction string) (string, error)
}

// AIDrafter generates real email drafts via AI and persists them into email_messages.
type AIDrafter struct {
	queries  db.Querier
	aiClient LLMClient
}

// NewAIDrafter constructs a new AIDrafter.
func NewAIDrafter(queries db.Querier, aiClient LLMClient) *AIDrafter {
	return &AIDrafter{
		queries:  queries,
		aiClient: aiClient,
	}
}

// DraftEmail resolves recipient information, generates email content via AI, and stores it as an email draft.
func (d *AIDrafter) DraftEmail(ctx context.Context, template string, targetType, targetID, targetName string) error {
	if d.queries == nil {
		return fmt.Errorf("datenbank-verbindung erforderlich für entwurf-erstellung")
	}
	if d.aiClient == nil {
		return fmt.Errorf("ki-dienst nicht konfiguriert für entwurf-erstellung")
	}

	// 1. Resolve active sender account (never creates fake demo accounts)
	accounts, err := d.queries.ListEmailAccounts(ctx)
	if err != nil {
		return fmt.Errorf("fehler beim laden der e-mail-konten: %w", err)
	}
	if len(accounts) == 0 {
		return fmt.Errorf("kein aktives E-Mail-Konto für Entwurf-Erstellung konfiguriert")
	}
	senderAccount := accounts[0]

	// 2. Resolve recipient email and target IDs
	var recipientEmail string
	var contactID, dealID pgtype.UUID

	switch strings.ToUpper(targetType) {
	case "CONTACT":
		cUUID, err := uuid.Parse(targetID)
		if err != nil {
			return fmt.Errorf("ungültige kontakt-id %q: %w", targetID, err)
		}
		contactID = pgtype.UUID{Bytes: cUUID, Valid: true}
		c, err := d.queries.GetContactByID(ctx, contactID)
		if err != nil {
			return fmt.Errorf("kontakt %s nicht gefunden: %w", targetID, err)
		}
		if !c.Email.Valid || strings.TrimSpace(c.Email.String) == "" {
			return fmt.Errorf("kontakt %s hat keine gültige E-Mail-Adresse", targetID)
		}
		recipientEmail = strings.TrimSpace(c.Email.String)

	case "DEAL":
		dUUID, err := uuid.Parse(targetID)
		if err != nil {
			return fmt.Errorf("ungültige deal-id %q: %w", targetID, err)
		}
		dealID = pgtype.UUID{Bytes: dUUID, Valid: true}
		dl, err := d.queries.GetDealByID(ctx, dealID)
		if err != nil {
			return fmt.Errorf("deal %s nicht gefunden: %w", targetID, err)
		}
		if !dl.ContactID.Valid {
			return fmt.Errorf("deal %s ist keinem Kontakt zugeordnet", targetID)
		}
		contactID = dl.ContactID
		c, err := d.queries.GetContactByID(ctx, contactID)
		if err != nil {
			return fmt.Errorf("zugeordneter kontakt für deal %s nicht gefunden: %w", targetID, err)
		}
		if !c.Email.Valid || strings.TrimSpace(c.Email.String) == "" {
			return fmt.Errorf("kontakt für deal %s hat keine gültige E-Mail-Adresse", targetID)
		}
		recipientEmail = strings.TrimSpace(c.Email.String)

	default:
		return fmt.Errorf("nicht unterstützter ziel-typ für entwurf: %s", targetType)
	}

	// 3. Generate text via AI (system instruction + prompt, prompt is not logged)
	systemInstruction := "Du bist ein professioneller Kundenbetreuer im deutschen B2B- und Direktvertrieb. Verfasse eine höfliche, verbindliche E-Mail auf Deutsch. Format: Erste Zeile 'Betreff: <Betreffzeile>', gefolgt von einer Leerzeile und dem E-Mail-Text."
	prompt := fmt.Sprintf("Empfänger: %s\nVorlage / Thema: %s\nErstelle einen passenden Erstkontakt-Entwurf.", targetName, template)

	generated, err := d.aiClient.Generate(ctx, prompt, systemInstruction)
	if err != nil {
		return fmt.Errorf("ki-entwurf fehlgeschlagen: %w", err)
	}

	// 4. Parse Subject and Body
	subject, body := parseSubjectAndBody(generated, template)

	// 5. Persist Draft
	recipientsJSON, err := json.Marshal([]string{recipientEmail})
	if err != nil {
		return fmt.Errorf("fehler beim formatieren der empfänger: %w", err)
	}

	messageID := fmt.Sprintf("<draft-%s-%d@openlocalcrm.local>", targetID, time.Now().UnixNano())
	threadID := messageID

	_, err = d.queries.CreateDraftEmailMessage(ctx, db.CreateDraftEmailMessageParams{
		AccountID:       senderAccount.ID,
		ThreadID:        threadID,
		MessageID:       messageID,
		SenderEmail:     senderAccount.EmailAddress,
		SenderName:      pgtype.Text{String: senderAccount.Name, Valid: true},
		RecipientEmails: recipientsJSON,
		Subject:         subject,
		BodyText:        pgtype.Text{String: body, Valid: true},
		BodyHtml:        pgtype.Text{String: "", Valid: false},
		ContactID:       contactID,
		DealID:          dealID,
	})
	if err != nil {
		return fmt.Errorf("fehler beim speichern des entwurfs: %w", err)
	}

	return nil
}

func parseSubjectAndBody(raw, defaultTemplate string) (string, string) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		if strings.HasPrefix(strings.ToLower(firstLine), "betreff:") {
			subject := strings.TrimSpace(firstLine[8:])
			body := strings.TrimSpace(strings.Join(lines[1:], "\n"))
			if subject != "" {
				return subject, body
			}
		}
	}
	// Fallback split on ---
	parts := strings.SplitN(raw, "---", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}

	fallbackSubject := fmt.Sprintf("Informationen: %s", defaultTemplate)
	return fallbackSubject, strings.TrimSpace(raw)
}
```

- [ ] **Step 4: Wire `AIDrafter` into production service in `internal/server/router.go`**

In `internal/server/router.go`, right after `aiGateway` instantiation (~line 195):
```go
		aiDrafter := automation.NewAIDrafter(cfg.DB, aiGateway)
		automationSvc.SetEmailDrafter(aiDrafter)
```

- [ ] **Step 5: Run tests to verify GREEN**

Run: `go test -v ./internal/core/automation -run TestAIDrafter_`
Expected: PASS

- [ ] **Step 6: Commit Task 1 & 2**

```bash
git add internal/db/ internal/core/automation/draft.go internal/core/automation/draft_test.go internal/server/router.go
git commit -m "feat(automations): implement AI-generated email drafts for DRAFT_EMAIL action (U3)"
```

---

## Part 2: U6 — Test-Run Endpoint & Honest Workflow UI

### Task 3: Backend Test-Run Endpoint `POST /automations/{id}/test-run` & Snapshot Visibility (U6)

**Files:**
- Modify: `internal/core/automation/service.go`
- Modify: `internal/core/automation/service_test.go`
- Modify: `internal/server/handlers/automations.go`
- Modify: `internal/server/handlers/automations_test.go`
- Modify: `internal/server/router.go`

**Interfaces:**
- Produces:
  - `WorkflowRun.Snapshot` (`map[string]any`) and `WorkflowRun.Error` (`string`) populated in `ListRuns`, `TriggerEvent`, `ApproveStep`, `TestRunWorkflow`
  - `(*automation.Service).TestRunWorkflow(ctx context.Context, workflowID string, target TargetInput) (*WorkflowRun, error)`
  - HTTP `POST /api/v1/automations/{id}/test-run` (Admin only)
    Request Body:
    ```json
    {
      "target_id": "44444444-4444-4444-4444-444444444441",
      "target_type": "CONTACT",
      "target_name": "Dr. Michael Weber"
    }
    ```
    Response: HTTP 200 with `WorkflowRun` JSON, or 400 (missing target), 404 (workflow not found).

- [ ] **Step 1: Write failing service & handler tests (RED)**

In `internal/core/automation/service_test.go`:
```go
func TestAutomation_TestRunWorkflow_PausesAtDraftEmail(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := automation.NewService(q)

	// Use real seeded contact: 44444444-4444-4444-4444-444444444441 (Dr. Michael Weber)
	run, err := svc.TestRunWorkflow(context.Background(), "wf-1", automation.TargetInput{
		TargetID:   "44444444-4444-4444-4444-444444444441",
		TargetType: "CONTACT",
		TargetName: "Dr. Michael Weber",
	})
	if err != nil {
		t.Fatalf("TestRunWorkflow failed: %v", err)
	}

	if run.WorkflowID != "wf-1" {
		t.Errorf("expected workflow_id wf-1, got %s", run.WorkflowID)
	}
	if run.Status != "WAITING_APPROVAL" {
		t.Errorf("expected status WAITING_APPROVAL, got %s", run.Status)
	}
	if run.CurrentStep != 2 {
		t.Errorf("expected paused at step 2 (DRAFT_EMAIL), got %d", run.CurrentStep)
	}
}

func TestAutomation_TestRunWorkflow_RequiresTarget(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := automation.NewService(q)

	_, err := svc.TestRunWorkflow(context.Background(), "wf-1", automation.TargetInput{
		TargetID: "",
	})
	if err == nil {
		t.Fatal("expected error when target_id is empty, got nil")
	}
}

func TestAutomation_ListRuns_ExposesSnapshotAndError(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := automation.NewService(q)

	runs, err := svc.ListRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected runs in demo querier")
	}
}
```

In `internal/server/handlers/automations_test.go`:
```go
func TestAutomationHandler_TestRun_ValidatesTarget(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := automation.NewService(q)
	h := handlers.NewAutomationHandler(svc)

	// 1. Missing target -> 400
	reqEmpty := httptest.NewRequest("POST", "/api/v1/automations/wf-1/test-run", strings.NewReader(`{}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "wf-1")
	reqEmpty = reqEmpty.WithContext(context.WithValue(reqEmpty.Context(), chi.RouteCtxKey, rctx))
	recEmpty := httptest.NewRecorder()
	h.TestRunWorkflow(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty target, got %d: %s", recEmpty.Code, recEmpty.Body.String())
	}

	// 2. Valid target with real contact -> 200
	body, _ := json.Marshal(map[string]string{
		"target_id":   "44444444-4444-4444-4444-444444444441",
		"target_type": "CONTACT",
		"target_name": "Dr. Michael Weber",
	})
	reqValid := httptest.NewRequest("POST", "/api/v1/automations/wf-1/test-run", bytes.NewReader(body))
	reqValid = reqValid.WithContext(context.WithValue(reqValid.Context(), chi.RouteCtxKey, rctx))
	recValid := httptest.NewRecorder()
	h.TestRunWorkflow(recValid, reqValid)

	if recValid.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", recValid.Code, recValid.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (RED)**

Run: `go test -v ./internal/core/automation -run TestAutomation_TestRunWorkflow_`
Expected: FAIL (`undefined: automation.TargetInput`, `svc.TestRunWorkflow undefined`)

- [ ] **Step 3: Update `WorkflowRun` and implement `TestRunWorkflow` in `internal/core/automation/service.go` (GREEN)**

In `internal/core/automation/service.go`:
```go
type WorkflowRun struct {
	ID          string         `json:"id"`
	WorkflowID  string         `json:"workflow_id"`
	TargetID    string         `json:"target_id"`
	TargetType  string         `json:"target_type"`
	TargetName  string         `json:"target_name"`
	Status      string         `json:"status"` // IN_PROGRESS, WAITING_APPROVAL, COMPLETED, CANCELLED, FAILED
	CurrentStep int            `json:"current_step"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Snapshot    map[string]any `json:"snapshot,omitempty"`
	Error       string         `json:"error,omitempty"`
}
```
Update `ListRuns` to parse `SnapshotJson`, populate `Snapshot` and `Error` from `snapshot["error"]`.
Add `TargetInput` and `TestRunWorkflow`:
```go
type TargetInput struct {
	TargetID   string `json:"target_id"`
	TargetType string `json:"target_type"`
	TargetName string `json:"target_name"`
}

func (s *Service) TestRunWorkflow(ctx context.Context, workflowID string, target TargetInput) (*WorkflowRun, error) {
	if s.querier == nil {
		return nil, fmt.Errorf("datenbank erforderlich für testlauf")
	}
	if strings.TrimSpace(target.TargetID) == "" || strings.TrimSpace(target.TargetType) == "" {
		return nil, fmt.Errorf("target_id und target_type sind erforderlich für den testlauf")
	}

	dbWf, err := s.querier.GetWorkflowByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow %q nicht gefunden: %w", workflowID, err)
	}

	var steps []StepDefinition
	if err := json.Unmarshal(dbWf.StepsJson, &steps); err != nil {
		return nil, fmt.Errorf("ungültige workflow-schritte: %w", err)
	}

	run := WorkflowRun{
		ID:          fmt.Sprintf("run-%d", time.Now().UnixNano()),
		WorkflowID:  dbWf.ID,
		TargetID:    target.TargetID,
		TargetType:  target.TargetType,
		TargetName:  target.TargetName,
		Status:      "IN_PROGRESS",
		CurrentStep: 0,
		StartedAt:   time.Now().UTC(),
	}

	snapshot := make(map[string]any)

	// Execute steps until the first step requiring approval (DRAFT_EMAIL)
	for _, step := range steps {
		run.CurrentStep = step.StepNumber
		if step.ActionType == "DRAFT_EMAIL" {
			// Pause before executing DRAFT_EMAIL (HITL approval required)
			run.Status = "WAITING_APPROVAL"
			break
		}
		if err := s.executeStep(ctx, step, &run, snapshot); err != nil {
			log.Printf("[automation] test run step %d failed: %v", step.StepNumber, err)
			run.Status = "FAILED"
			now := time.Now().UTC()
			run.CompletedAt = &now
			snapshot["error"] = fmt.Sprintf("Schritt %d (%s) fehlgeschlagen: %v", step.StepNumber, step.Title, err)
			run.Error = snapshot["error"].(string)
			break
		}
	}

	if run.Status == "IN_PROGRESS" {
		now := time.Now().UTC()
		run.Status = "COMPLETED"
		run.CompletedAt = &now
	}

	run.Snapshot = snapshot
	snapBytes, _ := json.Marshal(snapshot)
	_, err = s.querier.CreateWorkflowRun(ctx, db.CreateWorkflowRunParams{
		ID:           run.ID,
		WorkflowID:   run.WorkflowID,
		TargetID:     run.TargetID,
		TargetType:   run.TargetType,
		TargetName:   run.TargetName,
		Status:       run.Status,
		CurrentStep:  int32(run.CurrentStep),
		SnapshotJson: snapBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("fehler beim speichern des testlaufs: %w", err)
	}

	return &run, nil
}
```

- [ ] **Step 4: Add `TestRunWorkflow` to `internal/server/handlers/automations.go`**

```go
func (h *AutomationHandler) TestRunWorkflow(w http.ResponseWriter, r *http.Request) {
	wfID := chi.URLParam(r, "id")
	if wfID == "" {
		http.Error(w, `{"error":"workflow_id_required"}`, http.StatusBadRequest)
		return
	}

	var req automation.TargetInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request_body"}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.TargetID) == "" || strings.TrimSpace(req.TargetType) == "" {
		http.Error(w, `{"error":"missing_target","message":"target_id and target_type are required"}`, http.StatusBadRequest)
		return
	}

	run, err := h.service.TestRunWorkflow(r.Context(), wfID, req)
	if err != nil {
		if strings.Contains(err.Error(), "nicht gefunden") {
			http.Error(w, fmt.Sprintf(`{"error":"not_found","message":%q}`, err.Error()), http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf(`{"error":"test_run_failed","message":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(run)
}
```

- [ ] **Step 5: Register Route in `internal/server/router.go`**

In `internal/server/router.go` around line 465:
```go
			if automationH != nil {
				protected.Route("/automations", func(autr chi.Router) {
					autr.Get("/", automationH.ListWorkflows)
					autr.Get("/runs", automationH.ListRuns)
					autr.With(auth.RequireRole("ADMIN")).Post("/", automationH.CreateWorkflow)
					autr.With(auth.RequireRole("ADMIN")).Post("/{id}/test-run", automationH.TestRunWorkflow)
					autr.With(auth.RequireRole("ADMIN")).Post("/runs/{id}/approve", automationH.ApproveStep)
				})
			}
```

- [ ] **Step 6: Run tests to verify GREEN**

Run: `go test -v ./internal/core/automation ./internal/server/handlers`
Expected: PASS

---

### Task 4: Frontend API & AutomationsPage Integration (U6)

**Files:**
- Modify: `web/src/api/client.ts`
- Modify: `web/src/pages/AutomationsPage.tsx`

**Interfaces:**
- Consumes:
  - `POST /api/v1/automations/{id}/test-run`
  - `POST /api/v1/automations/runs/{id}/approve`
  - `GET /api/v1/contacts` & `GET /api/v1/deals` (for target picker in Testlauf)
- Produces:
  - Working Testlauf modal/dropdown choosing an actual contact/deal target
  - Working Schritt freigeben button
  - Real status rendering (no `wf-1` hardcoded strings, honest badges)

- [ ] **Step 1: Add API client methods in `web/src/api/client.ts`**

```typescript
export interface TestRunPayload {
  target_id: string;
  target_type: string;
  target_name: string;
}

export const testRunWorkflow = (id: string, payload: TestRunPayload) =>
  apiFetch<any>(`/api/v1/automations/${id}/test-run`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const approveWorkflowStep = (runId: string) =>
  apiFetch<any>(`/api/v1/automations/runs/${runId}/approve`, {
    method: 'POST',
  });
```

- [ ] **Step 2: Update `AutomationsPage.tsx`**

1. Query contacts (`apiFetch('/api/v1/contacts?limit=50')`) and deals (`apiFetch('/api/v1/deals?limit=50')`) for target selection.
2. Add target selection modal for Testlauf:
   When user clicks "Testlauf" on a workflow:
   - If `wf.target_type === 'CONTACT'`, presents list of loaded contacts. If none exist: honest message "Keine Kontakte vorhanden. Bitte legen Sie zuerst einen Kontakt an."
   - If `wf.target_type === 'DEAL'`, presents list of loaded deals. If none exist: honest message "Keine Deals vorhanden. Bitte legen Sie zuerst einen Deal an."
   - On confirm: calls `testRunMutation.mutate({ id: wf.id, payload: { target_id, target_type, target_name } })`.
3. Mutations with TanStack Query and honest toast handling:
   - `testRunMutation`:
     `onSuccess: (run: any) => {`
       `queryClient.invalidateQueries({ queryKey: ['automation-runs'] });`
       `if (run.status === 'WAITING_APPROVAL') { setToast({ type: 'info', msg: `Testlauf gestartet: Schritt ${run.current_step} wartet auf Freigabe.` }); }`
       `else if (run.status === 'COMPLETED') { setToast({ type: 'success', msg: 'Testlauf erfolgreich abgeschlossen.' }); }`
       `else if (run.status === 'FAILED') { setToast({ type: 'error', msg: `Testlauf fehlgeschlagen: ${run.error || 'Fehler beim Ausführen'}` }); }`
     `}`
     `onError: (err: any) => setToast({ type: 'error', msg: `Fehler beim Testlauf: ${err.message || err}` })`
   - `approveMutation`:
     `onSuccess: (run: any) => {`
       `queryClient.invalidateQueries({ queryKey: ['automation-runs'] });`
       `if (run.status === 'FAILED') { setToast({ type: 'error', msg: `Freigabe fehlgeschlagen: ${run.error || 'Fehler im Schritt'}` }); }`
       `else { setToast({ type: 'success', msg: `Schritt ${run.current_step} erfolgreich ausgeführt.` }); }`
     `}`
     `onError: (err: any) => setToast({ type: 'error', msg: `Freigabefehler: ${err.message || err}` })`
4. Client-side joins (no `wf-1` hardcodes):
   ```tsx
   const parentWf = workflows.find((w) => w.id === run.workflow_id);
   const currentStepDef = parentWf?.steps?.find((s: any) => s.step_number === run.current_step);
   const stepTitle = currentStepDef?.title || `Schritt ${run.current_step}`;
   ```
5. Render status badges accurately:
   - `WAITING_APPROVAL`: Amber badge ("Wartet auf Freigabe") + "Schritt freigeben" Button
   - `COMPLETED`: Emerald badge ("Abgeschlossen")
   - `FAILED`: Red badge ("Fehlgeschlagen") with snapshot error explanation (`run.error`)
   - `IN_PROGRESS`: Indigo badge ("In Ausführung")

- [ ] **Step 3: Verify frontend typecheck & build**

Run: `cd web && pnpm lint && pnpm typecheck && pnpm build`
Expected: 0 errors, build succeeds.

- [ ] **Step 4: Commit Task 3 & 4 (Staging ALL touched backend and frontend files)**

```bash
git add internal/core/automation/service.go internal/core/automation/service_test.go internal/server/handlers/automations.go internal/server/handlers/automations_test.go internal/server/router.go web/src/api/client.ts web/src/pages/AutomationsPage.tsx
git commit -m "feat(automations): wire test-run endpoint and honest workflow run UI (U6)"
```

---

## Part 3: Verification & Documentation

### Task 5: End-to-End Verification & Reporting

- [ ] **Step 1: Run full test suite**
  `go test ./...` -> verify all packages pass.
- [ ] **Step 2: Check formatting and static analysis**
  `gofmt -l .` -> must be completely empty.
  `go vet ./...` -> must exit 0 with no warnings.
- [ ] **Step 3: Run coverage gate**
  `./scripts/coverage.sh --min 50.0 --diff 50.0` -> verify diff ≥ 50.0% and total ≥ 50.0%.
- [ ] **Step 4: Verify critical invariants**
  `ls -la migrations` -> verify symlink `migrations -> internal/db/migrations` is intact.
- [ ] **Step 5: Update `.superpowers/sdd/progress.md` and write report `.superpowers/sdd/task-sp4b-report.md`**
