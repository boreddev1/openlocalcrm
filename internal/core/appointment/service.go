package appointment

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

var (
	ErrAppointmentNotFound = errors.New("appointment not found")
	ErrInvalidTimeRange    = errors.New("end time must be after start time")
	ErrMissingTitle        = errors.New("appointment title is required")
)

type Appointment struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	ContactID   *string   `json:"contact_id,omitempty"`
	CompanyID   *string   `json:"company_id,omitempty"`
	DealID      *string   `json:"deal_id,omitempty"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Location    string    `json:"location"`
	Notes       string    `json:"notes"`
	ICSUID      string    `json:"ics_uid"`
	IsExternal  bool      `json:"is_external"`
	Provider    string    `json:"provider,omitempty"`
	IsPrivate   bool      `json:"is_private,omitempty"`
	ContactName string    `json:"contact_name,omitempty"`
	CompanyName string    `json:"company_name,omitempty"`
	AssignedTo  string    `json:"assigned_to,omitempty"`
	Type        string    `json:"type,omitempty"`
	IsPushed    bool      `json:"is_pushed"`
}

type CreateAppointmentInput struct {
	Title        string    `json:"title"`
	ContactID    *string   `json:"contact_id,omitempty"`
	CompanyID    *string   `json:"company_id,omitempty"`
	DealID       *string   `json:"deal_id,omitempty"`
	ContactName  string    `json:"contact_name,omitempty"`
	ContactEmail string    `json:"contact_email,omitempty"`
	CompanyName  string    `json:"company_name,omitempty"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Location     string    `json:"location"`
	Notes        string    `json:"notes"`
	AssignedTo   string    `json:"assigned_to,omitempty"`
	Type         string    `json:"type,omitempty"`
	IsExternal   bool      `json:"is_external,omitempty"`
	Provider     string    `json:"provider,omitempty"`
	IsPrivate    bool      `json:"is_private,omitempty"`
}

type Service struct {
	mu      sync.RWMutex
	querier db.Querier
	sseHub  *sse.Hub
	inMem   []Appointment
}

func NewService(querier db.Querier, sseHub *sse.Hub) *Service {
	s := &Service{
		querier: querier,
		sseHub:  sseHub,
		inMem:   make([]Appointment, 0),
	}
	return s
}

func parseUUID(s *string) pgtype.UUID {
	if s == nil || *s == "" {
		return pgtype.UUID{Valid: false}
	}
	u, err := uuid.Parse(*s)
	if err != nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: u, Valid: true}
}

func uuidToStrPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	str := uuid.UUID(u.Bytes).String()
	return &str
}

