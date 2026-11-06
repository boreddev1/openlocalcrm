# SP5: Calendar, Telephony & Geocoding Implementation Plan (C1–C4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate fake M365/Google cloud sync, harden RFC 5545 `.ics` export with ownership enforcement and text escaping, make telephony call logging honest with native `tel:` links, and introduce bundled CC0 German 5-digit PLZ centroid geocoding with an honest MapView without random coordinate jitter.

**Architecture:** 
- The calendar operates honestly as a self-hosted local CRM calendar with RFC 5545 export via `GET /api/v1/appointments/{id}/ics` (enforcing ownership: non-admin users only download their assigned appointments, missing IDs return 404).
- Telephony removes fake `call-<nanos>` synthesis so DB errors propagate honestly as HTTP 500, with UI native `tel:` links and visible error reporting.
- Offline geocoding uses an embedded CC0 German 5-digit postal code centroid dataset (`internal/geocoding`) integrated into `contact.Service` (auto-populating coordinates only when both latitude and longitude are nil).
- MapView renders markers only for contacts with genuine coordinates, honestly segregating contacts without coordinates into a dedicated "Kontakte ohne Standort" panel.

**Tech Stack:** Go 1.26, Chi v5, PostgreSQL 16, pgx/v5, React 18, Leaflet / react-leaflet, TypeScript, Tailwind CSS.

## Global Constraints
- All Go tests must pass via `go test ./...`.
- Coverage gate: `scripts/coverage.sh --min 50.0 --diff 50.0` (≥ 50% diff coverage, ≥ 50% total coverage).
- Zero-Fake-Success: Missing records return 404, DB errors return 500, no synthetic fallback appointments or calls, no fake random coordinate jitter.
- The `migrations` symlink (`migrations -> internal/db/migrations`) must remain untouched.
- Conventional Commits without co-author lines.
- Geocoding dataset must be strictly CC0 / Open Data with documented provenance (no ODbL/CC-BY).
- Exact 5-digit PLZ matching only (no 2-digit regional fallback).

---

## Part 1: Calendar & RFC 5545 ICS (C1, C2)

### Task 1: Calendar Backend: Eliminate Fake External Push & Harden RFC 5545 ICS with Ownership (C1, C2)

**Files:**
- Modify: `internal/server/router.go`
- Modify: `internal/server/handlers/appointments.go`
- Modify: `internal/core/appointment/service.go`
- Modify: `internal/db/demo/querier.go`
- Modify: `internal/server/handlers/appointments_test.go`
- Modify: `internal/core/appointment/service_test.go`

**Interfaces:**
- Removes:
  - Route `POST /api/v1/appointments/{id}/push-external`
  - `(*handlers.AppointmentHandler).PushExternal(w http.ResponseWriter, r *http.Request)`
  - `(*appointment.Service).PushExternal(ctx context.Context, id string) error`
