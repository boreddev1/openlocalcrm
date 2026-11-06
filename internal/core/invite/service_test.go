package invite_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/invite"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

// --- test doubles -----------------------------------------------------------

type auditEntry struct {
	userID     pgtype.UUID
	entityType string
	entityID   pgtype.UUID
	action     string
	changes    any
	ip         string
	userAgent  string
}

type recordingAudit struct {
	mu      sync.Mutex
	entries []auditEntry
	err     error
}

func (a *recordingAudit) Log(_ context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID, action string, changes any, ip, userAgent string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	a.entries = append(a.entries, auditEntry{
		userID:     userID,
		entityType: entityType,
		entityID:   entityID,
		action:     action,
		changes:    changes,
		ip:         ip,
		userAgent:  userAgent,
	})
	return nil
}

func (a *recordingAudit) actions() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.entries))
	for _, e := range a.entries {
		out = append(out, e.action)
	}
	return out
}

func (a *recordingAudit) containsAction(want string) bool {
	for _, got := range a.actions() {
		if got == want {
			return true
		}
	}
	return false
}

func newEnv(t *testing.T) (*demo.InMemoryQuerier, *recordingAudit, *invite.Service) {
	t.Helper()
	q := demo.NewInMemoryQuerier()
	audit := &recordingAudit{}
	svc := invite.NewService(q, audit)
	return q, audit, svc
}

func adminActor(t *testing.T, q *demo.InMemoryQuerier) pgtype.UUID {
	t.Helper()
	admin, err := q.GetUserByEmail(context.Background(), "admin@openlocalcrm.local")
	if err != nil {
		t.Fatalf("demo admin user missing: %v", err)
	}
	return admin.ID
}

func issue(t *testing.T, svc *invite.Service, actor pgtype.UUID, email string) invite.IssuedInvite {
	t.Helper()
	issued, err := svc.Issue(context.Background(), actor, invite.CreateUserInput{
		Email:     email,
		FirstName: "Tim",
		LastName:  "Tester",
		Role:      "BENUTZER",
	}, "https://crm.example.de")
	if err != nil {
		t.Fatalf("Issue(%s) failed: %v", email, err)
	}
	return issued
}

// --- issuance ---------------------------------------------------------------

func TestIssuePersistsOnlyTheTokenHash(t *testing.T) {
	q, _, svc := newEnv(t)
	actor := adminActor(t, q)

	before := time.Now().UTC()
	issued := issue(t, svc, actor, "tim@example.de")
	after := time.Now().UTC()

	if issued.RawToken == "" {
		t.Fatal("Issue must return the raw token exactly once so the admin can copy the link")
	}

	raw, err := base64.RawURLEncoding.DecodeString(issued.RawToken)
	if err != nil {
		t.Fatalf("raw token must be base64.RawURLEncoding: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("raw token must decode to 32 random bytes, got %d", len(raw))
	}

	if issued.Invite.TokenHash == issued.RawToken {
		t.Fatal("the persisted token_hash must never be the raw token")
	}
	if issued.Invite.TokenHash == "" {
		t.Fatal("token_hash must be persisted so the token can be looked up")
	}
	if len(issued.Invite.TokenHash) != 64 {
		t.Fatalf("token_hash must be a 64 character SHA-256 hex digest, got %d characters", len(issued.Invite.TokenHash))
	}
	if strings.ToLower(issued.Invite.TokenHash) != issued.Invite.TokenHash {
		t.Fatalf("token_hash must be lowercase hex, got %q", issued.Invite.TokenHash)
	}
	sum := sha256.Sum256([]byte(issued.RawToken))
	if want := hex.EncodeToString(sum[:]); issued.Invite.TokenHash != want {
		t.Fatalf("token_hash must be the SHA-256 hex of the raw token: got %q want %q", issued.Invite.TokenHash, want)
	}

	// The raw token must not be recoverable from the stored row.
	row, err := q.GetUserInviteByTokenHash(context.Background(), issued.Invite.TokenHash)
	if err != nil {
		t.Fatalf("GetUserInviteByTokenHash failed: %v", err)
	}
	rowJSON, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal stored invite: %v", err)
	}
	if strings.Contains(string(rowJSON), issued.RawToken) {
		t.Fatalf("the stored invite row must not contain the raw token: %s", rowJSON)
	}
	if !row.ExpiresAt.Valid {
		t.Fatal("the stored invite must carry an expiry")
	}

	// 72 hour expiry, measured against the issuance window.
	low := before.Add(invite.TokenTTL - time.Minute)
	high := after.Add(invite.TokenTTL + time.Minute)
	if row.ExpiresAt.Time.Before(low) || row.ExpiresAt.Time.After(high) {
		t.Fatalf("invite must expire 72h after issuance: expires_at=%s window=[%s,%s]",
			row.ExpiresAt.Time.Format(time.RFC3339), low.Format(time.RFC3339), high.Format(time.RFC3339))
	}
	if invite.TokenTTL != 72*time.Hour {
		t.Fatalf("TokenTTL must be 72h, got %s", invite.TokenTTL)
	}
}

