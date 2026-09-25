package audit_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestAuditService_Log(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := audit.NewService(querier)

	userID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	entityID := pgtype.UUID{Bytes: [16]byte{2, 2, 2}, Valid: true}

	err := svc.Log(ctx, userID, "CONTACT", entityID, "UPDATE", map[string]string{"name": "Dr. Weber"}, "127.0.0.1", "Mozilla/5.0")
	if err != nil {
		t.Fatalf("failed to log audit event: %v", err)
	}

	// Logging with nil changes should also succeed
	errNil := svc.Log(ctx, userID, "DEAL", entityID, "DELETE", nil, "127.0.0.1", "Mozilla/5.0")
	if errNil != nil {
		t.Fatalf("failed to log audit event with nil changes: %v", errNil)
	}
}

func TestAuditService_LogDiff_PIISanitization(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := audit.NewService(querier)

	userID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	contactID := pgtype.UUID{Bytes: [16]byte{2, 2, 2}, Valid: true}

	oldVals := audit.DiffMap{
		"phone":              "+49 69 12345678",
		"email":              "muster@example.de",
		"consent_phone":      false,
		"consent_email":      false,
		"consent_updated_at": nil,
		"first_name":         "Erika",
	}
	newVals := audit.DiffMap{
		"phone":              "+49 69 98765432",
		"email":              "erika.neu@example.de",
		"consent_phone":      true,
		"consent_email":      true,
		"consent_updated_at": "2026-09-25T10:00:00Z",
		"first_name":         "Erika",
	}

	err := svc.LogDiff(ctx, userID, "CONTACT", contactID, "CONSENT_UPDATE", oldVals, newVals, "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("LogDiff failed: %v", err)
	}

	logs, err := querier.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "CONTACT",
		EntityID:   contactID,
		Limit:      10,
	})
	if err != nil || len(logs) == 0 {
		t.Fatalf("expected audit log entry, got %v (len %d)", err, len(logs))
	}

	entry := logs[0]
	// Verify old_values and new_values JSON contains sanitized phone/email
	var storedOld, storedNew audit.DiffMap
	if err := json.Unmarshal(entry.OldValues, &storedOld); err != nil {
		t.Fatalf("failed to unmarshal old_values: %v", err)
	}
	if err := json.Unmarshal(entry.NewValues, &storedNew); err != nil {
		t.Fatalf("failed to unmarshal new_values: %v", err)
	}

	// Phone and Email must be masked
	if storedOld["email"] == "muster@example.de" || storedNew["email"] == "erika.neu@example.de" {
		t.Errorf("PII email was not sanitized: old=%v, new=%v", storedOld["email"], storedNew["email"])
	}
	if storedOld["phone"] == "+49 69 12345678" || storedNew["phone"] == "+49 69 98765432" {
		t.Errorf("PII phone was not sanitized: old=%v, new=%v", storedOld["phone"], storedNew["phone"])
	}

	// Consent fields MUST be preserved exactly for UWG §7 proof
	if storedNew["consent_phone"] != true || storedNew["consent_email"] != true {
		t.Errorf("UWG consent flags were corrupted: %v", storedNew)
	}
	if storedNew["consent_updated_at"] != "2026-09-25T10:00:00Z" {
		t.Errorf("UWG consent_updated_at was corrupted: %v", storedNew["consent_updated_at"])
	}
}
