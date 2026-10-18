# Sub-Project 6 (SP6): CRM Data Truth & UWG/BGB Compliance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish end-to-end data truth and regulatory compliance across CRM records (UWG §7 phone/email consent, energy consumption metrics, §355 BGB consumer revocation lifecycle) and implement privacy-preserving audit logging with selective diffs and PII sanitization.

**Architecture:** Extend database schema with migration `00017_audit_logs_diff.sql` for audit diffs; update `contacts.sql`, `deals.sql`, and `audit.sql` queries with `sqlc` regeneration; wire consent/energy fields through `contact.Service`, `ContactHandler`, and `ExportContactsCSV`; implement deal revocation lifecycle in `deal.Service`, `POST /api/v1/deals/{id}/revoke`, and `DealsPage.tsx` (strictly rejecting WIDERRUFEN in both Create and Update endpoints/selects); add selective diff logging and PII masking to `audit.Service` and `contact.Service.Update`.

**Tech Stack:** Go 1.26, Chi, pgx/v5, sqlc, React 18, TypeScript, Tailwind CSS, TanStack Query.

## Global Constraints

- **Migrations Symlink:** `migrations` must remain a symlink to `internal/db/migrations`. Both directories must contain identical files.
- **SQLC Invocation:** SQLC binary is located at `/Users/ewenwitte/go/bin/sqlc`. Regenerate whenever `internal/db/queries/*.sql` changes.
- **RAM Target:** CRM runtime must remain strictly under **420 MB RAM**.
- **License Policy:** 100% permissive licenses only (MIT, Apache-2.0, BSD). No copyleft dependencies.
- **Coverage Gate:** Every new or changed line must satisfy `scripts/coverage.sh --min 50.0 --diff 50.0` (≥ 50% diff coverage and ≥ 50% total coverage).
- **Conventional Commits:** Enforced via `commit-msg` hook (`feat(...)`, `fix(...)`, `test(...)`).

---

### Task 1: Migration 00017 & SQL Queries (`audit_logs`, `contacts`, `deals`, `audit`, sqlc, demo querier)

**Files:**
- Create: `internal/db/migrations/00017_audit_logs_diff.sql`
- Modify: `internal/db/queries/contacts.sql`
- Modify: `internal/db/queries/deals.sql`
- Modify: `internal/db/queries/audit.sql`
- Modify: `internal/db/demo/querier.go`

**Interfaces:**
- Produces:
  - `db.CreateContactParams` with `ConsentPhone`, `ConsentEmail`, `ConsentUpdatedAt`, `StromverbrauchKwh`, `GasverbrauchKwh`, `Zaehlernummer`, `EigentuemerStatus`
  - `db.UpdateContactParams` with `ConsentPhone`, `ConsentEmail`, `ConsentUpdatedAt`, `StromverbrauchKwh`, `GasverbrauchKwh`, `Zaehlernummer`, `EigentuemerStatus`
  - `db.RevokeDealParams` with `ID`, `WiderrufenAt`, `WiderrufGrund`
  - `db.CreateAuditLogParams` with `OldValues`, `NewValues`

- [ ] **Step 1: Create migration `internal/db/migrations/00017_audit_logs_diff.sql`**

Write the migration adding `old_values` and `new_values` JSONB columns to `audit_logs`:

```sql
-- migrations/00017_audit_logs_diff.sql
-- +goose Up
ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS old_values JSONB,
    ADD COLUMN IF NOT EXISTS new_values JSONB;

-- +goose Down
ALTER TABLE audit_logs
    DROP COLUMN IF EXISTS new_values,
    DROP COLUMN IF EXISTS old_values;
```

- [ ] **Step 2: Update `internal/db/queries/contacts.sql`**

Replace `CreateContact` and `UpdateContact` in `internal/db/queries/contacts.sql` to persist all D1 fields:

```sql
-- name: CreateContact :one
INSERT INTO contacts (
    company_id,
    first_name,
    last_name,
    email,
    phone,
    mobile,
    position,
    lead_source,
    address_street,
    address_zip,
    address_city,
    latitude,
    longitude,
    consent_phone,
    consent_email,
    consent_updated_at,
    stromverbrauch_kwh,
    gasverbrauch_kwh,
    zaehlernummer,
    eigentuemer_status,
    custom_fields
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19, $20, $21
)
RETURNING *;

-- name: UpdateContact :one
UPDATE contacts
SET company_id = $2,
    first_name = $3,
    last_name = $4,
    email = $5,
    phone = $6,
    mobile = $7,
    position = $8,
    lead_source = $9,
    address_street = $10,
    address_zip = $11,
    address_city = $12,
    latitude = $13,
    longitude = $14,
    consent_phone = $15,
    consent_email = $16,
    consent_updated_at = $17,
    stromverbrauch_kwh = $18,
    gasverbrauch_kwh = $19,
    zaehlernummer = $20,
    eigentuemer_status = $21,
    custom_fields = $22,
    updated_at = NOW()
WHERE id = $1
RETURNING *;
```

- [ ] **Step 3: Update `internal/db/queries/deals.sql`**

Append `RevokeDeal` query to `internal/db/queries/deals.sql`:

```sql
-- name: RevokeDeal :one
UPDATE deals
SET stage = 'WIDERRUFEN',
    probability = 0,
    widerrufen_at = $2,
    widerruf_grund = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;
```

- [ ] **Step 4: Update `internal/db/queries/audit.sql`**

