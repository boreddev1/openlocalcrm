package deal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

var (
	ErrDealNotFound        = errors.New("deal not found")
	ErrInvalidDeal         = errors.New("deal title is required")
	ErrInvalidDealValue    = errors.New("deal value must be >= 0")
	ErrInvalidProbability  = errors.New("probability must be between 0 and 100")
	ErrConcurrencyConflict = errors.New("deal was modified by another user, please reload and retry")
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
	if input.Probability < 0 || input.Probability > 100 {
		return db.Deal{}, ErrInvalidProbability
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
		if err := s.audit.Log(ctx, actorID, "DEAL", deal.ID, "CREATE", deal, "", ""); err != nil {
			log.Printf("[audit] failed to log deal create: %v", err)
		}
	}

	return deal, nil
}

func (s *Service) UpdateStage(ctx context.Context, actorID, dealID pgtype.UUID, stage string, probability int32) (db.Deal, error) {
	if probability < 0 || probability > 100 {
		return db.Deal{}, ErrInvalidProbability
	}

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
		if err := s.audit.Log(ctx, actorID, "DEAL", deal.ID, "UPDATE_STAGE", map[string]any{
			"stage":       stage,
			"probability": probability,
		}, "", ""); err != nil {
			log.Printf("[audit] failed to log deal stage update: %v", err)
		}
	}

	return deal, nil
}

func (s *Service) ListByStage(ctx context.Context, stage string, pagination ...int32) ([]db.Deal, error) {
	deals, err := s.queries.ListDealsByStage(ctx, stage)
	if err != nil {
		return nil, err
	}
	limit := int32(100)
	offset := int32(0)
	if len(pagination) > 0 && pagination[0] > 0 {
		limit = pagination[0]
		if limit > 500 {
			limit = 500
		}
	}
	if len(pagination) > 1 && pagination[1] >= 0 {
		offset = pagination[1]
	}
	if int(offset) >= len(deals) {
		return []db.Deal{}, nil
	}
	end := int(offset + limit)
	if end > len(deals) {
		end = len(deals)
	}
	return deals[offset:end], nil
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
	ID           pgtype.UUID
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

func (s *Service) Update(ctx context.Context, actorID pgtype.UUID, input UpdateDealInput) (db.Deal, error) {
	if input.Title == "" {
		return db.Deal{}, ErrInvalidDeal
	}
	if input.Probability < 0 || input.Probability > 100 {
		return db.Deal{}, ErrInvalidProbability
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

	// Merge custom fields: preserve existing fields, overwrite with new ones
	mergedFields, err := s.mergeCustomFields(ctx, input.ID, input.CustomFields)
	if err != nil {
		return db.Deal{}, fmt.Errorf("failed to merge custom fields: %w", err)
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
		CustomFields: mergedFields,
	})
	if err != nil {
		return db.Deal{}, err
	}

	if s.audit != nil {
		if err := s.audit.Log(ctx, actorID, "DEAL", deal.ID, "UPDATE", deal, "", ""); err != nil {
			log.Printf("[audit] failed to log deal update: %v", err)
		}
	}

	return deal, nil
}

// mergeCustomFields merges new custom fields with existing ones from DB.
// Existing keys are preserved unless explicitly overwritten by new values.
func (s *Service) mergeCustomFields(ctx context.Context, dealID pgtype.UUID, newFields []byte) ([]byte, error) {
	if len(newFields) == 0 {
		// No new fields provided; load existing to preserve them
		existing, err := s.queries.GetDealByID(ctx, dealID)
		if err != nil {
			return []byte("{}"), nil // new deal or not found
		}
		if len(existing.CustomFields) > 0 {
			return existing.CustomFields, nil
		}
		return []byte("{}"), nil
	}

	// Load existing custom fields from DB
	existing, err := s.queries.GetDealByID(ctx, dealID)
	if err != nil {
		// Deal not found, just use the new fields
		return newFields, nil
	}

	existingMap := make(map[string]interface{})
	if len(existing.CustomFields) > 0 {
		_ = json.Unmarshal(existing.CustomFields, &existingMap)
	}

	newMap := make(map[string]interface{})
	if err := json.Unmarshal(newFields, &newMap); err != nil {
		return nil, fmt.Errorf("invalid custom fields JSON: %w", err)
	}

	// Merge: new values overwrite existing
	for k, v := range newMap {
		existingMap[k] = v
	}

	return json.Marshal(existingMap)
}

func (s *Service) Delete(ctx context.Context, actorID, dealID pgtype.UUID) error {
	err := s.queries.DeleteDeal(ctx, dealID)
	if err != nil {
		return err
	}
	if s.audit != nil {
		if err := s.audit.Log(ctx, actorID, "DEAL", dealID, "DELETE", map[string]string{"status": "deleted"}, "", ""); err != nil {
			log.Printf("[audit] failed to log deal delete: %v", err)
		}
	}
	return nil
}