func TestIssueBuildsTheOneTimeLinkAndTheInvitedUser(t *testing.T) {
	q, auditLog, svc := newEnv(t)
	actor := adminActor(t, q)

	issued := issue(t, svc, actor, "tim@example.de")

	if !strings.HasSuffix(issued.URL, "/accept-invite?token="+issued.RawToken) {
		t.Fatalf("the returned link must carry the raw token on the /accept-invite path, got %q", issued.URL)
	}
	if !strings.HasPrefix(issued.URL, "https://crm.example.de/") {
		t.Fatalf("the returned link must use the supplied base URL, got %q", issued.URL)
	}

	user, err := q.GetUserByID(context.Background(), issued.Invite.UserID)
	if err != nil {
		t.Fatalf("the invited user must exist: %v", err)
	}
	if user.Email != "tim@example.de" {
		t.Fatalf("unexpected invited user email %q", user.Email)
	}
	if user.Status != "INVITED" {
		t.Fatalf("an invited user must stay INVITED until the invite is accepted, got %q", user.Status)
	}
	if !strings.HasPrefix(user.PasswordHash, "$argon2") {
		t.Fatalf("the invited user must still get a real (discarded) Argon2id hash, got %q", user.PasswordHash)
	}
	if !issued.Invite.CreatedBy.Valid || issued.Invite.CreatedBy.Bytes != actor.Bytes {
		t.Fatalf("the invite must record the issuing admin as created_by")
	}

	if !auditLog.containsAction("USER_INVITE_CREATED") {
		t.Fatalf("issuance must be audited, got %v", auditLog.actions())
	}
}

func TestIssueIssuesADistinctTokenEveryTime(t *testing.T) {
	q, _, svc := newEnv(t)
	actor := adminActor(t, q)

	first := issue(t, svc, actor, "a@example.de")
	second := issue(t, svc, actor, "b@example.de")

	if first.RawToken == second.RawToken {
		t.Fatal("two invites must not share a raw token")
	}
	if first.Invite.TokenHash == second.Invite.TokenHash {
		t.Fatal("two invites must not share a token hash")
	}
}

func TestIssueRejectsAnInvalidRequest(t *testing.T) {
	q, auditLog, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()

	cases := map[string]invite.CreateUserInput{
		"missing email":      {FirstName: "Tim", LastName: "Tester", Role: "BENUTZER"},
		"missing first name": {Email: "tim@example.de", LastName: "Tester", Role: "BENUTZER"},
		"missing last name":  {Email: "tim@example.de", FirstName: "Tim", Role: "BENUTZER"},
		"hostile email":      {Email: "not-an-email", FirstName: "Tim", LastName: "Tester", Role: "BENUTZER"},
		"hostile role":       {Email: "tim@example.de", FirstName: "Tim", LastName: "Tester", Role: "SUPERADMIN'; DROP TABLE users;--"},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Issue(ctx, actor, input, "https://crm.example.de"); err == nil {
				t.Fatal("expected the invalid invite request to be refused")
			}
		})
	}

	if auditLog.containsAction("USER_INVITE_CREATED") {
		t.Fatalf("a refused request must not be audited as created: %v", auditLog.actions())
	}
}

func TestIssueRejectsADuplicateEmail(t *testing.T) {
	q, _, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()

	issue(t, svc, actor, "tim@example.de")
	if _, err := svc.Issue(ctx, actor, invite.CreateUserInput{
		Email: "TIM@example.de", FirstName: "Tim", LastName: "Tester", Role: "BENUTZER",
	}, "https://crm.example.de"); err == nil {
		t.Fatal("a second invite for the same email must be refused by the users.email UNIQUE constraint")
	}
}

// --- validation (GET) -------------------------------------------------------

