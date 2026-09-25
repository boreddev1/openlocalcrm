package contact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
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
	ErrInvalidConsent  = errors.New("consent fields must be booleans")
	ErrInvalidEnergy   = errors.New("energy values must be finite non-negative whole numbers within numeric(10,2)")
)

type OptionalBool struct {
	Value   bool
	Present bool
	Null    bool
}

func (value *OptionalBool) UnmarshalJSON(data []byte) error {
	value.Value = false
	value.Present = true
	value.Null = false
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Null = true
		return nil
	}
	if err := json.Unmarshal(data, &value.Value); err != nil {
		return err
	}
	return nil
}

type OptionalFloat64 struct {
	Value   float64
	Present bool
	Null    bool
}

func (value *OptionalFloat64) UnmarshalJSON(data []byte) error {
	value.Value = 0
	value.Present = true
	value.Null = false
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Null = true
		return nil
	}
	if err := json.Unmarshal(data, &value.Value); err != nil {
		return err
	}
	return nil
}

type OptionalString struct {
	Value   string
	Present bool
	Null    bool
}

func (value *OptionalString) UnmarshalJSON(data []byte) error {
	value.Value = ""
	value.Present = true
	value.Null = false
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Null = true
		return nil
	}
	if err := json.Unmarshal(data, &value.Value); err != nil {
		return err
	}
	return nil
}

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

func consentCreate(input OptionalBool) (bool, error) {
	if input.Null {
		return false, ErrInvalidConsent
	}
	return input.Value, nil
}

func consentUpdate(input OptionalBool, existing bool) (bool, error) {
	if input.Null {
		return false, ErrInvalidConsent
	}
	if !input.Present {
		return existing, nil
	}
	return input.Value, nil
}

func energyValue(input OptionalFloat64) (pgtype.Numeric, error) {
	if !input.Present || input.Null {
		return pgtype.Numeric{}, nil
	}
	if math.IsNaN(input.Value) || math.IsInf(input.Value, 0) || input.Value < 0 || math.Trunc(input.Value) != input.Value || input.Value > 99999999 {
		return pgtype.Numeric{}, ErrInvalidEnergy
	}
	var result pgtype.Numeric
	if err := result.Scan(fmt.Sprintf("%.2f", input.Value)); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("%w: %v", ErrInvalidEnergy, err)
	}
	return result, nil
}

func updatedEnergyValue(input OptionalFloat64, existing pgtype.Numeric) (pgtype.Numeric, error) {
	if !input.Present {
		return existing, nil
	}
	return energyValue(input)
}

func ownerValue(input OptionalString) pgtype.Text {
	if !input.Present || input.Null || input.Value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: input.Value, Valid: true}
}

func updatedOwnerValue(input OptionalString, existing pgtype.Text) pgtype.Text {
	if !input.Present {
		return existing
	}
	return ownerValue(input)
}

func textAuditValue(value pgtype.Text) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func timestampAuditValue(value pgtype.Timestamptz) any {
	if !value.Valid {
		return nil
	}
	return value.Time.Format(time.RFC3339)
}

func numericAuditValue(value pgtype.Numeric) (any, error) {
	if !value.Valid {
		return nil, nil
	}
	converted, err := value.Float64Value()
	if err != nil {
		return nil, err
	}
	return converted.Float64, nil
}

func uuidAuditValue(value pgtype.UUID) any {
	if !value.Valid {
		return nil
	}
	return value.String()
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
	ConsentPhone      OptionalBool
	ConsentEmail      OptionalBool
	StromverbrauchKwh OptionalFloat64
	GasverbrauchKwh   OptionalFloat64
	Zaehlernummer     string
	EigentuemerStatus OptionalString
	CustomFields      []byte
}

