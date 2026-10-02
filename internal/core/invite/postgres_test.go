package invite_test

// Live-PostgreSQL regression tests for the two SP7 invite statements.
//
// The in-memory demo querier and the unit tests can only prove what the Go code
// asks for. They cannot prove the SQL, and this SQL carries the security
// property: a one-time token must activate an account exactly once, and a
// concurrent second acceptance must be serialised rather than allowed to
// overwrite the password. A missing FOR UPDATE or an inverted predicate would
// leave every unit test green.
//
// The tests therefore run against a real database and skip cleanly when
// TEST_DATABASE_URL is unset, so `go test ./...` and the race detector are
// unaffected. scripts/test-backup-integration.sh sets the variable and runs
// them from `make check`.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// blockedWindow is how long a competing acceptance must stay blocked before the
// test concludes the row lock is not being taken. It is far longer than a local
// query needs and far shorter than a hung test would.
const blockedWindow = 500 * time.Millisecond

// releaseWindow bounds how long the blocked statement may take once the lock is
// released. Generous on purpose: a loaded machine must not fail the test.
const releaseWindow = 30 * time.Second

func postgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping the live PostgreSQL invite test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func rawToken(t *testing.T) (raw, hash string) {
	t.Helper()
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("crypto/rand: %v", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:])
}

