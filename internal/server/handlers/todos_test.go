package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/todo"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

func TestTodoHandler_Endpoints(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	auditService := audit.NewService(querier)
	svc := todo.NewService(querier, auditService)
	h := handlers.NewTodoHandler(svc)

	adminUUID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.UserContextKey, &auth.AccessClaims{
		UserID: adminUUID,
		Role:   "ADMIN",
	})

	// 1. Create todo
	createBody, _ := json.Marshal(handlers.CreateTodoRequest{
		Title:       "Kundenanruf Dr. Weber",
		Description: "Details zur Speicherförderung besprechen",
		Status:      "OPEN",
		Priority:    "HIGH",
	})
	reqCreate := httptest.NewRequest("POST", "/api/v1/todos", bytes.NewReader(createBody)).WithContext(ctx)
	recCreate := httptest.NewRecorder()
	h.Create(recCreate, reqCreate)

	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var created db.Todo
	_ = json.NewDecoder(recCreate.Body).Decode(&created)
	todoIDStr := uuid.UUID(created.ID.Bytes).String()

	// 2. List todos
	reqList := httptest.NewRequest("GET", "/api/v1/todos", nil).WithContext(ctx)
	recList := httptest.NewRecorder()
	h.List(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recList.Code)
	}

	// 3. Get todo by ID
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", todoIDStr)
	reqGet := httptest.NewRequest("GET", "/api/v1/todos/"+todoIDStr, nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recGet := httptest.NewRecorder()
	h.Get(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", recGet.Code)
	}

	// 4. Update todo
	updateBody, _ := json.Marshal(handlers.UpdateTodoRequest{
		Title:  "Kundenanruf Dr. Weber - Erledigt",
		Status: "DONE",
	})
	reqUpdate := httptest.NewRequest("PUT", "/api/v1/todos/"+todoIDStr, bytes.NewReader(updateBody)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recUpdate := httptest.NewRecorder()
	h.Update(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on update, got %d: %s", recUpdate.Code, recUpdate.Body.String())
	}

	// 5. Delete todo
	reqDelete := httptest.NewRequest("DELETE", "/api/v1/todos/"+todoIDStr, nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recDelete := httptest.NewRecorder()
	h.Delete(recDelete, reqDelete)
	if recDelete.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on delete, got %d: %s", recDelete.Code, recDelete.Body.String())
	}
}
