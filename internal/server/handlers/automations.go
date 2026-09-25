package handlers

import (
	"encoding/json"
	"fmt"
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if workflows == nil {
		workflows = []automation.Workflow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workflows)
}

func (h *AutomationHandler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var wf automation.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	created, err := h.service.CreateWorkflow(r.Context(), wf)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (h *AutomationHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.service.ListRuns(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "run id is required", http.StatusBadRequest)
		return
	}

	run, err := h.service.ApproveStep(r.Context(), runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(run)
}

// TestRunWorkflow handles POST /automations/{id}/test-run
// Validates target input and executes a test run for the specified workflow.
func (h *AutomationHandler) TestRunWorkflow(w http.ResponseWriter, r *http.Request) {
	wfID := chi.URLParam(r, "id")
	if wfID == "" {
		http.Error(w, `{"error":"workflow_id_required"}`, http.StatusBadRequest)
		return
	}

	var req automation.TargetInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request_body"}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.TargetID) == "" || strings.TrimSpace(req.TargetType) == "" {
		http.Error(w, `{"error":"missing_target","message":"target_id and target_type are required"}`, http.StatusBadRequest)
		return
	}

	run, err := h.service.TestRunWorkflow(r.Context(), wfID, req)
	if err != nil {
		if strings.Contains(err.Error(), "nicht gefunden") {
			http.Error(w, fmt.Sprintf(`{"error":"not_found","message":%q}`, err.Error()), http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf(`{"error":"test_run_failed","message":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(run)
}
