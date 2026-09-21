package reports

import (
	"context"
	"math"
	"strings"

	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type MonthlyForecast struct {
	MonthName    string  `json:"month_name"`
	WeightedEUR  float64 `json:"weighted_eur"`
	CommittedEUR float64 `json:"committed_eur"`
	DealCount    int     `json:"deal_count"`
}

type ConversionStats struct {
	TotalLeads       int     `json:"total_leads"`
	WonDeals         int     `json:"won_deals"`
	LostDeals        int     `json:"lost_deals"`
	RevokedDeals     int     `json:"revoked_deals"` // § 355 BGB Widerrufe separat
	ConversionRate   float64 `json:"conversion_rate"`
	AvgDealVolumeEUR float64 `json:"avg_deal_volume_eur"`
}

type SalesReport struct {
	Forecast        []MonthlyForecast `json:"forecast"`
	ConversionStats ConversionStats   `json:"conversion_stats"`
}

type Service struct {
	querier db.Querier
}

func NewService(querier db.Querier) *Service {
	return &Service{querier: querier}
}

func (s *Service) GetSalesReport(ctx context.Context) (SalesReport, error) {
	if s.querier != nil {
		deals, err := s.querier.ListDeals(ctx, db.ListDealsParams{Limit: 10000, Offset: 0})
		if err != nil {
			return SalesReport{
				Forecast:        []MonthlyForecast{},
				ConversionStats: ConversionStats{},
			}, nil
		}
		contacts, _ := s.querier.ListContacts(ctx, db.ListContactsParams{Limit: 10000, Offset: 0})

		totalLeads := len(contacts)
		wonDeals := 0
		lostDeals := 0
		revokedDeals := 0
		var wonVolume float64

		type monthBucket struct {
			committed float64
			weighted  float64
			deals     int
		}
		buckets := make(map[string]*monthBucket)

		for _, d := range deals {
			val, _ := d.Value.Float64Value()
			dealVal := val.Float64
			prob := float64(d.Probability) / 100.0

			// Widerruf check (§ 355 BGB)
			if d.WiderrufenAt.Valid || strings.EqualFold(d.Stage, "REVOKED") || strings.EqualFold(d.Stage, "WIDERRUFEN") {
				revokedDeals++
				continue
			}

			switch strings.ToUpper(d.Stage) {
			case "WON", "GEWONNEN":
				wonDeals++
				wonVolume += dealVal
			case "LOST", "VERLOREN":
				lostDeals++
			}

			// Monthly forecast aggregation by deal created/updated month
			mName := d.CreatedAt.Time.Format("Jan 2006")
			if b, exists := buckets[mName]; exists {
				if strings.ToUpper(d.Stage) == "WON" || strings.ToUpper(d.Stage) == "GEWONNEN" {
					b.committed += dealVal
				}
				b.weighted += dealVal * prob
				b.deals++
			} else {
				comm := 0.0
				if strings.ToUpper(d.Stage) == "WON" || strings.ToUpper(d.Stage) == "GEWONNEN" {
					comm = dealVal
				}
				buckets[mName] = &monthBucket{
					committed: comm,
					weighted:  dealVal * prob,
					deals:     1,
				}
			}
		}

		convRate := 0.0
		if totalLeads > 0 {
			convRate = math.Round((float64(wonDeals)/float64(totalLeads))*1000.0) / 10.0
		}
		avgDealVol := 0.0
		if wonDeals > 0 {
			avgDealVol = math.Round((wonVolume/float64(wonDeals))*100.0) / 100.0
		}

		forecast := make([]MonthlyForecast, 0, len(buckets))
		for m, b := range buckets {
			forecast = append(forecast, MonthlyForecast{
				MonthName:    m,
				WeightedEUR:  math.Round(b.weighted*100.0) / 100.0,
				CommittedEUR: math.Round(b.committed*100.0) / 100.0,
				DealCount:    b.deals,
			})
		}

		return SalesReport{
			Forecast: forecast,
			ConversionStats: ConversionStats{
				TotalLeads:       totalLeads,
				WonDeals:         wonDeals,
				LostDeals:        lostDeals,
				RevokedDeals:     revokedDeals,
				ConversionRate:   convRate,
				AvgDealVolumeEUR: avgDealVol,
			},
		}, nil
	}

	return SalesReport{
		Forecast: []MonthlyForecast{},
		ConversionStats: ConversionStats{
			TotalLeads:       0,
			WonDeals:         0,
			LostDeals:        0,
			RevokedDeals:     0,
			ConversionRate:   0,
			AvgDealVolumeEUR: 0,
		},
	}, nil
}
