package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/todo"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type TodoHandler struct {
	service *todo.Service
}

func NewTodoHandler(service *todo.Service) *TodoHandler {
	return &TodoHandler{service: service}
}

type CreateTodoRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
}

func (h *TodoHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}

	var req CreateTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	newTodo, err := h.service.Create(r.Context(), actorID, todo.CreateTodoInput{
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
	})

	if err != nil {
		if err == todo.ErrInvalidTodo {
			http.Error(w, `{"error":"validation_error","message":"title is required"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"server_error","message":"failed to create todo"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(newTodo)
}

func (h *TodoHandler) List(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	todos, err := h.service.List(r.Context(), int32(limit), int32(offset))
	if err != nil {
		http.Error(w, `{"error":"failed to list todos"}`, http.StatusInternalServerError)
		return
	}

	if todos == nil {
		todos = []db.Todo{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(todos)
}

func (h *TodoHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	todoItem, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"todo_not_found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(todoItem)
}

type UpdateTodoRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	DueDate     string `json:"due_date"`
}

func (h *TodoHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID.String())
	}

	idStr := chi.URLParam(r, "id")
	var todoID pgtype.UUID
	if err := todoID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	var req UpdateTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	existing, err := h.service.GetByID(r.Context(), todoID)
	if err != nil {
		http.Error(w, `{"error":"todo_not_found"}`, http.StatusNotFound)
		return
	}

	title := req.Title
	if title == "" {
		title = existing.Title
	}
	status := req.Status
	if status == "" {
		status = string(existing.Status)
	}
	priority := req.Priority
	if priority == "" {
		priority = string(existing.Priority)
	}
	desc := req.Description
	if desc == "" && existing.Description.Valid {
		desc = existing.Description.String
	}

	var dueDatePtr *time.Time
	if req.DueDate != "" {
		if parsedTime, err := time.Parse(time.RFC3339, req.DueDate); err == nil {
			dueDatePtr = &parsedTime
		} else if parsedDate, err := time.Parse("2006-01-02", req.DueDate); err == nil {
			dueDatePtr = &parsedDate
		}
	} else if existing.DueDate.Valid {
		t := existing.DueDate.Time
		dueDatePtr = &t
	}

	updated, err := h.service.Update(r.Context(), actorID, todo.UpdateTodoInput{
		ID:          todoID,
		Title:       title,
		Description: desc,
		Status:      status,
		Priority:    priority,
		DueDate:     dueDatePtr,
	})
	if err != nil {
		http.Error(w, `{"error":"failed to update todo: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (h *TodoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	var todoID pgtype.UUID
	if err := todoID.Scan(idStr); err != nil {
		http.Error(w, `{"error":"invalid_id"}`, http.StatusBadRequest)
		return
	}

	if claims.Role != "ADMIN" {
		existing, err := h.service.GetByID(r.Context(), todoID)
		if err != nil {
			http.Error(w, `{"error":"todo_not_found"}`, http.StatusNotFound)
			return
		}
		if !existing.AssignedTo.Valid || existing.AssignedTo.Bytes != claims.UserID {
			http.Error(w, `{"error":"forbidden","message":"Sie können nur Ihnen zugewiesene Aufgaben löschen"}`, http.StatusForbidden)
			return
		}
	}

	var actorID pgtype.UUID
	_ = actorID.Scan(claims.UserID.String())

	if err := h.service.Delete(r.Context(), actorID, todoID); err != nil {
		http.Error(w, `{"error":"failed to delete todo"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