func TestValidateReturnsSafeMetadataAndMutatesNothing(t *testing.T) {
	q, auditLog, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()
	issued := issue(t, svc, actor, "tim@example.de")
	auditLog.mu.Lock()
	auditLog.entries = nil
	auditLog.mu.Unlock()

	view, err := svc.Validate(ctx, issued.RawToken)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if view.Email != "tim@example.de" {
		t.Fatalf("Validate must report the invited email, got %q", view.Email)
	}
	if view.FirstName != "Tim" {
		t.Fatalf("Validate must report the invited first name, got %q", view.FirstName)
	}
	if view.ExpiresAt.IsZero() {
		t.Fatal("Validate must report the expiry")
	}
	if view.ExpiresAt.Unix() != issued.Invite.ExpiresAt.Time.Unix() {
		t.Fatalf("Validate expiry must come from the stored invite row: got %s want %s",
			view.ExpiresAt.Format(time.RFC3339), issued.Invite.ExpiresAt.Time.Format(time.RFC3339))
	}

	// GET must not consume anything.
	stored, err := q.GetUserInviteByTokenHash(ctx, issued.Invite.TokenHash)
	if err != nil {
		t.Fatalf("GetUserInviteByTokenHash failed: %v", err)
	}
	if stored.ConsumedAt.Valid {
		t.Fatal("GET /accept-invite must never consume the invite")
	}
	if stored.RevokedAt.Valid {
		t.Fatal("GET /accept-invite must never revoke the invite")
	}
	user, err := q.GetUserByID(ctx, stored.UserID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if user.Status != "INVITED" {
		t.Fatalf("GET /accept-invite must not activate the account, status is %q", user.Status)
	}
	if auditLog.containsAction("USER_INVITE_ACCEPTED") {
		t.Fatalf("GET /accept-invite must not write an acceptance audit event: %v", auditLog.actions())
	}
}

func TestValidateRejectsEveryUnusableInviteWithTheSameStableError(t *testing.T) {
	q, auditLog, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()

	t.Run("unknown token", func(t *testing.T) {
		_, err := svc.Validate(ctx, "totally-made-up-token")
		requireNotUsable(t, err)
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := svc.Validate(ctx, "")
		requireNotUsable(t, err)
	})

	t.Run("already consumed", func(t *testing.T) {
		issued := issue(t, svc, actor, "consumed@example.de")
		if _, err := svc.Accept(ctx, issued.RawToken, "SuperSecret1!"); err != nil {
			t.Fatalf("Accept failed: %v", err)
		}
		_, err := svc.Validate(ctx, issued.RawToken)
		requireNotUsable(t, err)
	})

	t.Run("revoked", func(t *testing.T) {
		issued := issue(t, svc, actor, "revoked@example.de")
		if err := q.RevokeUserInvite(ctx, issued.Invite.ID); err != nil {
			t.Fatalf("RevokeUserInvite failed: %v", err)
		}
		_, err := svc.Validate(ctx, issued.RawToken)
		requireNotUsable(t, err)
	})

	t.Run("expired", func(t *testing.T) {
		issued := issue(t, svc, actor, "expired@example.de")
		expireInvite(t, q, issued.Invite, time.Now().UTC().Add(-time.Hour))
		_, err := svc.Validate(ctx, issued.RawToken)
		requireNotUsable(t, err)
	})

	if !auditLog.containsAction("USER_INVITE_REJECTED") {
		t.Fatalf("rejected validations must be audited: %v", auditLog.actions())
	}
}

// --- acceptance (POST) ------------------------------------------------------

func TestAcceptConsumesTheTokenAndActivatesTheAccount(t *testing.T) {
	q, auditLog, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()
	issued := issue(t, svc, actor, "tim@example.de")

	user, err := svc.Accept(ctx, issued.RawToken, "SuperSecret1!")
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	if user.Status != "ACTIVE" {
		t.Fatalf("Accept must activate the account, status is %q", user.Status)
	}
	if user.ID.Bytes != issued.Invite.UserID.Bytes {
		t.Fatal("Accept must activate the user the invite points at")
	}
	if !auth.CheckPassword(user.PasswordHash, "SuperSecret1!") {
		t.Fatal("Accept must store a real Argon2id hash of the chosen password")
	}

	stored, err := q.GetUserInviteByTokenHash(ctx, issued.Invite.TokenHash)
	if err != nil {
		t.Fatalf("GetUserInviteByTokenHash failed: %v", err)
	}
	if !stored.ConsumedAt.Valid {
		t.Fatal("Accept must consume the invite")
	}

	if _, err := svc.Validate(ctx, issued.RawToken); err == nil {
		t.Fatal("a consumed token must no longer validate")
	}
	if !auditLog.containsAction("USER_INVITE_ACCEPTED") {
		t.Fatalf("acceptance must be audited: %v", auditLog.actions())
	}
}

// TestAcceptConsumesExactlyTheInviteTheTokenNames pins demo/SQL parity. The
// stored procedure locks one invite row by token hash and consumes that row
// (WHERE i.id = l.id). A demo implementation that looks the user up and then
// consumes "an invite of that user" in map order would burn an unrelated invite
// and leave the presented token usable, which is precisely what the lock
// prevents.
func TestAcceptConsumesExactlyTheInviteTheTokenNames(t *testing.T) {
	q, _, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()
	issued := issue(t, svc, actor, "two-invites@example.de")

	// A second, independent invite for the same account. The HTTP API cannot
	// create this pair (users.email is UNIQUE), so it is built directly, which
	// is also the only way to reach the divergence.
	first, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
		UserID:    issued.Invite.UserID,
		TokenHash: strings.Repeat("a", 64),
		CreatedBy: actor,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(72 * time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateUserInvite failed: %v", err)
	}

	if _, err = svc.Accept(ctx, issued.RawToken, "SuperSecret1!"); err != nil {
		t.Fatalf("Accept failed: %v", err)
	}

	presented, err := q.GetUserInviteByTokenHash(ctx, issued.Invite.TokenHash)
	if err != nil {
		t.Fatalf("GetUserInviteByTokenHash failed: %v", err)
	}
	if !presented.ConsumedAt.Valid {
		t.Fatal("the invite named by the accepted token must be consumed")
	}
	if presented.ID.Bytes != issued.Invite.ID.Bytes {
		t.Fatal("the consumed row must be the row the token names")
	}
	untouched, err := q.GetUserInviteByTokenHash(ctx, first.TokenHash)
	if err != nil {
		t.Fatalf("GetUserInviteByTokenHash failed: %v", err)
	}
	if untouched.ConsumedAt.Valid {
		t.Fatal("an unrelated invite for the same account must stay usable")
	}
}

func TestAcceptRefusesAReusedTokenAndLeavesTheAccountActive(t *testing.T) {
	q, _, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()
	issued := issue(t, svc, actor, "tim@example.de")

	if _, err := svc.Accept(ctx, issued.RawToken, "SuperSecret1!"); err != nil {
		t.Fatalf("first Accept failed: %v", err)
	}

	if _, err := svc.Accept(ctx, issued.RawToken, "Attacker99!"); err == nil {
		t.Fatal("a second acceptance with the same token must fail")
	}

	// The password must be the one the legitimate invitee chose.
	user, err := q.GetUserByID(ctx, issued.Invite.UserID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if !auth.CheckPassword(user.PasswordHash, "SuperSecret1!") {
		t.Fatal("a reused token must not change the stored password")
	}
	if auth.CheckPassword(user.PasswordHash, "Attacker99!") {
		t.Fatal("a reused token must not be able to overwrite the password")
	}
}

func TestAcceptRejectsRevokedExpiredAndUnknownTokens(t *testing.T) {
	q, _, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()

	t.Run("revoked", func(t *testing.T) {
		issued := issue(t, svc, actor, "revoked@example.de")
		if err := q.RevokeUserInvite(ctx, issued.Invite.ID); err != nil {
			t.Fatalf("RevokeUserInvite failed: %v", err)
		}
		_, err := svc.Accept(ctx, issued.RawToken, "SuperSecret1!")
		requireNotUsable(t, err)
		requireStillInvited(t, q, issued.Invite.UserID)
	})

	t.Run("expired", func(t *testing.T) {
		issued := issue(t, svc, actor, "expired@example.de")
		expireInvite(t, q, issued.Invite, time.Now().UTC().Add(-time.Minute))
		_, err := svc.Accept(ctx, issued.RawToken, "SuperSecret1!")
		requireNotUsable(t, err)
		requireStillInvited(t, q, issued.Invite.UserID)
	})

	t.Run("unknown", func(t *testing.T) {
		_, err := svc.Accept(ctx, "nope", "SuperSecret1!")
		requireNotUsable(t, err)
	})

	t.Run("empty", func(t *testing.T) {
		_, err := svc.Accept(ctx, "", "SuperSecret1!")
		requireNotUsable(t, err)
	})
}

func TestAcceptRefusesAWeakPasswordAndKeepsTheInviteUsable(t *testing.T) {
	q, _, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()
	issued := issue(t, svc, actor, "tim@example.de")

	// The floor is the same 8 characters auth.ChangePassword applies, so an
	// invitee is not held to a different standard than an existing user.
	for _, password := range []string{"", "short1!", "1234567"} {
		_, err := svc.Accept(ctx, issued.RawToken, password)
		if err == nil {
			t.Fatalf("password %q must be refused", password)
		}
		if !errors.Is(err, invite.ErrWeakPassword) {
			t.Fatalf("a weak password must return ErrWeakPassword, got %v", err)
		}
	}

	// Nothing was consumed, so the invitee can still pick a good password.
	if _, err := svc.Validate(ctx, issued.RawToken); err != nil {
		t.Fatalf("a refused weak password must leave the invite usable, got %v", err)
	}
	if _, err := svc.Accept(ctx, issued.RawToken, "abcdefgh"); err != nil {
		t.Fatalf("Accept with an 8-character password failed: %v", err)
	}
}

func TestAcceptSucceedsExactlyOnceUnderConcurrentUse(t *testing.T) {
	q, _, svc := newEnv(t)
	actor := adminActor(t, q)
	issued := issue(t, svc, actor, "race@example.de")

	const racers = 8
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
		results   []error
	)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := svc.Accept(context.Background(), issued.RawToken, "SuperSecret1!")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				succeeded++
			}
			results = append(results, err)
		}(i)
	}
	wg.Wait()

	if succeeded != 1 {
		t.Fatalf("exactly one concurrent acceptance may win, got %d (errors: %v)", succeeded, results)
	}

	stored, err := q.GetUserInviteByTokenHash(context.Background(), issued.Invite.TokenHash)
	if err != nil {
		t.Fatalf("GetUserInviteByTokenHash failed: %v", err)
	}
	if !stored.ConsumedAt.Valid {
		t.Fatal("the winning acceptance must consume the invite")
	}
}