func (s *Service) List(ctx context.Context, pagination ...int) ([]Appointment, error) {
	limit := 100
	offset := 0
	if len(pagination) > 0 && pagination[0] > 0 {
		limit = pagination[0]
		if limit > 500 {
			limit = 500
		}
	}
	if len(pagination) > 1 && pagination[1] >= 0 {
		offset = pagination[1]
	}

	if s.querier != nil {
		dbApps, err := s.querier.ListAppointments(ctx)
		if err != nil {
			return nil, err
		}
		if offset >= len(dbApps) {
			return []Appointment{}, nil
		}
		end := offset + limit
		if end > len(dbApps) {
			end = len(dbApps)
		}
		paged := dbApps[offset:end]

		result := make([]Appointment, len(paged))
		for i, a := range paged {
			idStr := uuid.UUID(a.ID.Bytes).String()
			result[i] = Appointment{
				ID:         idStr,
				Title:      a.Title,
				ContactID:  uuidToStrPtr(a.ContactID),
				CompanyID:  uuidToStrPtr(a.CompanyID),
				DealID:     uuidToStrPtr(a.DealID),
				StartTime:  a.StartTime.Time,
				EndTime:    a.EndTime.Time,
				Location:   a.Location,
				Notes:      a.Notes,
				ICSUID:     a.IcsUid,
				AssignedTo: a.AssignedTo,
				Type:       a.Type,
				IsExternal: a.IsExternal,
				Provider:   a.Provider,
				IsPrivate:  a.IsPrivate,
				IsPushed:   a.IsPushed,
			}
		}
		return result, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if offset >= len(s.inMem) {
		return []Appointment{}, nil
	}
	end := offset + limit
	if end > len(s.inMem) {
		end = len(s.inMem)
	}
	return append([]Appointment(nil), s.inMem[offset:end]...), nil
}

func (s *Service) GetByID(ctx context.Context, id string) (Appointment, error) {
	if s.querier != nil {
		if u, err := uuid.Parse(id); err == nil {
			a, err := s.querier.GetAppointmentByID(ctx, pgtype.UUID{Bytes: u, Valid: true})
			if err == nil {
				return Appointment{
					ID:         uuid.UUID(a.ID.Bytes).String(),
					Title:      a.Title,
					ContactID:  uuidToStrPtr(a.ContactID),
					CompanyID:  uuidToStrPtr(a.CompanyID),
					DealID:     uuidToStrPtr(a.DealID),
					StartTime:  a.StartTime.Time,
					EndTime:    a.EndTime.Time,
					Location:   a.Location,
					Notes:      a.Notes,
					ICSUID:     a.IcsUid,
					AssignedTo: a.AssignedTo,
					Type:       a.Type,
					IsExternal: a.IsExternal,
					Provider:   a.Provider,
					IsPrivate:  a.IsPrivate,
					IsPushed:   a.IsPushed,
				}, nil
			}
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.inMem {
		if a.ID == id {
			return a, nil
		}
	}
	return Appointment{}, ErrAppointmentNotFound
}

func (s *Service) Create(ctx context.Context, actorID pgtype.UUID, input CreateAppointmentInput) (Appointment, error) {
	if input.Title == "" {
		return Appointment{}, ErrMissingTitle
	}
	if !input.EndTime.IsZero() && !input.StartTime.IsZero() && !input.EndTime.After(input.StartTime) {
		return Appointment{}, ErrInvalidTimeRange
	}

	appType := input.Type
	if appType == "" {
		appType = "CONSULTATION"
	}
	provider := input.Provider
	if provider == "" {
		provider = "internal"
	}

	if s.querier != nil {
		icsUID := fmt.Sprintf("%d@openlocalcrm.crm", time.Now().UnixNano())
		created, err := s.querier.CreateAppointment(ctx, db.CreateAppointmentParams{
			Title:      input.Title,
			ContactID:  parseUUID(input.ContactID),
			CompanyID:  parseUUID(input.CompanyID),
			DealID:     parseUUID(input.DealID),
			StartTime:  pgtype.Timestamptz{Time: input.StartTime, Valid: true},
			EndTime:    pgtype.Timestamptz{Time: input.EndTime, Valid: true},
			Location:   input.Location,
			Notes:      input.Notes,
			IcsUid:     icsUID,
			AssignedTo: input.AssignedTo,
			Type:       appType,
			IsExternal: input.IsExternal,
			Provider:   provider,
			IsPrivate:  input.IsPrivate,
			IsPushed:   false,
		})
		if err == nil {
			app := Appointment{
				ID:          uuid.UUID(created.ID.Bytes).String(),
				Title:       created.Title,
				ContactID:   uuidToStrPtr(created.ContactID),
				CompanyID:   uuidToStrPtr(created.CompanyID),
				DealID:      uuidToStrPtr(created.DealID),
				StartTime:   created.StartTime.Time,
				EndTime:     created.EndTime.Time,
				Location:    created.Location,
				Notes:       created.Notes,
				ICSUID:      created.IcsUid,
				AssignedTo:  created.AssignedTo,
				Type:        created.Type,
				IsExternal:  created.IsExternal,
				Provider:    created.Provider,
				IsPrivate:   created.IsPrivate,
				IsPushed:    created.IsPushed,
				ContactName: input.ContactName,
				CompanyName: input.CompanyName,
			}
			s.broadcast("appointment.created", app)
			return app, nil
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("app-%d", time.Now().UnixNano())
	app := Appointment{
		ID:          id,
		Title:       input.Title,
		ContactID:   input.ContactID,
		CompanyID:   input.CompanyID,
		DealID:      input.DealID,
		StartTime:   input.StartTime,
		EndTime:     input.EndTime,
		Location:    input.Location,
		Notes:       input.Notes,
		ICSUID:      id,
		AssignedTo:  input.AssignedTo,
		Type:        appType,
		IsExternal:  input.IsExternal,
		Provider:    provider,
		IsPrivate:   input.IsPrivate,
		ContactName: input.ContactName,
		CompanyName: input.CompanyName,
	}
	s.inMem = append([]Appointment{app}, s.inMem...)
	s.broadcast("appointment.created", app)
	return app, nil
}

func (s *Service) Update(ctx context.Context, app Appointment) (Appointment, error) {
	if !app.EndTime.IsZero() && !app.StartTime.IsZero() && !app.EndTime.After(app.StartTime) {
		return Appointment{}, ErrInvalidTimeRange
	}

	if s.querier != nil {
		if u, err := uuid.Parse(app.ID); err == nil {
			updated, err := s.querier.UpdateAppointment(ctx, db.UpdateAppointmentParams{
				ID:         pgtype.UUID{Bytes: u, Valid: true},
				Title:      app.Title,
				ContactID:  parseUUID(app.ContactID),
				StartTime:  pgtype.Timestamptz{Time: app.StartTime, Valid: true},
				EndTime:    pgtype.Timestamptz{Time: app.EndTime, Valid: true},
				Location:   app.Location,
				Notes:      app.Notes,
				AssignedTo: app.AssignedTo,
				Type:       app.Type,
			})
			if err == nil {
				app.Title = updated.Title
				app.Location = updated.Location
				app.Notes = updated.Notes
				app.AssignedTo = updated.AssignedTo
				app.Type = updated.Type
				s.broadcast("appointment.updated", app)
				return app, nil
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.inMem {
		if a.ID == app.ID {
			s.inMem[i] = app
			s.broadcast("appointment.updated", app)
			return app, nil
		}
	}
	s.inMem = append([]Appointment{app}, s.inMem...)
	s.broadcast("appointment.updated", app)
	return app, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if s.querier != nil {
		if u, err := uuid.Parse(id); err == nil {
			if err := s.querier.DeleteAppointment(ctx, pgtype.UUID{Bytes: u, Valid: true}); err != nil {
				return fmt.Errorf("failed to delete appointment: %w", err)
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]Appointment, 0, len(s.inMem))
	for _, a := range s.inMem {
		if a.ID != id {
			filtered = append(filtered, a)
		}
	}
	s.inMem = filtered
	s.broadcast("appointment.deleted", map[string]string{"id": id})
	return nil
}

func (s *Service) PushExternal(ctx context.Context, id string) error {
	if s.querier != nil {
		if u, err := uuid.Parse(id); err == nil {
			if _, err := s.querier.UpdateAppointmentPushStatus(ctx, db.UpdateAppointmentPushStatusParams{
				ID:       pgtype.UUID{Bytes: u, Valid: true},
				IsPushed: true,
			}); err != nil {
				return fmt.Errorf("failed to update push status in db: %w", err)
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.inMem {
		if a.ID == id {
			s.inMem[i].IsPushed = true
			break
		}
	}
	return nil
}

func (s *Service) broadcast(eventType string, data any) {
	if s.sseHub != nil {
		s.sseHub.Broadcast(sse.Event{
			Type: eventType,
			Data: data,
		})
	}
}

// GenerateICS formats an appointment as an RFC 5545 iCalendar string
func (s *Service) GenerateICS(app Appointment) string {
	dtFormat := "20060102T150405Z"
	nowStr := time.Now().UTC().Format(dtFormat)
	startStr := app.StartTime.UTC().Format(dtFormat)
	endStr := app.EndTime.UTC().Format(dtFormat)

	return fmt.Sprintf("BEGIN:VCALENDAR\r\n"+
		"VERSION:2.0\r\n"+
		"PRODID:-//OpenLocalCRM//Single-Tenant Sales//DE\r\n"+
		"CALSCALE:GREGORIAN\r\n"+
		"METHOD:REQUEST\r\n"+
		"BEGIN:VEVENT\r\n"+
		"UID:%s\r\n"+
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
		nowStr,
		startStr,
		endStr,
		app.Title,
		app.Location,
		app.Notes,
	)
}
