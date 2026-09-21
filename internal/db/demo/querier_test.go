package demo_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func TestDemoQuerierSeedAndUsers(t *testing.T) {
	ctx := context.Background()
	q := demo.NewInMemoryQuerier()

	// 1. Verify CountUsers
	count, err := q.CountUsers(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected at least 2 users seeded, got %d", count)
	}

	// 2. Verify admin user
	admin, err := q.GetUserByEmail(ctx, "admin@mavalio.local")
	if err != nil {
		t.Fatalf("admin user not found: %v", err)
	}
	if admin.Role != "ADMIN" {
		t.Errorf("expected role ADMIN, got %s", admin.Role)
	}
	if !auth.CheckPassword(admin.PasswordHash, "demo123") {
		t.Errorf("expected admin password to be 'demo123'")
	}

	// 3. Verify contacts
	contacts, err := q.ListContacts(ctx, db.ListContactsParams{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("error listing contacts: %v", err)
	}
	if len(contacts) < 2 {
		t.Fatalf("expected at least 2 contacts seeded, got %d", len(contacts))
	}

	// 4. Create, update, delete company
	comp, err := q.CreateCompany(ctx, db.CreateCompanyParams{
		Name: "Test AG",
	})
	if err != nil {
		t.Fatalf("create company failed: %v", err)
	}
	fetched, err := q.GetCompanyByID(ctx, comp.ID)
	if err != nil || fetched.Name != "Test AG" {
		t.Fatalf("get company failed: %v", err)
	}
	if err := q.DeleteCompany(ctx, comp.ID); err != nil {
		t.Fatalf("delete company failed: %v", err)
	}
	_, err = q.GetCompanyByID(ctx, comp.ID)
	if err == nil {
		t.Fatalf("expected error after delete, got nil")
	}

	// 5. Test user status & role update
	err = q.UpdateUserStatus(ctx, db.UpdateUserStatusParams{
		ID:     admin.ID,
		Status: "DEACTIVATED",
	})
	if err != nil {
		t.Fatalf("update status failed: %v", err)
	}
	adminUpdated, _ := q.GetUserByID(ctx, admin.ID)
	if adminUpdated.Status != "DEACTIVATED" {
		t.Errorf("expected status DEACTIVATED, got %s", adminUpdated.Status)
	}

	// 6. Test RefreshToken
	rf, err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    admin.ID,
		TokenHash: "hash123",
		UserAgent: pgtype.Text{String: "Go-Test", Valid: true},
	})
	if err != nil {
		t.Fatalf("create refresh token failed: %v", err)
	}
	gotRF, err := q.GetRefreshToken(ctx, "hash123")
	if err != nil || gotRF.TokenHash != rf.TokenHash {
		t.Fatalf("get refresh token failed: %v", err)
	}
	if err := q.DeleteRefreshToken(ctx, "hash123"); err != nil {
		t.Fatalf("delete refresh token failed: %v", err)
	}
}

func TestDemoQuerierEmailAccountLifecycle(t *testing.T) {
	ctx := context.Background()
	q := demo.NewEmptyInMemoryQuerier()

	acc, err := q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name:         "Vertrieb",
		EmailAddress: "vertrieb@openlocalcrm.local",
		Provider:     "IMAP",
		AccountType:  "team",
		IsActive:     true,
	})
	if err != nil {
		t.Fatalf("CreateEmailAccount failed: %v", err)
	}
	if acc.AccountType != "team" {
		t.Fatalf("expected account_type 'team', got %q", acc.AccountType)
	}

	updated, err := q.UpdateEmailAccount(ctx, db.UpdateEmailAccountParams{
		ID:       acc.ID,
		Name:     "Vertrieb (neu)",
		Username: pgtype.Text{String: "vertrieb2@openlocalcrm.local", Valid: true},
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("UpdateEmailAccount failed: %v", err)
	}
	if updated.Name != "Vertrieb (neu)" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}

	syncTime := pgtype.Timestamptz{Time: updated.CreatedAt.Time, Valid: true}
	if err := q.UpdateEmailAccountSyncState(ctx, db.UpdateEmailAccountSyncStateParams{
		ID:         acc.ID,
		LastSyncAt: syncTime,
		LastUid:    42,
	}); err != nil {
		t.Fatalf("UpdateEmailAccountSyncState failed: %v", err)
	}
	afterSync, err := q.GetEmailAccountByID(ctx, acc.ID)
	if err != nil {
		t.Fatalf("GetEmailAccountByID failed: %v", err)
	}
	if afterSync.LastUid != 42 {
		t.Fatalf("expected LastUid=42, got %d", afterSync.LastUid)
	}

	if err := q.DeleteEmailAccount(ctx, acc.ID); err != nil {
		t.Fatalf("DeleteEmailAccount failed: %v", err)
	}
	if _, err := q.GetEmailAccountByID(ctx, acc.ID); err == nil {
		t.Fatal("expected account to be gone after delete")
	}
}

func TestDemoQuerierUpdateEmailMessageTags(t *testing.T) {
	ctx := context.Background()
	q := demo.NewEmptyInMemoryQuerier()

	acc, err := q.CreateEmailAccount(ctx, db.CreateEmailAccountParams{
		Name:         "Postfach",
		EmailAddress: "postfach@openlocalcrm.local",
		Provider:     "IMAP",
		IsActive:     true,
	})
	if err != nil {
		t.Fatalf("CreateEmailAccount failed: %v", err)
	}

	msg, err := q.CreateEmailMessage(ctx, db.CreateEmailMessageParams{
		AccountID:       acc.ID,
		ThreadID:        "thread-1",
		MessageID:       "<msg-1@example.com>",
		Direction:       "INBOUND",
		SenderEmail:     "kunde@example.com",
		RecipientEmails: []byte("[]"),
		Subject:         "Anfrage",
	})
	if err != nil {
		t.Fatalf("CreateEmailMessage failed: %v", err)
	}

	updated, err := q.UpdateEmailMessageTags(ctx, db.UpdateEmailMessageTagsParams{
		ID:   msg.ID,
		Tags: []byte(`["wichtig","angebot"]`),
	})
	if err != nil {
		t.Fatalf("UpdateEmailMessageTags failed: %v", err)
	}
	if string(updated.Tags) != `["wichtig","angebot"]` {
		t.Fatalf("expected tags to be persisted, got %s", updated.Tags)
	}
}
