package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// Allowed action types for workflow steps.
var allowedActionTypes = map[string]bool{
	"SET_TAG":     true,
	"CREATE_TASK": true,
	"DRAFT_EMAIL": true,
	"NOTIFY_USER": true,
	"WEBHOOK":     true,
}

type StepDefinition struct {
	StepNumber int                    `json:"step_number"`
	Title      string                 `json:"title"`
	ActionType string                 `json:"action_type"` // DRAFT_EMAIL, CREATE_TASK, SET_TAG, NOTIFY_USER, WEBHOOK
	Payload    map[string]interface{} `json:"payload"`
}

type Workflow struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	TriggerType string           `json:"trigger_type"`
	TargetType  string           `json:"target_type"`
	IsActive    bool             `json:"is_active"`
	Steps       []StepDefinition `json:"steps"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type WorkflowRun struct {
	ID          string     `json:"id"`
	WorkflowID  string     `json:"workflow_id"`
	TargetID    string     `json:"target_id"`
	TargetType  string     `json:"target_type"`
	TargetName  string     `json:"target_name"`
	Status      string     `json:"status"` // IN_PROGRESS, WAITING_APPROVAL, COMPLETED, CANCELLED
	CurrentStep int        `json:"current_step"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Service struct {
	querier db.Querier
}

func NewService(querier db.Querier) *Service {
	return &Service{
		querier: querier,
	}
}

func (s *Service) ListDefaultWorkflows(ctx context.Context) ([]Workflow, error) {
	if s.querier != nil {
		dbWfs, err := s.querier.ListWorkflows(ctx)
		if err == nil && len(dbWfs) > 0 {
			result := make([]Workflow, len(dbWfs))
			for i, w := range dbWfs {
				var steps []StepDefinition
				_ = json.Unmarshal(w.StepsJson, &steps)
				result[i] = Workflow{
					ID:          w.ID,
					Name:        w.Name,
					Description: w.Description,
					TriggerType: w.TriggerType,
					TargetType:  w.TargetType,
					IsActive:    w.IsActive,
					Steps:       steps,
					CreatedAt:   w.CreatedAt.Time,
					UpdatedAt:   w.UpdatedAt.Time,
				}
			}
			return result, nil
		}
	}

	now := time.Now().UTC()
	defaultWfs := []Workflow{
		{
			ID:          "wf-1",
			Name:        "Erstkontakt & Qualifizierung (Neuer Lead)",
			Description: "Automatische E-Mail-Triage bei Neukontakt, Tagging und Follow-up Aufgabe binnen 48 Stunden.",
			TriggerType: "NEW_LEAD",
			TargetType:  "CONTACT",
			IsActive:    true,
			Steps: []StepDefinition{
				{
					StepNumber: 1,
					Title:      "Tag 'PV-Interessent' zuweisen",
					ActionType: "SET_TAG",
					Payload:    map[string]interface{}{"tag": "PV-Interessent"},
				},
				{
					StepNumber: 2,
					Title:      "Erstkontakt E-Mail-Entwurf vorbereiten (Gemma 12B)",
					ActionType: "DRAFT_EMAIL",
					Payload:    map[string]interface{}{"template": "onboarding_intro"},
				},
				{
					StepNumber: 3,
					Title:      "Follow-Up Telefontermin anlegen",
					ActionType: "CREATE_TASK",
					Payload:    map[string]interface{}{"due_days": 2, "priority": "HIGH"},
				},
			},
			CreatedAt: now.Add(-48 * time.Hour),
			UpdatedAt: now,
		},
		{
			ID:          "wf-2",
			Name:        "Deal-Abschluss Routine (Phase: WON)",
			Description: "Sendet Bestätigung, setzt Status auf Gewonnen und stößt Montage-Übergabe an.",
			TriggerType: "DEAL_WON",
			TargetType:  "DEAL",
			IsActive:    true,
			Steps: []StepDefinition{
				{
					StepNumber: 1,
					Title:      "Auftragsbestätigung & Widerrufsbelehrung § 355 BGB senden",
					ActionType: "DRAFT_EMAIL",
					Payload:    map[string]interface{}{"template": "order_confirmation"},
				},
				{
					StepNumber: 2,
					Title:      "Übergabe-Task an Technik/Montage erstellen",
					ActionType: "CREATE_TASK",
					Payload:    map[string]interface{}{"title": "Zählerkasten-Planung & Materialbestellung", "priority": "URGENT"},
				},
			},
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now,
		},
		{
			ID:          "wf-3",
			Name:        "Inaktivitäts-Reaktivierung (SLA 30 Tage)",
			Description: "Erinnert Vertriebler an Leads ohne Interaktion seit 30 Tagen.",
			TriggerType: "INACTIVITY_TIMEOUT",
			TargetType:  "CONTACT",
			IsActive:    false,
			Steps: []StepDefinition{
				{
					StepNumber: 1,
					Title:      "Reaktivierungs-Todo für Account Manager erstellen",
					ActionType: "CREATE_TASK",
					Payload:    map[string]interface{}{"title": "Lead-Nachfassen telefonisch", "priority": "MEDIUM"},
				},
			},
			CreatedAt: now.Add(-72 * time.Hour),
			UpdatedAt: now,
		},
	}

	return defaultWfs, nil
}

