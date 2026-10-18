package telephony

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

type CallActivity struct {
	ID              string    `json:"id"`
	ContactID       string    `json:"contact_id"`
	DurationSeconds int       `json:"duration_seconds"`
	Disposition     string    `json:"disposition"` // REACHED, NO_ANSWER, BUSY, VOICEMAIL
	Notes           string    `json:"notes"`
	CreatedAt       time.Time `json:"created_at"`
}

type LogCallInput struct {
	ContactID       string `json:"contact_id"`
	DurationSeconds int    `json:"duration_seconds"`
	Disposition     string `json:"disposition"`
	Notes           string `json:"notes"`
}

type Service struct {
	querier db.Querier
	sseHub  *sse.Hub
}

func NewService(querier db.Querier, sseHub *sse.Hub) *Service {
	return &Service{
		querier: querier,
		sseHub:  sseHub,
	}
}

func (s *Service) LogCall(ctx context.Context, input LogCallInput) (CallActivity, error) {
	if input.Disposition == "" {
		input.Disposition = "REACHED"
	}

	if s.querier == nil {
		return CallActivity{}, fmt.Errorf("database required for logging calls")
	}

	contactUUID, err := uuid.Parse(input.ContactID)
	if err != nil {
		return CallActivity{}, fmt.Errorf("invalid contact ID %q: %w", input.ContactID, err)
	}

	if input.DurationSeconds > math.MaxInt32 || input.DurationSeconds < math.MinInt32 {
		return CallActivity{}, fmt.Errorf("invalid call duration %d", input.DurationSeconds)
	}

	created, err := s.querier.CreateCallActivity(ctx, db.CreateCallActivityParams{
		ContactID:       pgtype.UUID{Bytes: contactUUID, Valid: true},
		DurationSeconds: int32(input.DurationSeconds), //nolint:gosec // G115: range checked above
		Disposition:     input.Disposition,
		Notes:           input.Notes,
	})
	if err != nil {
		return CallActivity{}, fmt.Errorf("failed to persist call activity: %w", err)
	}

	call := CallActivity{
		ID:              uuid.UUID(created.ID.Bytes).String(),
		ContactID:       input.ContactID,
		DurationSeconds: int(created.DurationSeconds),
		Disposition:     created.Disposition,
		Notes:           created.Notes,
		CreatedAt:       created.CreatedAt.Time,
	}
	s.broadcast(call)
	return call, nil
}

func (s *Service) broadcast(call CallActivity) {
	if s.sseHub != nil {
		s.sseHub.Broadcast(sse.Event{
			Type: "call.logged",
			Data: map[string]any{
				"id":         call.ID,
				"contact_id": call.ContactID,
				"duration":   call.DurationSeconds,
			},
		})
	}
}
