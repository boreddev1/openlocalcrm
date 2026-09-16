package export_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/core/export"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestExportContactsCSV_FormulaInjectionSanitization(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(querier)
	contactSvc := contact.NewService(querier, auditSvc)
	exportSvc := export.NewService(querier, contactSvc)

	// Create contact with formula injection attempt
	var actorID pgtype.UUID
	_ = actorID.Scan("11111111-1111-1111-1111-111111111111")

	_, err := querier.CreateContact(ctx, db.CreateContactParams{
		FirstName: "=cmd|' /C calc'!A0",
		LastName:  "@SUM(1+1)",
		Email:     pgtype.Text{String: "+49123456@evil.com", Valid: true},
	})
	if err != nil {
		t.Fatalf("failed creating test contact: %v", err)
	}

	csvBytes, err := exportSvc.ExportContactsCSV(ctx)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	content := string(csvBytes)

	// Assert that fields starting with '=', '@', '+' are escaped with a leading single quote
	if strings.Contains(content, ",=cmd") {
		t.Fatalf("unquoted formula injection found: %s", content)
	}
	if !strings.Contains(content, "'=cmd") {
		t.Fatalf("expected escaped '=cmd, got %s", content)
	}
	if !strings.Contains(content, "'@SUM") {
		t.Fatalf("expected escaped '@SUM, got %s", content)
	}
	if !strings.Contains(content, "'+49123456") {
		t.Fatalf("expected escaped '+49123456, got %s", content)
	}
}

func TestImportContactsCSV_Streaming(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(querier)
	contactSvc := contact.NewService(querier, auditSvc)
	exportSvc := export.NewService(querier, contactSvc)

	csvData := "Vorname,Nachname,Email\nAnna,Musterfrau,anna@example.com\nBernd,Beispiel,bernd@example.com\n"
	var actorID pgtype.UUID
	_ = actorID.Scan("11111111-1111-1111-1111-111111111111")

	imported, err := exportSvc.ImportContactsCSV(ctx, actorID, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if imported != 2 {
		t.Fatalf("expected 2 contacts imported, got %d", imported)
	}
}
