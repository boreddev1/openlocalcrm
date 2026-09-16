package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/appointment"
)

type AppointmentHandler struct {
	svc *appointment.Service
}

func NewAppointmentHandler(svc *appointment.Service) *AppointmentHandler {
	return &AppointmentHandler{svc: svc}
}

func (h *AppointmentHandler) List(w http.ResponseWriter, r *http.Request) {
	// Sample appointments for calendar view
	now := time.Now()
	apps := []appointment.Appointment{
		{
			ID:        "app-1",
			Title:     "Vor-Ort-Beratung PV 25 kWp Dr. Weber",
			StartTime: now.Add(2 * time.Hour),
			EndTime:   now.Add(3 * time.Hour),
			Location:  "Kaiserstraße 14, Frankfurt am Main",
			Notes:     "Dachbegehung & Zählerkasten-Prüfung",
			ICSUID:    "app-1-ics",
		},
		{
			ID:        "app-2",
			Title:     "Vertragsunterzeichnung Sabine Mustermann",
			StartTime: now.Add(26 * time.Hour),
			EndTime:   now.Add(27 * time.Hour),
			Location:  "Goethestraße 8, Frankfurt am Main",
			Notes:     "Widerrufsbelehrung § 355 BGB aushändigen",
			ICSUID:    "app-2-ics",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(apps)
}

func (h *AppointmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input appointment.CreateAppointmentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	var actorID pgtype.UUID
	_ = actorID.Scan("00000000-0000-0000-0000-000000000001")

	app, err := h.svc.Create(r.Context(), actorID, input)
	if err != nil {
		http.Error(w, `{"error":"failed to create appointment: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(app)
}

func (h *AppointmentHandler) DownloadICS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app := appointment.Appointment{
		ID:        id,
		Title:     "Kundentermin",
		StartTime: time.Now().Add(24 * time.Hour),
		EndTime:   time.Now().Add(25 * time.Hour),
		Location:  "Kundenadresse",
		Notes:     "OpenLocalCRM Termin",
		ICSUID:    id,
	}

	icsContent := h.svc.GenerateICS(app)

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="termin.ics"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(icsContent))
}
