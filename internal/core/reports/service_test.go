package reports_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/deal"
	"github.com/openlocalcrm/openlocalcrm/internal/core/reports"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/stretchr/testify/require"
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

func TestReportsService_RevokedDealsCount(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(querier)
	dealSvc := deal.NewService(querier, auditSvc)
	reportsSvc := reports.NewService(querier)

	actorID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	d, err := dealSvc.Create(ctx, actorID, deal.CreateDealInput{
		Title:       "BGB Widerruf Test Deal",
		Stage:       "WON",
		Probability: 100,
	})
	require.NoError(t, err)

	_, err = dealSvc.Revoke(ctx, actorID, d.ID, time.Now().UTC(), "Fristgerechter Widerruf")
	require.NoError(t, err)

	report, err := reportsSvc.GetSalesReport(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, report.ConversionStats.RevokedDeals)
}
