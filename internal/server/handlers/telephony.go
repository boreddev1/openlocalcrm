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
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Ungültiger Anfrageinhalt")
		return
	}

	call, err := h.svc.LogCall(r.Context(), input)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "log_call_failed", "Anruf konnte nicht protokolliert werden")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(call)
}