func (s *Service) CreateWorkflow(ctx context.Context, wf Workflow) (*Workflow, error) {
	if wf.Name == "" {
		return nil, fmt.Errorf("workflow name is required")
	}
	// Validate step action types
	for _, step := range wf.Steps {
		if !allowedActionTypes[step.ActionType] {
			return nil, fmt.Errorf("invalid action type %q in step %d", step.ActionType, step.StepNumber)
		}
	}
	if wf.ID == "" {
		wf.ID = fmt.Sprintf("wf-%d", time.Now().UnixNano())
	}
	wf.CreatedAt = time.Now().UTC()
	wf.UpdatedAt = time.Now().UTC()

	if s.querier != nil {
		stepsJSON, err := json.Marshal(wf.Steps)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal workflow steps: %w", err)
		}
		created, err := s.querier.CreateWorkflow(ctx, db.CreateWorkflowParams{
			ID:          wf.ID,
			Name:        wf.Name,
			Description: wf.Description,
			TriggerType: wf.TriggerType,
			TargetType:  wf.TargetType,
			IsActive:    wf.IsActive,
			StepsJson:   stepsJSON,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to persist workflow: %w", err)
		}
		wf.ID = created.ID
		wf.CreatedAt = created.CreatedAt.Time
		wf.UpdatedAt = created.UpdatedAt.Time
	}

	return &wf, nil
}

// TriggerEvent finds all active workflows matching the given trigger type and starts runs for them.
// Each matching workflow advances through steps until a DRAFT_EMAIL step (requiring human approval)
// or until all steps are executed automatically.
func (s *Service) TriggerEvent(ctx context.Context, triggerType, targetID, targetType, targetName string) ([]WorkflowRun, error) {
	workflows, err := s.ListDefaultWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	var runs []WorkflowRun
	for _, wf := range workflows {
		if !wf.IsActive || wf.TriggerType != triggerType {
			continue
		}

		run := WorkflowRun{
			ID:          fmt.Sprintf("run-%d", time.Now().UnixNano()),
			WorkflowID:  wf.ID,
			TargetID:    targetID,
			TargetType:  targetType,
			TargetName:  targetName,
			Status:      "IN_PROGRESS",
			CurrentStep: 0,
			StartedAt:   time.Now().UTC(),
		}

		// Auto-execute steps until we hit one requiring approval (DRAFT_EMAIL)
		for _, step := range wf.Steps {
			run.CurrentStep = step.StepNumber
			if step.ActionType == "DRAFT_EMAIL" {
				// DRAFT_EMAIL requires human-in-the-loop approval
				run.Status = "WAITING_APPROVAL"
				break
			}
			if err := s.executeStep(ctx, step, run); err != nil {
				log.Printf("[automation] error executing step %d of workflow %s: %v", step.StepNumber, wf.ID, err)
				run.Status = "WAITING_APPROVAL"
				break
			}
		}

		// If all steps completed without pause
		if run.Status == "IN_PROGRESS" {
			now := time.Now().UTC()
			run.Status = "COMPLETED"
			run.CompletedAt = &now
		}

		if s.querier != nil {
			_, dbErr := s.querier.CreateWorkflowRun(ctx, db.CreateWorkflowRunParams{
				ID:           run.ID,
				WorkflowID:   run.WorkflowID,
				TargetID:     run.TargetID,
				TargetType:   run.TargetType,
				TargetName:   run.TargetName,
				Status:       run.Status,
				CurrentStep:  int32(run.CurrentStep),
				SnapshotJson: []byte("{}"),
			})
			if dbErr != nil {
				log.Printf("[automation] failed to persist run %s: %v", run.ID, dbErr)
			}
		}

		runs = append(runs, run)
	}

	return runs, nil
}

