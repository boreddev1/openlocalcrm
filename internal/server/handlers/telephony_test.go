package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/telephony"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestTelephonyHandler_LogCall(t *testing.T) {
	querier := demo.NewInMemoryQuerier()
	hub := sse.NewHub()
	svc := telephony.NewService(querier, hub)
	h := handlers.NewTelephonyHandler(svc)

	// 1. Valid call log
	payload := telephony.LogCallInput{
		ContactID:       "44444444-4444-4444-4444-444444444441",
		DurationSeconds: 65,
		Disposition:     "REACHED",
		Notes:           "Termin für Montag 10:00 Uhr vereinbart.",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/v1/telephony/calls", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.LogCall(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Bad JSON
	reqBad := httptest.NewRequest("POST", "/api/v1/telephony/calls", bytes.NewReader([]byte("{bad-json}")))
	recBad := httptest.NewRecorder()
	h.LogCall(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recBad.Code)
	}
}