// uniqueEmail keeps the tests independent of each other and of the demo seed,
// because users.email is UNIQUE in the schema the tests run against.
func uniqueEmail(t *testing.T) string {
	t.Helper()
	return "invite-pg-" + strings.ToLower(uuid.NewString()) + "@example.de"
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

// seededInvite is the user plus invite one live test created.
type seededInvite struct {
	Email     string
	TokenHash string
	UserID    pgtype.UUID
}

// seedInvite creates an INVITED user with one invite and registers the cleanup
// that removes both again.
//
// The cleanup is registered before the first INSERT, so it runs even when the
// test body fails half way through, and it does not depend on the test having
// reached any particular assertion.
//
// scripts/test-backup-integration.sh also refuses to point the live tests at a
// database it did not provision unless the operator opts in, but a test that
// leaves ten accounts behind in a throwaway container is still untidy, and an
// opted-in long-lived install must not accumulate residue.
func seedInvite(t *testing.T, pool *pgxpool.Pool, expires time.Time) seededInvite {
	t.Helper()

	_, tokenHash := rawToken(t)
	email := uniqueEmail(t)
	invite, err := db.New(pool).CreateInvitedUserWithInvite(context.Background(), db.CreateInvitedUserWithInviteParams{
		Email:        email,
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaGhhc2g",
		FirstName:    "Ines",
		LastName:     "Bar",
		Role:         "BENUTZER",
		TokenHash:    tokenHash,
		ExpiresAt:    pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		t.Fatalf("seed an invite: %v", err)
	}

	seeded := seededInvite{Email: email, TokenHash: tokenHash, UserID: invite.UserID}
	removeSeededRows(t, pool, seeded)
	return seeded
}

// removeSeededRows registers the teardown for one seeded user and its invites.
//
// Children are deleted before the parent, so the delete does not depend on the
// ON DELETE CASCADE of user_invites.user_id, and a delete that fails is reported
// as a test failure rather than swallowed: a cleanup that quietly stopped
// working is exactly the defect this exists to prevent.
func removeSeededRows(t *testing.T, pool *pgxpool.Pool, seeded seededInvite) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM user_invites WHERE user_id = $1`, seeded.UserID); err != nil {
			t.Errorf("cleanup: delete the invites of %s: %v", seeded.Email, err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, seeded.UserID); err != nil {
			t.Errorf("cleanup: delete the user %s: %v", seeded.Email, err)
		}
	})
}

// TestInviteSeededRowsAreCleanedUpAgainstRealPostgres exercises the cleanup
// itself rather than assuming it works.
//
// It needs no second database: the rows are seeded inside a subtest, whose
// t.Cleanup callbacks have all run by the time t.Run returns, so the parent can
// then assert the rows are gone. A cleanup that silently stopped deleting would
// otherwise only show up as slowly growing residue in somebody's database.
func TestInviteSeededRowsAreCleanedUpAgainstRealPostgres(t *testing.T) {
	pool := postgresPool(t)
	expires := time.Now().UTC().Add(72 * time.Hour)

	var seeded seededInvite
	t.Run("seeded", func(t *testing.T) {
		seeded = seedInvite(t, pool, expires)
		if got := countRows(t, pool, `SELECT COUNT(*) FROM users WHERE id = $1`, seeded.UserID); got != 1 {
			t.Fatalf("the seeded user must exist while the subtest runs, found %d", got)
		}
		if got := countRows(t, pool, `SELECT COUNT(*) FROM user_invites WHERE user_id = $1`, seeded.UserID); got != 1 {
			t.Fatalf("the seeded invite must exist while the subtest runs, found %d", got)
		}
	})

	// The subtest is over, so its cleanup has run.
	if got := countRows(t, pool, `SELECT COUNT(*) FROM user_invites WHERE user_id = $1`, seeded.UserID); got != 0 {
		t.Errorf("%d invite row(s) survived the cleanup of %s", got, seeded.Email)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM users WHERE id = $1`, seeded.UserID); got != 0 {
		t.Errorf("%d user row(s) survived the cleanup of %s", got, seeded.Email)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM users WHERE email = $1`, seeded.Email); got != 0 {
		t.Errorf("%d row(s) for %s survived the cleanup", got, seeded.Email)
	}
}

// TestInviteIssuanceIsOneStatementAgainstRealPostgres proves the issuance CTE
// and the users.email constraint as PostgreSQL enforces them, not as the demo
// approximates them.
func TestInviteIssuanceIsOneStatementAgainstRealPostgres(t *testing.T) {
	pool := postgresPool(t)
	q := db.New(pool)
	ctx := context.Background()

	_, tokenHash := rawToken(t)
	email := uniqueEmail(t)
	expires := time.Now().UTC().Add(72 * time.Hour)
	invite, err := q.CreateInvitedUserWithInvite(ctx, db.CreateInvitedUserWithInviteParams{
		Email:        email,
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaGhhc2g",
		FirstName:    "Sabine",
		LastName:     "Mustermann",
		Role:         "VERTRIEB",
		TokenHash:    tokenHash,
		ExpiresAt:    pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateInvitedUserWithInvite: %v", err)
	}
	// Registered before the first assertion, so the rows are removed even if one
	// of them below fails.
	removeSeededRows(t, pool, seededInvite{Email: email, TokenHash: tokenHash, UserID: invite.UserID})
	if invite.TokenHash != tokenHash {
		t.Fatalf("the stored invite must keep the hash, got %q", invite.TokenHash)
	}
	if !invite.UserID.Valid {
		t.Fatal("the returned invite must reference the user it created")
	}

	user, err := q.GetUserByID(ctx, invite.UserID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user.Email != email {
		t.Fatalf("email = %q, want %q", user.Email, email)
	}
	if user.Status != "INVITED" {
		t.Fatalf("status = %q, want INVITED: the account must not be able to log in before acceptance", user.Status)
	}
	if user.Role != "VERTRIEB" {
		t.Fatalf("role = %q, want VERTRIEB: the bind parameter must survive the enum column", user.Role)
	}
	if user.TotpEnabled {
		t.Fatal("a freshly invited account must not report an active second factor")
	}
	if !strings.HasPrefix(user.PasswordHash, "$argon2id$") {
		t.Fatalf("the INVITED account needs a real password hash to be discarded, got %q", user.PasswordHash)
	}
	if got := invite.ExpiresAt.Time.UTC().Sub(time.Now().UTC()); got > 72*time.Hour || got < 71*time.Hour {
		t.Fatalf("the invite must expire in 72 hours, it expires in %s", got)
	}

	// A duplicate address must be a real unique violation, which is the only
	// thing that lets the handler claim a 409.
	_, secondHash := rawToken(t)
	usersBefore := countRows(t, pool, `SELECT COUNT(*) FROM users`)
	invitesBefore := countRows(t, pool, `SELECT COUNT(*) FROM user_invites`)
	_, err = q.CreateInvitedUserWithInvite(ctx, db.CreateInvitedUserWithInviteParams{
		Email:        email,
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaGhhc2g",
		FirstName:    "Sabine",
		LastName:     "Dopplett",
		Role:         "BENUTZER",
		TokenHash:    secondHash,
		ExpiresAt:    pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err == nil {
		t.Fatal("a duplicate email must be refused by users.email UNIQUE")
	}
	if !db.IsUniqueViolation(err) {
		t.Fatalf("a duplicate email must be a real 23505 so a 409 is honest, got %v", err)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM users`); got != usersBefore {
		t.Fatalf("a refused issuance must write no user: %d → %d", usersBefore, got)
	}
	if got := countRows(t, pool, `SELECT COUNT(*) FROM user_invites`); got != invitesBefore {
		t.Fatalf("a refused issuance must write no invite: %d → %d", invitesBefore, got)
	}
}