// --- audit and link ---------------------------------------------------------

func TestInviteAuditEventsNeverCarryTheRawToken(t *testing.T) {
	q, auditLog, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()
	issued := issue(t, svc, actor, "tim@example.de")
	if _, err := svc.Accept(ctx, issued.RawToken, "SuperSecret1!"); err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	_, _ = svc.Validate(ctx, issued.RawToken)

	auditLog.mu.Lock()
	defer auditLog.mu.Unlock()
	if len(auditLog.entries) == 0 {
		t.Fatal("expected audit entries")
	}
	for _, e := range auditLog.entries {
		blob, err := json.Marshal(map[string]any{
			"action": e.action, "changes": e.changes, "entity": e.entityID, "ip": e.ip, "ua": e.userAgent,
		})
		if err != nil {
			t.Fatalf("marshal audit entry: %v", err)
		}
		if strings.Contains(string(blob), issued.RawToken) {
			t.Fatalf("audit entry %q leaked the raw invite token: %s", e.action, blob)
		}
		if strings.Contains(string(blob), issued.Invite.TokenHash) {
			t.Fatalf("audit entry %q leaked the token hash: %s", e.action, blob)
		}
	}
}

// TestBuildInviteURLAssemblesFromASanitisedOrigin covers the pure assembler.
// The scheme and host decisions are not tested here: they are the HTTP layer's
// job and are covered end to end in
// TestInviteLinkOriginFollowsTheRequest, because that is the only place they run.
func TestBuildInviteURLAssemblesFromASanitisedOrigin(t *testing.T) {
	cases := []struct {
		name string
		base string
		want string
	}{
		{name: "configured domain without a scheme", base: "crm.example.de", want: "https://crm.example.de/accept-invite?token=abc"},
		{name: "an explicit scheme is kept", base: "http://crm.example.de", want: "http://crm.example.de/accept-invite?token=abc"},
		{name: "a trailing slash does not double up", base: "https://crm.example.de/", want: "https://crm.example.de/accept-invite?token=abc"},
		{name: "no origin degrades to the path", base: "", want: "/accept-invite?token=abc"},
		{name: "surrounding whitespace is ignored", base: "  https://crm.example.de  ", want: "https://crm.example.de/accept-invite?token=abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := invite.BuildInviteURL(tc.base, "abc")
			if got != tc.want {
				t.Fatalf("BuildInviteURL(%q) = %q, want %q", tc.base, got, tc.want)
			}
		})
	}
}

