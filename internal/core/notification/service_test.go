package notification_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/notification"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

func TestNotificationService_Lifecycle(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	hub := sse.NewHub()
	svc := notification.NewService(querier, hub)

	userID := pgtype.UUID{Bytes: [16]byte{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11}, Valid: true}

	// 1. Create notification
	n, err := svc.Create(ctx, notification.CreateNotificationInput{
		UserID:  &userID,
		Type:    "LEAD_ASSIGNED",
		Title:   "Neuer Lead zugewiesen",
		Message: "Ihnen wurde Dr. Michael Weber zugewiesen.",
		Link:    "/contacts",
	})
	if err != nil {
		t.Fatalf("failed to create notification: %v", err)
	}
	if n.Title != "Neuer Lead zugewiesen" {
		t.Fatalf("unexpected title: %s", n.Title)
	}

	// 2. List notifications
	list, err := svc.ListUnread(ctx, &userID, 10)
	if err != nil {
		t.Fatalf("failed to list notifications: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("expected at least 1 notification")
	}

	// 3. Mark as read
	err = svc.MarkAsRead(ctx, n.ID, &userID)
	if err != nil {
		t.Fatalf("failed to mark notification as read: %v", err)
	}

	// 4. Mark all as read
	err = svc.MarkAllAsRead(ctx, &userID)
	if err != nil {
		t.Fatalf("failed to mark all as read: %v", err)
	}
}
