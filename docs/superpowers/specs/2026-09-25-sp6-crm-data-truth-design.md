# Sub-Project 6 (SP6): CRM Data Truth & UWG/BGB Compliance Design Spec

**Status:** Approved
**Target Branch:** `main` @ `55ae299`
**Findings Addressed:** D1, D2, D3, F-52
**Parent Document:** `docs/superpowers/specs/2026-09-21-fake-feature-elimination-design.md` §10

---

## 1. Purpose and Baseline

SP6 establishes data truth for contact consent and energy data, §355 BGB deal revocation, privacy-preserving audit history, and revocation reporting. This document describes the approved behavior on `main` at `55ae299`; it does not reopen completed implementation work or introduce another feature branch.

The completed implementation includes:

- migration `00017_audit_logs_diff.sql` and regenerated sqlc bindings;
- contact consent and energy persistence across core, HTTP, demo, and CSV paths;
- selective audit diffs and recursive PII masking;
- the guarded deal-revocation endpoint and terminal revoked state;
- contact consent/energy editing and the deals revocation workflow.

## 2. Contact Consent and Energy Data

### 2.1 Data Model

The contact consent and energy columns originate in `00004_energy_d2d_and_consent.sql` and remain unchanged by SP6:

- `consent_phone BOOLEAN NOT NULL DEFAULT FALSE`;
- `consent_email BOOLEAN NOT NULL DEFAULT FALSE`;
- `consent_updated_at TIMESTAMPTZ`, nullable;
- `stromverbrauch_kwh NUMERIC(10,2)`, nullable;
- `gasverbrauch_kwh NUMERIC(10,2)`, nullable;
- `zaehlernummer VARCHAR(100)`, nullable;
- `eigentuemer_status VARCHAR(50) DEFAULT 'UNBEKANNT'`, a text field, not an integer type.

The generated contact model uses `pgtype.Numeric` for both consumption values and `pgtype.Text` for `eigentuemer_status`. SP6 does not add a migration backfill or retype these columns.

### 2.2 End-to-End Propagation

Consent and energy values flow through all of these paths:

1. `contact.CreateContactInput` and `contact.UpdateContactInput`;
2. `CreateContact` and `UpdateContact` SQL queries and regenerated sqlc parameters;
3. contact HTTP request decoding and handler-to-service mapping;
4. the in-memory demo querier;
5. contact CSV export.

The lead-intake connector uses `contact.OptionalBool` for `consent_phone` and `consent_email`, so omitted, explicit `false`, and explicit `true` values retain their JSON presence semantics when passed to contact creation.

SP6 does not change the existing `LookupPLZ` fallback in contact create or update.

### 2.3 Consent Timestamp Semantics

- Contact creation sets `consent_updated_at` to the current UTC time only when at least one resolved consent flag is `true`.
- Contact update resolves omitted consent fields to the existing values.
- `consent_updated_at` changes only when a resolved `consent_phone` or `consent_email` value actually changes.
- If consent remains unchanged, the previous `consent_updated_at` is preserved, including `NULL` when no timestamp has been recorded.
- A missing or zero timestamp is never converted into a valid `0001-01-01` PostgreSQL timestamp.
- Explicit JSON `null` for a consent boolean is invalid and returns HTTP 400.

The update audit action is `CONSENT_UPDATE` when consent changes and `UPDATE` otherwise.

### 2.4 Partial Updates

Omitted update fields do not erase stored consent or energy values:

- omitted `consent_phone` or `consent_email` preserves the existing boolean;
- omitted `stromverbrauch_kwh` or `gasverbrauch_kwh` preserves the existing numeric value;
- explicit JSON `null` for an energy value clears it;
- omitted `eigentuemer_status` preserves the existing text value;
- explicit JSON `null` or an empty owner status clears it;
- an empty `zaehlernummer` on update preserves an existing meter number.

The custom JSON types distinguish an omitted field (`Present == false`) from an explicit JSON `null` (`Present == true, Null == true`).

### 2.5 CSV Export

`ExportContactsCSV` writes exactly 17 columns:

1. `ID`
2. `Vorname`
3. `Nachname`
4. `Email`
5. `Telefon`
6. `Position`
7. `Strasse`
8. `PLZ`
9. `Ort`
10. `Einwilligung_Telefon`
11. `Einwilligung_Email`
12. `Einwilligung_Aktualisiert_Am`
13. `Stromverbrauch_kWh`
14. `Gasverbrauch_kWh`
15. `Zaehlernummer`
16. `Eigentuemer_Status`
17. `Erstellt_Am`

