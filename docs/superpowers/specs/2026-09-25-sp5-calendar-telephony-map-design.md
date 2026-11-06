# Design Specification: SP5 — Calendar, Telephony & Geocoding (C1–C4)

**Date:** 2026-09-25  
**Branch:** `fix/sp5-calendar-telephony-map`  
**Status:** Approved with revisions (Round 2)  
**Scope:** C1, C2, C3, C4  

---

## 1. Problem Statement & Scope

In the existing codebase, features in the calendar, telephony, and map domains were either simulated, fabricated, or lacked honest error handling:
1. **C1 (Fake M365 / Google Sync & Push-External):** `CalendarPage.tsx` claimed "M365 & Google OAuth verbunden", rendered card badges ("Microsoft 365 Sync", "Google Calendar Sync"), filtered "externe Termine", and offered external sync buttons calling `POST /api/v1/appointments/{id}/push-external`. The backend returned a fabricated JSON `{ "status": "synced", "provider": "microsoft_and_google" }`. The demo querier seeded external cloud-sync fixtures (`Internes Vertriebs-Meeting (M365)` / `Privater Termin / Facharzt (Google)` with `provider: microsoft|google, is_external: true, is_pushed: true`). Furthermore, `CalendarPage.tsx` generated mock `.ics` strings in the client browser (`handleDownloadICS`) and toasted simulated email invitations (`handleSendInviteEmail`), while `e2e/calendar-and-telephony.spec.ts` asserted against these mock features.
2. **C2 (Fabricated ICS Fallback & IDOR):** `GET /api/v1/appointments/{id}/ics` returned a fabricated fallback appointment (`Kundentermin`) if the requested ID was missing from the database. Furthermore, it lacked the ownership checks present on `Update`/`Delete`, enabling any authenticated user to download ICS files of appointments assigned to other users (IDOR). `GenerateICS` used `METHOD:REQUEST` instead of `METHOD:PUBLISH` and text fields were not RFC 5545 escaped.
3. **C3 (Telephony Simulation & Swallowed Errors):** In `internal/core/telephony/service.go:73-83`, `LogCall` caught database errors and silently fell through to fabricate a fake `call-<nanos>` record, broadcasting it via SSE and returning `nil` error to the handler. In `CallModal.tsx`, the action was mislabeled as a VoIP browser call, lacked a native `tel:` click-to-call link, and silently swallowed persistence errors in `catch {}`.
4. **C4 (Fake Map Coordinates & Missing Geocoding):** `MapViewPage.tsx` fabricated random coordinate jitter around Frankfurt am Main (`50.1109 + (Math.random() - 0.5) * 0.08`) for contacts without geographic coordinates. There was no offline postal code (PLZ) centroid resolution.

---

## 2. Architecture & Design Decisions

### 2.1 C1 — Elimination of Fake M365/Google Sync & Client-Side Mock Exports
- **Backend:**
  - Remove route `POST /api/v1/appointments/{id}/push-external` from `internal/server/router.go`.
  - Remove `PushExternal` handler from `internal/server/handlers/appointments.go` and `PushExternal` method from `internal/core/appointment/service.go`.
  - In `internal/db/demo/querier.go:294-333`: Re-seed appointments honestly as local appointments:
    - `app1ID`: Title `"Internes Vertriebs-Meeting"`, `Provider: ""`, `IsExternal: false`, `IsPushed: false`
    - `app2ID`: Title `"Privater Termin / Facharzt"`, `Provider: ""`, `IsExternal: false`, `IsPushed: false`
    - Remove external sync tags and flags.
  - The calendar is maintained honestly as a focused, self-hosted local CRM calendar.
- **Frontend (`CalendarPage.tsx`):**
  - Remove the fake status badge "M365 & Google OAuth verbunden", external sync bar ("Jetzt abgleichen"), and `showExternal` visibility toggle.
  - Remove card-level provider badges ("Microsoft 365 Sync", "Google Calendar Sync"); appointment cards always render their actual local category `typeConfig.label`.
  - Remove `!app.is_external` conditional wrappers so editing and deleting are available on all local appointments.
  - Remove client-side mock ICS generator `handleDownloadICS` (lines 172-181) and client-side invitation modal `handleSendInviteEmail` (lines 183-190).
  - The **only** export mechanism is the real backend endpoint `GET /api/v1/appointments/{id}/ics` (triggered via direct link or download button).
- **E2E Specs (`e2e/calendar-and-telephony.spec.ts`):**
  - Remove assertions for fake external M365/Google sync, external push, and invitation toasts.
  - Assert honest appointment CRUD and verification of the backend ICS export endpoint.

### 2.2 C2 — Honest RFC 5545 ICS Export with Ownership Enforcement & Text Escaping
- **Endpoint:** `GET /api/v1/appointments/{id}/ics`
- **Ownership & Access Control:**
  - Retrieve claims from `r.Context()`.
  - Load appointment from database via `h.svc.GetByID(ctx, id)`.
  - If appointment does not exist -> return **HTTP 404 Not Found** (`{"error":"appointment not found"}`).
  - Never fabricate fake fallback appointment data.
  - Enforce ownership mirroring `Update` and `Delete` (`handlers/appointments.go:100-110, 133-143`):
    ```go
    if claims != nil && claims.Role != "ADMIN" {
        if app.AssignedTo != "" && app.AssignedTo != claims.Email && app.AssignedTo != claims.UserID.String() {
            http.Error(w, `{"error":"forbidden","message":"Sie können nur Ihre eigenen Termine als ICS exportieren"}`, http.StatusForbidden)
            return
        }
    }
    ```
