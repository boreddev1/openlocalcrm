package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/appointment"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestAppointmentHandler_Endpoints(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	hub := sse.NewHub()
	svc := appointment.NewService(querier, hub)
	h := handlers.NewAppointmentHandler(svc)

	ctx := context.Background()

	// 1. List appointments
	reqList := httptest.NewRequest("GET", "/api/v1/appointments", nil)
	recList := httptest.NewRecorder()
	h.List(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list, got %d", recList.Code)
	}

	// 2. Create appointment
	start := time.Now().Add(24 * time.Hour)
	end := start.Add(1 * time.Hour)
	createBody, _ := json.Marshal(map[string]any{
		"title":        "Dachbegehung Weber",
		"type":         "ON_SITE",
		"contact_name": "Dr. Michael Weber",
		"start_time":   start,
		"end_time":     end,
		"location":     "Frankfurt",
		"notes":        "Leiter und Messwerkzeug mitbringen",
	})
	reqCreate := httptest.NewRequest("POST", "/api/v1/appointments", bytes.NewReader(createBody))
	recCreate := httptest.NewRecorder()
	h.Create(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var created appointment.Appointment
	_ = json.NewDecoder(recCreate.Body).Decode(&created)
	appID := created.ID

	// 3. Update appointment
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", appID)
	updateBody, _ := json.Marshal(map[string]any{
		"title":    "Dachbegehung Weber (Verschoben)",
		"location": "Frankfurt am Main",
	})
	reqUpdate := httptest.NewRequest("PUT", "/api/v1/appointments/"+appID, bytes.NewReader(updateBody)).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recUpdate := httptest.NewRecorder()
	h.Update(recUpdate, reqUpdate)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on update, got %d: %s", recUpdate.Code, recUpdate.Body.String())
	}

	// 4. Download ICS
	reqICS := httptest.NewRequest("GET", "/api/v1/appointments/"+appID+"/ics", nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recICS := httptest.NewRecorder()
	h.DownloadICS(recICS, reqICS)
	if recICS.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on ics download, got %d", recICS.Code)
	}
	if !bytes.Contains(recICS.Body.Bytes(), []byte("BEGIN:VCALENDAR")) {
		t.Fatalf("expected VCALENDAR content in ICS download")
	}

	// 6. Delete appointment
	reqDelete := httptest.NewRequest("DELETE", "/api/v1/appointments/"+appID, nil).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	recDelete := httptest.NewRecorder()
	h.Delete(recDelete, reqDelete)
	if recDelete.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on delete, got %d", recDelete.Code)
	}
}

func TestAppointmentHandler_DownloadICS_NotFoundReturns404(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := appointment.NewService(q, nil)
	h := handlers.NewAppointmentHandler(svc)

	req := httptest.NewRequest("GET", "/api/v1/appointments/00000000-0000-0000-0000-000000000099/ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "00000000-0000-0000-0000-000000000099")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.DownloadICS(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing appointment must return 404, got %d", rec.Code)
	}
}

func TestAppointmentHandler_DownloadICS_EnforcesOwnership(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	svc := appointment.NewService(q, nil)
	h := handlers.NewAppointmentHandler(svc)

	// Seed appointment assigned to other user
	app, err := svc.Create(context.Background(), pgtype.UUID{}, appointment.CreateAppointmentInput{
		Title:      "Geheimes Strategiegespräch",
		StartTime:  time.Now().Add(24 * time.Hour),
		EndTime:    time.Now().Add(25 * time.Hour),
		AssignedTo: "andere.person@example.de",
	})
	if err != nil {
		t.Fatalf("failed creating appointment: %v", err)
	}

	// Non-admin user attempts download
	claims := &auth.AccessClaims{
		UserID: uuid.New(),
		Email:  "hacker@example.de",
		Role:   "VERTRIEB",
	}
	req := httptest.NewRequest("GET", "/api/v1/appointments/"+app.ID+"/ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, auth.UserContextKey, claims)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.DownloadICS(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin cannot download appointment assigned to someone else, got %d", rec.Code)
	}
}
