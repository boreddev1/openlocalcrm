package db_test

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/stretchr/testify/require"
)

func TestUserInviteAndTemplateContracts(t *testing.T) {
	t.Run("invite token hash is unique", func(t *testing.T) {
		migrations, err := db.GetEmbeddedMigrations()
		require.NoError(t, err)
		require.Contains(t, migrations, "00018_user_invites.sql")
		require.Contains(t, migrations, "00019_email_templates.sql")
	})

	t.Run("demo invite state is persisted", func(t *testing.T) {
		q := demo.NewInMemoryQuerier()
		ctx := context.Background()
		userID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
		expires := time.Now().UTC().Add(72 * time.Hour)
		invite, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
			UserID: userID, TokenHash: strings.Repeat("a", 64),
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		require.NoError(t, err)
		got, err := q.GetUserInviteByTokenHash(ctx, strings.Repeat("a", 64))
		require.NoError(t, err)
		require.Equal(t, invite.ID, got.ID)
	})
}

func testUserID(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}

func TestDemoUserInviteStorage(t *testing.T) {
	ctx := context.Background()
	userID := testUserID(1)
	expires := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Microsecond)

	newQuerier := func(t *testing.T) *demo.InMemoryQuerier {
		t.Helper()
		return demo.NewInMemoryQuerier()
	}

	t.Run("duplicate token hashes are rejected and keep the original row", func(t *testing.T) {
		q := newQuerier(t)
		otherUserID := testUserID(2)
		tokenHash := strings.Repeat("7", 64)

		original, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
			UserID: userID, TokenHash: tokenHash,
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		require.NoError(t, err)

		duplicate, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
			UserID: otherUserID, TokenHash: tokenHash,
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		require.Error(t, err, "token_hash is UNIQUE and the duplicate insert must fail")
		require.Equal(t, db.UserInvite{}, duplicate, "a failed insert must not return a row")

		list, err := q.ListUserInvites(ctx)
		require.NoError(t, err)
		require.Len(t, list, 1, "the rejected insert must not be persisted")

		got, err := q.GetUserInviteByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.Equal(t, original.ID, got.ID, "the original row must remain addressable")
		require.Equal(t, userID, got.UserID, "the original row must not be overwritten")
	})

	t.Run("get by token hash reports unknown hashes", func(t *testing.T) {
		q := newQuerier(t)
		_, err := q.GetUserInviteByTokenHash(ctx, strings.Repeat("f", 64))
		require.ErrorIs(t, err, pgx.ErrNoRows)
	})

	t.Run("list returns newest first", func(t *testing.T) {
		q := newQuerier(t)
		older, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
			UserID: userID, TokenHash: strings.Repeat("1", 64),
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)
		newer, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
			UserID: userID, TokenHash: strings.Repeat("2", 64),
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		require.NoError(t, err)

		list, err := q.ListUserInvites(ctx)
		require.NoError(t, err)
		require.Len(t, list, 2)
		require.Equal(t, newer.ID, list[0].ID)
		require.Equal(t, older.ID, list[1].ID)
	})

	t.Run("revoke does not mutate consumed invites", func(t *testing.T) {
		q := newQuerier(t)
		invite, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
			UserID: userID, TokenHash: strings.Repeat("3", 64),
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		require.NoError(t, err)

		consumedAt := time.Now().UTC().Truncate(time.Microsecond)
		consumed, err := json.Marshal(db.UserInvite{
			ID: invite.ID, UserID: userID, TokenHash: invite.TokenHash,
			ExpiresAt:  pgtype.Timestamptz{Time: expires, Valid: true},
			ConsumedAt: pgtype.Timestamptz{Time: consumedAt, Valid: true},
			CreatedAt:  pgtype.Timestamptz{Time: consumedAt, Valid: true},
		})
		require.NoError(t, err)
		require.NoError(t, q.UpsertUserInvitesRow(ctx, consumed))

		require.NoError(t, q.RevokeUserInvite(ctx, invite.ID))
		got, err := q.GetUserInviteByTokenHash(ctx, invite.TokenHash)
		require.NoError(t, err)
		require.True(t, got.ConsumedAt.Valid)
		require.False(t, got.RevokedAt.Valid, "consumed invites must not be revoked")
	})

	t.Run("revoke marks active invites and tolerates unknown ids", func(t *testing.T) {
		q := newQuerier(t)
		invite, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
			UserID: userID, TokenHash: strings.Repeat("4", 64),
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		require.NoError(t, err)

		require.NoError(t, q.RevokeUserInvite(ctx, invite.ID))
		got, err := q.GetUserInviteByTokenHash(ctx, invite.TokenHash)
		require.NoError(t, err)
		require.True(t, got.RevokedAt.Valid)

		require.NoError(t, q.RevokeUserInvite(ctx, testUserID(9)))
	})

	t.Run("export rows are ordered by primary key", func(t *testing.T) {
		q := newQuerier(t)
		for i := range 3 {
			_, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
				UserID: userID, TokenHash: strings.Repeat(string(rune('a'+i)), 64),
				ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
			})
			require.NoError(t, err)
		}

		rows, err := q.ExportUserInvitesRows(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 3)

		var ids []string
		for _, row := range rows {
			var invite db.UserInvite
			require.NoError(t, json.Unmarshal(row, &invite))
			ids = append(ids, invite.ID.String())
		}
		require.True(t, sort.StringsAreSorted(ids), "export must be ordered by id, got %v", ids)
	})

	t.Run("upsert is idempotent and preserves created_at", func(t *testing.T) {
		source := newQuerier(t)
		invite, err := source.CreateUserInvite(ctx, db.CreateUserInviteParams{
			UserID: userID, TokenHash: strings.Repeat("5", 64),
			ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		})
		require.NoError(t, err)
		rows, err := source.ExportUserInvitesRows(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1)

		target := newQuerier(t)
		require.NoError(t, target.UpsertUserInvitesRow(ctx, rows[0]))
		require.NoError(t, target.UpsertUserInvitesRow(ctx, rows[0]))

		restored, err := target.ExportUserInvitesRows(ctx)
		require.NoError(t, err)
		require.Equal(t, rows, restored)

		got, err := target.GetUserInviteByTokenHash(ctx, strings.Repeat("5", 64))
		require.NoError(t, err)
		require.Equal(t, invite.ID, got.ID)
		require.Equal(t, invite.CreatedAt, got.CreatedAt)
	})

	t.Run("upsert rejects invalid rows", func(t *testing.T) {
		q := newQuerier(t)
		require.Error(t, q.UpsertUserInvitesRow(ctx, []byte("not-json")))

		row, err := json.Marshal(db.UserInvite{UserID: userID, TokenHash: strings.Repeat("6", 64)})
		require.NoError(t, err)
		require.Error(t, q.UpsertUserInvitesRow(ctx, row))
	})
}

