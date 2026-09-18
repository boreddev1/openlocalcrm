package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/reports"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

func TestReportsHandler_GetSalesReport(t *testing.T) {
	svc := reports.NewService()
	h := handlers.NewReportsHandler(svc)

	req := httptest.NewRequest("GET", "/api/v1/reports/sales", nil)
	rec := httptest.NewRecorder()

	h.GetSalesReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res reports.SalesReport
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(res.Forecast) == 0 {
		t.Fatalf("expected forecast data")
	}
	if res.ConversionStats.TotalLeads <= 0 {
		t.Fatalf("expected conversion stats")
	}
}