- Modifies:
  - `(*handlers.AppointmentHandler).DownloadICS(w http.ResponseWriter, r *http.Request)`:
    - 404 on missing appointment.
    - 500 on database error.
    - 403 on non-admin user attempting to download an appointment assigned to another user (`claims.Role != "ADMIN"` and `app.AssignedTo != "" && app.AssignedTo != claims.Email && app.AssignedTo != claims.UserID.String()`).
  - `(*appointment.Service).GenerateICS(app Appointment) string`:
    - `METHOD:PUBLISH`
    - RFC 5545 text escaping (`\`, `;`, `,`, newlines) on `SUMMARY`, `LOCATION`, `DESCRIPTION`.
  - `internal/db/demo/querier.go`:
    - `app1ID`: Title `"Internes Vertriebs-Meeting"`, `Location: ""`, `IcsUid: "crm-int-1"`, `Provider: ""`, `IsExternal: false`, `IsPushed: false`
    - `app2ID`: Title `"Privater Termin / Facharzt"`, `Location: ""`, `IcsUid: "crm-int-2"`, `Provider: ""`, `IsExternal: false`, `IsPushed: false`

- [ ] **Step 1: Write failing tests in `internal/server/handlers/appointments_test.go` and `internal/core/appointment/service_test.go` (RED)**

In `internal/server/handlers/appointments_test.go`:
```go
func TestAppointmentHandler_DownloadICS_NotFoundReturns404(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := appointment.NewService(q, nil)
	h := handlers.NewAppointmentHandler(svc)

	req := httptest.NewRequest("GET", "/api/v1/appointments/00000000-0000-0000-0000-000000000099/ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "00000000-0000-0000-0000-000000000099")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.DownloadICS(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code, "missing appointment must return 404, never fabricate a fallback")
}

func TestAppointmentHandler_DownloadICS_EnforcesOwnership(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := appointment.NewService(q, nil)
	h := handlers.NewAppointmentHandler(svc)

	// Seed appointment assigned to other user
	app, err := svc.Create(context.Background(), pgtype.UUID{}, appointment.CreateAppointmentInput{
		Title:      "Geheimes Strategiegespräch",
		StartTime:  time.Now().Add(24 * time.Hour),
		EndTime:    time.Now().Add(25 * time.Hour),
		AssignedTo: "andere.person@example.de",
	})
	require.NoError(t, err)

	// Non-admin user attempts download
	claims := &auth.AccessClaims{
		UserID: uuid.New(),
		Email:  "hacker@example.de",
		Role:   "VERTRIEB",
	}
	req := httptest.NewRequest("GET", "/api/v1/appointments/"+app.ID+"/ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, auth.UserContextKey, claims)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.DownloadICS(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code, "non-admin cannot download appointment assigned to someone else")
}
```

In `internal/core/appointment/service_test.go`:
```go
func TestAppointmentService_GenerateICS_EscapesSpecialCharactersAndUsesPublish(t *testing.T) {
	svc := appointment.NewService(nil, nil)
	app := appointment.Appointment{
		ID:        "test-app-1",
		Title:     "Treffen mit Müller, Meier; Schmidt",
		Location:  "Büro: Raum 1.02; Nebengebäude",
		Notes:     "Zeile 1\nZeile 2, mit Komma; und Semikolon",
		StartTime: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC),
		ICSUID:    "test-uid-123@openlocalcrm",
	}

	ics := svc.GenerateICS(app)
	require.Contains(t, ics, "METHOD:PUBLISH")
	require.NotContains(t, ics, "METHOD:REQUEST")
	require.Contains(t, ics, "SUMMARY:Treffen mit Müller\\, Meier\\; Schmidt")
	require.Contains(t, ics, "LOCATION:Büro: Raum 1.02\\; Nebengebäude")
	require.Contains(t, ics, "DESCRIPTION:Zeile 1\\nZeile 2\\, mit Komma\\; und Semikolon")
}
```

- [ ] **Step 2: Run tests to verify they fail (RED)**

Run: `go test -v ./internal/core/appointment -run TestAppointmentService_GenerateICS`
Expected: FAIL (`METHOD:REQUEST` instead of `METHOD:PUBLISH`, unescaped text)
Run: `go test -v ./internal/server/handlers -run TestAppointmentHandler_DownloadICS`
Expected: FAIL (returns 200 with fabricated appointment instead of 404 / 403)

- [ ] **Step 3: Implement RFC 5545 escaping and METHOD:PUBLISH in `internal/core/appointment/service.go` (GREEN)**

In `internal/core/appointment/service.go`:
```go
func escapeICS(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, "\r\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\n`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// GenerateICS formats an appointment as an RFC 5545 iCalendar string
func (s *Service) GenerateICS(app Appointment) string {
	dtFormat := "20060102T150405Z"
	nowStr := time.Now().UTC().Format(dtFormat)
	startStr := app.StartTime.UTC().Format(dtFormat)
	endStr := app.EndTime.UTC().Format(dtFormat)

	uid := app.ICSUID
	if uid == "" {
		uid = app.ID + "@openlocalcrm.local"
	}

	return fmt.Sprintf("BEGIN:VCALENDAR\r\n"+
		"VERSION:2.0\r\n"+
		"PRODID:-//OpenLocalCRM//DE\r\n"+
		"CALSCALE:GREGORIAN\r\n"+
		"METHOD:PUBLISH\r\n"+
		"BEGIN:VEVENT\r\n"+
		"UID:%s\r\n"+
		"DTSTAMP:%s\r\n"+
		"DTSTART:%s\r\n"+
		"DTEND:%s\r\n"+
		"SUMMARY:%s\r\n"+
		"LOCATION:%s\r\n"+
		"DESCRIPTION:%s\r\n"+
		"STATUS:CONFIRMED\r\n"+
		"END:VEVENT\r\n"+
		"END:VCALENDAR\r\n",
		uid,
		nowStr,
		startStr,
		endStr,
		escapeICS(app.Title),
		escapeICS(app.Location),
		escapeICS(app.Notes),
	)
}
```
Remove `PushExternal` method from `appointment.Service`.

- [ ] **Step 4: Update `DownloadICS` and remove `PushExternal` in `internal/server/handlers/appointments.go` (GREEN)**

In `internal/server/handlers/appointments.go`:
```go
func (h *AppointmentHandler) DownloadICS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, appointment.ErrAppointmentNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(strings.ToLower(err.Error()), "no rows") {
			http.Error(w, `{"error":"not_found","message":"Termin nicht gefunden"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to get appointment"}`, http.StatusInternalServerError)
		return
	}

	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims != nil && claims.Role != "ADMIN" {
		if app.AssignedTo != "" && app.AssignedTo != claims.Email && app.AssignedTo != claims.UserID.String() {
			http.Error(w, `{"error":"forbidden","message":"Sie können nur Ihre eigenen Termine als ICS exportieren"}`, http.StatusForbidden)
			return
		}
	}

	icsContent := h.svc.GenerateICS(app)

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="termin-%s.ics"`, app.ID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(icsContent))
}
```
Remove `PushExternal` method from `AppointmentHandler`.

- [ ] **Step 5: Remove Route in `internal/server/router.go` and clean Demo Seeds in `internal/db/demo/querier.go`**

In `internal/server/router.go`:
Remove line: `appr.Post("/{id}/push-external", appointmentH.PushExternal)`.

In `internal/db/demo/querier.go`:
Update `app1ID` and `app2ID` seed appointments:
```go
	q.appointments[uuidToStr(app1ID)] = db.Appointment{
		ID:         app1ID,
		Title:      "Internes Vertriebs-Meeting",
		ContactID:  pgtype.UUID{Valid: false},
		CompanyID:  pgtype.UUID{Valid: false},
		DealID:     pgtype.UUID{Valid: false},
		StartTime:  pgtype.Timestamptz{Time: nowTime.Add(1 * time.Hour), Valid: true},
		EndTime:    pgtype.Timestamptz{Time: nowTime.Add(2 * time.Hour), Valid: true},
		Location:   "",
		Notes:      "Wöchentliches Pipeline-Update Vertrieb",
		IcsUid:     "crm-int-1",
		AssignedTo: "Max Vertriebsleiter",
		Type:       "MEETING",
		IsExternal: false,
		Provider:   "",
		IsPrivate:  false,
		IsPushed:   false,
		CreatedAt:  nowTimestamptz(),
		UpdatedAt:  nowTimestamptz(),
	}
	q.appointments[uuidToStr(app2ID)] = db.Appointment{
		ID:         app2ID,
		Title:      "Privater Termin / Facharzt",
		ContactID:  pgtype.UUID{Valid: false},
		CompanyID:  pgtype.UUID{Valid: false},
		DealID:     pgtype.UUID{Valid: false},
		StartTime:  pgtype.Timestamptz{Time: nowTime.Add(5 * time.Hour), Valid: true},
		EndTime:    pgtype.Timestamptz{Time: nowTime.Add(6 * time.Hour), Valid: true},
		Location:   "",
		Notes:      "Privatblocker",
		IcsUid:     "crm-int-2",
		AssignedTo: "Max Vertriebsleiter",
		Type:       "MEETING",
		IsExternal: false,
		Provider:   "",
		IsPrivate:  true,
		IsPushed:   false,
		CreatedAt:  nowTimestamptz(),
		UpdatedAt:  nowTimestamptz(),
	}
```

- [ ] **Step 6: Run tests to verify GREEN**

Run: `go test -v ./internal/core/appointment/... ./internal/server/handlers/...`
Expected: PASS

- [ ] **Step 7: Commit Task 1**

```bash
git add internal/server/router.go internal/server/handlers/appointments.go internal/server/handlers/appointments_test.go internal/core/appointment/service.go internal/core/appointment/service_test.go internal/db/demo/querier.go
git commit -m "fix(calendar): remove fake external push and enforce ownership on RFC 5545 ICS download (C1, C2)"
```

---

### Task 2: Calendar Frontend & E2E Cleanup (C1, C2)

**Files:**
- Modify: `web/src/pages/CalendarPage.tsx`
- Modify: `e2e/calendar-and-telephony.spec.ts`

**Interfaces:**
- Consumes: `GET /api/v1/appointments/{id}/ics`
- Removes from `CalendarPage.tsx`:
  - `M365 & Google OAuth verbunden` banner
  - `handleDownloadICS` client-side template
  - `handleSendInviteEmail` client-side template & invite modal
  - Card provider badges (`app.provider === 'microsoft' ? ...`)
  - `showExternal` toggle and sync bar

- [ ] **Step 1: Clean `CalendarPage.tsx`**

1. Remove fake banner and sync bar:
   - Remove "Termine, Kalender & M365 / Google Sync (§3.6)" title -> "Termine & Kalender".
   - Remove "Vor-Ort-Beratungen, Closer-Termine, M365/Google OAuth-Sync & RFC 5545" subtitle -> "Vor-Ort-Beratungen, Closer-Termine & lokaler RFC 5545 Kalender".
   - Remove "M365 & Google OAuth verbunden" banner.
   - Remove "Jetzt abgleichen" button.
   - Remove `showExternal` state and toggle buttons.
2. Remove `handleDownloadICS` (lines 172-181) and client-side ICS string template.
3. Remove `handleSendInviteEmail` (lines 183-190) and `inviteModalApp` state and JSX modal.
4. Replace ICS download with direct backend download:
   ```tsx
   <a
     href={`/api/v1/appointments/${app.id}/ics`}
     download={`termin-${app.id}.ics`}
     className="p-1.5 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded-lg text-xs inline-flex items-center gap-1 transition-colors"
     title="Als RFC 5545 .ics exportieren"
   >
     <Download className="w-3.5 h-3.5" />
     <span className="hidden sm:inline">ICS Export</span>
   </a>
   ```
5. Remove provider badges from card (`app.provider === 'microsoft' ? ...`); always render `typeConfig.label`.
6. Remove `!app.is_external` conditionals so all local appointments have edit/delete controls.

- [ ] **Step 2: Update E2E spec in `e2e/calendar-and-telephony.spec.ts`**

Update `e2e/calendar-and-telephony.spec.ts`:
- Remove assertions on `M365 & Google OAuth verbunden`, "Jetzt abgleichen", and "An M365 pushen".
- Assert appointment creation, appointment display, ICS download link presence (`href` containing `/api/v1/appointments/` and `/ics`), editing, and deleting.

- [ ] **Step 3: Run web lint, typecheck and format**

Run: `cd web && pnpm format && pnpm lint && pnpm typecheck && pnpm build && cd ..`
Expected: 0 errors, build succeeds.

- [ ] **Step 4: Commit Task 2**

```bash
git add web/src/pages/CalendarPage.tsx e2e/calendar-and-telephony.spec.ts
git commit -m "feat(calendar): remove simulated external cloud sync and wire direct ICS export (C1, C2)"
```

---

## Part 2: Telephony Call Logging (C3)

### Task 3: Honest Telephony Core & Click-to-Call Modal (C3)

**Files:**
- Modify: `internal/core/telephony/service.go`
- Modify: `internal/core/telephony/service_test.go`
- Modify: `web/src/components/telephony/CallModal.tsx`

**Interfaces:**
- `(*telephony.Service).LogCall(ctx context.Context, input LogCallInput) (CallActivity, error)`:
  - Fails honestly if DB insert fails or if `s.querier == nil`. Never fabricates `call-<nanos>`.
- `CallModal.tsx`:
  - Renders `<a href={'tel:' + contact.phone.trim()}>` only when `contact.phone && contact.phone.trim() !== ''`.
  - Re-labels as "Anruf-Protokoll".
  - Shows error alert on save failure.

- [ ] **Step 1: Write failing test in `internal/core/telephony/service_test.go` (RED)**

```go
func TestTelephonyService_LogCall_PropagatesDBError(t *testing.T) {
	// Create service with nil querier to trigger failure
	svc := telephony.NewService(nil, nil)

	_, err := svc.LogCall(context.Background(), telephony.LogCallInput{
		ContactID:       "44444444-4444-4444-4444-444444444441",
		DurationSeconds: 120,
		Disposition:     "REACHED",
		Notes:           "Kunde erreicht",
	})

	require.Error(t, err, "LogCall must fail honestly when DB is unavailable, never synthesize fake call")
	require.Contains(t, err.Error(), "database required")
}
```

- [ ] **Step 2: Run test to verify it fails (RED)**

Run: `go test -v ./internal/core/telephony -run TestTelephonyService_LogCall_PropagatesDBError`
Expected: FAIL (currently returns synthesized fake call with `err == nil`)

- [ ] **Step 3: Fix `LogCall` in `internal/core/telephony/service.go` (GREEN)**

In `internal/core/telephony/service.go`:
```go
func (s *Service) LogCall(ctx context.Context, input LogCallInput) (CallActivity, error) {
	if s.querier == nil {
		return CallActivity{}, fmt.Errorf("database required for logging calls")
	}

	contactUUID, err := uuid.Parse(input.ContactID)
	if err != nil {
		return CallActivity{}, fmt.Errorf("invalid contact ID %q: %w", input.ContactID, err)
	}

	created, err := s.querier.CreateCallActivity(ctx, db.CreateCallActivityParams{
		ContactID:       pgtype.UUID{Bytes: contactUUID, Valid: true},
		DurationSeconds: int32(input.DurationSeconds),
		Disposition:     input.Disposition,
		Notes:           input.Notes,
	})
	if err != nil {
		return CallActivity{}, fmt.Errorf("failed to persist call activity: %w", err)
	}

	call := CallActivity{
		ID:              uuid.UUID(created.ID.Bytes).String(),
		ContactID:       input.ContactID,
		DurationSeconds: int(created.DurationSeconds),
		Disposition:     created.Disposition,
		Notes:           created.Notes,
		CreatedAt:       created.CreatedAt.Time,
	}
	s.broadcast(call)
	return call, nil
}
```

- [ ] **Step 4: Update `web/src/components/telephony/CallModal.tsx`**

1. Re-label header from "Click-to-Call Telefonie (§7.1a)" to "Anruf-Protokoll".
2. Add conditional native `tel:` link when `contact.phone && contact.phone.trim() !== ''`:
   ```tsx
   {contact.phone && contact.phone.trim() !== '' && (
     <a
       href={`tel:${contact.phone.trim()}`}
       className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 text-xs font-bold rounded-lg transition-colors"
     >
       <Phone className="w-3.5 h-3.5" />
       Nummer wählen ({contact.phone})
     </a>
   )}
   ```
3. Add `error: string | null` state. In `handleSave`, on error set `setError(err.message || 'Fehler beim Speichern des Anrufprotokolls')` and display error banner in the modal instead of silently closing.

- [ ] **Step 5: Run tests and frontend checks to verify GREEN**

Run: `go test -v ./internal/core/telephony/... ./internal/server/handlers/...`
Run: `cd web && pnpm format && pnpm lint && pnpm typecheck && cd ..`
Expected: PASS

- [ ] **Step 6: Commit Task 3**

```bash
git add internal/core/telephony/service.go internal/core/telephony/service_test.go web/src/components/telephony/CallModal.tsx
git commit -m "fix(telephony): eliminate fake call synthesis and add native click-to-call link (C3)"
```

---

## Part 3: Geocoding & MapView (C4)

### Task 4: Bundled Offline German PLZ Centroid Geocoding & Contact Service Integration (C4)

**Files:**
- Create: `internal/geocoding/geocoding.go`
- Create: `internal/geocoding/data.go`
- Create: `internal/geocoding/geocoding_test.go`
- Modify: `internal/core/contact/service.go`
- Modify: `internal/core/contact/service_test.go`

**Interfaces:**
- Produces:
  - `geocoding.LookupPLZ(plz string) (lat, lon float64, ok bool)`
  - Exact 5-digit German postal code matching only. No 2-digit fallback.
  - License: CC0 / Public Domain documented in package header.
- Modifies:
  - `(*contact.Service).Create` and `(*contact.Service).Update`:
    - Auto-geocodes via `LookupPLZ` **only when BOTH `input.Latitude == nil && input.Longitude == nil`**.
    - If either coordinate is already set manually, preserves it without overwriting.
    - No database migration backfill on existing records.

- [ ] **Step 1: Write failing tests in `internal/geocoding/geocoding_test.go` (RED)**

```go
package geocoding_test

import (
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/geocoding"
	"github.com/stretchr/testify/require"
)

func TestLookupPLZ_ValidGermanPLZ(t *testing.T) {
	// Berlin Mitte
	lat, lon, ok := geocoding.LookupPLZ("10115")
	require.True(t, ok)
	require.InDelta(t, 52.53, lat, 0.1)
	require.InDelta(t, 13.38, lon, 0.1)

	// Frankfurt am Main
	lat, lon, ok = geocoding.LookupPLZ("60311")
	require.True(t, ok)
	require.InDelta(t, 50.11, lat, 0.1)
	require.InDelta(t, 8.68, lon, 0.1)

	// Munich
	lat, lon, ok = geocoding.LookupPLZ("80331")
	require.True(t, ok)
	require.InDelta(t, 48.13, lat, 0.1)
	require.InDelta(t, 11.57, lon, 0.1)
}

func TestLookupPLZ_InvalidOrUnknownPLZ(t *testing.T) {
	_, _, ok := geocoding.LookupPLZ("")
	require.False(t, ok)

	_, _, ok = geocoding.LookupPLZ("9999999") // too long
	require.False(t, ok)

	_, _, ok = geocoding.LookupPLZ("12") // 2 digits: NO fallback
	require.False(t, ok)

	_, _, ok = geocoding.LookupPLZ("ABCDE")
	require.False(t, ok)
}

func TestLookupPLZ_TrimsWhitespace(t *testing.T) {
	lat, lon, ok := geocoding.LookupPLZ("  10115 \n")
	require.True(t, ok)
	require.InDelta(t, 52.53, lat, 0.1)
	require.InDelta(t, 13.38, lon, 0.1)
}
```

- [ ] **Step 2: Run geocoding tests to verify they fail (RED)**

Run: `go test -v ./internal/geocoding`
Expected: FAIL (`no Go files in .../internal/geocoding`)

- [ ] **Step 3: Implement `internal/geocoding/geocoding.go` and `internal/geocoding/data.go` (GREEN)**

1. `data.go`:
   - Document dataset provenance and license:
     `// Dataset: German Postal Codes (PLZ) Centroids`
     `// License: CC0 1.0 Universal (Public Domain Dedication)`
   - Embed compact map of German 5-digit PLZ centroids `map[string][2]float64`.
   - Comprehensive coverage of German postal code ranges (`01067` to `99998`).
2. `geocoding.go`:
   ```go
   package geocoding

   import (
   	"strings"
   )

   // LookupPLZ returns the latitude and longitude centroid for an exact 5-digit German postal code.
   // Returns ok = false if the postal code is invalid or not found.
   func LookupPLZ(plz string) (lat, lon float64, ok bool) {
   	clean := strings.TrimSpace(plz)
   	if len(clean) != 5 {
   		return 0, 0, false
   	}
   	coords, found := plzCentroids[clean]
   	if !found {
   		return 0, 0, false
   	}
   	return coords[0], coords[1], true
   }
   ```

- [ ] **Step 4: Integrate into `internal/core/contact/service.go` and write tests (GREEN)**

In `internal/core/contact/service_test.go`:
```go
func TestContactService_AutoGeocodingOnCreate(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := contact.NewService(q, nil)

	// 1. Create with PLZ 10115 and nil coordinates -> coordinates automatically populated
	c, err := svc.Create(context.Background(), pgtype.UUID{}, contact.CreateContactInput{
		FirstName:   "Erika",
		LastName:    "Musterfrau",
		AddressZip:  "10115",
		AddressCity: "Berlin",
	})
	require.NoError(t, err)
	require.True(t, c.Latitude.Valid)
	require.True(t, c.Longitude.Valid)
	require.InDelta(t, 52.53, c.Latitude.Float64, 0.1)
	require.InDelta(t, 13.38, c.Longitude.Float64, 0.1)

	// 2. Create with manual latitude set -> manual coordinate is preserved, NOT overwritten
	manualLat := 49.0
	c2, err := svc.Create(context.Background(), pgtype.UUID{}, contact.CreateContactInput{
		FirstName:   "Hans",
		LastName:    "Manual",
		AddressZip:  "10115",
		AddressCity: "Berlin",
		Latitude:    &manualLat,
	})
	require.NoError(t, err)
	require.True(t, c2.Latitude.Valid)
	require.Equal(t, 49.0, c2.Latitude.Float64)
	require.False(t, c2.Longitude.Valid, "longitude was nil and latitude was set, so neither was auto-overwritten")
}
```

In `internal/core/contact/service.go`:
In `Create`:
```go
	// Auto-geocode via PLZ centroid only when BOTH coordinates are nil
	if input.Latitude == nil && input.Longitude == nil && strings.TrimSpace(input.AddressZip) != "" {
		if glat, glon, ok := geocoding.LookupPLZ(input.AddressZip); ok {
			lat = pgtype.Float8{Float64: glat, Valid: true}
			lon = pgtype.Float8{Float64: glon, Valid: true}
		}
	}
```
In `Update`:
```go
	// Auto-geocode via PLZ centroid only when BOTH coordinates are nil
	if input.Latitude == nil && input.Longitude == nil && strings.TrimSpace(input.AddressZip) != "" {
		if glat, glon, ok := geocoding.LookupPLZ(input.AddressZip); ok {
			lat = pgtype.Float8{Float64: glat, Valid: true}
			lon = pgtype.Float8{Float64: glon, Valid: true}
		}
	}
```

- [ ] **Step 5: Run tests to verify GREEN**

Run: `go test -v ./internal/geocoding/... ./internal/core/contact/...`
Expected: PASS

- [ ] **Step 6: Commit Task 4**

```bash
git add internal/geocoding/ internal/core/contact/service.go internal/core/contact/service_test.go
git commit -m "feat(geocoding): add bundled CC0 German PLZ centroid lookup and auto-geocoding (C4)"
```

---

### Task 5: Honest MapView without Frankfurt Jitter (C4)

**Files:**
- Modify: `web/src/pages/MapViewPage.tsx`

**Interfaces:**
- Consumes: Contacts from `GET /api/v1/contacts`
- Modifies:
  - Eliminates `Math.random() - 0.5 * 0.08` jitter.
  - Partitions contacts into `mappedContacts` (both `latitude` and `longitude` are present/valid numbers) and `unmappedContacts`.
  - Renders markers only for `mappedContacts`.
  - Adds collapsible panel "Kontakte ohne Standort (X)" displaying contacts lacking coordinates.
  - Centers map dynamically on Germany `[51.1657, 10.4515]` with zoom 6.

- [ ] **Step 1: Update `web/src/pages/MapViewPage.tsx`**

1. Remove Frankfurt random jitter:
   ```tsx
   const mappedContacts = contacts.filter(
     (c) => typeof c.latitude === 'number' && typeof c.longitude === 'number' && !isNaN(c.latitude) && !isNaN(c.longitude)
   );
   const unmappedContacts = contacts.filter(
     (c) => !(typeof c.latitude === 'number' && typeof c.longitude === 'number' && !isNaN(c.latitude) && !isNaN(c.longitude))
   );
   ```
2. Center map:
   Default center: `[51.1657, 10.4515]`, zoom: 6.
3. Map markers only loop over `mappedContacts`.
4. Render collapsible side drawer / tab "Kontakte ohne Standort ({unmappedContacts.length})":
   - When opened, lists contacts with their name, address, and an "Adresse bearbeiten" link/button.
   - Shows empty state: "Alle Kontakte verfügen über gültige Geokoordinaten."

- [ ] **Step 2: Run web lint, typecheck, format and build**

Run: `cd web && pnpm format && pnpm lint && pnpm typecheck && pnpm build && cd ..`
Expected: 0 errors, build succeeds.

- [ ] **Step 3: Commit Task 5**

```bash
git add web/src/pages/MapViewPage.tsx
git commit -m "feat(map): remove random coordinate jitter and display unmapped contacts honestly (C4)"
```

---

## Part 4: Verification & Documentation

### Task 6: End-to-End Verification & Reporting

- [ ] **Step 1: Run full test suite**
  `go test ./...` -> verify all packages pass.
- [ ] **Step 2: Check formatting and static analysis**
  `gofmt -l .` -> must be completely empty.
  `go vet ./...` -> must exit 0 with no warnings.
- [ ] **Step 3: Run coverage gate**
  `./scripts/coverage.sh --min 50.0 --diff 50.0` -> verify diff ≥ 50.0% and total ≥ 50.0%.
- [ ] **Step 4: Verify critical invariants**
  `ls -la migrations` -> verify symlink `migrations -> internal/db/migrations` is intact.
- [ ] **Step 5: Update `.superpowers/sdd/progress.md` and write report `.superpowers/sdd/task-sp5-report.md`**
