package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/openlocalcrm/openlocalcrm/internal/core/telephony"
)

type TelephonyHandler struct {
	svc *telephony.Service
}

func NewTelephonyHandler(svc *telephony.Service) *TelephonyHandler {
	return &TelephonyHandler{svc: svc}
}

func (h *TelephonyHandler) LogCall(w http.ResponseWriter, r *http.Request) {
	var input telephony.LogCallInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	call, err := h.svc.LogCall(r.Context(), input)
	if err != nil {
		http.Error(w, `{"error":"failed to log call: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(call)
}
