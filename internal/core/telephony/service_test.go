package telephony_test

import (
	"context"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/telephony"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestTelephonyService_LogCall(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	hub := sse.NewHub()
	svc := telephony.NewService(querier, hub)

	input := telephony.LogCallInput{
		ContactID:       "44444444-4444-4444-4444-444444444441",
		DurationSeconds: 145,
		Disposition:     "REACHED",
		Notes:           "Kunde wünscht Rückruf am Dienstag um 14:00 Uhr.",
	}

	call, err := svc.LogCall(ctx, input)
	if err != nil {
		t.Fatalf("failed to log call: %v", err)
	}

	if call.ContactID != input.ContactID {
		t.Fatalf("expected contact ID %s, got %s", input.ContactID, call.ContactID)
	}
	if call.DurationSeconds != 145 {
		t.Fatalf("expected duration 145, got %d", call.DurationSeconds)
	}
	if call.Disposition != "REACHED" {
		t.Fatalf("expected disposition REACHED, got %s", call.Disposition)
	}
	if call.Notes != input.Notes {
		t.Fatalf("unexpected notes: %s", call.Notes)
	}
}