- **RFC 5545 Text Escaping & Compliance:**
  - In `internal/core/appointment/service.go` (`GenerateICS`):
    - Use `METHOD:PUBLISH` (not `METHOD:REQUEST`), ensuring calendar clients (e.g. Outlook, Apple Calendar) import the downloaded `.ics` file as a calendar entry rather than an invitation awaiting RSVP.
    - Implement `escapeICS(s string) string` per RFC 5545 §3.3.11:
      - `\` -> `\\`
      - `;` -> `\;`
      - `,` -> `\,`
      - Newlines (`\r\n`, `\r`, `\n`) -> literal `\n`
    - Apply `escapeICS` to `SUMMARY`, `LOCATION`, and `DESCRIPTION`.
  - Format VEVENT with:
    - Content-Type: `text/calendar; charset=utf-8`
    - Content-Disposition: `attachment; filename="termin-<id>.ics"`
    - `VERSION:2.0`, `PRODID:-//OpenLocalCRM//DE`
    - `UID`, `DTSTAMP`, `DTSTART`, `DTEND`, `SUMMARY`, `LOCATION`, `DESCRIPTION`, `STATUS:CONFIRMED`
    - UTC timestamps (`YYYYMMDDTHHMMSSZ`).

### 2.3 C3 — Honest Telephony: Core Error Propagation & Native Click-to-Call
- **Core Service (`internal/core/telephony/service.go`):**
  - Fix `LogCall(ctx, input)`: Remove lines 73-84 (fake `call-<nanos>` synthesis).
  - If `s.querier == nil`: return `CallActivity{}, fmt.Errorf("database required for logging calls")`.
  - If `s.querier.CreateCallActivity(...)` fails: return `CallActivity{}, fmt.Errorf("failed to persist call activity: %w", err)`.
  - Never swallow database errors and never broadcast fake call activities on failure.
  - `handlers/telephony.go:26` already returns HTTP 500 when `svc.LogCall` returns an error; it will now receive real errors and fail closed.
- **Frontend (`CallModal.tsx`):**
  - Re-label title and badge as "Anruf-Protokoll" (Call Logging), clarifying that this records a call rather than executing in-browser VoIP.
  - Add native HTML click-to-call link when phone is present:
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
  - Fix error handling in `handleSave`:
    If `apiFetch('/api/v1/telephony/calls', ...)` fails, display an error banner / message in the modal rather than silently closing and swallowing the error in `catch {}`.

### 2.4 C4 — Exact Offline PLZ Centroid Geocoding & Honest MapView
- **Package `internal/geocoding`:**
  - Bundled dataset of German postal codes (5-digit PLZ) centroids (`latitude`, `longitude`) derived from open data (CC0 / Open Data).
  - License and provenance documented in source headers.
  - Compact in-memory map (< 300 KB embedded dataset).
  - Lookup function:
    ```go
    func LookupPLZ(plz string) (lat, lon float64, ok bool)
    ```
  - **Strict matching rule:** Exact 5-digit PLZ matching only. **No 2-digit regional fallback.** If a postal code is invalid or missing from the German dataset, `ok = false`.
- **Contact Service Integration (`internal/core/contact/service.go`):**
  - In `Create` and `Update`:
    ```go
    // Only resolve coordinates if BOTH are nil. Never overwrite a manually specified coordinate.
    if input.Latitude == nil && input.Longitude == nil && strings.TrimSpace(input.AddressZip) != "" {
        if lat, lon, ok := geocoding.LookupPLZ(strings.TrimSpace(input.AddressZip)); ok {
            latVal := lat
            lonVal := lon
            input.Latitude = &latVal
            input.Longitude = &lonVal
        }
    }
    ```
  - Manually provided latitude/longitude are preserved.
  - **No migration backfill:** Existing database contacts are not modified by a migration; they remain in the "ohne Standort" list until saved/updated.
  - **External geocoder:** Marked as **OUT OF SCOPE** (no external HTTP calls or PII leakage).
- **Frontend (`MapViewPage.tsx`):**
  - Completely remove `Math.random() - 0.5 * 0.08` Frankfurt jitter.
  - Partition contacts into:
    - Contacts with valid `latitude` and `longitude`: rendered as map markers.
    - Contacts without coordinates (`!contact.latitude || !contact.longitude`): rendered in an honest collapsible drawer or list "Kontakte ohne Standort (X)" with address details.
  - Default map center: Germany overview (`[51.1657, 10.4515]`, zoom 6) or bounding centroid of valid plotted markers.

---

## 3. Invariants & Constraints

1. **Migrations Symlink:** `migrations -> internal/db/migrations` must remain untouched.
2. **Zero-Fake-Success:** No fallback appointment synthesis on missing ID (404), no fake call record synthesis on DB error (500), no fake map coordinate jitter.
3. **IDOR Security:** `GET /api/v1/appointments/{id}/ics` enforces identical ownership rules as `Update` and `Delete`.
4. **Coverage Gate:** Minimum 50.0% total and 50.0% diff coverage enforced via `./scripts/coverage.sh --min 50.0 --diff 50.0`.
5. **License Policy:** Bundled geocoding dataset must be 100% permissive (CC0 / Open Data), documented in headers.