Update `CreateAuditLog` in `internal/db/queries/audit.sql`:

```sql
-- name: CreateAuditLog :one
INSERT INTO audit_logs (
    user_id,
    entity_type,
    entity_id,
    action,
    changes,
    old_values,
    new_values,
    ip_address,
    user_agent
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;
```

- [ ] **Step 5: Run SQLC generate**

Run:
```bash
/Users/ewenwitte/go/bin/sqlc generate
```
Expected: command exits 0, regenerating `internal/db/querier.go`, `internal/db/models.go`, `internal/db/contacts.sql.go`, `internal/db/deals.sql.go`, `internal/db/audit.sql.go`.

- [ ] **Step 6: Update `internal/db/demo/querier.go`**

Update `InMemoryQuerier` to implement the updated SQLC signatures:
1. `CreateContact`: populate `ConsentPhone`, `ConsentEmail`, `ConsentUpdatedAt`, `StromverbrauchKwh`, `GasverbrauchKwh`, `Zaehlernummer`, `EigentuemerStatus` from `arg`.
2. `UpdateContact`: update `ConsentPhone`, `ConsentEmail`, `ConsentUpdatedAt`, `StromverbrauchKwh`, `GasverbrauchKwh`, `Zaehlernummer`, `EigentuemerStatus` on the stored contact.
3. `CreateAuditLog`: populate `OldValues: arg.OldValues`, `NewValues: arg.NewValues`.
4. Add `RevokeDeal(ctx context.Context, arg db.RevokeDealParams) (db.Deal, error)`:
```go
func (q *InMemoryQuerier) RevokeDeal(ctx context.Context, arg db.RevokeDealParams) (db.Deal, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	idStr := uuidToStr(arg.ID)
	d, ok := q.deals[idStr]
	if !ok {
		return db.Deal{}, ErrNotFound
	}
	d.Stage = "WIDERRUFEN"
	d.Probability = 0
	d.WiderrufenAt = arg.WiderrufenAt
	d.WiderrufGrund = arg.WiderrufGrund
	d.UpdatedAt = nowTimestamptz()
	q.deals[idStr] = d
	return d, nil
}
```

- [ ] **Step 7: Verify compilation and tests**

Run:
```bash
go test ./internal/db/...
```
Expected: PASS.

- [ ] **Step 8: Commit Task 1**

Stage all updated database files (including `querier.go` and `models.go` to satisfy pre-commit hook):
```bash
git add internal/db/migrations/00017_audit_logs_diff.sql internal/db/queries/ internal/db/querier.go internal/db/models.go internal/db/*.sql.go internal/db/demo/querier.go
git commit -m "feat(db): add audit diff columns and wire consent, energy and revocation queries"
```

---

### Task 2: Audit Service Selective Diffs & PII Sanitization (D3 & F-52)

**Files:**
- Modify: `internal/core/audit/service.go`
- Modify: `internal/core/audit/service_test.go`

**Interfaces:**
- Produces:
  - `type DiffMap map[string]any`
  - `func (s *Service) LogDiff(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, oldValues, newValues DiffMap, ip, userAgent string) error`
  - `func SanitizeDiffValue(key string, val any) any`

- [ ] **Step 1: Write failing tests in `internal/core/audit/service_test.go`**

Add tests verifying `LogDiff`, selective diff recording, masking of phone/email PII, and preservation of UWG consent fields:

```go
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
	json.Unmarshal(entry.OldValues, &storedOld)
	json.Unmarshal(entry.NewValues, &storedNew)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/core/audit -run TestAuditService_LogDiff_PIISanitization`
Expected: FAIL (`svc.LogDiff undefined`).

- [ ] **Step 3: Implement `LogDiff` and `SanitizeDiffValue` in `internal/core/audit/service.go`**

```go
package audit

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type DiffMap map[string]any

type Service struct {
	queries db.Querier
}

func NewService(queries db.Querier) *Service {
	return &Service{queries: queries}
}

// SanitizeDiffValue masks sensitive PII (email, phone, auth tokens) while preserving
// exact legal evidence fields (consent_phone, consent_email, consent_updated_at).
func SanitizeDiffValue(key string, val any) any {
	if val == nil {
		return nil
	}
	lower := strings.ToLower(key)

	// Preserve legal compliance fields in full
	if strings.HasPrefix(lower, "consent_") {
		return val
	}

	// Mask phone numbers
	if strings.Contains(lower, "phone") || strings.Contains(lower, "mobile") || strings.Contains(lower, "telefon") {
		str, ok := val.(string)
		if !ok || len(str) < 4 {
			return "***"
		}
		return str[:2] + " *** " + str[len(str)-2:]
	}

	// Mask email addresses
	if strings.Contains(lower, "email") || strings.Contains(lower, "mail") {
		str, ok := val.(string)
		if !ok || !strings.Contains(str, "@") {
			return "***"
		}
		parts := strings.SplitN(str, "@", 2)
		if len(parts[0]) <= 1 {
			return "*@" + parts[1]
		}
		return parts[0][:1] + "***@" + parts[1]
	}

	// Redact security credentials
	if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") {
		return "[REDACTED]"
	}

	return val
}

func sanitizeMap(m DiffMap) DiffMap {
	if m == nil {
		return DiffMap{}
	}
	res := make(DiffMap, len(m))
	for k, v := range m {
		res[k] = SanitizeDiffValue(k, v)
	}
	return res
}

func (s *Service) LogDiff(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, oldValues, newValues DiffMap, ip, userAgent string) error {
	sanitizedOld := sanitizeMap(oldValues)
	sanitizedNew := sanitizeMap(newValues)

	oldBytes, err := json.Marshal(sanitizedOld)
	if err != nil {
		oldBytes = []byte("{}")
	}
	newBytes, err := json.Marshal(sanitizedNew)
	if err != nil {
		newBytes = []byte("{}")
	}

	// changes summary contains the delta keys
	diffKeys := make([]string, 0)
	for k := range sanitizedNew {
		diffKeys = append(diffKeys, k)
	}
	changesBytes, _ := json.Marshal(map[string]any{
		"action":       action,
		"changed_keys": diffKeys,
	})

	_, err = s.queries.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		Changes:    changesBytes,
		OldValues:  oldBytes,
		NewValues:  newBytes,
		IpAddress:  pgtype.Text{String: ip, Valid: ip != ""},
		UserAgent:  pgtype.Text{String: userAgent, Valid: userAgent != ""},
	})
	return err
}

func (s *Service) Log(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, changes any, ip, userAgent string) error {
	var changesBytes []byte
	var err error
	if changes != nil {
		changesBytes, err = json.Marshal(changes)
		if err != nil {
			changesBytes = []byte("{}")
		}
	} else {
		changesBytes = []byte("{}")
	}

	_, err = s.queries.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		Changes:    changesBytes,
		OldValues:  []byte("{}"),
		NewValues:  []byte("{}"),
		IpAddress:  pgtype.Text{String: ip, Valid: ip != ""},
		UserAgent:  pgtype.Text{String: userAgent, Valid: userAgent != ""},
	})
	return err
}
```