func (s *Service) Create(ctx context.Context, actorID pgtype.UUID, input CreateContactInput) (db.Contact, error) {
	if input.FirstName == "" || input.LastName == "" {
		return db.Contact{}, ErrInvalidContact
	}
	if !validateEmail(input.Email) {
		return db.Contact{}, ErrInvalidEmail
	}
	consentPhone, err := consentCreate(input.ConsentPhone)
	if err != nil {
		return db.Contact{}, err
	}
	consentEmail, err := consentCreate(input.ConsentEmail)
	if err != nil {
		return db.Contact{}, err
	}
	stromVal, err := energyValue(input.StromverbrauchKwh)
	if err != nil {
		return db.Contact{}, err
	}
	gasVal, err := energyValue(input.GasverbrauchKwh)
	if err != nil {
		return db.Contact{}, err
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
	if consentPhone || consentEmail {
		consentUpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}
	status := ownerValue(input.EigentuemerStatus)

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
		ConsentPhone:      consentPhone,
		ConsentEmail:      consentEmail,
		ConsentUpdatedAt:  consentUpdatedAt,
		StromverbrauchKwh: stromVal,
		GasverbrauchKwh:   gasVal,
		Zaehlernummer:     pgtype.Text{String: input.Zaehlernummer, Valid: input.Zaehlernummer != ""},
		EigentuemerStatus: status,
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
	ConsentPhone      OptionalBool
	ConsentEmail      OptionalBool
	StromverbrauchKwh OptionalFloat64
	GasverbrauchKwh   OptionalFloat64
	Zaehlernummer     string
	EigentuemerStatus OptionalString
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

	consentPhone, err := consentUpdate(input.ConsentPhone, existing.ConsentPhone)
	if err != nil {
		return db.Contact{}, err
	}
	consentEmail, err := consentUpdate(input.ConsentEmail, existing.ConsentEmail)
	if err != nil {
		return db.Contact{}, err
	}
	consentChanged := consentPhone != existing.ConsentPhone || consentEmail != existing.ConsentEmail
	consentUpdatedAt := existing.ConsentUpdatedAt
	if consentChanged {
		consentUpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}
	stromVal, err := updatedEnergyValue(input.StromverbrauchKwh, existing.StromverbrauchKwh)
	if err != nil {
		return db.Contact{}, err
	}
	gasVal, err := updatedEnergyValue(input.GasverbrauchKwh, existing.GasverbrauchKwh)
	if err != nil {
		return db.Contact{}, err
	}
	status := updatedOwnerValue(input.EigentuemerStatus, existing.EigentuemerStatus)

	zaehler := input.Zaehlernummer
	if zaehler == "" && existing.Zaehlernummer.Valid {
		zaehler = existing.Zaehlernummer.String
	}

	var oldAuditValues, newAuditValues audit.DiffMap
	if s.audit != nil {
		oldStrom, err := numericAuditValue(existing.StromverbrauchKwh)
		if err != nil {
			return db.Contact{}, fmt.Errorf("convert existing stromverbrauch_kwh: %w", err)
		}
		oldGas, err := numericAuditValue(existing.GasverbrauchKwh)
		if err != nil {
			return db.Contact{}, fmt.Errorf("convert existing gasverbrauch_kwh: %w", err)
		}
		newStrom, err := numericAuditValue(stromVal)
		if err != nil {
			return db.Contact{}, fmt.Errorf("convert stromverbrauch_kwh: %w", err)
		}
		newGas, err := numericAuditValue(gasVal)
		if err != nil {
			return db.Contact{}, fmt.Errorf("convert gasverbrauch_kwh: %w", err)
		}
		oldAuditValues = audit.DiffMap{
			"company_id":         uuidAuditValue(existing.CompanyID),
			"first_name":         existing.FirstName,
			"last_name":          existing.LastName,
			"email":              textAuditValue(existing.Email),
			"phone":              textAuditValue(existing.Phone),
			"mobile":             textAuditValue(existing.Mobile),
			"position":           textAuditValue(existing.Position),
			"lead_source":        textAuditValue(existing.LeadSource),
			"address_street":     textAuditValue(existing.AddressStreet),
			"address_zip":        textAuditValue(existing.AddressZip),
			"address_city":       textAuditValue(existing.AddressCity),
			"latitude":           existing.Latitude,
			"longitude":          existing.Longitude,
			"consent_phone":      existing.ConsentPhone,
			"consent_email":      existing.ConsentEmail,
			"consent_updated_at": timestampAuditValue(existing.ConsentUpdatedAt),
			"stromverbrauch_kwh": oldStrom,
			"gasverbrauch_kwh":   oldGas,
			"zaehlernummer":      textAuditValue(existing.Zaehlernummer),
			"eigentuemer_status": textAuditValue(existing.EigentuemerStatus),
			"custom_fields":      existing.CustomFields,
		}
		newAuditValues = audit.DiffMap{
			"company_id":         uuidAuditValue(compID),
			"first_name":         input.FirstName,
			"last_name":          input.LastName,
			"email":              textAuditValue(pgtype.Text{String: input.Email, Valid: input.Email != ""}),
			"phone":              textAuditValue(pgtype.Text{String: input.Phone, Valid: input.Phone != ""}),
			"mobile":             textAuditValue(pgtype.Text{String: input.Mobile, Valid: input.Mobile != ""}),
			"position":           textAuditValue(pgtype.Text{String: input.Position, Valid: input.Position != ""}),
			"lead_source":        textAuditValue(pgtype.Text{String: input.LeadSource, Valid: input.LeadSource != ""}),
			"address_street":     textAuditValue(pgtype.Text{String: input.AddressStreet, Valid: input.AddressStreet != ""}),
			"address_zip":        textAuditValue(pgtype.Text{String: input.AddressZip, Valid: input.AddressZip != ""}),
			"address_city":       textAuditValue(pgtype.Text{String: input.AddressCity, Valid: input.AddressCity != ""}),
			"latitude":           lat,
			"longitude":          lon,
			"consent_phone":      consentPhone,
			"consent_email":      consentEmail,
			"consent_updated_at": timestampAuditValue(consentUpdatedAt),
			"stromverbrauch_kwh": newStrom,
			"gasverbrauch_kwh":   newGas,
			"zaehlernummer":      textAuditValue(pgtype.Text{String: zaehler, Valid: zaehler != ""}),
			"eigentuemer_status": textAuditValue(status),
			"custom_fields":      customFields,
		}
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
		ConsentPhone:      consentPhone,
		ConsentEmail:      consentEmail,
		ConsentUpdatedAt:  consentUpdatedAt,
		StromverbrauchKwh: stromVal,
		GasverbrauchKwh:   gasVal,
		Zaehlernummer:     pgtype.Text{String: zaehler, Valid: zaehler != ""},
		EigentuemerStatus: status,
		CustomFields:      customFields,
	})
	if err != nil {
		return db.Contact{}, err
	}

	if s.audit != nil {
		action := "UPDATE"
		if consentChanged {
			action = "CONSENT_UPDATE"
		}
		if err := s.audit.LogDiff(ctx, actorID, "CONTACT", contact.ID, action, oldAuditValues, newAuditValues, "", ""); err != nil {
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
