package reports_test

import (
	"context"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/reports"
)

func TestReportsService_GetSalesReport(t *testing.T) {
	ctx := context.Background()
	svc := reports.NewService()

	report, err := svc.GetSalesReport(ctx)
	if err != nil {
		t.Fatalf("failed to get sales report: %v", err)
	}

	if len(report.Forecast) == 0 {
		t.Fatalf("expected non-empty forecast")
	}

	if report.ConversionStats.TotalLeads <= 0 {
		t.Fatalf("expected positive total leads")
	}
	if report.ConversionStats.ConversionRate <= 0 {
		t.Fatalf("expected positive conversion rate")
	}
	if report.ConversionStats.RevokedDeals < 0 {
		t.Fatalf("expected non-negative revoked deals")
	}
}
