package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

func TestCalculatorHandler_CalculateSolar(t *testing.T) {
	h := handlers.NewCalculatorHandler()

	// 1. Valid request
	payload := handlers.SolarCalcRequest{
		Kwp:         12.5,
		Consumption: 5000.0,
		PricePerKwh: 0.40,
		StorageKwh:  10.0,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/v1/calculator/solar", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.CalculateSolar(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res handlers.SolarCalcResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.YearlyGenerationKwh <= 0 {
		t.Fatalf("expected positive yearly generation, got %d", res.YearlyGenerationKwh)
	}
	if res.AutarkyRatePercent <= 0 || res.AutarkyRatePercent > 100 {
		t.Fatalf("invalid autarky rate: %d", res.AutarkyRatePercent)
	}

	// 2. Default fallback on zero values
	zeroPayload := handlers.SolarCalcRequest{}
	zeroBody, _ := json.Marshal(zeroPayload)
	reqZero := httptest.NewRequest("POST", "/api/v1/calculator/solar", bytes.NewReader(zeroBody))
	recZero := httptest.NewRecorder()

	h.CalculateSolar(recZero, reqZero)
	if recZero.Code != http.StatusOK {
		t.Fatalf("expected status 200 on zero values, got %d", recZero.Code)
	}

	// 3. Bad JSON request
	reqBad := httptest.NewRequest("POST", "/api/v1/calculator/solar", bytes.NewReader([]byte("{invalid-json}")))
	recBad := httptest.NewRecorder()
	h.CalculateSolar(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 on invalid JSON, got %d", recBad.Code)
	}
}
