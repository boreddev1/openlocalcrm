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
	accounts, err := d.queries.ListEmailAccounts(ctx, pgtype.UUID{})
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
