package telephony

import (
	"context"
	"fmt"
	"time"

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
	sseHub *sse.Hub
}

func NewService(sseHub *sse.Hub) *Service {
	return &Service{sseHub: sseHub}
}

func (s *Service) LogCall(ctx context.Context, input LogCallInput) (CallActivity, error) {
	if input.Disposition == "" {
		input.Disposition = "REACHED"
	}

	call := CallActivity{
		ID:              fmt.Sprintf("call-%d", time.Now().UnixNano()),
		ContactID:       input.ContactID,
		DurationSeconds: input.DurationSeconds,
		Disposition:     input.Disposition,
		Notes:           input.Notes,
		CreatedAt:       time.Now(),
	}

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

	return call, nil
}
