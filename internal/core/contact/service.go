package contact

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/geocoding"
)

var (
	ErrContactNotFound = errors.New("contact not found")
	ErrInvalidContact  = errors.New("first name and last name are required")
	ErrInvalidEmail    = errors.New("invalid email address format")
)

// validateEmail performs basic email syntax validation.
func validateEmail(email string) bool {
	if email == "" {
		return true // empty is allowed (optional field)
	}
	at := strings.Index(email, "@")
	if at < 1 {
		return false
	}
	dot := strings.LastIndex(email[at:], ".")
	return dot > 1 && dot < len(email[at:])-1
}

type Service struct {
	queries db.Querier
	audit   *audit.Service
}

func NewService(queries db.Querier, audit *audit.Service) *Service {
	return &Service{
		queries: queries,
		audit:   audit,
	}
}

type CreateContactInput struct {
	CompanyID         *pgtype.UUID
	FirstName         string
	LastName          string
	Email             string
	Phone             string
	Mobile            string
	Position          string
	LeadSource        string
	AddressStreet     string
	AddressZip        string
	AddressCity       string
	Latitude          *float64
	Longitude         *float64
	ConsentPhone      bool
	ConsentEmail      bool
	StromverbrauchKwh *float64
	GasverbrauchKwh   *float64
	Zaehlernummer     string
	EigentuemerStatus string
	CustomFields      []byte
}

