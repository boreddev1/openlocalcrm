package company

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

var (
	ErrCompanyNotFound = errors.New("company not found")
	ErrInvalidCompany  = errors.New("company name is required")
)

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

type CreateCompanyInput struct {
	Name           string
	Domain         string
	Phone          string
	Email          string
	AddressStreet  string
	AddressZip     string
	AddressCity    string
	AddressCountry string
	CustomFields   []byte
}

func (s *Service) Create(ctx context.Context, actorID pgtype.UUID, input CreateCompanyInput) (db.Company, error) {
	if input.Name == "" {
		return db.Company{}, ErrInvalidCompany
	}

	customFields := input.CustomFields
	if len(customFields) == 0 {
		customFields = []byte("{}")
	}

	country := input.AddressCountry
	if country == "" {
		country = "DE"
	}

	comp, err := s.queries.CreateCompany(ctx, db.CreateCompanyParams{
		Name:           input.Name,
		Domain:         pgtype.Text{String: input.Domain, Valid: input.Domain != ""},
		Phone:          pgtype.Text{String: input.Phone, Valid: input.Phone != ""},
		Email:          pgtype.Text{String: input.Email, Valid: input.Email != ""},
		AddressStreet:  pgtype.Text{String: input.AddressStreet, Valid: input.AddressStreet != ""},
		AddressZip:     pgtype.Text{String: input.AddressZip, Valid: input.AddressZip != ""},
		AddressCity:    pgtype.Text{String: input.AddressCity, Valid: input.AddressCity != ""},
		AddressCountry: pgtype.Text{String: country, Valid: true},
		CustomFields:   customFields,
	})

	if err != nil {
		return db.Company{}, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "COMPANY", comp.ID, "CREATE", comp, "", "")
	}

	return comp, nil
}

func (s *Service) GetByID(ctx context.Context, id pgtype.UUID) (db.Company, error) {
	return s.queries.GetCompanyByID(ctx, id)
}

func (s *Service) List(ctx context.Context, limit, offset int32) ([]db.Company, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.queries.ListCompanies(ctx, db.ListCompaniesParams{
		Limit:  limit,
		Offset: offset,
	})
}

type UpdateCompanyInput struct {
	ID             pgtype.UUID
	Name           string
	Domain         string
	Phone          string
	Email          string
	AddressStreet  string
	AddressZip     string
	AddressCity    string
	AddressCountry string
	CustomFields   []byte
}

func (s *Service) Update(ctx context.Context, actorID pgtype.UUID, input UpdateCompanyInput) (db.Company, error) {
	if input.Name == "" {
		return db.Company{}, ErrInvalidCompany
	}

	customFields := input.CustomFields
	if len(customFields) == 0 {
		customFields = []byte("{}")
	}

	country := input.AddressCountry
	if country == "" {
		country = "DE"
	}

	comp, err := s.queries.UpdateCompany(ctx, db.UpdateCompanyParams{
		ID:             input.ID,
		Name:           input.Name,
		Domain:         pgtype.Text{String: input.Domain, Valid: input.Domain != ""},
		Phone:          pgtype.Text{String: input.Phone, Valid: input.Phone != ""},
		Email:          pgtype.Text{String: input.Email, Valid: input.Email != ""},
		AddressStreet:  pgtype.Text{String: input.AddressStreet, Valid: input.AddressStreet != ""},
		AddressZip:     pgtype.Text{String: input.AddressZip, Valid: input.AddressZip != ""},
		AddressCity:    pgtype.Text{String: input.AddressCity, Valid: input.AddressCity != ""},
		AddressCountry: pgtype.Text{String: country, Valid: true},
		CustomFields:   customFields,
	})
	if err != nil {
		return db.Company{}, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "COMPANY", comp.ID, "UPDATE", comp, "", "")
	}

	return comp, nil
}

func (s *Service) Delete(ctx context.Context, actorID, compID pgtype.UUID) error {
	err := s.queries.DeleteCompany(ctx, compID)
	if err != nil {
		return err
	}
	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "COMPANY", compID, "DELETE", map[string]string{"status": "deleted"}, "", "")
	}
	return nil
}
