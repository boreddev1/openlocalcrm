package handlers

import (
	"encoding/json"
	"net/http"
	"time"

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
	now := time.Now()
	runs := []automation.WorkflowRun{
		{
			ID:          "run-101",
			WorkflowID:  "wf-1",
			TargetID:    "contact-1",
			TargetType:  "CONTACT",
			TargetName:  "Dr. Michael Weber",
			Status:      "WAITING_APPROVAL",
			CurrentStep: 2,
			StartedAt:   now.Add(-10 * time.Minute),
		},
		{
			ID:          "run-102",
			WorkflowID:  "wf-2",
			TargetID:    "deal-2",
			TargetType:  "DEAL",
			TargetName:  "30 kWp Gewerbedach Solaranlage",
			Status:      "COMPLETED",
			CurrentStep: 2,
			StartedAt:   now.Add(-2 * time.Hour),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}