The export uses the stored values. Consent timestamps use their stored value, valid energy values are formatted with two decimals, and null values are exported as empty CSV fields.

## 3. Audit Logging and PII Masking

### 3.1 Schema

Migration `00017_audit_logs_diff.sql` adds nullable `old_values JSONB` and `new_values JSONB` columns to `audit_logs`. It contains no data backfill. The `migrations` symlink remains intact.

### 3.2 Selective Diff Logging

`audit.Service.LogDiff` is used when old and new values are available. It:

- sanitizes values in both maps;
- stores the sanitized maps in `old_values` and `new_values`;
- stores only the action and sorted changed field names in `changes`;
- masks `phone`, `mobile`, and email-like fields recursively;
- redacts password, secret, and token values;
- preserves `consent_phone`, `consent_email`, and `consent_updated_at` unmasked as UWG evidence.

Contact update uses `LogDiff`. General `Log` calls recursively sanitize the complete `changes` blob and always store `{}` for both `old_values` and `new_values`; `Log` does not manufacture selective diffs.

Contact and deal create/delete paths use `Log`, while contact update, deal update, stage update, and revocation paths use `LogDiff` where old and new values are available.

## 4. §355 BGB Deal Revocation

### 4.1 API and Service Boundary

`POST /api/v1/deals/{id}/revoke` is the only path that enters the revoked state. It is restricted to `ADMIN`, `VERTRIEB`, and `BACKOFFICE` by the router.

The request accepts:

- optional `widerrufen_at`, defaulting to the current UTC time when omitted or zero;
- optional `widerruf_grund`.

The service records the timestamp and reason, sets the stage to `WIDERRUFEN`, and sets probability to zero. Repeating revocation returns the already-revoked deal without replacing the original revocation evidence.

Create and update reject both `WIDERRUFEN` and `REVOKED` as direct input with HTTP 400. The SQL update and delete boundaries and the service mutation checks make `WIDERRUFEN` terminal.

### 4.2 Deals Frontend

`web/src/pages/DealsPage.tsx` treats `WIDERRUFEN` as a dedicated Kanban stage:

- the stage has its own visible column;
- create, edit, and card stage selectors do not offer `WIDERRUFEN` as a normal transition;
- an already-revoked card may display its current value but remains terminal and disabled;
- dragging a card into the `WIDERRUFEN` column opens the revocation modal instead of calling the normal update mutation;
- `Widerruf erfassen` opens the same modal for direct revocation;
- the modal submits the selected date and reason to the revoke endpoint;
- `revokeMutation.onError` stores the error in modal state and leaves the modal open;
- successful revocation invalidates both the `deals` and `sales-report` query keys so the pipeline and report do not retain stale cached counts.

`web/src/pages/ContactsPage.tsx` exposes consent checkboxes, electricity and gas consumption, meter number, and owner status in both contact creation and contact editing. Nullable values are displayed and submitted without converting missing data to zero.

## 5. Revocation Reporting

`internal/core/reports/service.go` counts a deal in `ConversionStats.RevokedDeals` when:

- `widerrufen_at` is set; or
- the stage is `WIDERRUFEN`; or
- the legacy stage alias `REVOKED` is present.

Revoked deals are excluded from won/lost conversion and forecast aggregation. The backend computes the count; `ReportsPage.tsx` fetches and renders the returned value.

The `+4.2%` text in `ReportsPage.tsx` belongs to S5 and is outside SP6. SP6 does not change that display text.

## 6. Explicit Non-Goals

SP6 does not include:

- a migration backfill;
- changes to `LookupPLZ` in contact create or update;
- integer typing or a migration retype for energy or owner-status fields;
- connector system-user provisioning or broader audit-actor policy;
- transfer-framing hardening unrelated to the approved revocation contract;
- changes to the S5 `+4.2%` report text;
- unrelated product or UI features.

## 7. Verification Contract

Task 6 is complete only after all of the following pass from `main`:

```bash
cd web && pnpm format && pnpm lint && pnpm typecheck && pnpm build
go test -race ./...
./scripts/coverage.sh --min 50.0 --diff 50.0
make check
test -L migrations && ls -l migrations
```

Coverage must remain at least 50% overall and at least 50% for changed lines. The migrations symlink must continue to point to `internal/db/migrations`. No push is part of this work.
