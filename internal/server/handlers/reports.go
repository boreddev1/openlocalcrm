package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/openlocalcrm/openlocalcrm/internal/core/reports"
)

type ReportsHandler struct {
	svc *reports.Service
}

func NewReportsHandler(svc *reports.Service) *ReportsHandler {
	return &ReportsHandler{svc: svc}
}

func (h *ReportsHandler) GetSalesReport(w http.ResponseWriter, r *http.Request) {
	rep, err := h.svc.GetSalesReport(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to generate sales report: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rep)
}
