package todo

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

var (
	ErrInvalidTodo  = errors.New("todo title is required")
	ErrTodoNotFound = errors.New("todo not found")
)

type Service struct {
	queries db.Querier
	audit   *audit.Service
}

func NewService(queries db.Querier, audit *audit.Service) *Service {
	return &Service{
		queries: queries,
		audit:   audit,
	}
}

type CreateTodoInput struct {
	Title       string
	Description string
	DueDate     *time.Time
	Status      string
	Priority    string
	AssignedTo  *pgtype.UUID
	ContactID   *pgtype.UUID
	DealID      *pgtype.UUID
}

func (s *Service) Create(ctx context.Context, actorID pgtype.UUID, input CreateTodoInput) (db.Todo, error) {
	if input.Title == "" {
		return db.Todo{}, ErrInvalidTodo
	}

	var assignID, contID, dealID pgtype.UUID
	if input.AssignedTo != nil {
		assignID = *input.AssignedTo
	}
	if input.ContactID != nil {
		contID = *input.ContactID
	}
	if input.DealID != nil {
		dealID = *input.DealID
	}

	status := input.Status
	if status == "" {
		status = "OPEN"
	}
	priority := input.Priority
	if priority == "" {
		priority = "MEDIUM"
	}

	var dueDate pgtype.Timestamptz
	if input.DueDate != nil {
		dueDate = pgtype.Timestamptz{Time: *input.DueDate, Valid: true}
	}

	todo, err := s.queries.CreateTodo(ctx, db.CreateTodoParams{
		Title:       input.Title,
		Description: pgtype.Text{String: input.Description, Valid: input.Description != ""},
		DueDate:     dueDate,
		Status:      status,
		Priority:    priority,
		AssignedTo:  assignID,
		ContactID:   contID,
		DealID:      dealID,
	})

	if err != nil {
		return db.Todo{}, err
	}

	if s.audit != nil {
		if auditErr := s.audit.Log(ctx, actorID, "TODO", todo.ID, "CREATE", todo, "", ""); auditErr != nil {
			log.Printf("[AUDIT_ERROR] Failed logging todo creation: %v", auditErr)
		}
	}

	return todo, nil
}

func (s *Service) UpdateStatus(ctx context.Context, actorID, todoID pgtype.UUID, status string) (db.Todo, error) {
	todo, err := s.queries.UpdateTodoStatus(ctx, db.UpdateTodoStatusParams{
		ID:     todoID,
		Status: status,
	})

	if err != nil {
		return db.Todo{}, err
	}

	if s.audit != nil {
		if auditErr := s.audit.Log(ctx, actorID, "TODO", todo.ID, "UPDATE_STATUS", map[string]string{"status": status}, "", ""); auditErr != nil {
			log.Printf("[AUDIT_ERROR] Failed logging todo status update: %v", auditErr)
		}
	}

	return todo, nil
}

func (s *Service) List(ctx context.Context, limit, offset int32) ([]db.Todo, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.queries.ListTodos(ctx, db.ListTodosParams{
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Service) GetByID(ctx context.Context, id pgtype.UUID) (db.Todo, error) {
	return s.queries.GetTodoByID(ctx, id)
}

type UpdateTodoInput struct {
	ID          pgtype.UUID
	Title       string
	Description string
	DueDate     *time.Time
	Status      string
	Priority    string
	AssignedTo  *pgtype.UUID
	ContactID   *pgtype.UUID
	DealID      *pgtype.UUID
}

func (s *Service) Update(ctx context.Context, actorID pgtype.UUID, input UpdateTodoInput) (db.Todo, error) {
	if input.Title == "" {
		return db.Todo{}, ErrInvalidTodo
	}

	var assignID, contID, dealID pgtype.UUID
	if input.AssignedTo != nil {
		assignID = *input.AssignedTo
	}
	if input.ContactID != nil {
		contID = *input.ContactID
	}
	if input.DealID != nil {
		dealID = *input.DealID
	}

	status := input.Status
	if status == "" {
		status = "OPEN"
	}
	priority := input.Priority
	if priority == "" {
		priority = "MEDIUM"
	}

	var dueDate pgtype.Timestamptz
	if input.DueDate != nil {
		dueDate = pgtype.Timestamptz{Time: *input.DueDate, Valid: true}
	}

	todo, err := s.queries.UpdateTodo(ctx, db.UpdateTodoParams{
		ID:          input.ID,
		Title:       input.Title,
		Description: pgtype.Text{String: input.Description, Valid: input.Description != ""},
		DueDate:     dueDate,
		Status:      status,
		Priority:    priority,
		AssignedTo:  assignID,
		ContactID:   contID,
		DealID:      dealID,
	})
	if err != nil {
		return db.Todo{}, err
	}

	if s.audit != nil {
		if auditErr := s.audit.Log(ctx, actorID, "TODO", todo.ID, "UPDATE", todo, "", ""); auditErr != nil {
			log.Printf("[AUDIT_ERROR] Failed logging todo update: %v", auditErr)
		}
	}

	return todo, nil
}

func (s *Service) Delete(ctx context.Context, actorID, todoID pgtype.UUID) error {
	err := s.queries.DeleteTodo(ctx, todoID)
	if err != nil {
		return err
	}
	if s.audit != nil {
		if auditErr := s.audit.Log(ctx, actorID, "TODO", todoID, "DELETE", map[string]string{"status": "deleted"}, "", ""); auditErr != nil {
			log.Printf("[AUDIT_ERROR] Failed logging todo deletion: %v", auditErr)
		}
	}
	return nil
}
