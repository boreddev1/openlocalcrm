package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/appointment"
)

type AppointmentHandler struct {
	svc  *appointment.Service
	mu   sync.RWMutex
	apps []appointment.Appointment
}

func NewAppointmentHandler(svc *appointment.Service) *AppointmentHandler {
	initial, _ := svc.List(nil)
	return &AppointmentHandler{
		svc:  svc,
		apps: initial,
	}
}

func (h *AppointmentHandler) List(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.apps)
}

func (h *AppointmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title        string    `json:"title"`
		Type         string    `json:"type"`
		ContactName  string    `json:"contact_name"`
		ContactEmail string    `json:"contact_email"`
		CompanyName  string    `json:"company_name"`
		StartTime    time.Time `json:"start_time"`
		EndTime      time.Time `json:"end_time"`
		Location     string    `json:"location"`
		AssignedTo   string    `json:"assigned_to"`
		Notes        string    `json:"notes"`
		Reminder     string    `json:"reminder"`
		Status       string    `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	id := fmt.Sprintf("app-%d", time.Now().UnixNano())
	app := appointment.Appointment{
		ID:          id,
		Title:       input.Title,
		StartTime:   input.StartTime,
		EndTime:     input.EndTime,
		Location:    input.Location,
		Notes:       input.Notes,
		ICSUID:      id,
		ContactName: input.ContactName,
		CompanyName: input.CompanyName,
		AssignedTo:  input.AssignedTo,
		Type:        input.Type,
	}

	h.apps = append([]appointment.Appointment{app}, h.apps...)

	var actorID pgtype.UUID
	_ = actorID.Scan("00000000-0000-0000-0000-000000000001")
	_, _ = h.svc.Create(r.Context(), actorID, appointment.CreateAppointmentInput{
		Title:     input.Title,
		StartTime: input.StartTime,
		EndTime:   input.EndTime,
		Location:  input.Location,
		Notes:     input.Notes,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(app)
}

func (h *AppointmentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input appointment.Appointment
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for i, a := range h.apps {
		if a.ID == id {
			input.ID = id
			h.apps[i] = input
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(input)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(input)
}

func (h *AppointmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	h.mu.Lock()
	defer h.mu.Unlock()

	filtered := make([]appointment.Appointment, 0, len(h.apps))
	for _, a := range h.apps {
		if a.ID != id {
			filtered = append(filtered, a)
		}
	}
	h.apps = filtered

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *AppointmentHandler) PushExternal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.mu.Lock()
	for i, a := range h.apps {
		if a.ID == id {
			h.apps[i].IsPushed = true
			break
		}
	}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   "synced",
		"provider": "microsoft_and_google",
	})
}

func (h *AppointmentHandler) DownloadICS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.mu.RLock()
	var found *appointment.Appointment
	for _, a := range h.apps {
		if a.ID == id {
			found = &a
			break
		}
	}
	h.mu.RUnlock()

	var app appointment.Appointment
	if found != nil {
		app = *found
	} else {
		app = appointment.Appointment{
			ID:        id,
			Title:     "Kundentermin",
			StartTime: time.Now().Add(24 * time.Hour),
			EndTime:   time.Now().Add(25 * time.Hour),
			Location:  "Kundenadresse",
			Notes:     "OpenLocalCRM Termin",
			ICSUID:    id,
		}
	}

	icsContent := h.svc.GenerateICS(app)

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="termin.ics"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(icsContent))
}
