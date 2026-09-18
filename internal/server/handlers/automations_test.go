package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/automation"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

func TestAutomationHandler(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := automation.NewService(q)
	h := handlers.NewAutomationHandler(svc)

	t.Run("ListWorkflows returns defaults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/automations/workflows", nil)
		rec := httptest.NewRecorder()

		h.ListWorkflows(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var workflows []automation.Workflow
		err := json.NewDecoder(rec.Body).Decode(&workflows)
		require.NoError(t, err)
		require.NotEmpty(t, workflows)
	})

	t.Run("CreateWorkflow creates custom workflow", func(t *testing.T) {
		newWf := automation.Workflow{
			Name:        "Solar Lead Nurturing",
			Description: "Automatische Nachfassung nach 48 Stunden",
			TriggerType: "DEAL_STAGE_CHANGED",
			TargetType:  "DEAL",
			IsActive:    true,
			Steps: []automation.StepDefinition{
				{
					StepNumber: 1,
					Title:      "E-Mail Entwurf erstellen",
					ActionType: "DRAFT_EMAIL",
				},
			},
		}
		body, _ := json.Marshal(newWf)
		req := httptest.NewRequest(http.MethodPost, "/api/automations/workflows", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.CreateWorkflow(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var created automation.Workflow
		err := json.NewDecoder(rec.Body).Decode(&created)
		require.NoError(t, err)
		require.Equal(t, newWf.Name, created.Name)
	})

	t.Run("CreateWorkflow invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/automations/workflows", bytes.NewReader([]byte("{invalid-json")))
		rec := httptest.NewRecorder()

		h.CreateWorkflow(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ListRuns returns simulated runs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/automations/runs", nil)
		rec := httptest.NewRecorder()

		h.ListRuns(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var runs []automation.WorkflowRun
		err := json.NewDecoder(rec.Body).Decode(&runs)
		require.NoError(t, err)
		require.Len(t, runs, 2)
	})
}
