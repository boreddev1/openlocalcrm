package note_test

import (
	"context"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/note"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestNoteServiceCRUD(t *testing.T) {
	hub := sse.NewHub()
	svc := note.NewService(nil, hub)
	ctx := context.Background()

	// 1. Initial list
	initialNotes, err := svc.List(ctx, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(initialNotes) != 3 {
		t.Fatalf("expected 3 initial notes, got %d", len(initialNotes))
	}

	// 2. Create note validation
	_, err = svc.Create(ctx, note.CreateNoteInput{})
	if err != note.ErrInvalidNote {
		t.Fatalf("expected ErrInvalidNote, got %v", err)
	}

	// 3. Create valid note
	created, err := svc.Create(ctx, note.CreateNoteInput{
		EntityType: "contact",
		EntityID:   "c-99",
		Type:       "CALL",
		Author:     "Max Closer",
		Content:    "Kunde möchte PV mit 20 kWh Speicher",
	})
	if err != nil {
		t.Fatalf("failed to create note: %v", err)
	}
	if created.ID == "" || created.Content != "Kunde möchte PV mit 20 kWh Speicher" {
		t.Fatalf("unexpected created note: %+v", created)
	}

	// 4. Get by ID
	fetched, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to get note by ID: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected ID %s, got %s", created.ID, fetched.ID)
	}

	// 5. Update note
	updated, err := svc.Update(ctx, created.ID, "Kunde möchte PV mit 25 kWh Speicher", "NOTE")
	if err != nil {
		t.Fatalf("failed to update note: %v", err)
	}
	if updated.Content != "Kunde möchte PV mit 25 kWh Speicher" || updated.Type != "NOTE" {
		t.Fatalf("unexpected updated note: %+v", updated)
	}

	// 6. Delete note
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("failed to delete note: %v", err)
	}

	// 7. Verify deletion
	_, err = svc.GetByID(ctx, created.ID)
	if err != note.ErrNoteNotFound {
		t.Fatalf("expected ErrNoteNotFound after delete, got %v", err)
	}
}
