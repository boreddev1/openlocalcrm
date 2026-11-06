package appointment_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/appointment"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestAppointmentService_CRUD_And_ICS(t *testing.T) {
	hub := sse.NewHub()
	querier := demo.NewInMemoryQuerier()
	svc := appointment.NewService(querier, hub)
	ctx := context.Background()

	// 1. List initial seeded appointments
	list, err := svc.List(ctx, pgtype.UUID{})
	if err != nil {
		t.Fatalf("failed listing appointments: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("expected seeded appointments, got 0")
	}

	// 2. Create new appointment
	now := time.Now().UTC()
	start := now.Add(2 * time.Hour)
	end := now.Add(3 * time.Hour)
	actorID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	created, err := svc.Create(ctx, actorID, appointment.CreateAppointmentInput{
		Title:       "Photovoltaik Vor-Ort Beratung",
		Location:    "Sonnenstraße 42, 70173 Stuttgart",
		StartTime:   start,
		EndTime:     end,
		Notes:       "Dachflächenaufmaß und Zählerkastenprüfung",
		AssignedTo:  "Max Vertriebsleiter",
		Type:        "CONSULTATION",
		ContactName: "Dr. Michael Weber",
		CompanyName: "Weber Maschinenbau GmbH",
	})
	if err != nil {
		t.Fatalf("failed creating appointment: %v", err)
	}
	if created.ID == "" || created.Title != "Photovoltaik Vor-Ort Beratung" {
		t.Fatalf("unexpected created appointment: %+v", created)
	}

	// 3. Get by ID
	fetched, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed fetching appointment by ID: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected ID %s, got %s", created.ID, fetched.ID)
	}

	// 4. Update appointment
	fetched.Title = "Verschobene Photovoltaik Beratung"
	updated, err := svc.Update(ctx, fetched)
	if err != nil {
		t.Fatalf("failed updating appointment: %v", err)
	}
	if updated.Title != "Verschobene Photovoltaik Beratung" {
		t.Fatalf("expected updated title, got %s", updated.Title)
	}

	// 5. Generate RFC 5545 ICS
	ics := svc.GenerateICS(updated)
	if !strings.Contains(ics, "BEGIN:VCALENDAR") || !strings.Contains(ics, "END:VCALENDAR") {
		t.Fatalf("invalid ICS structure:\n%s", ics)
	}
	if !strings.Contains(ics, "Verschobene Photovoltaik Beratung") {
		t.Fatalf("ICS missing appointment summary:\n%s", ics)
	}

	// 7. Delete appointment
	err = svc.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed deleting appointment: %v", err)
	}
}

func TestAppointmentService_GenerateICS_EscapesSpecialCharactersAndUsesPublish(t *testing.T) {
	svc := appointment.NewService(nil, nil)
	app := appointment.Appointment{
		ID:        "test-app-1",
		Title:     "Treffen mit Müller, Meier; Schmidt",
		Location:  "Büro: Raum 1.02; Nebengebäude",
		Notes:     "Zeile 1\nZeile 2, mit Komma; und Semikolon",
		StartTime: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC),
		ICSUID:    "test-uid-123@openlocalcrm",
	}

	ics := svc.GenerateICS(app)
	if !strings.Contains(ics, "METHOD:PUBLISH") {
		t.Errorf("expected METHOD:PUBLISH, got:\n%s", ics)
	}
	if strings.Contains(ics, "METHOD:REQUEST") {
		t.Errorf("expected no METHOD:REQUEST, got:\n%s", ics)
	}
	if !strings.Contains(ics, "SUMMARY:Treffen mit Müller\\, Meier\\; Schmidt") {
		t.Errorf("expected escaped summary, got:\n%s", ics)
	}
	if !strings.Contains(ics, "LOCATION:Büro: Raum 1.02\\; Nebengebäude") {
		t.Errorf("expected escaped location, got:\n%s", ics)
	}
	if !strings.Contains(ics, "DESCRIPTION:Zeile 1\\nZeile 2\\, mit Komma\\; und Semikolon") {
		t.Errorf("expected escaped description, got:\n%s", ics)
	}
}
