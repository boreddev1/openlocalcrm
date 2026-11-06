package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/contact"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type Service struct {
	queries    db.Querier
	contactSvc *contact.Service
}

func NewService(queries db.Querier, contactSvc *contact.Service) *Service {
	return &Service{
		queries:    queries,
		contactSvc: contactSvc,
	}
}

// sanitizeCSVField prepends a single quote if the field starts with a formula trigger character
func sanitizeCSVField(val string) string {
	if len(val) > 0 {
		first := val[0]
		if first == '=' || first == '+' || first == '-' || first == '@' || first == '\t' || first == '\r' {
			return "'" + val
		}
	}
	return val
}

// ExportContactsCSV generates a standard CSV file with all contacts for reporting and DSGVO portability
func (s *Service) ExportContactsCSV(ctx context.Context) ([]byte, error) {
	contacts, err := s.queries.ListContacts(ctx, db.ListContactsParams{
		Limit:  10000,
		Offset: 0,
	})
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Write CSV Header
	_ = w.Write([]string{
		"ID",
		"Vorname",
		"Nachname",
		"Email",
		"Telefon",
		"Position",
		"Strasse",
		"PLZ",
		"Ort",
		"Einwilligung_Telefon",
		"Einwilligung_Email",
		"Einwilligung_Aktualisiert_Am",
		"Stromverbrauch_kWh",
		"Gasverbrauch_kWh",
		"Zaehlernummer",
		"Eigentuemer_Status",
		"Erstellt_Am",
	})

	for _, c := range contacts {
		consentTimeStr := ""
		if c.ConsentUpdatedAt.Valid {
			consentTimeStr = c.ConsentUpdatedAt.Time.Format("2006-01-02 15:04:05")
		}
		stromStr := ""
		if c.StromverbrauchKwh.Valid {
			stromVal, err := c.StromverbrauchKwh.Float64Value()
			if err != nil {
				return nil, fmt.Errorf("convert stromverbrauch_kwh: %w", err)
			}
			stromStr = fmt.Sprintf("%.2f", stromVal.Float64)
		}
		gasStr := ""
		if c.GasverbrauchKwh.Valid {
			gasVal, err := c.GasverbrauchKwh.Float64Value()
			if err != nil {
				return nil, fmt.Errorf("convert gasverbrauch_kwh: %w", err)
			}
			gasStr = fmt.Sprintf("%.2f", gasVal.Float64)
		}

		_ = w.Write([]string{
			sanitizeCSVField(c.ID.String()),
			sanitizeCSVField(c.FirstName),
			sanitizeCSVField(c.LastName),
			sanitizeCSVField(c.Email.String),
			sanitizeCSVField(c.Phone.String),
			sanitizeCSVField(c.Position.String),
			sanitizeCSVField(c.AddressStreet.String),
			sanitizeCSVField(c.AddressZip.String),
			sanitizeCSVField(c.AddressCity.String),
			fmt.Sprintf("%t", c.ConsentPhone),
			fmt.Sprintf("%t", c.ConsentEmail),
			consentTimeStr,
			stromStr,
			gasStr,
			sanitizeCSVField(c.Zaehlernummer.String),
			sanitizeCSVField(c.EigentuemerStatus.String),
			c.CreatedAt.Time.Format("2006-01-02 15:04:05"),
		})
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}

// ImportContactsCSV imports contacts from a CSV file using streaming line-by-line reading
func (s *Service) ImportContactsCSV(ctx context.Context, actorID pgtype.UUID, r io.Reader) (int, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	// Read and parse header row
	headers, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return 0, nil
		}
		return 0, fmt.Errorf("ungültige CSV-Kopfzeile: %w", err)
	}

	colMap := make(map[string]int)
	for i, h := range headers {
		norm := strings.ToLower(strings.TrimSpace(h))
		norm = strings.ReplaceAll(norm, "-", "")
		norm = strings.ReplaceAll(norm, "_", "")
		norm = strings.ReplaceAll(norm, " ", "")
		colMap[norm] = i
	}

	resolveCol := func(names ...string) (int, bool) {
		for _, name := range names {
			norm := strings.ToLower(strings.TrimSpace(name))
			norm = strings.ReplaceAll(norm, "-", "")
			norm = strings.ReplaceAll(norm, "_", "")
			norm = strings.ReplaceAll(norm, " ", "")
			if idx, ok := colMap[norm]; ok {
				return idx, true
			}
		}
		return -1, false
	}

	lastNameIdx, hasLastName := resolveCol("nachname", "lastname", "last_name", "name")
	if !hasLastName {
		return 0, errors.New("erforderliche Spalte 'Nachname' fehlt in CSV")
	}

	firstNameIdx, _ := resolveCol("vorname", "firstname", "first_name", "first")
	emailIdx, _ := resolveCol("email", "e-mail", "mail")
	phoneIdx, _ := resolveCol("phone", "telefon", "telefonnummer", "fon", "tel", "mobile")
	posIdx, _ := resolveCol("position", "funktion", "titel")
	streetIdx, _ := resolveCol("strasse", "straße", "street", "address_street", "adresse")
	zipIdx, _ := resolveCol("plz", "postleitzahl", "zip", "zipcode", "address_zip")
	cityIdx, _ := resolveCol("stadt", "ort", "city", "address_city")

	count := 0
	const maxRows = 5000

	for line := 2; ; line++ {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			// Skip malformed row and continue
			continue
		}

		if len(row) <= lastNameIdx {
			continue
		}

		lastName := strings.TrimSpace(row[lastNameIdx])
		if lastName == "" {
			continue
		}

		var firstName, email, phone, pos, street, zip, city string
		if firstNameIdx >= 0 && firstNameIdx < len(row) {
			firstName = strings.TrimSpace(row[firstNameIdx])
		}
		if emailIdx >= 0 && emailIdx < len(row) {
			email = strings.TrimSpace(row[emailIdx])
		}
		if phoneIdx >= 0 && phoneIdx < len(row) {
			phone = strings.TrimSpace(row[phoneIdx])
		}
		if posIdx >= 0 && posIdx < len(row) {
			pos = strings.TrimSpace(row[posIdx])
		}
		if streetIdx >= 0 && streetIdx < len(row) {
			street = strings.TrimSpace(row[streetIdx])
		}
		if zipIdx >= 0 && zipIdx < len(row) {
			zip = strings.TrimSpace(row[zipIdx])
		}
		if cityIdx >= 0 && cityIdx < len(row) {
			city = strings.TrimSpace(row[cityIdx])
		}

		_, err = s.contactSvc.Create(ctx, actorID, contact.CreateContactInput{
			FirstName:     firstName,
			LastName:      lastName,
			Email:         email,
			Phone:         phone,
			Position:      pos,
			AddressStreet: street,
			AddressZip:    zip,
			AddressCity:   city,
		})
		if err == nil {
			count++
		}

		if count >= maxRows {
			break
		}
	}

	return count, nil
}