func (s *Service) Create(ctx context.Context, actorID pgtype.UUID, input CreateContactInput) (db.Contact, error) {
	if input.FirstName == "" || input.LastName == "" {
		return db.Contact{}, ErrInvalidContact
	}
	if !validateEmail(input.Email) {
		return db.Contact{}, ErrInvalidEmail
	}

	var compID pgtype.UUID
	if input.CompanyID != nil {
		compID = *input.CompanyID
	}

	var lat, lon pgtype.Float8
	if input.Latitude != nil {
		lat = pgtype.Float8{Float64: *input.Latitude, Valid: true}
	}
	if input.Longitude != nil {
		lon = pgtype.Float8{Float64: *input.Longitude, Valid: true}
	}
	if input.Latitude == nil && input.Longitude == nil && strings.TrimSpace(input.AddressZip) != "" {
		if glat, glon, ok := geocoding.LookupPLZ(input.AddressZip); ok {
			lat = pgtype.Float8{Float64: glat, Valid: true}
			lon = pgtype.Float8{Float64: glon, Valid: true}
		}
	}

	customFields := input.CustomFields
	if len(customFields) == 0 {
		customFields = []byte("{}")
	}

	var consentUpdatedAt pgtype.Timestamptz
	if input.ConsentPhone || input.ConsentEmail {
		consentUpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	var stromVal, gasVal pgtype.Numeric
	if input.StromverbrauchKwh != nil {
		_ = stromVal.Scan(fmt.Sprintf("%.2f", *input.StromverbrauchKwh))
	}
	if input.GasverbrauchKwh != nil {
		_ = gasVal.Scan(fmt.Sprintf("%.2f", *input.GasverbrauchKwh))
	}

	status := input.EigentuemerStatus
	if status == "" {
		status = "UNBEKANNT"
	}

	contact, err := s.queries.CreateContact(ctx, db.CreateContactParams{
		CompanyID:         compID,
		FirstName:         input.FirstName,
		LastName:          input.LastName,
		Email:             pgtype.Text{String: input.Email, Valid: input.Email != ""},
		Phone:             pgtype.Text{String: input.Phone, Valid: input.Phone != ""},
		Mobile:            pgtype.Text{String: input.Mobile, Valid: input.Mobile != ""},
		Position:          pgtype.Text{String: input.Position, Valid: input.Position != ""},
		LeadSource:        pgtype.Text{String: input.LeadSource, Valid: input.LeadSource != ""},
		AddressStreet:     pgtype.Text{String: input.AddressStreet, Valid: input.AddressStreet != ""},
		AddressZip:        pgtype.Text{String: input.AddressZip, Valid: input.AddressZip != ""},
		AddressCity:       pgtype.Text{String: input.AddressCity, Valid: input.AddressCity != ""},
		Latitude:          lat,
		Longitude:         lon,
		ConsentPhone:      input.ConsentPhone,
		ConsentEmail:      input.ConsentEmail,
		ConsentUpdatedAt:  consentUpdatedAt,
		StromverbrauchKwh: stromVal,
		GasverbrauchKwh:   gasVal,
		Zaehlernummer:     pgtype.Text{String: input.Zaehlernummer, Valid: input.Zaehlernummer != ""},
		EigentuemerStatus: pgtype.Text{String: status, Valid: true},
		CustomFields:      customFields,
	})

	if err != nil {
		return db.Contact{}, err
	}

	if s.audit != nil {
		if err := s.audit.Log(ctx, actorID, "CONTACT", contact.ID, "CREATE", contact, "", ""); err != nil {
			log.Printf("[audit] failed to log contact create: %v", err)
		}
	}

	return contact, nil
}

func (s *Service) GetByID(ctx context.Context, id pgtype.UUID) (db.Contact, error) {
	return s.queries.GetContactByID(ctx, id)
}

func (s *Service) List(ctx context.Context, limit, offset int32) ([]db.Contact, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.queries.ListContacts(ctx, db.ListContactsParams{
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Service) Search(ctx context.Context, query string, limit int32) ([]db.Contact, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.queries.SearchContacts(ctx, db.SearchContactsParams{
		Column1: pgtype.Text{String: query, Valid: true},
		Limit:   limit,
	})
}

type UpdateContactInput struct {
	ID                pgtype.UUID
	CompanyID         *pgtype.UUID
	FirstName         string
	LastName          string
	Email             string
	Phone             string
	Mobile            string
	Position          string
	LeadSource        string
	AddressStreet     string
	AddressZip        string
	AddressCity       string
	Latitude          *float64
	Longitude         *float64
	ConsentPhone      bool
	ConsentEmail      bool
	StromverbrauchKwh *float64
	GasverbrauchKwh   *float64
	Zaehlernummer     string
	EigentuemerStatus string
	CustomFields      []byte
}

func (s *Service) Update(ctx context.Context, actorID pgtype.UUID, input UpdateContactInput) (db.Contact, error) {
	if input.FirstName == "" || input.LastName == "" {
		return db.Contact{}, ErrInvalidContact
	}
	if !validateEmail(input.Email) {
		return db.Contact{}, ErrInvalidEmail
	}

	existing, err := s.queries.GetContactByID(ctx, input.ID)
	if err != nil {
		return db.Contact{}, ErrContactNotFound
	}

	var compID pgtype.UUID
	if input.CompanyID != nil {
		compID = *input.CompanyID
	}

	var lat, lon pgtype.Float8
	if input.Latitude != nil {
		lat = pgtype.Float8{Float64: *input.Latitude, Valid: true}
	}
	if input.Longitude != nil {
		lon = pgtype.Float8{Float64: *input.Longitude, Valid: true}
	}
	if input.Latitude == nil && input.Longitude == nil && strings.TrimSpace(input.AddressZip) != "" {
		if glat, glon, ok := geocoding.LookupPLZ(input.AddressZip); ok {
			lat = pgtype.Float8{Float64: glat, Valid: true}
			lon = pgtype.Float8{Float64: glon, Valid: true}
		}
	}

	customFields := input.CustomFields
	if len(customFields) == 0 {
		customFields = []byte("{}")
	}

	consentChanged := (input.ConsentPhone != existing.ConsentPhone) || (input.ConsentEmail != existing.ConsentEmail)
	consentUpdatedAt := existing.ConsentUpdatedAt
	if consentChanged {
		consentUpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	var stromVal, gasVal pgtype.Numeric
	if input.StromverbrauchKwh != nil {
		_ = stromVal.Scan(fmt.Sprintf("%.2f", *input.StromverbrauchKwh))
	} else {
		stromVal = existing.StromverbrauchKwh
	}
	if input.GasverbrauchKwh != nil {
		_ = gasVal.Scan(fmt.Sprintf("%.2f", *input.GasverbrauchKwh))
	} else {
		gasVal = existing.GasverbrauchKwh
	}

	status := input.EigentuemerStatus
	if status == "" {
		status = existing.EigentuemerStatus.String
	}
	if status == "" {
		status = "UNBEKANNT"
	}

	zaehler := input.Zaehlernummer
	if zaehler == "" && existing.Zaehlernummer.Valid {
		zaehler = existing.Zaehlernummer.String
	}

	contact, err := s.queries.UpdateContact(ctx, db.UpdateContactParams{
		ID:                input.ID,
		CompanyID:         compID,
		FirstName:         input.FirstName,
		LastName:          input.LastName,
		Email:             pgtype.Text{String: input.Email, Valid: input.Email != ""},
		Phone:             pgtype.Text{String: input.Phone, Valid: input.Phone != ""},
		Mobile:            pgtype.Text{String: input.Mobile, Valid: input.Mobile != ""},
		Position:          pgtype.Text{String: input.Position, Valid: input.Position != ""},
		LeadSource:        pgtype.Text{String: input.LeadSource, Valid: input.LeadSource != ""},
		AddressStreet:     pgtype.Text{String: input.AddressStreet, Valid: input.AddressStreet != ""},
		AddressZip:        pgtype.Text{String: input.AddressZip, Valid: input.AddressZip != ""},
		AddressCity:       pgtype.Text{String: input.AddressCity, Valid: input.AddressCity != ""},
		Latitude:          lat,
		Longitude:         lon,
		ConsentPhone:      input.ConsentPhone,
		ConsentEmail:      input.ConsentEmail,
		ConsentUpdatedAt:  consentUpdatedAt,
		StromverbrauchKwh: stromVal,
		GasverbrauchKwh:   gasVal,
		Zaehlernummer:     pgtype.Text{String: zaehler, Valid: zaehler != ""},
		EigentuemerStatus: pgtype.Text{String: status, Valid: true},
		CustomFields:      customFields,
	})
	if err != nil {
		return db.Contact{}, err
	}

	if s.audit != nil {
		var oldConsentTime any = nil
		if existing.ConsentUpdatedAt.Valid {
			oldConsentTime = existing.ConsentUpdatedAt.Time.Format(time.RFC3339)
		}
		var newConsentTime any = nil
		if consentUpdatedAt.Valid {
			newConsentTime = consentUpdatedAt.Time.Format(time.RFC3339)
		}

		oldVals := audit.DiffMap{
			"first_name":         existing.FirstName,
			"last_name":          existing.LastName,
			"email":              existing.Email.String,
			"phone":              existing.Phone.String,
			"mobile":             existing.Mobile.String,
			"position":           existing.Position.String,
			"address_street":     existing.AddressStreet.String,
			"address_zip":        existing.AddressZip.String,
			"address_city":       existing.AddressCity.String,
			"consent_phone":      existing.ConsentPhone,
			"consent_email":      existing.ConsentEmail,
			"consent_updated_at": oldConsentTime,
			"zaehlernummer":      existing.Zaehlernummer.String,
			"eigentuemer_status": existing.EigentuemerStatus.String,
		}
		newVals := audit.DiffMap{
			"first_name":         contact.FirstName,
			"last_name":          contact.LastName,
			"email":              contact.Email.String,
			"phone":              contact.Phone.String,
			"mobile":             contact.Mobile.String,
			"position":           contact.Position.String,
			"address_street":     contact.AddressStreet.String,
			"address_zip":        contact.AddressZip.String,
			"address_city":       contact.AddressCity.String,
			"consent_phone":      contact.ConsentPhone,
			"consent_email":      contact.ConsentEmail,
			"consent_updated_at": newConsentTime,
			"zaehlernummer":      contact.Zaehlernummer.String,
			"eigentuemer_status": contact.EigentuemerStatus.String,
		}

		action := "UPDATE"
		if consentChanged {
			action = "CONSENT_UPDATE"
		}
		if err := s.audit.LogDiff(ctx, actorID, "CONTACT", contact.ID, action, oldVals, newVals, "", ""); err != nil {
			log.Printf("[audit] failed to log contact update: %v", err)
		}
	}

	return contact, nil
}

func (s *Service) Delete(ctx context.Context, actorID pgtype.UUID, id pgtype.UUID) error {
	err := s.queries.DeleteContact(ctx, id)
	if err != nil {
		return err
	}
	if s.audit != nil {
		if err := s.audit.Log(ctx, actorID, "CONTACT", id, "DELETE", map[string]string{"status": "deleted"}, "", ""); err != nil {
			log.Printf("[audit] failed to log contact delete: %v", err)
		}
	}
	return nil
}
