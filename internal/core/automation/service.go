package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/notification"
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
	ID          string         `json:"id"`
	WorkflowID  string         `json:"workflow_id"`
	TargetID    string         `json:"target_id"`
	TargetType  string         `json:"target_type"`
	TargetName  string         `json:"target_name"`
	Status      string         `json:"status"` // IN_PROGRESS, WAITING_APPROVAL, COMPLETED, CANCELLED, FAILED
	CurrentStep int            `json:"current_step"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Snapshot    map[string]any `json:"snapshot,omitempty"`
	Error       string         `json:"error,omitempty"`
}

type TargetInput struct {
	TargetID   string `json:"target_id"`
	TargetType string `json:"target_type"`
	TargetName string `json:"target_name"`
}

// UserNotifier is a narrow interface for creating notifications with SSE broadcast.
type UserNotifier interface {
	Create(ctx context.Context, input notification.CreateNotificationInput) (db.Notification, error)
}

// EmailDrafter is a narrow interface for drafting emails via AI gateway (Task B).
type EmailDrafter interface {
	DraftEmail(ctx context.Context, template string, targetType, targetID, targetName string) error
}

type Service struct {
	querier  db.Querier
	notifier UserNotifier
	drafter  EmailDrafter
}

func NewService(querier db.Querier, notifier ...UserNotifier) *Service {
	s := &Service{
		querier: querier,
	}
	if len(notifier) > 0 {
		s.notifier = notifier[0]
	}
	return s
}

func (s *Service) SetNotifier(n UserNotifier) {
	s.notifier = n
}

func (s *Service) SetEmailDrafter(d EmailDrafter) {
	s.drafter = d
}

func (s *Service) ListDefaultWorkflows(ctx context.Context) ([]Workflow, error) {
	if s.querier != nil {
		dbWfs, err := s.querier.ListWorkflows(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list workflows: %w", err)
		}
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
	return []Workflow{}, nil
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

		snapshot := make(map[string]any)

		// Auto-execute steps until we hit one requiring approval (DRAFT_EMAIL)
		for _, step := range wf.Steps {
			run.CurrentStep = step.StepNumber
			if step.ActionType == "DRAFT_EMAIL" {
				// DRAFT_EMAIL requires human-in-the-loop approval
				run.Status = "WAITING_APPROVAL"
				break
			}
			if err := s.executeStep(ctx, step, &run, snapshot); err != nil {
				log.Printf("[automation] error executing step %d of workflow %s: %v", step.StepNumber, wf.ID, err)
				run.Status = "FAILED"
				now := time.Now().UTC()
				run.CompletedAt = &now
				snapshot["error"] = fmt.Sprintf("Schritt %d (%s) fehlgeschlagen: %v", step.StepNumber, step.Title, err)
				break
			}
		}

		// If all steps completed without pause
		if run.Status == "IN_PROGRESS" {
			now := time.Now().UTC()
			run.Status = "COMPLETED"
			run.CompletedAt = &now
		}

		run.Snapshot = snapshot
		if e, ok := snapshot["error"].(string); ok {
			run.Error = e
		}

		if s.querier != nil {
			snapBytes, _ := json.Marshal(snapshot)
			_, dbErr := s.querier.CreateWorkflowRun(ctx, db.CreateWorkflowRunParams{
				ID:           run.ID,
				WorkflowID:   run.WorkflowID,
				TargetID:     run.TargetID,
				TargetType:   run.TargetType,
				TargetName:   run.TargetName,
				Status:       run.Status,
				CurrentStep:  int32(run.CurrentStep),
				SnapshotJson: snapBytes,
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

	snapshot := make(map[string]any)
	if len(dbRun.SnapshotJson) > 0 {
		_ = json.Unmarshal(dbRun.SnapshotJson, &snapshot)
	}

	// Execute from current step onwards
	startFrom := run.CurrentStep
	run.Status = "IN_PROGRESS"
	var execErr error
	for _, step := range steps {
		if step.StepNumber < startFrom {
			continue
		}
		run.CurrentStep = step.StepNumber
		if err := s.executeStep(ctx, step, &run, snapshot); err != nil {
			log.Printf("[automation] error executing approved step %d: %v", step.StepNumber, err)
			execErr = err
			run.Status = "FAILED"
			now := time.Now().UTC()
			run.CompletedAt = &now
			snapshot["error"] = fmt.Sprintf("Schritt %d (%s) fehlgeschlagen: %v", step.StepNumber, step.Title, err)
			break
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

	run.Snapshot = snapshot
	if e, ok := snapshot["error"].(string); ok {
		run.Error = e
	}

	// Update run status in DB
	var completedAt pgtype.Timestamptz
	if run.CompletedAt != nil {
		completedAt = pgtype.Timestamptz{Time: *run.CompletedAt, Valid: true}
	}
	snapBytes, _ := json.Marshal(snapshot)
	if s.querier != nil {
		if _, err := s.querier.UpdateWorkflowRunStatus(ctx, db.UpdateWorkflowRunStatusParams{
			ID:           runID,
			Status:       run.Status,
			CurrentStep:  int32(run.CurrentStep),
			CompletedAt:  completedAt,
			SnapshotJson: snapBytes,
		}); err != nil {
			log.Printf("[automation] failed to update workflow run status in db: %v", err)
			if execErr == nil {
				return nil, fmt.Errorf("failed to persist workflow run status: %w", err)
			}
		}
	}

	if execErr != nil {
		return &run, fmt.Errorf("workflow execution failed at step %d: %w", run.CurrentStep, execErr)
	}

	return &run, nil
}

// executeStep dispatches the actual action for a workflow step.
func (s *Service) executeStep(ctx context.Context, step StepDefinition, run *WorkflowRun, snapshot map[string]any) error {
	switch step.ActionType {
	case "SET_TAG":
		tag, _ := step.Payload["tag"].(string)
		if tag == "" {
			return fmt.Errorf("SET_TAG requires non-empty tag in payload")
		}
		log.Printf("[automation] SET_TAG: applying tag %q to %s %s (%s)", tag, run.TargetType, run.TargetID, run.TargetName)
		if s.querier != nil {
			targetUUID, err := uuid.Parse(run.TargetID)
			if err != nil {
				return fmt.Errorf("invalid target ID %q for SET_TAG: %w", run.TargetID, err)
			}
			tagBytes, err := json.Marshal([]string{tag})
			if err != nil {
				return fmt.Errorf("failed to marshal tag: %w", err)
			}

			if strings.EqualFold(run.TargetType, "contact") {
				if _, err := s.querier.UpdateContactTags(ctx, db.UpdateContactTagsParams{
					Tags: tagBytes,
					ID:   pgtype.UUID{Bytes: targetUUID, Valid: true},
				}); err != nil {
					log.Printf("[automation_error] SET_TAG on contact failed: %v", err)
					return err
				}
			} else if strings.EqualFold(run.TargetType, "deal") {
				if _, err := s.querier.UpdateDealTags(ctx, db.UpdateDealTagsParams{
					Tags: tagBytes,
					ID:   pgtype.UUID{Bytes: targetUUID, Valid: true},
				}); err != nil {
					log.Printf("[automation_error] SET_TAG on deal failed: %v", err)
					return err
				}
			}

			details, _ := json.Marshal(map[string]any{
				"tag":         tag,
				"workflow_id": run.WorkflowID,
				"run_id":      run.ID,
			})
			if _, err := s.querier.CreateAuditLog(ctx, db.CreateAuditLogParams{
				EntityType: run.TargetType,
				EntityID:   pgtype.UUID{Bytes: targetUUID, Valid: targetUUID != uuid.Nil},
				Action:     "TAG_APPLIED",
				Changes:    details,
			}); err != nil {
				log.Printf("[automation_error] SET_TAG audit log failed: %v", err)
				return err
			}
		}
		return nil

	case "CREATE_TASK":
		title, _ := step.Payload["title"].(string)
		if title == "" {
			title = step.Title
		}
		priority, _ := step.Payload["priority"].(string)
		if priority == "" {
			priority = "MEDIUM"
		}
		log.Printf("[automation] CREATE_TASK: %q (priority: %s) for %s %s", title, priority, run.TargetType, run.TargetID)
		if s.querier != nil {
			var contactID, dealID pgtype.UUID
			if targetUUID, err := uuid.Parse(run.TargetID); err == nil {
				if strings.EqualFold(run.TargetType, "contact") {
					contactID = pgtype.UUID{Bytes: targetUUID, Valid: true}
				} else if strings.EqualFold(run.TargetType, "deal") {
					dealID = pgtype.UUID{Bytes: targetUUID, Valid: true}
				}
			}
			_, err := s.querier.CreateTodo(ctx, db.CreateTodoParams{
				Title:       title,
				Description: pgtype.Text{String: fmt.Sprintf("Automatisch erstellt durch Workflow %s für %s (%s)", run.WorkflowID, run.TargetName, run.TargetType), Valid: true},
				DueDate:     pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
				Status:      "OPEN",
				Priority:    priority,
				ContactID:   contactID,
				DealID:      dealID,
			})
			if err != nil {
				log.Printf("[automation_error] CREATE_TASK failed: %v", err)
				return err
			}
		}
		return nil

	case "DRAFT_EMAIL":
		template, _ := step.Payload["template"].(string)
		log.Printf("[automation] DRAFT_EMAIL: template %q for %s %s", template, run.TargetType, run.TargetID)
		if s.drafter != nil {
			return s.drafter.DraftEmail(ctx, template, run.TargetType, run.TargetID, run.TargetName)
		}
		return fmt.Errorf("email draft service not configured")

	case "NOTIFY_USER":
		msg, _ := step.Payload["message"].(string)
		if msg == "" {
			msg = fmt.Sprintf("Workflow %s: Schritt %d ausgeführt für %s", run.WorkflowID, step.StepNumber, run.TargetName)
		}
		log.Printf("[automation] NOTIFY_USER: resolving assignee for %s %s", run.TargetType, run.TargetID)
		if strings.EqualFold(run.TargetType, "contact") {
			// Contacts have NO owner column. Honest status: SKIPPED
			log.Printf("[automation] NOTIFY_USER skipped for contact %s: kein zuständiger Benutzer", run.TargetID)
			if snapshot != nil {
				snapshot[fmt.Sprintf("step_%d_status", step.StepNumber)] = "SKIPPED"
				snapshot[fmt.Sprintf("step_%d_reason", step.StepNumber)] = "Kein zuständiger Benutzer ermittelt"
			}
			return nil
		}
		if strings.EqualFold(run.TargetType, "deal") {
			dealUUID, err := uuid.Parse(run.TargetID)
			if err != nil {
				return fmt.Errorf("invalid deal ID for NOTIFY_USER: %w", err)
			}
			var assigneeID pgtype.UUID
			if s.querier != nil {
				dealObj, err := s.querier.GetDealByID(ctx, pgtype.UUID{Bytes: dealUUID, Valid: true})
				if err != nil {
					return fmt.Errorf("failed to retrieve deal for notification: %w", err)
				}
				if dealObj.AssignedTo.Valid && dealObj.AssignedTo.Bytes != uuid.Nil {
					assigneeID = dealObj.AssignedTo
				}
			}
			if !assigneeID.Valid || assigneeID.Bytes == uuid.Nil {
				log.Printf("[automation] NOTIFY_USER skipped for unassigned deal %s", run.TargetID)
				if snapshot != nil {
					snapshot[fmt.Sprintf("step_%d_status", step.StepNumber)] = "SKIPPED"
					snapshot[fmt.Sprintf("step_%d_reason", step.StepNumber)] = "Kein zuständiger Benutzer ermittelt"
				}
				return nil
			}

			if s.notifier != nil {
				_, err := s.notifier.Create(ctx, notification.CreateNotificationInput{
					UserID:  &assigneeID,
					Type:    "WORKFLOW_COMPLETED",
					Title:   step.Title,
					Message: msg,
					Link:    "/automations",
				})
				if err != nil {
					return fmt.Errorf("failed to send notification: %w", err)
				}
			} else if s.querier != nil {
				_, err := s.querier.CreateNotification(ctx, db.CreateNotificationParams{
					UserID:  assigneeID,
					Type:    "WORKFLOW_COMPLETED",
					Title:   step.Title,
					Message: msg,
					Link:    pgtype.Text{String: "/automations", Valid: true},
					IsRead:  false,
				})
				if err != nil {
					return fmt.Errorf("failed to send notification: %w", err)
				}
			}
			return nil
		}
		if snapshot != nil {
			snapshot[fmt.Sprintf("step_%d_status", step.StepNumber)] = "SKIPPED"
			snapshot[fmt.Sprintf("step_%d_reason", step.StepNumber)] = "Kein zuständiger Benutzer ermittelt"
		}
		return nil

	case "WEBHOOK":
		webhookURL, _ := step.Payload["url"].(string)
		log.Printf("[automation] WEBHOOK: calling %s for %s %s", webhookURL, run.TargetType, run.TargetID)
		if webhookURL == "" {
			return fmt.Errorf("webhook action requires url payload")
		}
		client := &http.Client{Timeout: 5 * time.Second}
		payloadData, _ := json.Marshal(map[string]any{
			"workflow_id": run.WorkflowID,
			"run_id":      run.ID,
			"step_number": step.StepNumber,
			"step_title":  step.Title,
			"target_type": run.TargetType,
			"target_id":   run.TargetID,
			"target_name": run.TargetName,
			"payload":     step.Payload,
		})
		req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payloadData))
		if err != nil {
			return fmt.Errorf("failed to create webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("webhook request to %s failed: %w", webhookURL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("webhook responded with HTTP status %d", resp.StatusCode)
		}
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
	var snap map[string]any
	var errMsg string
	if len(r.SnapshotJson) > 0 {
		_ = json.Unmarshal(r.SnapshotJson, &snap)
		if e, ok := snap["error"].(string); ok {
			errMsg = e
		}
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
		Snapshot:    snap,
		Error:       errMsg,
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
			var snap map[string]any
			var errMsg string
			if len(r.SnapshotJson) > 0 {
				_ = json.Unmarshal(r.SnapshotJson, &snap)
				if e, ok := snap["error"].(string); ok {
					errMsg = e
				}
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
				Snapshot:    snap,
				Error:       errMsg,
			}
		}
		return result, nil
	}
	return []WorkflowRun{}, nil
}

// TestRunWorkflow executes a workflow test run for a specific target up to the first DRAFT_EMAIL action
// (which pauses for HITL approval) or until completion/failure.
func (s *Service) TestRunWorkflow(ctx context.Context, workflowID string, target TargetInput) (*WorkflowRun, error) {
	if s.querier == nil {
		return nil, fmt.Errorf("datenbank erforderlich für testlauf")
	}
	if strings.TrimSpace(target.TargetID) == "" || strings.TrimSpace(target.TargetType) == "" {
		return nil, fmt.Errorf("target_id und target_type sind erforderlich für den testlauf")
	}

	dbWf, err := s.querier.GetWorkflowByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow %q nicht gefunden: %w", workflowID, err)
	}

	var steps []StepDefinition
	if err := json.Unmarshal(dbWf.StepsJson, &steps); err != nil {
		return nil, fmt.Errorf("ungültige workflow-schritte: %w", err)
	}

	run := WorkflowRun{
		ID:          fmt.Sprintf("run-%d", time.Now().UnixNano()),
		WorkflowID:  dbWf.ID,
		TargetID:    target.TargetID,
		TargetType:  target.TargetType,
		TargetName:  target.TargetName,
		Status:      "IN_PROGRESS",
		CurrentStep: 0,
		StartedAt:   time.Now().UTC(),
	}

	snapshot := make(map[string]any)

	// Execute steps until the first step requiring approval (DRAFT_EMAIL)
	for _, step := range steps {
		run.CurrentStep = step.StepNumber
		if step.ActionType == "DRAFT_EMAIL" {
			// Pause before executing DRAFT_EMAIL (HITL approval required)
			run.Status = "WAITING_APPROVAL"
			break
		}
		if err := s.executeStep(ctx, step, &run, snapshot); err != nil {
			log.Printf("[automation] test run step %d failed: %v", step.StepNumber, err)
			run.Status = "FAILED"
			now := time.Now().UTC()
			run.CompletedAt = &now
			snapshot["error"] = fmt.Sprintf("Schritt %d (%s) fehlgeschlagen: %v", step.StepNumber, step.Title, err)
			run.Error = snapshot["error"].(string)
			break
		}
	}

	if run.Status == "IN_PROGRESS" {
		now := time.Now().UTC()
		run.Status = "COMPLETED"
		run.CompletedAt = &now
	}

	run.Snapshot = snapshot
	if run.Error == "" {
		if e, ok := snapshot["error"].(string); ok {
			run.Error = e
		}
	}

	snapBytes, _ := json.Marshal(snapshot)
	_, err = s.querier.CreateWorkflowRun(ctx, db.CreateWorkflowRunParams{
		ID:           run.ID,
		WorkflowID:   run.WorkflowID,
		TargetID:     run.TargetID,
		TargetType:   run.TargetType,
		TargetName:   run.TargetName,
		Status:       run.Status,
		CurrentStep:  int32(run.CurrentStep),
		SnapshotJson: snapBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("fehler beim speichern des testlaufs: %w", err)
	}

	return &run, nil
}
