package deal

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

var (
	ErrDealNotFound = errors.New("deal not found")
	ErrInvalidDeal  = errors.New("deal title is required")
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

type CreateDealInput struct {
	Title        string
	CompanyID    *pgtype.UUID
	ContactID    *pgtype.UUID
	Value        pgtype.Numeric
	Currency     string
	Stage        string
	Probability  int32
	AssignedTo   *pgtype.UUID
	CustomFields []byte
}

func (s *Service) Create(ctx context.Context, actorID pgtype.UUID, input CreateDealInput) (db.Deal, error) {
	if input.Title == "" {
		return db.Deal{}, ErrInvalidDeal
	}

	var compID, contID, assignID pgtype.UUID
	if input.CompanyID != nil {
		compID = *input.CompanyID
	}
	if input.ContactID != nil {
		contID = *input.ContactID
	}
	if input.AssignedTo != nil {
		assignID = *input.AssignedTo
	}

	stage := input.Stage
	if stage == "" {
		stage = "LEAD"
	}
	curr := input.Currency
	if curr == "" {
		curr = "EUR"
	}

	customFields := input.CustomFields
	if len(customFields) == 0 {
		customFields = []byte("{}")
	}

	deal, err := s.queries.CreateDeal(ctx, db.CreateDealParams{
		Title:        input.Title,
		CompanyID:    compID,
		ContactID:    contID,
		Value:        input.Value,
		Currency:     curr,
		Stage:        stage,
		Probability:  input.Probability,
		AssignedTo:   assignID,
		CustomFields: customFields,
	})

	if err != nil {
		return db.Deal{}, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "DEAL", deal.ID, "CREATE", deal, "", "")
	}

	return deal, nil
}

func (s *Service) UpdateStage(ctx context.Context, actorID, dealID pgtype.UUID, stage string, probability int32) (db.Deal, error) {
	var closedAt pgtype.Timestamptz
	if stage == "WON" || stage == "LOST" {
		closedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	deal, err := s.queries.UpdateDealStage(ctx, db.UpdateDealStageParams{
		ID:          dealID,
		Stage:       stage,
		Probability: probability,
		ClosedAt:    closedAt,
	})

	if err != nil {
		return db.Deal{}, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "DEAL", deal.ID, "UPDATE_STAGE", map[string]any{
			"stage":       stage,
			"probability": probability,
		}, "", "")
	}

	return deal, nil
}

func (s *Service) ListByStage(ctx context.Context, stage string) ([]db.Deal, error) {
	return s.queries.ListDealsByStage(ctx, stage)
}

func (s *Service) List(ctx context.Context, limit, offset int32) ([]db.Deal, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.queries.ListDeals(ctx, db.ListDealsParams{
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Service) GetByID(ctx context.Context, id pgtype.UUID) (db.Deal, error) {
	return s.queries.GetDealByID(ctx, id)
}

type UpdateDealInput struct {
	ID          pgtype.UUID
	Title       string
	CompanyID   *pgtype.UUID
	ContactID   *pgtype.UUID
	Value       pgtype.Numeric
	Currency    string
	Stage       string
	Probability int32
	AssignedTo  *pgtype.UUID
}

func (s *Service) Update(ctx context.Context, actorID pgtype.UUID, input UpdateDealInput) (db.Deal, error) {
	if input.Title == "" {
		return db.Deal{}, ErrInvalidDeal
	}

	var compID, contID, assignID pgtype.UUID
	if input.CompanyID != nil {
		compID = *input.CompanyID
	}
	if input.ContactID != nil {
		contID = *input.ContactID
	}
	if input.AssignedTo != nil {
		assignID = *input.AssignedTo
	}

	curr := input.Currency
	if curr == "" {
		curr = "EUR"
	}

	var closedAt pgtype.Timestamptz
	if input.Stage == "WON" || input.Stage == "LOST" {
		closedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	deal, err := s.queries.UpdateDeal(ctx, db.UpdateDealParams{
		ID:           input.ID,
		Title:        input.Title,
		CompanyID:    compID,
		ContactID:    contID,
		Value:        input.Value,
		Currency:     curr,
		Stage:        input.Stage,
		Probability:  input.Probability,
		AssignedTo:   assignID,
		ClosedAt:     closedAt,
		CustomFields: []byte("{}"),
	})
	if err != nil {
		return db.Deal{}, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "DEAL", deal.ID, "UPDATE", deal, "", "")
	}

	return deal, nil
}

func (s *Service) Delete(ctx context.Context, actorID, dealID pgtype.UUID) error {
	err := s.queries.DeleteDeal(ctx, dealID)
	if err != nil {
		return err
	}
	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "DEAL", dealID, "DELETE", map[string]string{"status": "deleted"}, "", "")
	}
	return nil
}

