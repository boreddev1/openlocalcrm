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

// maxImportBytes bounds an import request body (10 MB payload plus multipart overhead).
const maxImportBytes = 10<<20 + 1<<20

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

	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil { //nolint:gosec // G120: request body is capped with http.MaxBytesReader on the line above; limit is 10 MB plus multipart overhead
		writeJSONError(w, http.StatusBadRequest, "file_too_large", "Datei zu groß oder ungültiges Formular")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "missing_file", "Dateifeld 'file' fehlt")
		return
	}
	defer file.Close()

	importedCount, err := h.service.ImportContactsCSV(r.Context(), actorID, file)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "import_failed", "CSV-Import fehlgeschlagen: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"status":"success","imported_count":%d}`, importedCount)
}
