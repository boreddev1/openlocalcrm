// Package emailtemplate owns durable e-mail template CRUD and the real AI
// template generator.
//
// Two rules govern this package:
//
//   - Every mutation goes through the persisted db.Querier. A template that is
//     not in email_templates does not exist as far as the API is concerned.
//   - Generation calls the configured LLM. There is no canned fallback: an
//     unreachable provider or a non-JSON answer is an error the caller must
//     surface. A generated draft is only a draft; persisting it is an explicit,
//     separate create or update.
package emailtemplate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// DefaultLocale is applied when a caller omits the locale.
//
// The generated INSERT names locale explicitly, so the column's DEFAULT
// 'de-DE' never fires and PostgreSQL would store the empty string. Defaulting
// belongs here, in the validation layer, so the demo and the real database agree
// on what an omitted locale means.
const DefaultLocale = "de-DE"

const (
	maxKeyLength  = 100
	maxNameLength = 255
)

var (
	// ErrNotFound reports a key that does not exist.
	ErrNotFound = errors.New("email template not found")
	// ErrInvalidInput refuses a malformed template before anything is written.
	ErrInvalidInput = errors.New("invalid email template")
	// ErrGenerationFailed reports that the provider failed or answered with
	// something that is not a template. It never carries a substitute body.
	ErrGenerationFailed = errors.New("template generation failed")
	// ErrNoGateway signals a service built without a database.
	ErrNoGateway = errors.New("email template service has no database")
)

// Generator is the real LLM seam. *ai.Gateway satisfies it.
type Generator interface {
	Generate(ctx context.Context, prompt string, systemInstruction string) (string, error)
}

// Input is the admin-supplied template content.
type Input struct {
	Key      string
	Name     string
	Subject  string
	Body     string
	Category string
	Locale   string
}

// normalize trims every field and applies the locale default.
func (in Input) normalize() Input {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	in.Subject = strings.TrimSpace(in.Subject)
	in.Body = strings.TrimSpace(in.Body)
	in.Category = strings.TrimSpace(in.Category)
	in.Locale = strings.TrimSpace(in.Locale)
	if in.Locale == "" {
		in.Locale = DefaultLocale
	}
	return in
}

func (in Input) validate(requireKey bool) error {
	if requireKey {
		if in.Key == "" {
			return errors.New("key is required")
		}
		if len(in.Key) > maxKeyLength {
			return errors.New("key is longer than 100 characters")
		}
		if strings.ContainsAny(in.Key, " \t\r\n/?#&") {
			return errors.New("key must not contain whitespace, slashes or URL separators")
		}
	}
	if in.Name == "" {
		return errors.New("name is required")
	}
	if len(in.Name) > maxNameLength {
		return errors.New("name is longer than 255 characters")
	}
	if in.Subject == "" {
		return errors.New("subject is required")
	}
	if in.Body == "" {
		return errors.New("body is required")
	}
	if strings.ContainsAny(in.Locale, " \t\r\n") {
		return errors.New("locale must be a single BCP 47 tag")
	}
	return nil
}

// Service is the template CRUD and generation facade.
type Service struct {
	queries   db.Querier
	generator Generator
}

func NewService(queries db.Querier, generator Generator) *Service {
	return &Service{queries: queries, generator: generator}
}

func (s *Service) ready() error {
	if s == nil || s.queries == nil {
		return ErrNoGateway
	}
	return nil
}

// List returns every persisted template.
func (s *Service) List(ctx context.Context) ([]db.EmailTemplate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.queries.ListEmailTemplates(ctx)
}

// Get returns one persisted template by key.
func (s *Service) Get(ctx context.Context, key string) (db.EmailTemplate, error) {
	if err := s.ready(); err != nil {
		return db.EmailTemplate{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return db.EmailTemplate{}, ErrInvalidInput
	}
	tpl, err := s.queries.GetEmailTemplateByKey(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.EmailTemplate{}, ErrNotFound
		}
		return db.EmailTemplate{}, err
	}
	return tpl, nil
}

// Create persists a new template.
//
// A duplicate key surfaces as the store's own error. Callers map
// db.IsUniqueViolation to a 409; the in-memory demo cannot report a unique
// violation, so in demo mode the caller must answer with its generic failure
// instead of a conflict it never saw.
func (s *Service) Create(ctx context.Context, in Input) (db.EmailTemplate, error) {
	if err := s.ready(); err != nil {
		return db.EmailTemplate{}, err
	}
	in = in.normalize()
	if err := in.validate(true); err != nil {
		return db.EmailTemplate{}, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}
	return s.queries.CreateEmailTemplate(ctx, db.CreateEmailTemplateParams{
		Key:      in.Key,
		Name:     in.Name,
		Subject:  in.Subject,
		Body:     in.Body,
		Category: in.Category,
		Locale:   in.Locale,
	})
}

