package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
)

type AutomationHandler struct {
	service *automation.Service
}

func NewAutomationHandler(service *automation.Service) *AutomationHandler {
	return &AutomationHandler{service: service}
}

func (h *AutomationHandler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	workflows, err := h.service.ListDefaultWorkflows(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "workflow_list_failed", "Workflows konnten nicht geladen werden")
		return
	}
	if workflows == nil {
		workflows = []automation.Workflow{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(workflows)
}

func (h *AutomationHandler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var wf automation.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_body", "Ungültiger Anfrageinhalt")
		return
	}
	created, err := h.service.CreateWorkflow(r.Context(), wf)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "workflow_create_failed", "Workflow konnte nicht erstellt werden")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func (h *AutomationHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.service.ListRuns(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "run_list_failed", "Workflow-Ausführungen konnten nicht geladen werden")
		return
	}
	if runs == nil {
		runs = []automation.WorkflowRun{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runs)
}

// ApproveStep handles POST /automations/runs/{id}/approve
// Advances a WAITING_APPROVAL workflow run by executing the pending step.
func (h *AutomationHandler) ApproveStep(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	if runID == "" {
		writeJSONError(w, http.StatusBadRequest, "run_id_required", "Workflow-Run-ID ist erforderlich")
		return
	}

	run, err := h.service.ApproveStep(r.Context(), runID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "approve_step_failed", "Schritt konnte nicht genehmigt werden")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(run)
}

// TestRunWorkflow handles POST /automations/{id}/test-run
// Validates target input and executes a test run for the specified workflow.
func (h *AutomationHandler) TestRunWorkflow(w http.ResponseWriter, r *http.Request) {
	wfID := chi.URLParam(r, "id")
	if wfID == "" {
		writeJSONError(w, http.StatusBadRequest, "workflow_id_required", "Workflow-ID ist erforderlich")
		return
	}

	var req automation.TargetInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_body", "Ungültiger Anfrageinhalt")
		return
	}
	if strings.TrimSpace(req.TargetID) == "" || strings.TrimSpace(req.TargetType) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_target", "target_id und target_type sind erforderlich")
		return
	}

	run, err := h.service.TestRunWorkflow(r.Context(), wfID, req)
	if err != nil {
		if strings.Contains(err.Error(), "nicht gefunden") {
			writeJSONError(w, http.StatusNotFound, "not_found", "Zielobjekt nicht gefunden")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "test_run_failed", "Test-Ausführung fehlgeschlagen")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(run)
}
