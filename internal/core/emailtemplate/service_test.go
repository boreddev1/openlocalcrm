package emailtemplate_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/core/emailtemplate"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

// --- generator double -------------------------------------------------------

type stubGenerator struct {
	mu       sync.Mutex
	answer   string
	err      error
	calls    int
	lastPrmt string
	lastSys  string
}

func (g *stubGenerator) Generate(_ context.Context, prompt, systemInstruction string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	g.lastPrmt = prompt
	g.lastSys = systemInstruction
	return g.answer, g.err
}

func newEnv(t *testing.T, generator emailtemplate.Generator) (*demo.InMemoryQuerier, *emailtemplate.Service) {
	t.Helper()
	q := demo.NewInMemoryQuerier()
	return q, emailtemplate.NewService(q, generator)
}

func validInput() emailtemplate.Input {
	return emailtemplate.Input{
		Key:      "pv-first-offer",
		Name:     "PV Erstangebot",
		Subject:  "Ihr Angebot für {{contact_name}}",
		Body:     "Hallo {{contact_name}}, hier ist Ihr Angebot über {{deal_amount}} €.",
		Category: "vertrieb",
	}
}

// --- CRUD -------------------------------------------------------------------

func TestTemplateCrudPersistsRealRows(t *testing.T) {
	q, svc := newEnv(t, nil)
	ctx := context.Background()

	if list, err := svc.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("a fresh store must be empty, got %d rows (err %v)", len(list), err)
	}

	created, err := svc.Create(ctx, validInput())
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.Key != "pv-first-offer" || created.Name != "PV Erstangebot" {
		t.Fatalf("unexpected created template %+v", created)
	}

	// The row really exists, not just in the return value.
	if _, err := q.GetEmailTemplateByKey(ctx, "pv-first-offer"); err != nil {
		t.Fatalf("Create must persist the row: %v", err)
	}

	fetched, err := svc.Get(ctx, "pv-first-offer")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if fetched.Subject != validInput().Subject {
		t.Fatalf("Get returned %q, want %q", fetched.Subject, validInput().Subject)
	}

	in := validInput()
	in.Name = "PV Erstangebot v2"
	in.Subject = "Ihr Angebot v2"
	updated, err := svc.Update(ctx, "pv-first-offer", in)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Name != "PV Erstangebot v2" {
		t.Fatalf("Update must persist the new name, got %q", updated.Name)
	}
	if stored, _ := q.GetEmailTemplateByKey(ctx, "pv-first-offer"); stored.Name != "PV Erstangebot v2" {
		t.Fatalf("Update must be visible in the store, got %q", stored.Name)
	}

	if err := svc.Delete(ctx, "pv-first-offer"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := q.GetEmailTemplateByKey(ctx, "pv-first-offer"); err == nil {
		t.Fatal("Delete must remove the row")
	}
}

func TestTemplateCreateDefaultsAnEmptyLocaleToDeDE(t *testing.T) {
	_, svc := newEnv(t, nil)
	ctx := context.Background()

	in := validInput()
	in.Locale = ""
	created, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	// The generated INSERT names locale explicitly, so DEFAULT 'de-DE' never
	// fires. Defaulting has to happen here or an empty locale is stored.
	if created.Locale != "de-DE" {
		t.Fatalf("an omitted locale must become de-DE, got %q", created.Locale)
	}
	if emailtemplate.DefaultLocale != "de-DE" {
		t.Fatalf("DefaultLocale must be de-DE, got %q", emailtemplate.DefaultLocale)
	}

	// The same rule applies on update.
	in.Name = "v2"
	updated, err := svc.Update(ctx, created.Key, in)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Locale != "de-DE" {
		t.Fatalf("Update must default an omitted locale too, got %q", updated.Locale)
	}

	// An explicit locale is respected.
	in.Locale = "en-GB"
	updated, err = svc.Update(ctx, created.Key, in)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Locale != "en-GB" {
		t.Fatalf("an explicit locale must be kept, got %q", updated.Locale)
	}
}

