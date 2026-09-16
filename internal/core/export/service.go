package export

import (
	"bytes"
	"context"
	"encoding/csv"
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
		"E-Mail",
		"Telefon",
		"Position",
		"Strasse",
		"PLZ",
		"Ort",
		"Einwilligung_Telefon",
		"Einwilligung_Email",
		"Zaehlernummer",
		"Erstellt_Am",
	})

	for _, c := range contacts {
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
			sanitizeCSVField(c.Zaehlernummer.String),
			c.CreatedAt.Time.Format("2006-01-02 15:04:05"),
		})
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}

// ImportContactsCSV imports contacts from a CSV file using streaming line-by-line reading
func (s *Service) ImportContactsCSV(ctx context.Context, actorID pgtype.UUID, r io.Reader) (int, error) {
	reader := csv.NewReader(r)

	// Read and discard header row
	_, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	const maxRows = 5000

	for {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			// Skip malformed row and continue
			continue
		}

		if len(row) < 2 {
			continue
		}

		firstName := strings.TrimSpace(row[0])
		lastName := strings.TrimSpace(row[1])
		email := ""
		if len(row) > 2 {
			email = strings.TrimSpace(row[2])
		}

		if lastName == "" {
			continue
		}

		_, err = s.contactSvc.Create(ctx, actorID, contact.CreateContactInput{
			FirstName: firstName,
			LastName:  lastName,
			Email:     email,
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