// TestInviteAcceptanceAgainstRealPostgres proves that a first acceptance
// activates the account and that every unusable token matches zero rows, so the
// handler answers 400 and never reactivates anything.
func TestInviteAcceptanceAgainstRealPostgres(t *testing.T) {
	pool := postgresPool(t)
	q := db.New(pool)
	ctx := context.Background()

	t.Run("first acceptance activates the account", func(t *testing.T) {
		seeded := seedInvite(t, pool, time.Now().UTC().Add(72*time.Hour))

		const chosen = "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$Y2hvc2Vu"
		user, err := q.AcceptUserInvite(ctx, db.AcceptUserInviteParams{
			TokenHash:    seeded.TokenHash,
			PasswordHash: chosen,
		})
		if err != nil {
			t.Fatalf("AcceptUserInvite: %v", err)
		}
		if user.Status != "ACTIVE" {
			t.Fatalf("status = %q, want ACTIVE", user.Status)
		}
		if user.ID.Bytes != seeded.UserID.Bytes {
			t.Fatal("acceptance must activate the account the invite points at")
		}
		if user.PasswordHash != chosen {
			t.Fatalf("the chosen password hash must be stored, got %q", user.PasswordHash)
		}
		if user.LastLoginAt.Valid {
			t.Fatal("accepting an invite is not a login and must not stamp last_login_at")
		}

		stored, err := q.GetUserInviteByTokenHash(ctx, seeded.TokenHash)
		if err != nil {
			t.Fatalf("GetUserInviteByTokenHash: %v", err)
		}
		if !stored.ConsumedAt.Valid {
			t.Fatal("acceptance must consume the invite in the same statement")
		}
		if stored.RevokedAt.Valid {
			t.Fatal("acceptance must not also revoke the invite")
		}
	})

	// The four unusable states must be indistinguishable to the caller: one
	// stable error and no state change at all.
	unusable := map[string]func(t *testing.T) string{
		"consumed": func(t *testing.T) string {
			hash := seedInvite(t, pool, time.Now().UTC().Add(72*time.Hour)).TokenHash
			if _, err := q.AcceptUserInvite(ctx, db.AcceptUserInviteParams{TokenHash: hash, PasswordHash: "x"}); err != nil {
				t.Fatalf("first acceptance must succeed: %v", err)
			}
			return hash
		},
		"revoked": func(t *testing.T) string {
			seeded := seedInvite(t, pool, time.Now().UTC().Add(72*time.Hour))
			var id pgtype.UUID
			if err := pool.QueryRow(ctx, `SELECT id FROM user_invites WHERE user_id = $1`, seeded.UserID).Scan(&id); err != nil {
				t.Fatalf("read invite id: %v", err)
			}
			if err := q.RevokeUserInvite(ctx, id); err != nil {
				t.Fatalf("RevokeUserInvite: %v", err)
			}
			return seeded.TokenHash
		},
		"expired": func(t *testing.T) string {
			return seedInvite(t, pool, time.Now().UTC().Add(-time.Minute)).TokenHash
		},
		"unknown": func(t *testing.T) string {
			_, hash := rawToken(t)
			return hash
		},
	}
	for name, seed := range unusable {
		t.Run(name+" token returns no rows", func(t *testing.T) {
			hash := seed(t)
			userBefore := countRows(t, pool, `SELECT COUNT(*) FROM users WHERE status = 'ACTIVE'`)

			user, err := q.AcceptUserInvite(ctx, db.AcceptUserInviteParams{
				TokenHash:    hash,
				PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdQ$YXR0YWNrZXI",
			})
			if err == nil {
				t.Fatalf("a %s token must not be accepted, got user %+v", name, user)
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("an unusable token must be the stable no-rows error, got %v", err)
			}
			if got := countRows(t, pool, `SELECT COUNT(*) FROM users WHERE status = 'ACTIVE'`); got != userBefore {
				t.Fatalf("a %s token must not activate anything: ACTIVE count %d → %d", name, userBefore, got)
			}
		})
	}
}

