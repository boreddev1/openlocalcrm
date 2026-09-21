package handlers

import (
	"encoding/json"
	"net/http"

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