- [ ] **Step 4: Run audit tests to verify they pass**

Run: `go test -v ./internal/core/audit/...`
Expected: PASS.

- [ ] **Step 5: Commit Task 2**

```bash
git add internal/core/audit/
git commit -m "feat(audit): implement selective diff logging and PII masking with UWG consent retention"
```

---

### Task 3: Contact Core Service, Handlers & CSV Export (D1, F-52)

**Files:**
- Modify: `internal/core/contact/service.go`
- Modify: `internal/server/handlers/contacts.go`
- Modify: `internal/core/export/service.go`
- Modify: `internal/core/contact/service_test.go`
- Modify: `internal/server/handlers/contacts_test.go`
- Modify: `internal/core/export/service_test.go`

**Interfaces:**
- Consumes:
  - `db.CreateContactParams` / `db.UpdateContactParams`
  - `audit.Service.LogDiff`
- Produces:
  - Updated `CreateContactInput` and `UpdateContactInput` with consent & energy fields
  - Updated `CreateContactRequest` and `UpdateContactRequest`
  - `contact.Service.Update` emits sanitized diff via `LogDiff`
  - Updated `ExportContactsCSV` with all 17 standard columns

- [ ] **Step 1: Write failing tests in `internal/core/contact/service_test.go`**

Add tests verifying:
1. When creating contact with consent = true, `consent_updated_at` is set.
2. When updating contact and consent changes, `consent_updated_at` is updated to current time and audit log `CONSENT_UPDATE` is emitted.
3. When updating contact without changing consent, `consent_updated_at` is preserved.
4. General contact updates use `LogDiff` with masked phone/email while keeping consent fields unmasked.
5. Energy consumption metrics (`stromverbrauch_kwh`, `gasverbrauch_kwh`, `zaehlernummer`, `eigentuemer_status`) are persisted.
6. The existing `LookupPLZ` coordinate lookup logic is preserved and functional.

