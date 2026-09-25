package export_test

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestExportContactsCSV_AllColumns(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	auditSvc := audit.NewService(querier)
	contactSvc := contact.NewService(querier, auditSvc)
	exportSvc := export.NewService(querier, contactSvc)

	var strom, gas pgtype.Numeric
	_ = strom.Scan("3500.00")
	_ = gas.Scan("11000.00")

	_, err := querier.CreateContact(ctx, db.CreateContactParams{
		FirstName:         "Sabine",
		LastName:          "Sonnenschein",
		Email:             pgtype.Text{String: "sabine@sonne.de", Valid: true},
		Phone:             pgtype.Text{String: "+496912345", Valid: true},
		ConsentPhone:      true,
		ConsentEmail:      true,
		ConsentUpdatedAt:  pgtype.Timestamptz{Time: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC), Valid: true},
		StromverbrauchKwh: strom,
		GasverbrauchKwh:   gas,
		Zaehlernummer:     pgtype.Text{String: "1EMH998877", Valid: true},
		EigentuemerStatus: pgtype.Text{String: "EIGENTUEMER", Valid: true},
	})
	if err != nil {
		t.Fatalf("failed creating contact: %v", err)
	}

	csvBytes, err := exportSvc.ExportContactsCSV(ctx)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	content := string(csvBytes)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header and row, got: %s", content)
	}

	header := lines[0]
	expectedCols := []string{
		"ID", "Vorname", "Nachname", "Email", "Telefon", "Position", "Strasse", "PLZ", "Ort",
		"Einwilligung_Telefon", "Einwilligung_Email", "Einwilligung_Aktualisiert_Am",
		"Stromverbrauch_kWh", "Gasverbrauch_kWh", "Zaehlernummer", "Eigentuemer_Status", "Erstellt_Am",
	}
	for _, col := range expectedCols {
		if !strings.Contains(header, col) {
			t.Errorf("missing column %s in header: %s", col, header)
		}
	}

	var foundRow string
	for _, l := range lines[1:] {
		if strings.Contains(l, "Sonnenschein") {
			foundRow = l
			break
		}
	}
	if foundRow == "" {
		t.Fatalf("created contact Sonnenschein not found in CSV output: %s", content)
	}

	if !strings.Contains(foundRow, "Sabine") || !strings.Contains(foundRow, "1EMH998877") || !strings.Contains(foundRow, "EIGENTUEMER") {
		t.Errorf("unexpected data row: %s", foundRow)
	}
	if !strings.Contains(foundRow, "3500.00") || !strings.Contains(foundRow, "11000.00") {
		t.Errorf("energy consumption values missing in row: %s", foundRow)
	}
}