func TestTemplateCreateRefusesAMalformedInput(t *testing.T) {
	_, svc := newEnv(t, nil)
	ctx := context.Background()

	cases := map[string]func(in emailtemplate.Input) emailtemplate.Input{
		"no key":      func(in emailtemplate.Input) emailtemplate.Input { in.Key = ""; return in },
		"no name":     func(in emailtemplate.Input) emailtemplate.Input { in.Name = ""; return in },
		"no subject":  func(in emailtemplate.Input) emailtemplate.Input { in.Subject = ""; return in },
		"no body":     func(in emailtemplate.Input) emailtemplate.Input { in.Body = ""; return in },
		"slashed key": func(in emailtemplate.Input) emailtemplate.Input { in.Key = "a/b"; return in },
		"spaced key":  func(in emailtemplate.Input) emailtemplate.Input { in.Key = "a b"; return in },
		"bad locale":  func(in emailtemplate.Input) emailtemplate.Input { in.Locale = "de DE"; return in },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Create(ctx, mutate(validInput())); err == nil {
				t.Fatal("expected the malformed template to be refused")
			}
		})
	}

	if list, _ := svc.List(ctx); len(list) != 0 {
		t.Fatalf("a refused create must not write anything, got %d rows", len(list))
	}
}

func TestTemplateUpdateAndDeleteReportAMissingKey(t *testing.T) {
	_, svc := newEnv(t, nil)
	ctx := context.Background()

	if _, err := svc.Update(ctx, "does-not-exist", validInput()); !errors.Is(err, emailtemplate.ErrNotFound) {
		t.Fatalf("Update on a missing key must report ErrNotFound, got %v", err)
	}
	// Delete is an :exec, so it reports no affected rows. A pre-read turns that
	// into an honest 404 instead of a 200 that deleted nothing.
	if err := svc.Delete(ctx, "does-not-exist"); !errors.Is(err, emailtemplate.ErrNotFound) {
		t.Fatalf("Delete on a missing key must report ErrNotFound, got %v", err)
	}
	if _, err := svc.Get(ctx, "does-not-exist"); !errors.Is(err, emailtemplate.ErrNotFound) {
		t.Fatalf("Get on a missing key must report ErrNotFound, got %v", err)
	}
}

func TestTemplateKeyIsTheIdentityAndCannotBeChangedByAnUpdate(t *testing.T) {
	q, svc := newEnv(t, nil)
	ctx := context.Background()

	if _, err := svc.Create(ctx, validInput()); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	// An update that tries to move the key must not create a second row.
	in := validInput()
	in.Key = "somewhere-else"
	in.Name = "moved"
	updated, err := svc.Update(ctx, "pv-first-offer", in)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Key != "pv-first-offer" {
		t.Fatalf("the key must stay the identity, got %q", updated.Key)
	}
	list, _ := svc.List(ctx)
	if len(list) != 1 {
		t.Fatalf("an update must not create a second row, got %d", len(list))
	}
	if _, err := q.GetEmailTemplateByKey(ctx, "somewhere-else"); err == nil {
		t.Fatal("an update must not write the supplied key")
	}
}

// --- generation -------------------------------------------------------------

func TestGenerateParsesAModelAnswer(t *testing.T) {
	gen := &stubGenerator{answer: `{"name":"Nachfass-Mail","subject":"Ihre Anfrage vom {{contact_name}}","body":"Hallo {{contact_name}},\n\nanbei das Angebot."}`}
	_, svc := newEnv(t, gen)

	out, err := svc.Generate(context.Background(), "Erstelle eine Nachfass-Mail")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if out.Name != "Nachfass-Mail" || out.Subject == "" || out.Body == "" {
		t.Fatalf("unexpected generated template %+v", out)
	}
	if gen.calls != 1 {
		t.Fatalf("the real provider must be called exactly once, got %d", gen.calls)
	}
	if !strings.Contains(gen.lastPrmt, "Nachfass-Mail") {
		t.Fatalf("the prompt must reach the provider, got %q", gen.lastPrmt)
	}
	if !strings.Contains(gen.lastSys, "JSON") {
		t.Fatalf("the system instruction must demand JSON, got %q", gen.lastSys)
	}
}

