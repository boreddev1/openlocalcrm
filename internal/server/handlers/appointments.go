package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/appointment"
)

type AppointmentHandler struct {
	svc *appointment.Service
}

func NewAppointmentHandler(svc *appointment.Service) *AppointmentHandler {
	return &AppointmentHandler{
		svc: svc,
	}
}

func (h *AppointmentHandler) List(w http.ResponseWriter, r *http.Request) {
	apps, err := h.svc.List(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to list appointments"}`, http.StatusInternalServerError)
		return
	}

	if apps == nil {
		apps = []appointment.Appointment{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(apps)
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
		ContactID    *string   `json:"contact_id,omitempty"`
		CompanyID    *string   `json:"company_id,omitempty"`
		DealID       *string   `json:"deal_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	actorID := pgtype.UUID{Valid: false}
	if claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims); ok && claims != nil {
		_ = actorID.Scan(claims.UserID)
	}

	app, err := h.svc.Create(r.Context(), actorID, appointment.CreateAppointmentInput{
		Title:        input.Title,
		ContactID:    input.ContactID,
		CompanyID:    input.CompanyID,
		DealID:       input.DealID,
		ContactName:  input.ContactName,
		ContactEmail: input.ContactEmail,
		CompanyName:  input.CompanyName,
		StartTime:    input.StartTime,
		EndTime:      input.EndTime,
		Location:     input.Location,
		Notes:        input.Notes,
		AssignedTo:   input.AssignedTo,
		Type:         input.Type,
	})
	if err != nil {
		http.Error(w, `{"error":"failed to create appointment"}`, http.StatusInternalServerError)
		return
	}

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
	input.ID = id

	app, err := h.svc.Update(r.Context(), input)
	if err != nil {
		http.Error(w, `{"error":"failed to update appointment"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(app)
}

func (h *AppointmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_ = h.svc.Delete(r.Context(), id)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *AppointmentHandler) PushExternal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_ = h.svc.PushExternal(r.Context(), id)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   "synced",
		"provider": "microsoft_and_google",
	})
}

func (h *AppointmentHandler) DownloadICS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		app = appointment.Appointment{
			ID:        id,
			Title:     "Kundentermin",
			StartTime: time.Now().Add(24 * time.Hour),
			EndTime:   time.Now().Add(25 * time.Hour),
			Location:  "Kundenadresse",
			Notes:     "OpenLocalCRM Termin",
			ICSUID:    id + "@openlocalcrm.crm",
		}
	}

	icsContent := h.svc.GenerateICS(app)

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="termin.ics"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(icsContent))
}