// TestSanitizeHostRefusesAnythingThatIsNotOnlyAHost pins the gate the HTTP layer
// puts the client-supplied Host header through. The composed consequence is
// covered in internal/server/handlers by TestInviteLinkOriginFollowsTheRequest.
func TestSanitizeHostRefusesAnythingThatIsNotOnlyAHost(t *testing.T) {
	accepted := []string{
		"crm.example.de",
		"crm.example.de:8443",
		"localhost:8080",
		"192.0.2.10",
		"[2001:db8::1]:8443",
		"xn--crm-6db.example.de",
	}
	for _, host := range accepted {
		if got := invite.SanitizeHost(host); got != host {
			t.Errorf("SanitizeHost(%q) = %q, want it unchanged", host, got)
		}
	}

	refused := map[string]string{
		"empty":            "",
		"whitespace only":  "   ",
		"carriage return":  "evil.example/\r\nX-Injected: 1",
		"header injection": "evil.example\r\nSet-Cookie: a=b",
		"path":             "crm.example.de/admin",
		"backslash":        `crm.example.de\admin`,
		"credentials":      "user@evil.example",
		"query":            "crm.example.de?a=1",
		"fragment":         "crm.example.de#x",
		"embedded scheme":  "https://evil.example",
		"embedded space":   "crm example.de",
		"tab":              "crm.example.de\tx",
		"non ascii":        "cr\u00fcm.example.de",
		"vertical tab":     "crm.example.de\x0b",
	}
	for name, host := range refused {
		if got := invite.SanitizeHost(host); got != "" {
			t.Errorf("%s: SanitizeHost(%q) = %q, want it refused", name, host, got)
		}
	}
}

