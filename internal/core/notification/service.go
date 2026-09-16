package notification

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

type Service struct {
	queries db.Querier
	sseHub  *sse.Hub
}

func NewService(queries db.Querier, sseHub *sse.Hub) *Service {
	return &Service{
		queries: queries,
		sseHub:  sseHub,
	}
}

type CreateNotificationInput struct {
	UserID  *pgtype.UUID
	Type    string // EMAIL_RECEIVED, LEAD_ASSIGNED, TODO_DUE, WIDERRUF, SYSTEM
	Title   string
	Message string
	Link    string
}

func (s *Service) Create(ctx context.Context, input CreateNotificationInput) (db.Notification, error) {
	var uid pgtype.UUID
	if input.UserID != nil {
		uid = *input.UserID
	}

	n, err := s.queries.CreateNotification(ctx, db.CreateNotificationParams{
		UserID:  uid,
		Type:    input.Type,
		Title:   input.Title,
		Message: input.Message,
		Link:    pgtype.Text{String: input.Link, Valid: input.Link != ""},
		IsRead:  false,
	})
	if err != nil {
		return db.Notification{}, err
	}

	if s.sseHub != nil {
		s.sseHub.Broadcast(sse.Event{
			Type: "notification_created",
			Data: map[string]any{
				"id":         n.ID,
				"type":       n.Type,
				"title":      n.Title,
				"message":    n.Message,
				"link":       n.Link.String,
				"created_at": n.CreatedAt.Time,
			},
		})
	}

	return n, nil
}

func (s *Service) ListUnread(ctx context.Context, userID *pgtype.UUID, limit int32) ([]db.Notification, error) {
	var uid pgtype.UUID
	if userID != nil {
		uid = *userID
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.queries.ListUnreadNotifications(ctx, db.ListUnreadNotificationsParams{
		UserID: uid,
		Limit:  limit,
	})
}

func (s *Service) MarkAsRead(ctx context.Context, id pgtype.UUID) error {
	return s.queries.MarkNotificationAsRead(ctx, id)
}

func (s *Service) MarkAllAsRead(ctx context.Context, userID *pgtype.UUID) error {
	var uid pgtype.UUID
	if userID != nil {
		uid = *userID
	}
	return s.queries.MarkAllNotificationsAsRead(ctx, uid)
}