```go
func TestContactService_ConsentAndEnergyFields(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := contact.NewService(querier, auditService)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	strom := 4500.50
	gas := 12000.00

	// 1. Create with consent
	c1, err := svc.Create(ctx, actorID, contact.CreateContactInput{
		FirstName:         "Max",
		LastName:          "Mustermann",
		Email:             "max@mustermann.de",
		Phone:             "+49 69 112233",
		ConsentPhone:      true,
		ConsentEmail:      false,
		StromverbrauchKwh: &strom,
		GasverbrauchKwh:   &gas,
		Zaehlernummer:     "1EMH12345678",
		EigentuemerStatus: "EIGENTUEMER",
	})
	require.NoError(t, err)
	require.True(t, c1.ConsentPhone)
	require.False(t, c1.ConsentEmail)
	require.True(t, c1.ConsentUpdatedAt.Valid)
	initialConsentTime := c1.ConsentUpdatedAt.Time

	// Energy fields verified
	require.Equal(t, "1EMH12345678", c1.Zaehlernummer.String)
	require.Equal(t, "EIGENTUEMER", c1.EigentuemerStatus.String)

	// 2. Update without consent change -> consent_updated_at must remain identical, audit log must mask PII
	c2, err := svc.Update(ctx, actorID, contact.UpdateContactInput{
		ID:                c1.ID,
		FirstName:         "Maximilian",
		LastName:          "Mustermann",
		Email:             "max.neu@mustermann.de",
		Phone:             "+49 69 998877",
		ConsentPhone:      true,
		ConsentEmail:      false,
		Zaehlernummer:     "1EMH12345678",
		EigentuemerStatus: "EIGENTUEMER",
	})
	require.NoError(t, err)
	require.Equal(t, initialConsentTime, c2.ConsentUpdatedAt.Time)

	// Verify that the UPDATE audit log masked email and phone (F-52)
	logs, err := querier.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "CONTACT",
		EntityID:   c1.ID,
		Limit:      10,
	})
	require.NoError(t, err)
	require.True(t, len(logs) >= 2)
	var updateOld, updateNew map[string]any
	json.Unmarshal(logs[0].OldValues, &updateOld)
	json.Unmarshal(logs[0].NewValues, &updateNew)
	require.NotEqual(t, "max.neu@mustermann.de", updateNew["email"])
	require.NotEqual(t, "+49 69 998877", updateNew["phone"])

	// 3. Update with consent change -> consent_updated_at changes, audit log recorded
	time.Sleep(10 * time.Millisecond)
	c3, err := svc.Update(ctx, actorID, contact.UpdateContactInput{
		ID:                c1.ID,
		FirstName:         "Maximilian",
		LastName:          "Mustermann",
		Email:             "max.neu@mustermann.de",
		ConsentPhone:      true,
		ConsentEmail:      true, // Changed!
		Zaehlernummer:     "1EMH12345678",
		EigentuemerStatus: "EIGENTUEMER",
	})
	require.NoError(t, err)
	require.True(t, c3.ConsentEmail)
	require.True(t, c3.ConsentUpdatedAt.Time.After(initialConsentTime))

	// Verify CONSENT_UPDATE audit log
	logsAfter, _ := querier.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "CONTACT",
		EntityID:   c1.ID,
		Limit:      10,
	})
	foundConsentAudit := false
	for _, l := range logsAfter {
		if l.Action == "CONSENT_UPDATE" {
			foundConsentAudit = true
			var newV map[string]any
			json.Unmarshal(l.NewValues, &newV)
			require.Equal(t, true, newV["consent_email"])
			require.Equal(t, true, newV["consent_phone"])
			break
		}
	}
	require.True(t, foundConsentAudit, "expected CONSENT_UPDATE audit log entry")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/core/contact -run TestContactService_ConsentAndEnergyFields`
Expected: FAIL (fields not defined on `CreateContactInput`).

- [ ] **Step 3: Update `internal/core/contact/service.go`**

Preserve all existing PLZ geocoding logic, add consent and energy fields, handle automatic `consent_updated_at`, and replace the raw `s.audit.Log` on update with `s.audit.LogDiff` (sanitizing PII while preserving unmasked consent proof):

```go
type CreateContactInput struct {
	CompanyID         *pgtype.UUID
	FirstName         string
	LastName          string
	Email             string
	Phone             string
	Mobile            string
	Position          string
	LeadSource        string
	AddressStreet     string
	AddressZip        string
	AddressCity       string
	Latitude          *float64
	Longitude         *float64
	ConsentPhone      bool
	ConsentEmail      bool
	StromverbrauchKwh *float64
	GasverbrauchKwh   *float64
	Zaehlernummer     string
	EigentuemerStatus string
	CustomFields      []byte
}

type UpdateContactInput struct {
	ID                pgtype.UUID
	CompanyID         *pgtype.UUID
	FirstName         string
	LastName          string
	Email             string
	Phone             string
	Mobile            string
	Position          string
	LeadSource        string
	AddressStreet     string
	AddressZip        string
	AddressCity       string
	Latitude          *float64
	Longitude         *float64
	ConsentPhone      bool
	ConsentEmail      bool
	StromverbrauchKwh *float64
	GasverbrauchKwh   *float64
	Zaehlernummer     string
	EigentuemerStatus string
	CustomFields      []byte
}
```

In `Create`:
- Retain existing coordinate assignment and PLZ centroid fallback:
  ```go
	var lat, lon pgtype.Float8
	if input.Latitude != nil {
		lat = pgtype.Float8{Float64: *input.Latitude, Valid: true}
	}
	if input.Longitude != nil {
		lon = pgtype.Float8{Float64: *input.Longitude, Valid: true}
	}
	if input.Latitude == nil && input.Longitude == nil && strings.TrimSpace(input.AddressZip) != "" {
		if glat, glon, ok := geocoding.LookupPLZ(input.AddressZip); ok {
			lat = pgtype.Float8{Float64: glat, Valid: true}
			lon = pgtype.Float8{Float64: glon, Valid: true}
		}
	}
  ```
- Set `consent_updated_at`:
  ```go
  var consentUpdatedAt pgtype.Timestamptz
  if input.ConsentPhone || input.ConsentEmail {
      consentUpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
  }
  ```
- Convert numeric values:
  ```go
  var stromVal, gasVal pgtype.Numeric
  if input.StromverbrauchKwh != nil {
      _ = stromVal.Scan(fmt.Sprintf("%.2f", *input.StromverbrauchKwh))
  }
  if input.GasverbrauchKwh != nil {
      _ = gasVal.Scan(fmt.Sprintf("%.2f", *input.GasverbrauchKwh))
  }
  status := input.EigentuemerStatus
  if status == "" {
      status = "UNBEKANNT"
  }
  ```
- Pass to `s.queries.CreateContact`.

In `Update`:
- Retrieve existing contact via `s.queries.GetContactByID(ctx, input.ID)`.
- Retain existing coordinate assignment and PLZ centroid fallback.
- Detect consent change:
  ```go
  consentChanged := (input.ConsentPhone != existing.ConsentPhone) || (input.ConsentEmail != existing.ConsentEmail)
  consentUpdatedAt := existing.ConsentUpdatedAt
  if consentChanged {
      consentUpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
  }
  ```
