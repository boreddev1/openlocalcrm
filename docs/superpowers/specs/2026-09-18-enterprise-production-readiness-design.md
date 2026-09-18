# Enterprise Production Readiness Design v1.0 — OpenLocalCRM

> **Status:** Approved (Autonomous Goal Execution)  
> **Date:** 2026-09-18  
> **Target:** Enterprise-Ready Single-Tenant CRM (Production Grade)

---

## 1. Executive Summary & Goals

OpenLocalCRM is being prepared for robust, daily production use in commercial enterprises (focus: sales, energy, field sales / D2D, SMBs).
To be truly enterprise-ready, all aspects of the application must transition from demo/prototype mechanics to durable, secure, and resilient enterprise standards:
1. **Durable Data Persistence**: Every business object (appointments, notes, call logs, workflows, knowledge base) must be stored durably in PostgreSQL with proper schemas, foreign key constraints, indexes, and migrations, while retaining zero-dependency in-memory mode for tests and standalone launchers.
2. **Enterprise Security & RBAC**: Strict role enforcement (`ADMIN` vs. `BENUTZER`) across sensitive endpoints (user management, CSV exports, backup drills, workflow configurations), token/session revocation in PostgreSQL, and automated audit logging for all mutations.
3. **Core Sales & D2D Lifecycle**: Robust validation of deal pipeline stages, setter/closer roles, solar calculation attachment to deals, ICS standard export/push, and background worker job processing.
4. **Frontend Enterprise Resilience**: Error boundaries, structured error toasts, optimistic updates with rollback on network failure, and responsive ergonomics for field sales on tablets and smartphones.
5. **Continuous Verification**: Unit tests, integration tests, Playwright E2E tests, and license compliance passing 100% green.

---

## 2. Phase Breakdown

### Phase 1: PostgreSQL Data Persistence & Schema Migrations
- Add migration `00009_enterprise_persistence.sql` covering:
  - `knowledge_base_articles` (ID, title, content, category, tags, timestamps)
  - Extensions to `appointments` (`is_external`, `provider`, `is_private`, `is_pushed`, `assigned_to`, `type`)
  - Full CRUD queries in `internal/db/queries/` for:
    - `appointments.sql`
    - `notes.sql`
    - `call_activities.sql`
    - `workflows.sql`
    - `knowledge_base.sql`
- Implement SQL-backed service repositories that seamlessly fallback to in-memory during `DEMO_MODE=true`.
- Connect real PostgreSQL persistence to `appointment.Service`, `note.Service`, `telephony.Service`, `automation.Service`, and `ai.KnowledgeBase`.

### Phase 2: Enterprise Security, RBAC & Audit Trails
- Implement `RequireRole(role string)` middleware in `internal/auth/middleware.go`.
- Apply `RequireRole("ADMIN")` to:
  - `/api/v1/users/*` (team member invites, role updates, deactivations)
  - `/api/v1/backup/*` (restore drill, raw data exports)
  - `/api/v1/export/contacts.csv`
  - `/api/v1/automations` (creating/modifying enterprise automation rules)
- Ensure all mutations (`POST`, `PUT`, `DELETE` on contacts, deals, companies, appointments, notes, todos) record structured audit logs into `audit_logs` table.
- Implement session revocation checking against database sessions.

### Phase 3: Sales Pipeline, Calculations & Calendar Sync
- Deal stage progression validation (Lead -> Qualifiziert -> Angebot -> Verhandlung -> Gewonnen / Verloren).
- Setter/Closer assignment checks.
- Attach solar & battery calculation payloads to deal metadata / notes.
- Ensure RFC 5545 compliant `.ics` calendar generation in `appointment.Service`.

### Phase 4: Frontend Enterprise Polishing & Field Usability
- Add global React `ErrorBoundary` in `web/src/components/common/ErrorBoundary.tsx` preventing white screens.
- Enhance tablet & mobile responsiveness on Deals Kanban and Contacts views.
- Improve form validation indicators and user feedback toasts.

### Phase 5: Comprehensive Verification & Packaging
- Run all Go unit tests (`go test -v ./...`).
- Run Playwright E2E suite (`./scripts/run-e2e.sh`).
- Run open-source license compliance audit (`./scripts/check-licenses.sh`).
- Update documentation and commit all improvements.