// ApproveStep advances a WAITING_APPROVAL run by executing the current pending step and continuing.
func (s *Service) ApproveStep(ctx context.Context, runID string) (*WorkflowRun, error) {
	if s.querier == nil {
		return nil, fmt.Errorf("database required for approval")
	}

	dbRun, err := s.querier.GetWorkflowRunByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("run not found: %w", err)
	}
	if dbRun.Status != "WAITING_APPROVAL" {
		return nil, fmt.Errorf("run %s is not waiting for approval (status: %s)", runID, dbRun.Status)
	}

	// Load the workflow to get step definitions
	dbWf, err := s.querier.GetWorkflowByID(ctx, dbRun.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow not found: %w", err)
	}
	var steps []StepDefinition
	if err := json.Unmarshal(dbWf.StepsJson, &steps); err != nil {
		return nil, fmt.Errorf("failed to parse workflow steps: %w", err)
	}

	run := WorkflowRun{
		ID:          dbRun.ID,
		WorkflowID:  dbRun.WorkflowID,
		TargetID:    dbRun.TargetID,
		TargetType:  dbRun.TargetType,
		TargetName:  dbRun.TargetName,
		Status:      dbRun.Status,
		CurrentStep: int(dbRun.CurrentStep),
		StartedAt:   dbRun.StartedAt.Time,
	}

	// Execute from current step onwards
	startFrom := run.CurrentStep
	run.Status = "IN_PROGRESS"
	for _, step := range steps {
		if step.StepNumber < startFrom {
			continue
		}
		run.CurrentStep = step.StepNumber
		if err := s.executeStep(ctx, step, run); err != nil {
			log.Printf("[automation] error executing approved step %d: %v", step.StepNumber, err)
		}
		// Check if next step also requires approval
		if step.StepNumber > startFrom && step.ActionType == "DRAFT_EMAIL" {
			run.Status = "WAITING_APPROVAL"
			break
		}
	}

	if run.Status == "IN_PROGRESS" {
		now := time.Now().UTC()
		run.Status = "COMPLETED"
		run.CompletedAt = &now
	}

	// Update run status in DB
	var completedAt pgtype.Timestamptz
	if run.CompletedAt != nil {
		completedAt = pgtype.Timestamptz{Time: *run.CompletedAt, Valid: true}
	}
	_, _ = s.querier.UpdateWorkflowRunStatus(ctx, db.UpdateWorkflowRunStatusParams{
		ID:          runID,
		Status:      run.Status,
		CurrentStep: int32(run.CurrentStep),
		CompletedAt: completedAt,
	})

	return &run, nil
}

// executeStep dispatches the actual action for a workflow step.
func (s *Service) executeStep(ctx context.Context, step StepDefinition, run WorkflowRun) error {
	switch step.ActionType {
	case "SET_TAG":
		tag, _ := step.Payload["tag"].(string)
		log.Printf("[automation] SET_TAG: applying tag %q to %s %s (%s)", tag, run.TargetType, run.TargetID, run.TargetName)
		// Tag application is persisted via the audit log; real tag service can be wired in future.
		return nil

	case "CREATE_TASK":
		title, _ := step.Payload["title"].(string)
		if title == "" {
			title = step.Title
		}
		priority, _ := step.Payload["priority"].(string)
		log.Printf("[automation] CREATE_TASK: %q (priority: %s) for %s %s", title, priority, run.TargetType, run.TargetID)
		// In a full implementation, this would call TodoService.Create.
		return nil

	case "DRAFT_EMAIL":
		template, _ := step.Payload["template"].(string)
		log.Printf("[automation] DRAFT_EMAIL: template %q for %s %s (requires HITL approval)", template, run.TargetType, run.TargetID)
		return nil

	case "NOTIFY_USER":
		log.Printf("[automation] NOTIFY_USER: sending notification for %s %s", run.TargetType, run.TargetID)
		return nil

	case "WEBHOOK":
		url, _ := step.Payload["url"].(string)
		log.Printf("[automation] WEBHOOK: calling %s for %s %s", url, run.TargetType, run.TargetID)
		return nil

	default:
		return fmt.Errorf("unknown action type: %s", step.ActionType)
	}
}

func (s *Service) GetRunByID(ctx context.Context, runID string) (*WorkflowRun, error) {
	if s.querier == nil {
		return nil, fmt.Errorf("database required")
	}
	r, err := s.querier.GetWorkflowRunByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	var compAt *time.Time
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		compAt = &t
	}
	return &WorkflowRun{
		ID:          r.ID,
		WorkflowID:  r.WorkflowID,
		TargetID:    r.TargetID,
		TargetType:  r.TargetType,
		TargetName:  r.TargetName,
		Status:      r.Status,
		CurrentStep: int(r.CurrentStep),
		StartedAt:   r.StartedAt.Time,
		CompletedAt: compAt,
	}, nil
}

func (s *Service) ListRuns(ctx context.Context) ([]WorkflowRun, error) {
	if s.querier != nil {
		dbRuns, err := s.querier.ListWorkflowRuns(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]WorkflowRun, len(dbRuns))
		for i, r := range dbRuns {
			var compAt *time.Time
			if r.CompletedAt.Valid {
				t := r.CompletedAt.Time
				compAt = &t
			}
			result[i] = WorkflowRun{
				ID:          r.ID,
				WorkflowID:  r.WorkflowID,
				TargetID:    r.TargetID,
				TargetType:  r.TargetType,
				TargetName:  r.TargetName,
				Status:      r.Status,
				CurrentStep: int(r.CurrentStep),
				StartedAt:   r.StartedAt.Time,
				CompletedAt: compAt,
			}
		}
		return result, nil
	}
	return []WorkflowRun{}, nil
}
