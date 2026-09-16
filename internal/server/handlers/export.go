package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/export"
)

type ExportHandler struct {
	service *export.Service
}

func NewExportHandler(service *export.Service) *ExportHandler {
	return &ExportHandler{service: service}
}

func (h *ExportHandler) ExportContactsCSV(w http.ResponseWriter, r *http.Request) {
	data, err := h.service.ExportContactsCSV(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to generate contacts csv"}`, http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("kontakte_export_%s.csv", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *ExportHandler) ImportContactsCSV(w http.ResponseWriter, r *http.Request) {
	claims, _ := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	var actorID pgtype.UUID
	if claims != nil {
		_ = actorID.Scan(claims.UserID)
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB limit
		http.Error(w, `{"error":"file too large or invalid form"}`, http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"missing file field"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	importedCount, err := h.service.ImportContactsCSV(r.Context(), actorID, file)
	if err != nil {
		http.Error(w, `{"error":"failed to import csv: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"success","imported_count":%d}`, importedCount)))
}
