package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

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
	if wf.ID == "" {
		wf.ID = fmt.Sprintf("wf-%d", time.Now().UnixNano())
	}
	wf.CreatedAt = time.Now().UTC()
	wf.UpdatedAt = time.Now().UTC()

	if s.querier != nil {
		stepsJSON, _ := json.Marshal(wf.Steps)
		created, err := s.querier.CreateWorkflow(ctx, db.CreateWorkflowParams{
			ID:          wf.ID,
			Name:        wf.Name,
			Description: wf.Description,
			TriggerType: wf.TriggerType,
			TargetType:  wf.TargetType,
			IsActive:    wf.IsActive,
			StepsJson:   stepsJSON,
		})
		if err == nil {
			wf.ID = created.ID
			wf.CreatedAt = created.CreatedAt.Time
			wf.UpdatedAt = created.UpdatedAt.Time
		}
	}

	return &wf, nil
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