- Pass to `s.queries.UpdateContact`.
- Emit masked audit diff via `s.audit.LogDiff`:
  ```go
  if s.audit != nil {
      var oldConsentTime any = nil
      if existing.ConsentUpdatedAt.Valid {
          oldConsentTime = existing.ConsentUpdatedAt.Time.Format(time.RFC3339)
      }
      var newConsentTime any = nil
      if consentUpdatedAt.Valid {
          newConsentTime = consentUpdatedAt.Time.Format(time.RFC3339)
      }

      oldVals := audit.DiffMap{
          "first_name":         existing.FirstName,
          "last_name":          existing.LastName,
          "email":              existing.Email.String,
          "phone":              existing.Phone.String,
          "mobile":             existing.Mobile.String,
          "position":           existing.Position.String,
          "address_street":     existing.AddressStreet.String,
          "address_zip":        existing.AddressZip.String,
          "address_city":       existing.AddressCity.String,
          "consent_phone":      existing.ConsentPhone,
          "consent_email":      existing.ConsentEmail,
          "consent_updated_at": oldConsentTime,
          "zaehlernummer":      existing.Zaehlernummer.String,
          "eigentuemer_status": existing.EigentuemerStatus.String,
      }
      newVals := audit.DiffMap{
          "first_name":         contact.FirstName,
          "last_name":          contact.LastName,
          "email":              contact.Email.String,
          "phone":              contact.Phone.String,
          "mobile":             contact.Mobile.String,
          "position":           contact.Position.String,
          "address_street":     contact.AddressStreet.String,
          "address_zip":        contact.AddressZip.String,
          "address_city":       contact.AddressCity.String,
          "consent_phone":      contact.ConsentPhone,
          "consent_email":      contact.ConsentEmail,
          "consent_updated_at": newConsentTime,
          "zaehlernummer":      contact.Zaehlernummer.String,
          "eigentuemer_status": contact.EigentuemerStatus.String,
      }

      action := "UPDATE"
      if consentChanged {
          action = "CONSENT_UPDATE"
      }
      if err := s.audit.LogDiff(ctx, actorID, "CONTACT", contact.ID, action, oldVals, newVals, "", ""); err != nil {
          log.Printf("[audit] failed to log contact update: %v", err)
      }
  }
  ```

- [ ] **Step 4: Update `internal/server/handlers/contacts.go`**

Update `CreateContactRequest` and `ContactHandler.Create` / `ContactHandler.Update` to parse and pass:
```go
type CreateContactRequest struct {
	FirstName         string   `json:"first_name"`
	LastName          string   `json:"last_name"`
	Email             string   `json:"email"`
	Phone             string   `json:"phone"`
	Mobile            string   `json:"mobile"`
	Position          string   `json:"position"`
	LeadSource        string   `json:"lead_source"`
	AddressStreet     string   `json:"address_street"`
	AddressZip        string   `json:"address_zip"`
	AddressCity       string   `json:"address_city"`
	Latitude          *float64 `json:"latitude"`
	Longitude         *float64 `json:"longitude"`
	ConsentPhone      bool     `json:"consent_phone"`
	ConsentEmail      bool     `json:"consent_email"`
	StromverbrauchKwh *float64 `json:"stromverbrauch_kwh"`
	GasverbrauchKwh   *float64 `json:"gasverbrauch_kwh"`
	Zaehlernummer     string   `json:"zaehlernummer"`
	EigentuemerStatus string   `json:"eigentuemer_status"`
}
```

- [ ] **Step 5: Update `internal/core/export/service.go`**

Update `ExportContactsCSV`:
```go
// Write CSV Header
_ = w.Write([]string{
    "ID",
    "Vorname",
    "Nachname",
    "Email",
    "Telefon",
    "Position",
    "Strasse",
    "PLZ",
    "Ort",
    "Einwilligung_Telefon",
    "Einwilligung_Email",
    "Einwilligung_Aktualisiert_Am",
    "Stromverbrauch_kWh",
    "Gasverbrauch_kWh",
    "Zaehlernummer",
    "Eigentuemer_Status",
    "Erstellt_Am",
})

for _, c := range contacts {
    consentTimeStr := ""
    if c.ConsentUpdatedAt.Valid {
        consentTimeStr = c.ConsentUpdatedAt.Time.Format("2006-01-02 15:04:05")
    }
    stromStr := ""
    if c.StromverbrauchKwh.Valid {
        stromVal, _ := c.StromverbrauchKwh.Float64Value()
        stromStr = fmt.Sprintf("%.2f", stromVal.Float64)
    }
    gasStr := ""
    if c.GasverbrauchKwh.Valid {
        gasVal, _ := c.GasverbrauchKwh.Float64Value()
        gasStr = fmt.Sprintf("%.2f", gasVal.Float64)
    }

    _ = w.Write([]string{
        sanitizeCSVField(c.ID.String()),
        sanitizeCSVField(c.FirstName),
        sanitizeCSVField(c.LastName),
        sanitizeCSVField(c.Email.String),
        sanitizeCSVField(c.Phone.String),
        sanitizeCSVField(c.Position.String),
        sanitizeCSVField(c.AddressStreet.String),
        sanitizeCSVField(c.AddressZip.String),
        sanitizeCSVField(c.AddressCity.String),
        fmt.Sprintf("%t", c.ConsentPhone),
        fmt.Sprintf("%t", c.ConsentEmail),
        consentTimeStr,
        stromStr,
        gasStr,
        sanitizeCSVField(c.Zaehlernummer.String),
        sanitizeCSVField(c.EigentuemerStatus.String),
        c.CreatedAt.Time.Format("2006-01-02 15:04:05"),
    })
}
```

