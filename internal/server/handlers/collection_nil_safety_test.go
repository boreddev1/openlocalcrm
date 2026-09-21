package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/appointment"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/core/company"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/core/note"
	"github.com/openlocalcrm/openlocalcrm/internal/core/todo"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestCollectionEndpoints_EmptyArraySerialization(t *testing.T) {
	ctx := context.Background()

	t.Run("Deals List and ListByStage empty array serialization", func(t *testing.T) {
		querier := demo.NewEmptyInMemoryQuerier()
		auditService := audit.NewService(querier)
		dealSvc := deal.NewService(querier, auditService)
		h := handlers.NewDealHandler(dealSvc)

		// 1. Default List
		req := httptest.NewRequest("GET", "/api/v1/deals", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.List(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array '[]', got: %q", body)
		}

		// 2. List with stage filter
		reqStage := httptest.NewRequest("GET", "/api/v1/deals?stage=WON", nil).WithContext(ctx)
		recStage := httptest.NewRecorder()
		h.List(recStage, reqStage)
		if recStage.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recStage.Code)
		}
		if body := strings.TrimSpace(recStage.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array '[]', got: %q", body)
		}
	})

	t.Run("Contacts List and Search empty array serialization", func(t *testing.T) {
		querier := demo.NewEmptyInMemoryQuerier()
		auditService := audit.NewService(querier)
		contactSvc := contact.NewService(querier, auditService)
		h := handlers.NewContactHandler(contactSvc)

		// 1. Default List
		req := httptest.NewRequest("GET", "/api/v1/contacts", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.List(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array '[]', got: %q", body)
		}

		// 2. Search contacts
		reqSearch := httptest.NewRequest("GET", "/api/v1/contacts?q=nobody", nil).WithContext(ctx)
		recSearch := httptest.NewRecorder()
		h.List(recSearch, reqSearch)
		if recSearch.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recSearch.Code)
		}
		if body := strings.TrimSpace(recSearch.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array '[]', got: %q", body)
		}
	})

	t.Run("Companies List empty array serialization", func(t *testing.T) {
		querier := demo.NewEmptyInMemoryQuerier()
		auditService := audit.NewService(querier)
		compSvc := company.NewService(querier, auditService)
		h := handlers.NewCompanyHandler(compSvc)

		req := httptest.NewRequest("GET", "/api/v1/companies", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.List(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array '[]', got: %q", body)
		}
	})

	t.Run("Todos List empty array serialization", func(t *testing.T) {
		querier := demo.NewEmptyInMemoryQuerier()
		auditService := audit.NewService(querier)
		todoSvc := todo.NewService(querier, auditService)
		h := handlers.NewTodoHandler(todoSvc)

		req := httptest.NewRequest("GET", "/api/v1/todos", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.List(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array '[]', got: %q", body)
		}
	})

	t.Run("Appointments List empty array serialization", func(t *testing.T) {
		querier := demo.NewEmptyInMemoryQuerier()
		hub := sse.NewHub()
		appSvc := appointment.NewService(querier, hub)
		h := handlers.NewAppointmentHandler(appSvc)

		req := httptest.NewRequest("GET", "/api/v1/appointments", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.List(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array '[]', got: %q", body)
		}
	})

	t.Run("Notes List empty array serialization", func(t *testing.T) {
		hub := sse.NewHub()
		noteSvc := note.NewService(nil, hub)
		h := handlers.NewNoteHandler(noteSvc, nil)

		req := httptest.NewRequest("GET", "/api/v1/notes?entity_id=nonexistent", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.List(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
			t.Fatalf("expected empty JSON array '[]', got: %q", body)
		}
	})

	t.Run("Automations List empty array serialization", func(t *testing.T) {
		q := demo.NewInMemoryQuerier()
		wfSvc := automation.NewService(q)
		h := handlers.NewAutomationHandler(wfSvc)

		req := httptest.NewRequest("GET", "/api/v1/automations", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.ListWorkflows(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if !strings.HasPrefix(rec.Body.String(), "[") {
			t.Fatalf("expected JSON array, got: %s", rec.Body.String())
		}
	})

	t.Run("AI KnowledgeBase and Research empty array serialization", func(t *testing.T) {
		h := handlers.NewAIHandler(nil, nil, nil, nil, nil, nil)

		reqKB := httptest.NewRequest("GET", "/api/v1/ai/kb", nil).WithContext(ctx)
		recKB := httptest.NewRecorder()
		h.ListKB(recKB, reqKB)
		if recKB.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recKB.Code)
		}
		if !strings.HasPrefix(recKB.Body.String(), "[") {
			t.Fatalf("expected JSON array, got: %s", recKB.Body.String())
		}

		reqJobs := httptest.NewRequest("GET", "/api/v1/ai/research/jobs", nil).WithContext(ctx)
		recJobs := httptest.NewRecorder()
		h.ListResearchJobs(recJobs, reqJobs)
		if recJobs.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recJobs.Code)
		}
		if !strings.HasPrefix(recJobs.Body.String(), "[") {
			t.Fatalf("expected JSON array, got: %s", recJobs.Body.String())
		}
	})
}