func TestGenerateToleratesAMarkdownFenceButNothingElse(t *testing.T) {
	cases := map[string]struct {
		answer    string
		wantOK    bool
		wantField string
	}{
		"fenced json": {
			answer:    "```json\n{\"name\":\"A\",\"subject\":\"B\",\"body\":\"C\"}\n```",
			wantOK:    true,
			wantField: "A",
		},
		"bare fence": {
			answer:    "```\n{\"name\":\"A\",\"subject\":\"B\",\"body\":\"C\"}\n```",
			wantOK:    true,
			wantField: "A",
		},
		"prose": {
			answer: "Hier ist Ihre Vorlage: Sehr geehrte Damen und Herren, ...",
		},
		"json array": {
			answer: `[{"name":"A","subject":"B","body":"C"}]`,
		},
		"json string": {
			answer: `"just a string"`,
		},
		"missing body": {
			answer: `{"name":"A","subject":"B"}`,
		},
		"empty body": {
			answer: `{"name":"A","subject":"B","body":"   "}`,
		},
		"extra field": {
			answer: `{"name":"A","subject":"B","body":"C","category":"x"}`,
		},
		"empty answer": {
			answer: "   ",
		},
		"truncated json": {
			answer: `{"name":"A","subject":`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gen := &stubGenerator{answer: tc.answer}
			_, svc := newEnv(t, gen)
			out, err := svc.Generate(context.Background(), "prompt")
			if !tc.wantOK {
				if err == nil {
					t.Fatalf("expected a refusal, got template %+v", out)
				}
				if !errors.Is(err, emailtemplate.ErrGenerationFailed) {
					t.Fatalf("expected ErrGenerationFailed, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			if out.Name != tc.wantField {
				t.Fatalf("expected name %q, got %q", tc.wantField, out.Name)
			}
		})
	}
}

func TestGenerateReportsAProviderFailureInsteadOfInventingATemplate(t *testing.T) {
	upstream := errors.New("dial tcp 127.0.0.1:11434: connect: connection refused")
	gen := &stubGenerator{err: upstream}
	_, svc := newEnv(t, gen)

	out, err := svc.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatalf("an unreachable provider must be an error, got %+v", out)
	}
	if !errors.Is(err, emailtemplate.ErrGenerationFailed) {
		t.Fatalf("expected ErrGenerationFailed, got %v", err)
	}
	if !errors.Is(err, upstream) {
		t.Fatalf("the real provider cause must stay inspectable, got %v", err)
	}
	if out.Body != "" {
		t.Fatalf("a failed generation must not return a body, got %q", out.Body)
	}
}

func TestGenerateNeverPersistsAnything(t *testing.T) {
	q, svc := newEnv(t, &stubGenerator{answer: `{"name":"A","subject":"B","body":"C"}`})
	ctx := context.Background()

	if _, err := svc.Generate(ctx, "prompt"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("Generate must not write a row, got %d templates", len(list))
	}
	if _, err := q.GetEmailTemplateByKey(ctx, "A"); err == nil {
		t.Fatal("Generate must not persist under any key")
	}
}

