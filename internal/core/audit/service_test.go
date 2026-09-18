package audit_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/core/audit"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestAuditService_Log(t *testing.T) {
	ctx := context.Background()
	querier := demo.NewInMemoryQuerier()
	svc := audit.NewService(querier)

	userID := pgtype.UUID{Bytes: [16]byte{1, 1, 1}, Valid: true}
	entityID := pgtype.UUID{Bytes: [16]byte{2, 2, 2}, Valid: true}

	err := svc.Log(ctx, userID, "CONTACT", entityID, "UPDATE", map[string]string{"name": "Dr. Weber"}, "127.0.0.1", "Mozilla/5.0")
	if err != nil {
		t.Fatalf("failed to log audit event: %v", err)
	}

	// Logging with nil changes should also succeed
	errNil := svc.Log(ctx, userID, "DEAL", entityID, "DELETE", nil, "127.0.0.1", "Mozilla/5.0")
	if errNil != nil {
		t.Fatalf("failed to log audit event with nil changes: %v", errNil)
	}
}
