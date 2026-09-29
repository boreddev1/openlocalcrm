package deal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
	ErrDealRevoked         = errors.New("revoked deals are terminal")
	ErrDirectRevocation    = errors.New("direct transition to revoked is prohibited")
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

func isRevokedStage(stage string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(stage))
	return normalized == "WIDERRUFEN" || normalized == "REVOKED"
}

func dealAuditValues(item db.Deal) audit.DiffMap {
	return audit.DiffMap{
		"title":          item.Title,
		"company_id":     item.CompanyID,
		"contact_id":     item.ContactID,
		"value":          item.Value,
		"currency":       item.Currency,
		"stage":          item.Stage,
		"probability":    item.Probability,
		"assigned_to":    item.AssignedTo,
		"closed_at":      item.ClosedAt,
		"custom_fields":  item.CustomFields,
		"widerrufen_at":  item.WiderrufenAt,
		"widerruf_grund": item.WiderrufGrund,
	}
}

func (s *Service) mutableDeal(ctx context.Context, id pgtype.UUID) (db.Deal, error) {
	item, err := s.GetByID(ctx, id)
	if err != nil {
		return db.Deal{}, err
	}
	if isRevokedStage(item.Stage) {
		return db.Deal{}, ErrDealRevoked
	}
	return item, nil
}

func (s *Service) classifyMutationError(ctx context.Context, id pgtype.UUID, err error) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	item, getErr := s.GetByID(ctx, id)
	if getErr != nil {
		return getErr
	}
	if isRevokedStage(item.Stage) {
		return ErrDealRevoked
	}
	return ErrDealNotFound
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
	if isRevokedStage(input.Stage) {
		return db.Deal{}, ErrDirectRevocation
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
	existing, err := s.mutableDeal(ctx, dealID)
	if err != nil {
		return db.Deal{}, err
	}
	if isRevokedStage(stage) {
		return db.Deal{}, ErrDirectRevocation
	}
	if probability < 0 || probability > 100 {
		return db.Deal{}, ErrInvalidProbability
	}

	var closedAt pgtype.Timestamptz
	if stage == "WON" || stage == "LOST" {
		closedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	dealItem, err := s.queries.UpdateDealStage(ctx, db.UpdateDealStageParams{
		ID:          dealID,
		Stage:       stage,
		Probability: probability,
		ClosedAt:    closedAt,
	})
	if err != nil {
		return db.Deal{}, s.classifyMutationError(ctx, dealID, err)
	}

	if s.audit != nil {
		if err := s.audit.LogDiff(ctx, actorID, "DEAL", dealItem.ID, "UPDATE_STAGE", dealAuditValues(existing), dealAuditValues(dealItem), "", ""); err != nil {
			log.Printf("[audit] failed to log deal stage update: %v", err)
		}
	}

	return dealItem, nil
}

func (s *Service) ListByStage(ctx context.Context, stage string, assignedTo pgtype.UUID, pagination ...int32) ([]db.Deal, error) {
	deals, err := s.queries.ListDealsByStage(ctx, db.ListDealsByStageParams{
		Stage:      stage,
		AssignedTo: assignedTo,
	})
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

func (s *Service) List(ctx context.Context, limit, offset int32, assignedTo ...pgtype.UUID) ([]db.Deal, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var assignee pgtype.UUID
	if len(assignedTo) > 0 {
		assignee = assignedTo[0]
	}
	return s.queries.ListDeals(ctx, db.ListDealsParams{
		Limit:      limit,
		Offset:     offset,
		AssignedTo: assignee,
	})
}

func (s *Service) GetByID(ctx context.Context, id pgtype.UUID) (db.Deal, error) {
	item, err := s.queries.GetDealByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Deal{}, ErrDealNotFound
	}
	return item, err
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
	existing, err := s.mutableDeal(ctx, input.ID)
	if err != nil {
		return db.Deal{}, err
	}
	if input.Title == "" {
		return db.Deal{}, ErrInvalidDeal
	}
	if input.Probability < 0 || input.Probability > 100 {
		return db.Deal{}, ErrInvalidProbability
	}
	if isRevokedStage(input.Stage) {
		return db.Deal{}, ErrDirectRevocation
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

	dealItem, err := s.queries.UpdateDeal(ctx, db.UpdateDealParams{
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
		return db.Deal{}, s.classifyMutationError(ctx, input.ID, err)
	}

	if s.audit != nil {
		if err := s.audit.LogDiff(ctx, actorID, "DEAL", dealItem.ID, "UPDATE", dealAuditValues(existing), dealAuditValues(dealItem), "", ""); err != nil {
			log.Printf("[audit] failed to log deal update: %v", err)
		}
	}

	return dealItem, nil
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
	if _, err := s.mutableDeal(ctx, dealID); err != nil {
		return err
	}
	if _, err := s.queries.DeleteDeal(ctx, dealID); err != nil {
		return s.classifyMutationError(ctx, dealID, err)
	}
	if s.audit != nil {
		if err := s.audit.Log(ctx, actorID, "DEAL", dealID, "DELETE", map[string]string{"status": "deleted"}, "", ""); err != nil {
			log.Printf("[audit] failed to log deal delete: %v", err)
		}
	}
	return nil
}

func (s *Service) Revoke(ctx context.Context, actorID pgtype.UUID, id pgtype.UUID, widerrufenAt time.Time, grund string) (db.Deal, error) {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return db.Deal{}, err
	}
	if isRevokedStage(existing.Stage) {
		return existing, nil
	}
	if widerrufenAt.IsZero() {
		widerrufenAt = time.Now().UTC()
	}

	dealItem, err := s.queries.RevokeDeal(ctx, db.RevokeDealParams{
		ID:            id,
		WiderrufenAt:  pgtype.Timestamptz{Time: widerrufenAt, Valid: true},
		WiderrufGrund: pgtype.Text{String: grund, Valid: grund != ""},
	})
	if err != nil {
		return db.Deal{}, s.classifyMutationError(ctx, id, err)
	}

	if s.audit != nil {
		if err := s.audit.LogDiff(ctx, actorID, "DEAL", dealItem.ID, "REVOKE", dealAuditValues(existing), dealAuditValues(dealItem), "", ""); err != nil {
			log.Printf("[audit] failed to log deal revocation: %v", err)
		}
	}

	return dealItem, nil
}
