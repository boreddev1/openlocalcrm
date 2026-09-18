package todo_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/todo"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestTodoService_Lifecycle(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := todo.NewService(querier, auditService)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}

	// 1. Validation error on empty title
	_, err := svc.Create(ctx, actorID, todo.CreateTodoInput{Title: ""})
	if err != todo.ErrInvalidTodo {
		t.Fatalf("expected ErrInvalidTodo, got %v", err)
	}

	// 2. Create valid todo
	due := time.Now().Add(48 * time.Hour)
	created, err := svc.Create(ctx, actorID, todo.CreateTodoInput{
		Title:       "Angebot für PV-Erweiterung nachfassen",
		Description: "Kunde hat nach Speichergröße gefragt",
		DueDate:     &due,
		Priority:    "HIGH",
	})
	if err != nil {
		t.Fatalf("failed to create todo: %v", err)
	}
	if created.Title != "Angebot für PV-Erweiterung nachfassen" {
		t.Fatalf("unexpected title: %s", created.Title)
	}

	// 3. Get todo
	fetched, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to get todo: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected ID %v, got %v", created.ID, fetched.ID)
	}

	// 4. Update status
	updated, err := svc.UpdateStatus(ctx, actorID, created.ID, "DONE")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
	if updated.Status != "DONE" {
		t.Fatalf("unexpected status: %s", updated.Status)
	}

	// 5. List todos
	todos, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to list todos: %v", err)
	}
	if len(todos) == 0 {
		t.Fatalf("expected at least 1 todo")
	}

	// 6. Delete todo
	if err := svc.Delete(ctx, actorID, created.ID); err != nil {
		t.Fatalf("failed to delete todo: %v", err)
	}
}