// Update rewrites the content of an existing template. The key is the identity
// and is not changed, so the stored row is really updated rather than replaced.
func (s *Service) Update(ctx context.Context, key string, in Input) (db.EmailTemplate, error) {
	if err := s.ready(); err != nil {
		return db.EmailTemplate{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return db.EmailTemplate{}, ErrInvalidInput
	}
	in = in.normalize()
	in.Key = key
	if err := in.validate(false); err != nil {
		return db.EmailTemplate{}, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}
	tpl, err := s.queries.UpdateEmailTemplate(ctx, db.UpdateEmailTemplateParams{
		Key:      key,
		Name:     in.Name,
		Subject:  in.Subject,
		Body:     in.Body,
		Category: in.Category,
		Locale:   in.Locale,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.EmailTemplate{}, ErrNotFound
		}
		return db.EmailTemplate{}, err
	}
	return tpl, nil
}

// Delete removes a template.
func (s *Service) Delete(ctx context.Context, key string) error {
	if err := s.ready(); err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrInvalidInput
	}
	// The delete is an :exec, so it reports no affected rows. Read first so a
	// missing key is an honest 404 instead of a 200 that deleted nothing.
	if _, err := s.queries.GetEmailTemplateByKey(ctx, key); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return s.queries.DeleteEmailTemplate(ctx, key)
}

// GeneratedTemplate is the model output. It is a draft: nothing is persisted
// until the caller saves it through Create or Update.
type GeneratedTemplate struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// generatorSystemInstruction demands exactly one JSON object. Anything else is
// refused rather than half-parsed.
const generatorSystemInstruction = `Du bist ein E-Mail-Vorlagen-Generator für ein deutsches Vertriebs-CRM.
Antworte mit EXAKT einem JSON-Objekt und keinem anderen Text, ohne Markdown-Codeblock:
{"name":"<Anzeigename>","subject":"<Betreff mit {{contact_name}} Platzhaltern>","body":"<Mehrzeiliger Text mit {{contact_name}}, {{company_name}}, {{deal_amount}}, {{user_name}} Platzhaltern>"}
Jedes Feld muss ein nicht-leerer String sein. Keine zusätzlichen Felder.`

// Generate asks the configured provider for a template draft.
//
// There is no fallback content. A provider error, an empty answer or a body that
// is not a single JSON object with the three required fields all return
// ErrGenerationFailed, and the caller must show that to the user.
func (s *Service) Generate(ctx context.Context, prompt string) (GeneratedTemplate, error) {
	if err := s.ready(); err != nil {
		return GeneratedTemplate{}, err
	}
	if s.generator == nil {
		return GeneratedTemplate{}, fmt.Errorf("%w: no AI provider is configured", ErrGenerationFailed)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return GeneratedTemplate{}, ErrInvalidInput
	}
	if len(prompt) > 2000 {
		return GeneratedTemplate{}, ErrInvalidInput
	}

	raw, err := s.generator.Generate(ctx, prompt, generatorSystemInstruction)
	if err != nil {
		return GeneratedTemplate{}, errors.Join(ErrGenerationFailed, err)
	}
	return parseGenerated(raw)
}

// parseGenerated accepts only one JSON object carrying name, subject and body.
// Markdown fences are tolerated because models emit them, but a fenced
// non-object or a free-form answer is refused.
func parseGenerated(raw string) (GeneratedTemplate, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return GeneratedTemplate{}, fmt.Errorf("%w: the provider returned an empty answer", ErrGenerationFailed)
	}
	trimmed = stripCodeFence(trimmed)

	var probe any
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return GeneratedTemplate{}, fmt.Errorf("%w: the provider did not answer with JSON", ErrGenerationFailed)
	}
	if _, ok := probe.(map[string]any); !ok {
		return GeneratedTemplate{}, fmt.Errorf("%w: the provider answered with JSON that is not an object", ErrGenerationFailed)
	}

	var out GeneratedTemplate
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return GeneratedTemplate{}, fmt.Errorf("%w: the provider JSON does not match name/subject/body", ErrGenerationFailed)
	}
	// The model's whitespace is not content. Validate the normalized form, so a
	// body of "   " is refused instead of being saved as a blank template.
	if err := (Input{Name: out.Name, Subject: out.Subject, Body: out.Body}).normalize().validate(false); err != nil {
		return GeneratedTemplate{}, fmt.Errorf("%w: %s", ErrGenerationFailed, err.Error())
	}
	out.Name = strings.TrimSpace(out.Name)
	out.Subject = strings.TrimSpace(out.Subject)
	out.Body = strings.TrimSpace(out.Body)
	return out, nil
}

func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence (and an optional language tag), then the closing one.
	rest := s[3:]
	if idx := strings.IndexByte(rest, '\n'); idx >= 0 {
		if line := strings.TrimSpace(rest[:idx]); line == "" || !strings.ContainsAny(line, " \t") {
			rest = rest[idx+1:]
		}
	}
	if idx := strings.LastIndex(rest, "```"); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.TrimSpace(rest)
}

// Stamp renders the created/updated timestamps of a template for a JSON view.
func Stamp(t db.EmailTemplate) (string, string) {
	return formatTime(t.CreatedAt.Time), formatTime(t.UpdatedAt.Time)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z07:00")
}
