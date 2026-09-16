package appointment

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

type Appointment struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	ContactID *string   `json:"contact_id,omitempty"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Location  string    `json:"location"`
	Notes     string    `json:"notes"`
	ICSUID    string    `json:"ics_uid"`
}

type CreateAppointmentInput struct {
	Title     string    `json:"title"`
	ContactID *string   `json:"contact_id,omitempty"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Location  string    `json:"location"`
	Notes     string    `json:"notes"`
}

type Service struct {
	sseHub *sse.Hub
}

func NewService(sseHub *sse.Hub) *Service {
	return &Service{sseHub: sseHub}
}

func (s *Service) List(ctx context.Context) ([]Appointment, error) {
	now := time.Now()
	return []Appointment{
		{
			ID:        "app-1",
			Title:     "Vor-Ort-Beratung PV 25 kWp Dr. Weber",
			StartTime: now.Add(2 * time.Hour),
			EndTime:   now.Add(3 * time.Hour),
			Location:  "Kaiserstraße 14, Frankfurt am Main",
			Notes:     "Dachbegehung & Zählerkasten-Prüfung",
			ICSUID:    "app-1-ics-uid",
		},
		{
			ID:        "app-2",
			Title:     "Vertragsunterzeichnung Sabine Mustermann",
			StartTime: now.Add(26 * time.Hour),
			EndTime:   now.Add(27 * time.Hour),
			Location:  "Goethestraße 8, Frankfurt am Main",
			Notes:     "Widerrufsbelehrung § 355 BGB aushändigen",
			ICSUID:    "app-2-ics-uid",
		},
	}, nil
}

func (s *Service) Create(ctx context.Context, actorID pgtype.UUID, input CreateAppointmentInput) (Appointment, error) {
	app := Appointment{
		ID:        fmt.Sprintf("app-%d", time.Now().UnixNano()),
		Title:     input.Title,
		ContactID: input.ContactID,
		StartTime: input.StartTime,
		EndTime:   input.EndTime,
		Location:  input.Location,
		Notes:     input.Notes,
		ICSUID:    fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	if s.sseHub != nil {
		s.sseHub.Broadcast(sse.Event{
			Type: "appointment.created",
			Data: map[string]any{
				"id":    app.ID,
				"title": app.Title,
				"start": app.StartTime,
			},
		})
	}

	return app, nil
}

// GenerateICS formats an appointment as an RFC 5545 iCalendar string
func (s *Service) GenerateICS(app Appointment) string {
	dtFormat := "20060102T150405Z"
	return fmt.Sprintf("BEGIN:VCALENDAR\r\n"+
		"VERSION:2.0\r\n"+
		"PRODID:-//OpenLocalCRM//Single-Tenant Sales//DE\r\n"+
		"CALSCALE:GREGORIAN\r\n"+
		"METHOD:REQUEST\r\n"+
		"BEGIN:VEVENT\r\n"+
		"UID:%s@openlocalcrm.crm\r\n"+
		"DTSTAMP:%s\r\n"+
		"DTSTART:%s\r\n"+
		"DTEND:%s\r\n"+
		"SUMMARY:%s\r\n"+
		"LOCATION:%s\r\n"+
		"DESCRIPTION:%s\r\n"+
		"STATUS:CONFIRMED\r\n"+
		"END:VEVENT\r\n"+
		"END:VCALENDAR\r\n",
		app.ICSUID,
		time.Now().UTC().Format(dtFormat),
		app.StartTime.UTC().Format(dtFormat),
		app.EndTime.UTC().Format(dtFormat),
		app.Title,
		app.Location,
		app.Notes,
	)
}
