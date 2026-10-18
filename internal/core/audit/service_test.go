package audit_test

import (
	"context"
	"encoding/json"
	"strings"
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

	var changes struct {
		ChangedKeys []string `json:"changed_keys"`
	}
	if err := json.Unmarshal(entry.Changes, &changes); err != nil {
		t.Fatalf("failed to unmarshal changes: %v", err)
	}
	changed := make(map[string]bool, len(changes.ChangedKeys))
	for _, key := range changes.ChangedKeys {
		changed[key] = true
	}
	if changed["first_name"] {
		t.Errorf("unchanged field reported as changed: %v", changes.ChangedKeys)
	}
	if !changed["email"] || !changed["consent_email"] {
		t.Errorf("changed fields missing from audit diff: %v", changes.ChangedKeys)
	}
}

func TestAuditService_LogSanitizesNestedValues(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := audit.NewService(querier)
	userID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	entityID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	changes := map[string]any{
		"contact": map[string]any{
			"email":         "nested@example.de",
			"consent_email": true,
			"profiles": []any{
				map[string]any{"phone": "+49 69 12345678"},
			},
		},
	}
	if err := svc.Log(ctx, userID, "CONTACT", entityID, "CREATE", changes, "", ""); err != nil {
		t.Fatalf("failed to log nested audit event: %v", err)
	}

	logs, err := querier.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "CONTACT",
		EntityID:   entityID,
		Limit:      10,
	})
	if err != nil || len(logs) != 1 {
		t.Fatalf("expected one audit log, got %d: %v", len(logs), err)
	}
	storedJSON := string(logs[0].Changes)
	if storedJSON == "" || containsAny(storedJSON, "nested@example.de", "+49 69 12345678") {
		t.Fatalf("nested PII was not sanitized: %s", storedJSON)
	}

	var stored map[string]any
	if err := json.Unmarshal(logs[0].Changes, &stored); err != nil {
		t.Fatalf("failed to unmarshal nested audit event: %v", err)
	}
	contactValues, ok := stored["contact"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected contact audit value: %#v", stored["contact"])
	}
	if contactValues["consent_email"] != true {
		t.Errorf("nested consent evidence was altered: %#v", contactValues)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

type tamperedQuerier struct {
	*demo.InMemoryQuerier
	tamperFunc func([]db.AuditLog) []db.AuditLog
}

func (t *tamperedQuerier) ListAuditLogsAsc(ctx context.Context) ([]db.AuditLog, error) {
	logs, err := t.InMemoryQuerier.ListAuditLogsAsc(ctx)
	if err != nil {
		return nil, err
	}
	if t.tamperFunc != nil {
		return t.tamperFunc(logs), nil
	}
	return logs, nil
}

func TestAuditService_HashChain_Verification(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := audit.NewService(querier)

	userID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	contactID := pgtype.UUID{Bytes: [16]byte{2, 2, 2}, Valid: true}

	// 1. Log multiple events
	err := svc.Log(ctx, userID, "CONTACT", contactID, "CREATE", map[string]string{"name": "Alice"}, "127.0.0.1", "Agent1")
	if err != nil {
		t.Fatalf("Log 1 failed: %v", err)
	}

	err = svc.LogDiff(ctx, userID, "CONTACT", contactID, "UPDATE",
		audit.DiffMap{"name": "Alice"},
		audit.DiffMap{"name": "Alice Smith"},
		"127.0.0.1", "Agent1",
	)
	if err != nil {
		t.Fatalf("LogDiff 2 failed: %v", err)
	}

	err = svc.Log(ctx, userID, "DEAL", contactID, "WON", map[string]int{"volume": 5000}, "127.0.0.1", "Agent1")
	if err != nil {
		t.Fatalf("Log 3 failed: %v", err)
	}

	// 2. Verify hash chain
	valid, count, err := svc.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("VerifyAuditChain failed: %v", err)
	}
	if !valid {
		t.Fatalf("expected audit chain to be valid, got false")
	}
	if count != 3 {
		t.Fatalf("expected 3 verified logs, got %d", count)
	}
}

func TestAuditService_HashChain_TamperingDetected(t *testing.T) {
	ctx := context.Background()
	memQ := demo.NewInMemoryQuerier()
	svc := audit.NewService(memQ)

	userID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	contactID := pgtype.UUID{Bytes: [16]byte{2, 2, 2}, Valid: true}

	_ = svc.Log(ctx, userID, "CONTACT", contactID, "CREATE", map[string]string{"name": "Alice"}, "127.0.0.1", "Agent1")
	_ = svc.Log(ctx, userID, "CONTACT", contactID, "UPDATE", map[string]string{"name": "Bob"}, "127.0.0.1", "Agent1")
	_ = svc.Log(ctx, userID, "CONTACT", contactID, "DELETE", nil, "127.0.0.1", "Agent1")

	// Case 1: Tamper with action on second record
	tqTamperedContent := &tamperedQuerier{
		InMemoryQuerier: memQ,
		tamperFunc: func(logs []db.AuditLog) []db.AuditLog {
			tampered := make([]db.AuditLog, len(logs))
			copy(tampered, logs)
			tampered[1].Action = "MODIFIED_ACTION"
			return tampered
		},
	}
	svcTampered := audit.NewService(tqTamperedContent)
	valid, idx, err := svcTampered.VerifyAuditChain(ctx)
	if valid || err == nil {
		t.Fatalf("expected tampering detection for content change, got valid=%v, err=%v", valid, err)
	}
	if idx != 1 {
		t.Errorf("expected mismatch at index 1, got %d", idx)
	}

	// Case 2: Tamper with prev_hash chain link
	tqTamperedLink := &tamperedQuerier{
		InMemoryQuerier: memQ,
		tamperFunc: func(logs []db.AuditLog) []db.AuditLog {
			tampered := make([]db.AuditLog, len(logs))
			copy(tampered, logs)
			tampered[2].PrevHash = pgtype.Text{String: "corrupted_prev_hash", Valid: true}
			return tampered
		},
	}
	svcTamperedLink := audit.NewService(tqTamperedLink)
	valid, idx, err = svcTamperedLink.VerifyAuditChain(ctx)
	if valid || err == nil {
		t.Fatalf("expected tampering detection for prev_hash link, got valid=%v, err=%v", valid, err)
	}
	if idx != 2 {
		t.Errorf("expected broken chain at index 2, got %d", idx)
	}
}