- [ ] **Step 6: Update tests in `export_test.go` and `contacts_test.go`**

Ensure `internal/core/export/service_test.go` and `internal/server/handlers/contacts_test.go` assert all headers and fields.
Run:
```bash
go test -v ./internal/core/contact/... ./internal/core/export/... ./internal/server/handlers/... -run 'TestContact|TestExport'
```
Expected: PASS.

- [ ] **Step 7: Commit Task 3**

```bash
git add internal/core/contact/ internal/core/export/ internal/server/handlers/
git commit -m "feat(contact): persist UWG consent and energy data with masked audit diffs and CSV export"
```

---

### Task 4: Deal Revocation Core, Handler & Reports (D2)

**Files:**
- Modify: `internal/core/deal/service.go`
- Modify: `internal/server/handlers/deals.go`
- Modify: `internal/server/router.go`
- Modify: `internal/core/deal/service_test.go`
- Modify: `internal/server/handlers/deals_test.go`
- Modify: `internal/core/reports/service_test.go`

**Interfaces:**
- Consumes:
  - `db.RevokeDealParams`
  - `audit.Service.Log`
- Produces:
  - `func (s *Service) Revoke(ctx context.Context, actorID pgtype.UUID, id pgtype.UUID, widerrufenAt time.Time, grund string) (db.Deal, error)`
  - Rejection of `stage == "WIDERRUFEN"` in `DealHandler.Create` and `DealHandler.Update`
  - Route: `dr.With(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE")).Post("/{id}/revoke", dealH.Revoke)`

- [ ] **Step 1: Write failing tests in `internal/core/deal/service_test.go`, `internal/server/handlers/deals_test.go`, and `internal/core/reports/service_test.go`**

Add tests:
1. `TestDealService_Revoke`: verifies deal stage becomes `WIDERRUFEN`, probability becomes 0, `widerrufen_at` and `widerruf_grund` are recorded.
2. `TestDealHandler_Create_RejectsWiderrufenStage`: verifies creating a deal with `Stage: "WIDERRUFEN"` returns 400 Bad Request.
3. `TestDealHandler_Update_RejectsWiderrufenStage`: verifies updating a deal with `Stage: "WIDERRUFEN"` returns 400 Bad Request.
4. `TestDealHandler_Revoke`: verifies `POST /api/v1/deals/{id}/revoke` endpoint with Chi router, role checks (403 for `BENUTZER`, 200 for `VERTRIEB`/`ADMIN`), and persistence of `widerrufen_at` and `widerruf_grund`.
5. `TestReportsService_RevokedDealsCount`: creates a revoked deal and verifies `report.ConversionStats.RevokedDeals == 1`.

```go
func TestDealService_Revoke(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(querier)
	svc := deal.NewService(querier, auditSvc)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	d, err := svc.Create(ctx, actorID, deal.CreateDealInput{
		Title:       "Photovoltaikanlage 10 kWp",
		Stage:       "WON",
		Probability: 100,
	})
	require.NoError(t, err)

	revokedAt := time.Now().UTC()
	reason := "Kunde widerruft Haustürgeschäft fristgerecht (§355 BGB)"
	revoked, err := svc.Revoke(ctx, actorID, d.ID, revokedAt, reason)
	require.NoError(t, err)
	require.Equal(t, "WIDERRUFEN", revoked.Stage)
	require.Equal(t, int32(0), revoked.Probability)
	require.True(t, revoked.WiderrufenAt.Valid)
	require.Equal(t, reason, revoked.WiderrufGrund.String)
}

func TestReportsService_RevokedDealsCount(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(querier)
	dealSvc := deal.NewService(querier, auditSvc)
	reportsSvc := reports.NewService(querier)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	d, err := dealSvc.Create(ctx, actorID, deal.CreateDealInput{
		Title:       "BGB Widerruf Test Deal",
		Stage:       "WON",
		Probability: 100,
	})
	require.NoError(t, err)

	_, err = dealSvc.Revoke(ctx, actorID, d.ID, time.Now().UTC(), "Fristgerechter Widerruf")
	require.NoError(t, err)

	report, err := reportsSvc.GetSalesReport(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, report.ConversionStats.RevokedDeals)
}

func TestDealHandler_Revoke(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	dealService := deal.NewService(querier, auditService)
	dealH := handlers.NewDealHandler(dealService)

	adminUUID := uuid.New()
	benutzerUUID := uuid.New()

	// Seed deal
	d, err := dealService.Create(context.Background(), pgtype.UUID{Bytes: adminUUID, Valid: true}, deal.CreateDealInput{
		Title:       "PV Anlage Widerruf Test",
		Stage:       "WON",
		Probability: 100,
	})
	require.NoError(t, err)
	dealIDStr := uuid.UUID(d.ID.Bytes).String()

	r := chi.NewRouter()
	r.Route("/deals", func(dr chi.Router) {
		dr.With(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE")).Post("/{id}/revoke", dealH.Revoke)
	})

	revokeBody, _ := json.Marshal(handlers.RevokeDealRequest{
		WiderrufGrund: "Fristgerechter Widerruf nach Haustürgeschäft",
	})

	// 1. BENUTZER role receives 403 Forbidden
	reqBenutzer := httptest.NewRequest("POST", "/deals/"+dealIDStr+"/revoke", bytes.NewReader(revokeBody))
	reqBenutzer = reqBenutzer.WithContext(context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: benutzerUUID,
		Role:   "BENUTZER",
	}))
	recBenutzer := httptest.NewRecorder()
	r.ServeHTTP(recBenutzer, reqBenutzer)
	require.Equal(t, http.StatusForbidden, recBenutzer.Code)

	// 2. ADMIN role succeeds with 200 OK
	reqAdmin := httptest.NewRequest("POST", "/deals/"+dealIDStr+"/revoke", bytes.NewReader(revokeBody))
	reqAdmin = reqAdmin.WithContext(context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: adminUUID,
		Role:   "ADMIN",
	}))
	recAdmin := httptest.NewRecorder()
	r.ServeHTTP(recAdmin, reqAdmin)
	require.Equal(t, http.StatusOK, recAdmin.Code)

	var revoked db.Deal
	_ = json.NewDecoder(recAdmin.Body).Decode(&revoked)
	require.Equal(t, "WIDERRUFEN", revoked.Stage)
	require.Equal(t, int32(0), revoked.Probability)
	require.True(t, revoked.WiderrufenAt.Valid)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./internal/core/deal -run TestDealService_Revoke`
