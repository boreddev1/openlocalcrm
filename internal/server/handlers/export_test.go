package handlers_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/core/export"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

func TestExportHandler(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(q)
	contactSvc := contact.NewService(q, auditSvc)
	exportSvc := export.NewService(q, contactSvc)
	h := handlers.NewExportHandler(exportSvc)

	t.Run("ExportContactsCSV", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/export/contacts", nil)
		rec := httptest.NewRecorder()

		h.ExportContactsCSV(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "text/csv; charset=utf-8", rec.Header().Get("Content-Type"))
		require.Contains(t, rec.Body.String(), "Vorname,Nachname,E-Mail")
	})

	t.Run("ImportContactsCSV missing file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/export/contacts/import", nil)
		rec := httptest.NewRecorder()

		h.ImportContactsCSV(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ImportContactsCSV valid file", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "test.csv")
		require.NoError(t, err)

		csvContent := "Vorname,Nachname,Email\nMax,Mustermann,max@muster.de\n"
		_, err = part.Write([]byte(csvContent))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		req := httptest.NewRequest(http.MethodPost, "/api/export/contacts/import", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()

		h.ImportContactsCSV(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"imported_count":1`)
	})
}