func TestBuildInviteURLEscapesTheToken(t *testing.T) {
	got := invite.BuildInviteURL("https://crm.example.de", "a b&c=d")
	if !strings.Contains(got, "token=a+b%26c%3Dd") && !strings.Contains(got, "token=a%20b%26c%3Dd") {
		t.Fatalf("the token must be query-escaped, got %q", got)
	}
	if strings.Contains(got, "a b&c=d") {
		t.Fatalf("the raw token must not leak unescaped into the link, got %q", got)
	}
}

func TestValidateRejectsATokenThatDoesNotMatchAnyHash(t *testing.T) {
	q, _, svc := newEnv(t)
	actor := adminActor(t, q)
	ctx := context.Background()
	issued := issue(t, svc, actor, "tim@example.de")

	// A token of the right shape but the wrong bytes must not validate.
	wrong := issued.RawToken[:len(issued.RawToken)-1] + "A"
	if wrong == issued.RawToken {
		wrong = issued.RawToken[:len(issued.RawToken)-1] + "B"
	}
	_, err := svc.Validate(ctx, wrong)
	requireNotUsable(t, err)
}

// --- helpers ----------------------------------------------------------------

func requireNotUsable(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected the invite to be refused")
	}
	if !errors.Is(err, invite.ErrInviteNotUsable) {
		t.Fatalf("expected ErrInviteNotUsable, got %v", err)
	}
}

func requireStillInvited(t *testing.T, q *demo.InMemoryQuerier, userID pgtype.UUID) {
	t.Helper()
	user, err := q.GetUserByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if user.Status != "INVITED" {
		t.Fatalf("a refused acceptance must never activate the account, status is %q", user.Status)
	}
	if user.PasswordHash == "" {
		t.Fatal("a refused acceptance must not clear the stored password hash")
	}
}

func expireInvite(t *testing.T, q *demo.InMemoryQuerier, invite db.UserInvite, at time.Time) {
	t.Helper()
	// The demo store is only reachable through the generated surface, so an
	// already-issued invite is aged backwards through the restore upsert, whose
	// ON CONFLICT (id) DO UPDATE clause rewrites expires_at in place.
	invite.ExpiresAt = pgtype.Timestamptz{Time: at, Valid: true}
	row, err := json.Marshal(invite)
	if err != nil {
		t.Fatalf("marshal expired invite fixture: %v", err)
	}
	if err := q.UpsertUserInvitesRow(context.Background(), row); err != nil {
		t.Fatalf("seed expired invite fixture: %v", err)
	}
}