Expected: FAIL (`svc.Revoke undefined`).

- [ ] **Step 3: Implement `Revoke` in `internal/core/deal/service.go`**

```go
func (s *Service) Revoke(ctx context.Context, actorID pgtype.UUID, id pgtype.UUID, widerrufenAt time.Time, grund string) (db.Deal, error) {
	if widerrufenAt.IsZero() {
		widerrufenAt = time.Now().UTC()
	}

	dealItem, err := s.queries.RevokeDeal(ctx, db.RevokeDealParams{
		ID:            id,
		WiderrufenAt:  pgtype.Timestamptz{Time: widerrufenAt, Valid: true},
		WiderrufGrund: pgtype.Text{String: grund, Valid: grund != ""},
	})
	if err != nil {
		return db.Deal{}, err
	}

	if s.audit != nil {
		if err := s.audit.Log(ctx, actorID, "DEAL", dealItem.ID, "REVOKE", map[string]any{
			"stage":          "WIDERRUFEN",
			"widerrufen_at":  widerrufenAt,
			"widerruf_grund": grund,
		}, "", ""); err != nil {
			log.Printf("[audit] failed to log deal revocation: %v", err)
		}
	}

	return dealItem, nil
}
```

- [ ] **Step 4: Implement `Revoke` handler and reject direct `WIDERRUFEN` stage in Create and Update in `internal/server/handlers/deals.go` & register route in `internal/server/router.go`**

