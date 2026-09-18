package handlers

import (
	"encoding/json"
	"math"
	"net/http"
)

type SolarCalcRequest struct {
	Kwp         float64 `json:"kwp"`
	Consumption float64 `json:"consumption"`
	PricePerKwh float64 `json:"pricePerKwh"`
	StorageKwh  float64 `json:"storageKwh"`
}

type SolarCalcResponse struct {
	AutarkyRatePercent         int     `json:"autarkyRatePercent"`
	SelfConsumptionRatePercent int     `json:"selfConsumptionRatePercent"`
	YearlyGenerationKwh        int     `json:"yearlyGenerationKwh"`
	YearlySavingsEuro          int     `json:"yearlySavingsEuro"`
	DirectSavingsEuro          int     `json:"directSavingsEuro"`
	PaybackPeriodYears         float64 `json:"paybackPeriodYears"`
	TwentyYearSavingsEuro      int     `json:"twentyYearSavingsEuro"`
	SystemCostGrossEuro        int     `json:"systemCostGrossEuro"`
	SystemCostNetEuro          int     `json:"systemCostNetEuro"`
}

type CalculatorHandler struct{}

func NewCalculatorHandler() *CalculatorHandler {
	return &CalculatorHandler{}
}

func (h *CalculatorHandler) CalculateSolar(w http.ResponseWriter, r *http.Request) {
	var req SolarCalcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.Kwp <= 0 {
		req.Kwp = 14.5
	}
	if req.Consumption <= 0 {
		req.Consumption = 6500.0
	}
	if req.PricePerKwh <= 0 {
		req.PricePerKwh = 0.385
	}
	if req.StorageKwh <= 0 {
		req.StorageKwh = 12.0
	}

	yearlyGen := int(math.Round(req.Kwp * 980))
	autarky := int(math.Min(85, math.Round(35+(req.StorageKwh*3.2)+(req.Kwp*1.5))))
	if autarky > 90 {
		autarky = 90
	}
	selfCons := int(math.Min(80, math.Round(30+(req.StorageKwh*2.5))))

	directSavings := int(math.Round(req.Consumption * (float64(autarky) / 100.0) * req.PricePerKwh))
	feedIn := int(math.Max(0, math.Round((float64(yearlyGen)-(req.Consumption*(float64(autarky)/100.0)))*0.081)))
	yearlySavings := directSavings + feedIn

	baseCost := int(math.Round(req.Kwp*1050 + req.StorageKwh*650))
	payback := math.Round((float64(baseCost)/math.Max(1, float64(yearlySavings)))*10) / 10
	twentyYear := (yearlySavings * 20) - baseCost

	res := SolarCalcResponse{
		AutarkyRatePercent:         autarky,
		SelfConsumptionRatePercent: selfCons,
		YearlyGenerationKwh:        yearlyGen,
		YearlySavingsEuro:          yearlySavings,
		DirectSavingsEuro:          directSavings,
		PaybackPeriodYears:         payback,
		TwentyYearSavingsEuro:      twentyYear,
		SystemCostGrossEuro:        baseCost,
		SystemCostNetEuro:          baseCost,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
