# Phase 3: React Frontend & Playwright E2E Test Suite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scaffold and build the complete responsive React 18 + TypeScript + Tailwind CSS Frontend for OpenLocalCRM v3 (Auth, Dashboard, Contacts, Companies, Kanban Deal Pipeline with `@dnd-kit`, Todos, Leaflet Map View, SSE real-time notifications), embed it into the Go binary, and build an extensive Playwright E2E test suite covering all UI flows and roles.

**Architecture:** Single-Page App in `web/` built with Vite + React + TypeScript + Tailwind CSS + Lucide Icons. Client-side routing with `react-router-dom`, state management and caching via `@tanstack/react-query`, drag-and-drop via `@dnd-kit`, maps via `leaflet`. Embedded in `crm-server` via `//go:embed web/dist/*`. E2E tests in `e2e/` using `@playwright/test`.

**Tech Stack:** React 18, TypeScript, Vite, Tailwind CSS, `@tanstack/react-query`, `@dnd-kit/core`, `leaflet`, `lucide-react`, `@playwright/test`.

## Global Constraints

- Resource constraint: Frontend is compiled into static assets; 0 MB server runtime RAM overhead.
- Clean Design & Aesthetics: Modern CRM dashboard, responsive mobile view for D2D reps, dark/light theme support, rich micro-interactions.
- Strict License Compliance (§22): All npm packages must be MIT, Apache-2.0, BSD-2/3-Clause, ISC. No AGPL/GPL.
- Playwright E2E Tests: Full coverage of Login, Admin Setup, Contact CRUD & Search, Kanban Drag & Drop, Todo Management, SSE Live-Updates.

---

### Task 1: React + Vite + Tailwind Scaffold & Embedded Static Asset Serving in Go

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/tailwind.config.js`
- Create: `web/postcss.config.js`
- Create: `web/index.html`
- Create: `web/src/index.css`
- Create: `web/src/main.tsx`
- Modify: `internal/server/router.go`
- Test: `web/src/App.test.tsx`

---

### Task 2: Auth Flow, Navigation Layout & Realtime SSE Hook

**Files:**
- Create: `web/src/api/client.ts`
- Create: `web/src/hooks/useSSE.ts`
- Create: `web/src/context/AuthContext.tsx`
- Create: `web/src/components/layout/AppLayout.tsx`
- Create: `web/src/components/layout/Sidebar.tsx`
- Create: `web/src/components/layout/Header.tsx`
- Create: `web/src/pages/LoginPage.tsx`
- Create: `web/src/pages/DashboardPage.tsx`
- Test: `web/src/context/AuthContext.test.tsx`

---

### Task 3: Contacts & Companies Management Views

**Files:**
- Create: `web/src/pages/ContactsPage.tsx`
- Create: `web/src/pages/CompaniesPage.tsx`
- Create: `web/src/components/contacts/ContactModal.tsx`
- Create: `web/src/components/companies/CompanyModal.tsx`
- Create: `web/src/components/ui/DataTable.tsx`
- Test: `web/src/pages/ContactsPage.test.tsx`

---

### Task 4: Kanban Deal Pipeline & Setter/Closer Board (`@dnd-kit`)

**Files:**
- Create: `web/src/pages/DealsPage.tsx`
- Create: `web/src/components/deals/KanbanBoard.tsx`
- Create: `web/src/components/deals/KanbanColumn.tsx`
- Create: `web/src/components/deals/DealCard.tsx`
- Create: `web/src/components/deals/DealModal.tsx`
- Test: `web/src/components/deals/KanbanBoard.test.tsx`

---

### Task 5: Todos & Field Map View (Leaflet OpenStreetMap)

**Files:**
- Create: `web/src/pages/TodosPage.tsx`
- Create: `web/src/components/todos/TodoModal.tsx`
- Create: `web/src/pages/MapViewPage.tsx`
- Create: `web/src/components/map/D2DMap.tsx`
- Test: `web/src/pages/TodosPage.test.tsx`

---

### Task 6: Playwright E2E Test Suite (Comprehensive UI Testing)

**Files:**
- Create: `playwright.config.ts`
- Create: `e2e/auth.spec.ts`
- Create: `e2e/contacts.spec.ts`
- Create: `e2e/deals-kanban.spec.ts`
- Create: `e2e/todos.spec.ts`
- Create: `e2e/realtime-sse.spec.ts`
- Create: `scripts/run-e2e.sh`
