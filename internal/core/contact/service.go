package contact

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
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
	CompanyID     *pgtype.UUID
	FirstName     string
	LastName      string
	Email         string
	Phone         string
	Mobile        string
	Position      string
	LeadSource    string
	AddressStreet string
	AddressZip    string
	AddressCity   string
	Latitude      *float64
	Longitude     *float64
	CustomFields  []byte
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

	customFields := input.CustomFields
	if len(customFields) == 0 {
		customFields = []byte("{}")
	}

	contact, err := s.queries.CreateContact(ctx, db.CreateContactParams{
		CompanyID:     compID,
		FirstName:     input.FirstName,
		LastName:      input.LastName,
		Email:         pgtype.Text{String: input.Email, Valid: input.Email != ""},
		Phone:         pgtype.Text{String: input.Phone, Valid: input.Phone != ""},
		Mobile:        pgtype.Text{String: input.Mobile, Valid: input.Mobile != ""},
		Position:      pgtype.Text{String: input.Position, Valid: input.Position != ""},
		LeadSource:    pgtype.Text{String: input.LeadSource, Valid: input.LeadSource != ""},
		AddressStreet: pgtype.Text{String: input.AddressStreet, Valid: input.AddressStreet != ""},
		AddressZip:    pgtype.Text{String: input.AddressZip, Valid: input.AddressZip != ""},
		AddressCity:   pgtype.Text{String: input.AddressCity, Valid: input.AddressCity != ""},
		Latitude:      lat,
		Longitude:     lon,
		CustomFields:  customFields,
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
	ID            pgtype.UUID
	CompanyID     *pgtype.UUID
	FirstName     string
	LastName      string
	Email         string
	Phone         string
	Mobile        string
	Position      string
	LeadSource    string
	AddressStreet string
	AddressZip    string
	AddressCity   string
	Latitude      *float64
	Longitude     *float64
	CustomFields  []byte
}

func (s *Service) Update(ctx context.Context, actorID pgtype.UUID, input UpdateContactInput) (db.Contact, error) {
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

	customFields := input.CustomFields
	if len(customFields) == 0 {
		customFields = []byte("{}")
	}

	contact, err := s.queries.UpdateContact(ctx, db.UpdateContactParams{
		ID:            input.ID,
		CompanyID:     compID,
		FirstName:     input.FirstName,
		LastName:      input.LastName,
		Email:         pgtype.Text{String: input.Email, Valid: input.Email != ""},
		Phone:         pgtype.Text{String: input.Phone, Valid: input.Phone != ""},
		Mobile:        pgtype.Text{String: input.Mobile, Valid: input.Mobile != ""},
		Position:      pgtype.Text{String: input.Position, Valid: input.Position != ""},
		LeadSource:    pgtype.Text{String: input.LeadSource, Valid: input.LeadSource != ""},
		AddressStreet: pgtype.Text{String: input.AddressStreet, Valid: input.AddressStreet != ""},
		AddressZip:    pgtype.Text{String: input.AddressZip, Valid: input.AddressZip != ""},
		AddressCity:   pgtype.Text{String: input.AddressCity, Valid: input.AddressCity != ""},
		Latitude:      lat,
		Longitude:     lon,
		CustomFields:  customFields,
	})
	if err != nil {
		return db.Contact{}, err
	}

	if s.audit != nil {
		if err := s.audit.Log(ctx, actorID, "CONTACT", contact.ID, "UPDATE", contact, "", ""); err != nil {
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