func TestGenerateRejectsAnEmptyOrOversizedPromptBeforeCallingTheProvider(t *testing.T) {
	gen := &stubGenerator{answer: `{"name":"A","subject":"B","body":"C"}`}
	_, svc := newEnv(t, gen)

	for _, prompt := range []string{"", "   ", strings.Repeat("x", 2001)} {
		if _, err := svc.Generate(context.Background(), prompt); !errors.Is(err, emailtemplate.ErrInvalidInput) {
			t.Fatalf("prompt %q must be refused with ErrInvalidInput, got %v", prompt[:min(len(prompt), 12)], err)
		}
	}
	if gen.calls != 0 {
		t.Fatalf("an invalid prompt must never reach the provider, got %d calls", gen.calls)
	}
}

func TestGenerateWithoutAProviderIsAnHonestError(t *testing.T) {
	_, svc := newEnv(t, nil)
	if _, err := svc.Generate(context.Background(), "prompt"); !errors.Is(err, emailtemplate.ErrGenerationFailed) {
		t.Fatalf("expected ErrGenerationFailed without a provider, got %v", err)
	}
}

// --- construction guards ----------------------------------------------------

func TestServiceWithoutADatabaseReportsUnavailable(t *testing.T) {
	ctx := context.Background()
	svc := emailtemplate.NewService(nil, &stubGenerator{answer: `{"name":"A","subject":"B","body":"C"}`})

	if _, err := svc.List(ctx); !errors.Is(err, emailtemplate.ErrNoGateway) {
		t.Fatalf("List without a database must report ErrNoGateway, got %v", err)
	}
	if _, err := svc.Create(ctx, validInput()); !errors.Is(err, emailtemplate.ErrNoGateway) {
		t.Fatalf("Create without a database must report ErrNoGateway, got %v", err)
	}
	if _, err := svc.Get(ctx, "k"); !errors.Is(err, emailtemplate.ErrNoGateway) {
		t.Fatalf("Get without a database must report ErrNoGateway, got %v", err)
	}
	if _, err := svc.Update(ctx, "k", validInput()); !errors.Is(err, emailtemplate.ErrNoGateway) {
		t.Fatalf("Update without a database must report ErrNoGateway, got %v", err)
	}
	if err := svc.Delete(ctx, "k"); !errors.Is(err, emailtemplate.ErrNoGateway) {
		t.Fatalf("Delete without a database must report ErrNoGateway, got %v", err)
	}
	if _, err := svc.Generate(ctx, "prompt"); !errors.Is(err, emailtemplate.ErrNoGateway) {
		t.Fatalf("Generate without a database must report ErrNoGateway, got %v", err)
	}
}

// --- unique-key honesty -----------------------------------------------------

// A duplicate key must never be reported as a conflict the store did not raise.
// db.IsUniqueViolation only matches a real PostgreSQL SQLSTATE 23505, so the
// untyped demo error must fall through to a generic failure.
func TestDuplicateKeyIsNotClaimedWithoutARealUniqueViolation(t *testing.T) {
	q, svc := newEnv(t, nil)
	ctx := context.Background()

	if _, err := svc.Create(ctx, validInput()); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, dupErr := svc.Create(ctx, validInput())
	if dupErr == nil {
		t.Fatal("a duplicate key must fail")
	}
	if db.IsUniqueViolation(dupErr) {
		t.Fatalf("the demo store did not raise SQLSTATE 23505, so IsUniqueViolation must be false: %v", dupErr)
	}
	if !strings.Contains(dupErr.Error(), "already exists") {
		t.Fatalf("the real cause must stay visible, got %v", dupErr)
	}
	// The original row is untouched.
	if stored, _ := q.GetEmailTemplateByKey(ctx, "pv-first-offer"); stored.Name != "PV Erstangebot" {
		t.Fatalf("a refused duplicate must not modify the stored row, got %q", stored.Name)
	}
}

// --- projection -------------------------------------------------------------

func TestStampRendersTimestampsAndZeroTimes(t *testing.T) {
	var zero db.EmailTemplate
	created, updated := emailtemplate.Stamp(zero)
	if created != "" || updated != "" {
		t.Fatalf("a zero timestamp must render empty, got %q/%q", created, updated)
	}
}
