# Phase 2: CRM Core Data & API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the complete CRM Core domain (Contacts, Companies, Deals/Pipelines, Todos, Notes/Activities, Audit Log, and Custom Fields) with PostgreSQL migrations, type-safe queries via `sqlc`, business logic services, and REST endpoints.

**Architecture:** Clean architecture in Go. Database models and queries in `internal/db`, core domain services in `internal/core/{contact,company,deal,todo,audit}`, HTTP handlers in `internal/server/handlers`, tested via unit and integration tests.

**Tech Stack:** Go 1.22+, PostgreSQL 16 (`pg_trgm`, `tsvector`, `pgvector`), `sqlc`, `jackc/pgx/v5`, `go-chi/chi/v5`.

## Global Constraints

- Resource constraint: All queries must be indexed, optimized, zero-reflection.
- Single-Tenant: No tenant ID column; single organization context.
- Soft Deletes vs Hard Deletes: Contacts/Companies support archiving and GDPR-compliant hard deletion with audit logging.
- Audit Log: Every mutation (create, update, delete) generates an audit entry in `audit_logs`.
- Custom Fields: Extensible `custom_fields JSONB DEFAULT '{}'` on contacts, companies, deals.

---

### Task 1: Core Database Migrations (Contacts, Companies, Deals, Todos, Audit Log)

**Files:**
- Create: `migrations/00002_core_crm_schema.sql`
- Create: `internal/db/queries/companies.sql`
- Create: `internal/db/queries/contacts.sql`
- Create: `internal/db/queries/deals.sql`
- Create: `internal/db/queries/todos.sql`
- Create: `internal/db/queries/audit.sql`
- Test: `internal/db/core_schema_test.go`

**Interfaces:**
- Produces: Generated `sqlc` models and querier methods for Companies, Contacts, Deals, Todos, AuditLogs.

- [ ] **Step 1: Write migration 00002_core_crm_schema.sql**

```sql
-- migrations/00002_core_crm_schema.sql
-- +goose Up

-- Companies table
CREATE TABLE IF NOT EXISTS companies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255),
    phone VARCHAR(50),
    email VARCHAR(255),
    address_street VARCHAR(255),
    address_zip VARCHAR(20),
    address_city VARCHAR(100),
    address_country VARCHAR(50) DEFAULT 'DE',
    custom_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Contacts table
CREATE TABLE IF NOT EXISTS contacts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_id UUID REFERENCES companies(id) ON DELETE SET NULL,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(50),
    mobile VARCHAR(50),
    position VARCHAR(100),
    lead_source VARCHAR(100),
    address_street VARCHAR(255),
    address_zip VARCHAR(20),
    address_city VARCHAR(100),
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    custom_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Deal stages and Pipelines
CREATE TABLE IF NOT EXISTS deals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title VARCHAR(255) NOT NULL,
    company_id UUID REFERENCES companies(id) ON DELETE SET NULL,
    contact_id UUID REFERENCES contacts(id) ON DELETE SET NULL,
    value NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
    currency VARCHAR(3) NOT NULL DEFAULT 'EUR',
    stage VARCHAR(50) NOT NULL DEFAULT 'LEAD',
    probability INTEGER NOT NULL DEFAULT 10,
    assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
    closed_at TIMESTAMPTZ,
    custom_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Todos table
CREATE TYPE todo_status AS ENUM ('OPEN', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED');
CREATE TYPE todo_priority AS ENUM ('LOW', 'MEDIUM', 'HIGH', 'URGENT');

CREATE TABLE IF NOT EXISTS todos (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title VARCHAR(255) NOT NULL,
    description TEXT,
    due_date TIMESTAMPTZ,
    status todo_status NOT NULL DEFAULT 'OPEN',
    priority todo_priority NOT NULL DEFAULT 'MEDIUM',
    assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
    contact_id UUID REFERENCES contacts(id) ON DELETE CASCADE,
    deal_id UUID REFERENCES deals(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Audit Log table (immutable)
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    entity_type VARCHAR(50) NOT NULL,
    entity_id UUID NOT NULL,
    action VARCHAR(50) NOT NULL,
    changes JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for performance & search
CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts(email);
CREATE INDEX IF NOT EXISTS idx_contacts_name_trgm ON contacts USING gin ((first_name || ' ' || last_name) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_companies_name_trgm ON companies USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_deals_stage ON deals(stage);
CREATE INDEX IF NOT EXISTS idx_deals_assigned ON deals(assigned_to);
CREATE INDEX IF NOT EXISTS idx_todos_status_due ON todos(status, due_date);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_logs(entity_type, entity_id);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS todos;
DROP TYPE IF EXISTS todo_priority;
DROP TYPE IF EXISTS todo_status;
DROP TABLE IF EXISTS deals;
DROP TABLE IF EXISTS contacts;
DROP TABLE IF EXISTS companies;
```

- [ ] **Step 2: Define SQL queries for Contacts, Companies, Deals, Todos, and Audit Logs**
- [ ] **Step 3: Run `sqlc generate` to produce Go models and interface methods**
- [ ] **Step 4: Verify schema generation with unit tests**
- [ ] **Step 5: Commit Task 1**

---

### Task 2: Core Domain Services (Contact, Company, Deal, Todo, Audit)

**Files:**
- Create: `internal/core/contact/service.go`
- Create: `internal/core/company/service.go`
- Create: `internal/core/deal/service.go`
- Create: `internal/core/todo/service.go`
- Create: `internal/core/audit/service.go`
- Test: `internal/core/contact/service_test.go`
- Test: `internal/core/deal/service_test.go`

---

### Task 3: REST API Handlers & Validation

**Files:**
- Create: `internal/server/handlers/contacts.go`
- Create: `internal/server/handlers/companies.go`
- Create: `internal/server/handlers/deals.go`
- Create: `internal/server/handlers/todos.go`
- Create: `internal/server/handlers/search.go`
- Test: `internal/server/handlers/contacts_test.go`
- Test: `internal/server/handlers/deals_test.go`