In `internal/server/handlers/deals.go`:
```go
type RevokeDealRequest struct {
	WiderrufenAt *time.Time `json:"widerrufen_at"`
	WiderrufGrund string     `json:"widerruf_grund"`
}

func (h *DealHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}

	idStr := chi.URLParam(r, "id")
	var dealID pgtype.UUID
	if err := dealID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	existing, err := h.service.GetByID(r.Context(), dealID)
	if err != nil {
		http.Error(w, `{"error":"deal_not_found"}`, http.StatusNotFound)
		return
	}

	if claims != nil && claims.Role != "ADMIN" {
		if !existing.AssignedTo.Valid || existing.AssignedTo.Bytes != claims.UserID {
			http.Error(w, `{"error":"forbidden","message":"Sie können nur Ihnen zugewiesene Deals widerrufen"}`, http.StatusForbidden)
			return
		}
	}

	var req RevokeDealRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	wAt := time.Now().UTC()
	if req.WiderrufenAt != nil && !req.WiderrufenAt.IsZero() {
		wAt = *req.WiderrufenAt
	}

	updated, err := h.service.Revoke(r.Context(), actorID, dealID, wAt, req.WiderrufGrund)
	if err != nil {
		http.Error(w, `{"error":"failed to revoke deal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}
```

In `DealHandler.Create`:
Reject direct initialization with `WIDERRUFEN`:
```go
if strings.EqualFold(req.Stage, "WIDERRUFEN") || strings.EqualFold(req.Stage, "REVOKED") {
    http.Error(w, `{"error":"invalid_stage","message":"Stage 'WIDERRUFEN' kann nicht direkt beim Anlegen gewählt werden"}`, http.StatusBadRequest)
    return
}
```

In `DealHandler.Update`:
Reject direct transition to `WIDERRUFEN`:
```go
if strings.EqualFold(req.Stage, "WIDERRUFEN") || strings.EqualFold(req.Stage, "REVOKED") {
    http.Error(w, `{"error":"invalid_stage","message":"Stage 'WIDERRUFEN' kann nur über den Widerrufsdialog (§ 355 BGB) mit Datum und Grund erfasst werden"}`, http.StatusBadRequest)
    return
}
```

In `internal/server/router.go:355`:
Under `protected.Route("/deals", func(dr chi.Router) {`:
Add the role-guarded route:
```go
dr.With(auth.RequireAnyRole("ADMIN", "VERTRIEB", "BACKOFFICE")).Post("/{id}/revoke", dealH.Revoke)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -v ./internal/core/deal/... ./internal/core/reports/... ./internal/server/handlers/... -run 'TestDeal|TestReports'`
Expected: PASS.

- [ ] **Step 6: Commit Task 4**

```bash
git add internal/core/deal/ internal/core/reports/ internal/server/handlers/deals.go internal/server/handlers/deals_test.go internal/server/router.go
git commit -m "feat(deal): add §355 BGB deal revocation endpoint and reject direct stage creation and update"
```

---

### Task 5: Frontend UI Updates (D1 & D2)

**Files:**
- Modify: `web/src/pages/DealsPage.tsx`
- Modify: `web/src/pages/ContactsPage.tsx`

**Interfaces:**
- Produces:
  - `STAGES` includes `WIDERRUFEN` (column in Kanban board)
  - Card select, Create Deal select, and Edit Deal select exclude `WIDERRUFEN` (or card select triggers modal on select)
  - Dragging to `WIDERRUFEN` column triggers modal
  - Dedicated "Widerruf erfassen" action and modal
  - Contact edit modal includes UWG consent checkboxes and energy fields

- [ ] **Step 1: Update `web/src/pages/DealsPage.tsx`**

1. Add `widerrufen_at?: string; widerruf_grund?: string;` to `interface Deal`.
2. Add `WIDERRUFEN` to `STAGES`:
```ts
  {
    id: 'WIDERRUFEN',
    label: 'Widerrufen (§355 BGB)',
    color: 'border-orange-500/30 bg-orange-500/5 text-orange-400',
  },
```
3. Update `handleDragEnd`:
If a deal is dropped onto `WIDERRUFEN`, open the revocation modal instead of calling `updateMutation`:
```ts
if (newStage === 'WIDERRUFEN') {
  setRevokeModalDeal(deal);
  return;
}
```
4. Exclude `WIDERRUFEN` from:
   - Create Deal modal select (`DealsPage.tsx:453`):
     ```tsx
     {STAGES.filter((s) => s.id !== 'WIDERRUFEN').map((s) => (
       <option key={s.id} value={s.id}>
         {s.label}
       </option>
     ))}
     ```
   - Card stage transition selector (`DealsPage.tsx:344`):
     ```tsx
     <select
       value={deal.stage}
       onChange={(e) => {
         if (e.target.value === 'WIDERRUFEN') {
           setRevokeModalDeal(deal);
           return;
         }
         updateMutation.mutate({ ...deal, stage: e.target.value });
       }}
       className="text-[11px] bg-slate-900 border border-slate-800 text-slate-300 rounded px-2 py-1 focus:outline-none"
     >
       {deal.stage === 'WIDERRUFEN' && (
         <option value="WIDERRUFEN">Widerrufen (§355 BGB)</option>
       )}
       {STAGES.filter((s) => s.id !== 'WIDERRUFEN').map((s) => (
         <option key={s.id} value={s.id}>
           {s.label}
         </option>
       ))}
     </select>
     ```
   - Edit Deal modal select (`DealsPage.tsx:510`):
     ```tsx
     {STAGES.filter((s) => s.id !== 'WIDERRUFEN').map((s) => (
       <option key={s.id} value={s.id}>
         {s.label}
       </option>
     ))}
     ```
5. Add `revokeMutation`:
```ts
  const revokeMutation = useMutation({
    mutationFn: async ({ id, widerruf_grund }: { id: string; widerruf_grund: string }) => {
      return apiFetch(`/api/v1/deals/${id}/revoke`, {
        method: 'POST',
        body: JSON.stringify({
          widerruf_grund,
          widerrufen_at: new Date().toISOString(),
        }),
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deals'] });
      queryClient.invalidateQueries({ queryKey: ['sales-report'] });
      setRevokeModalDeal(null);
    },
  });
```
6. Add "Widerruf erfassen" button in deal card actions and render Widerruf modal allowing entry of revocation reason and timestamp.
7. Display badge on deals with `stage === 'WIDERRUFEN'` showing revocation reason and date.

- [ ] **Step 2: Update `web/src/pages/ContactsPage.tsx`**

1. In the edit modal (`editingContact`), add UWG Consent checkboxes (`consent_email`, `consent_phone`) and energy fields (`stromverbrauch_kwh`, `gasverbrauch_kwh`, `eigentuemer_status`).
2. Verify saving an edited contact sends all fields in `updateMutation`.

- [ ] **Step 3: Run web checks**

Run:
```bash
cd web && pnpm lint && pnpm typecheck && pnpm build && cd ..
```
Expected: All checks exit 0 without errors or warnings.

- [ ] **Step 4: Commit Task 5**

```bash
git add web/src/pages/DealsPage.tsx web/src/pages/ContactsPage.tsx
git commit -m "feat(web): add Widerrufen stage and modal to Deals and expose consent editing on Contacts"
```

---

### Task 6: Verification & Quality Gates

**Files:**
- Test all components end-to-end.

- [ ] **Step 1: Run complete Go test suite with race detector**

Run:
```bash
go test -race -v ./...
```
Expected: PASS across all packages.

- [ ] **Step 2: Run coverage script gate**

Run:
```bash
./scripts/coverage.sh --min 50.0 --diff 50.0
```
Expected: Total coverage ≥ 50.0% and diff coverage ≥ 50.0%.

- [ ] **Step 3: Run canonical local gate `make check`**

Run:
```bash
make check
```
Expected: fmt, lint, check-licenses, and tests all pass cleanly.

- [ ] **Step 4: Verify migrations symlink invariant**

Run:
```bash
git status
test -L migrations
ls -l migrations
```
Expected: `migrations` is a valid symlink to `internal/db/migrations`.