func TestDemoEmailTemplateStorage(t *testing.T) {
	ctx := context.Background()

	t.Run("create list update delete", func(t *testing.T) {
		q := demo.NewInMemoryQuerier()
		created, err := q.CreateEmailTemplate(ctx, db.CreateEmailTemplateParams{
			Key: "follow_up", Name: "Nachfassen", Subject: "Ihre Rückmeldung?",
			Body: "Hallo {{first_name}}", Category: "deals",
		})
		require.NoError(t, err)
		require.Equal(t, "", created.Locale, "an explicitly empty locale must stay empty; the column default never fires on an explicit insert")

		got, err := q.GetEmailTemplateByKey(ctx, "follow_up")
		require.NoError(t, err)
		require.Equal(t, created.ID, got.ID)

		updated, err := q.UpdateEmailTemplate(ctx, db.UpdateEmailTemplateParams{
			Key: "follow_up", Name: "Nachfassen v2", Subject: "Ihre Rückmeldung!",
			Body: "Hallo {{first_name}} v2", Category: "crm", Locale: "de-AT",
		})
		require.NoError(t, err)
		require.Equal(t, "Nachfassen v2", updated.Name)
		require.Equal(t, "de-AT", updated.Locale)
		require.Equal(t, created.ID, updated.ID, "update must keep the row identity")

		list, err := q.ListEmailTemplates(ctx)
		require.NoError(t, err)
		require.Len(t, list, 1)

		require.NoError(t, q.DeleteEmailTemplate(ctx, "follow_up"))
		_, err = q.GetEmailTemplateByKey(ctx, "follow_up")
		require.ErrorIs(t, err, pgx.ErrNoRows)
	})

	t.Run("duplicate keys and missing rows are rejected", func(t *testing.T) {
		q := demo.NewInMemoryQuerier()
		params := db.CreateEmailTemplateParams{
			Key: "deal_won", Name: "Deal gewonnen", Subject: "Glueckwunsch",
			Body: "Hallo {{first_name}}", Category: "deals", Locale: "de-DE",
		}
		_, err := q.CreateEmailTemplate(ctx, params)
		require.NoError(t, err)
		_, err = q.CreateEmailTemplate(ctx, params)
		require.Error(t, err)

		_, err = q.UpdateEmailTemplate(ctx, db.UpdateEmailTemplateParams{Key: "missing"})
		require.ErrorIs(t, err, pgx.ErrNoRows)
		_, err = q.GetEmailTemplateByKey(ctx, "missing")
		require.ErrorIs(t, err, pgx.ErrNoRows)
		require.NoError(t, q.DeleteEmailTemplate(ctx, "missing"))
	})

	t.Run("list is ordered by category then key", func(t *testing.T) {
		q := demo.NewInMemoryQuerier()
		for _, params := range []db.CreateEmailTemplateParams{
			{Key: "zeta", Name: "Zeta", Subject: "s", Body: "b", Category: "deals"},
			{Key: "alpha", Name: "Alpha", Subject: "s", Body: "b", Category: "deals"},
			{Key: "first", Name: "First", Subject: "s", Body: "b", Category: "appointments"},
		} {
			_, err := q.CreateEmailTemplate(ctx, params)
			require.NoError(t, err)
		}

		list, err := q.ListEmailTemplates(ctx)
		require.NoError(t, err)
		keys := make([]string, 0, len(list))
		for _, tpl := range list {
			keys = append(keys, tpl.Key)
		}
		require.Equal(t, []string{"first", "alpha", "zeta"}, keys)
	})

	t.Run("export and upsert round trip idempotently", func(t *testing.T) {
		source := demo.NewInMemoryQuerier()
		for _, params := range []db.CreateEmailTemplateParams{
			{Key: "follow_up", Name: "Nachfassen", Subject: "s", Body: "b", Category: "deals", Locale: "de-DE"},
			{Key: "deal_won", Name: "Deal gewonnen", Subject: "s", Body: "b", Category: "deals", Locale: "de-DE"},
		} {
			_, err := source.CreateEmailTemplate(ctx, params)
			require.NoError(t, err)
		}
		rows, err := source.ExportEmailTemplatesRows(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 2)

		var ids []string
		for _, row := range rows {
			var tpl db.EmailTemplate
			require.NoError(t, json.Unmarshal(row, &tpl))
			ids = append(ids, tpl.ID.String())
		}
		require.True(t, sort.StringsAreSorted(ids), "export must be ordered by id, got %v", ids)

		target := demo.NewInMemoryQuerier()
		for pass := 0; pass < 2; pass++ {
			for _, row := range rows {
				require.NoError(t, target.UpsertEmailTemplatesRow(ctx, row))
			}
		}
		restored, err := target.ExportEmailTemplatesRows(ctx)
		require.NoError(t, err)
		require.Equal(t, rows, restored)
	})

	t.Run("upsert keeps the stored id and rejects invalid rows", func(t *testing.T) {
		q := demo.NewInMemoryQuerier()
		created, err := q.CreateEmailTemplate(ctx, db.CreateEmailTemplateParams{
			Key: "follow_up", Name: "Nachfassen", Subject: "s", Body: "b", Category: "deals", Locale: "de-DE",
		})
		require.NoError(t, err)

		incoming, err := json.Marshal(db.EmailTemplate{
			ID: testUserID(7), Key: "follow_up", Name: "Nachfassen v2",
			Subject: "s", Body: "b", Category: "deals", Locale: "de-DE",
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			UpdatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		})
		require.NoError(t, err)
		require.NoError(t, q.UpsertEmailTemplatesRow(ctx, incoming))

		got, err := q.GetEmailTemplateByKey(ctx, "follow_up")
		require.NoError(t, err)
		require.Equal(t, created.ID, got.ID, "ON CONFLICT (key) must keep the stored id")
		require.Equal(t, "Nachfassen v2", got.Name)

		require.Error(t, q.UpsertEmailTemplatesRow(ctx, []byte("not-json")))

		noID, err := json.Marshal(db.EmailTemplate{Key: "orphan", Name: "n", Subject: "s", Body: "b"})
		require.NoError(t, err)
		require.Error(t, q.UpsertEmailTemplatesRow(ctx, noID))
	})
}