// TestInviteConsumeIsSerialisedAgainstRealPostgres is the test that makes the
// FOR UPDATE in AcceptUserInvite load-bearing.
//
// Two acceptances of the same token must not both succeed. The winner runs the
// real statement inside an open transaction, so the loser is evaluated against a
// snapshot taken before the winner committed. With the row lock in the locked
// CTE the loser blocks on the SELECT and, when the winner commits, re-checks the
// active predicates and matches nothing. Without the lock the loser reads the
// same unconsumed row, waits only on the later UPDATE, and then overwrites both
// consumed_at and the password hash.
//
// The test therefore has to assert the *outcome*, not only that the loser waits:
// waiting happens either way, because the UPDATE needs the same row lock.
func TestInviteConsumeIsSerialisedAgainstRealPostgres(t *testing.T) {
	pool := postgresPool(t)
	ctx := context.Background()
	tokenHash := seedInvite(t, pool, time.Now().UTC().Add(72*time.Hour)).TokenHash

	const winnerHash = "$argon2id$v=19$m=65536,t=3,p=4$c2FsdQ$d2lubmVy"
	const loserHash = "$argon2id$v=19$m=65536,t=3,p=4$c2FsdQ$YWxlcg=="

	// The winner: the genuine statement, in a transaction we hold open.
	winner, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin the winning transaction: %v", err)
	}
	t.Cleanup(func() { _ = winner.Rollback(context.Background()) })

	winnerUser, err := db.New(winner).AcceptUserInvite(ctx, db.AcceptUserInviteParams{
		TokenHash:    tokenHash,
		PasswordHash: winnerHash,
	})
	if err != nil {
		t.Fatalf("the winning acceptance failed: %v", err)
	}
	if winnerUser.Status != "ACTIVE" {
		t.Fatalf("status = %q, want ACTIVE", winnerUser.Status)
	}

	// The loser, started while the winner is still uncommitted.
	acceptCtx, cancelAccept := context.WithTimeout(ctx, releaseWindow)
	defer cancelAccept()
	done := make(chan error, 1)
	go func() {
		_, acceptErr := db.New(pool).AcceptUserInvite(acceptCtx, db.AcceptUserInviteParams{
			TokenHash:    tokenHash,
			PasswordHash: loserHash,
		})
		done <- acceptErr
	}()

	select {
	case doneErr := <-done:
		t.Fatalf("the competing acceptance completed before the winner committed (err=%v): "+
			"the invite row is not locked, so two acceptances can race", doneErr)
	case <-time.After(blockedWindow):
		// Expected: the statement is waiting for the invite row.
	}

	if err = winner.Commit(ctx); err != nil {
		t.Fatalf("commit the winning acceptance: %v", err)
	}

	var loserErr error
	select {
	case loserErr = <-done:
	case <-time.After(releaseWindow):
		t.Fatal("the blocked acceptance never completed after the winner committed")
	}
	if !errors.Is(loserErr, pgx.ErrNoRows) {
		t.Fatalf("the second acceptance of the same token must match no rows, got %v", loserErr)
	}

	// And the winner's password must be the one that survives.
	user, err := db.New(pool).GetUserByID(ctx, winnerUser.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user.PasswordHash != winnerHash {
		t.Fatalf("password_hash = %q, want the winner's %q: a one-time token was reused", user.PasswordHash, winnerHash)
	}
	if user.Status != "ACTIVE" {
		t.Fatalf("status = %q, want ACTIVE", user.Status)
	}
}
