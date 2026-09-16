package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

type Service struct {
	queries db.Querier
}

func NewService(queries db.Querier) *Service {
	return &Service{queries: queries}
}

func (s *Service) Log(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, changes any, ip, userAgent string) error {
	var changesBytes []byte
	var err error
	if changes != nil {
		changesBytes, err = json.Marshal(changes)
		if err != nil {
			changesBytes = []byte("{}")
		}
	} else {
		changesBytes = []byte("{}")
	}

	_, err = s.queries.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		Changes:    changesBytes,
		IpAddress:  pgtype.Text{String: ip, Valid: ip != ""},
		UserAgent:  pgtype.Text{String: userAgent, Valid: userAgent != ""},
	})
	return err
}
