package reports_test

import (
	"context"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/reports"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestReportsService_CleanEmptyState(t *testing.T) {
	ctx := context.Background()
	svc := reports.NewService(nil)

	report, err := svc.GetSalesReport(ctx)
	if err != nil {
		t.Fatalf("failed to get sales report: %v", err)
	}

	if len(report.Forecast) != 0 {
		t.Fatalf("expected empty forecast for clean state, got %d items", len(report.Forecast))
	}

	if report.ConversionStats.TotalLeads != 0 {
		t.Fatalf("expected 0 total leads, got %d", report.ConversionStats.TotalLeads)
	}
	if report.ConversionStats.WonDeals != 0 {
		t.Fatalf("expected 0 won deals, got %d", report.ConversionStats.WonDeals)
	}
	if report.ConversionStats.ConversionRate != 0.0 {
		t.Fatalf("expected 0.0 conversion rate, got %f", report.ConversionStats.ConversionRate)
	}
	if report.ConversionStats.AvgDealVolumeEUR != 0.0 {
		t.Fatalf("expected 0.0 avg volume, got %f", report.ConversionStats.AvgDealVolumeEUR)
	}
}

func TestReportsService_WithDemoQuerier(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := reports.NewService(querier)

	report, err := svc.GetSalesReport(ctx)
	if err != nil {
		t.Fatalf("failed to get sales report: %v", err)
	}

	if len(report.Forecast) == 0 {
		t.Fatalf("expected non-empty forecast with demo data")
	}

	if report.ConversionStats.TotalLeads <= 0 {
		t.Fatalf("expected positive total leads with demo data")
	}
	if report.ConversionStats.RevokedDeals < 0 {
		t.Fatalf("expected non-negative revoked deals")
	}
}
