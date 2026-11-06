package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/core/emailtemplate"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
)

type handlerGenerator struct {
	answer string
	err    error
}

func (g handlerGenerator) Generate(context.Context, string, string) (string, error) {
	return g.answer, g.err
}

func newTemplateHandler(t *testing.T, generator emailtemplate.Generator) (*demo.InMemoryQuerier, *handlers.EmailTemplateHandler) {
	t.Helper()
	q := demo.NewInMemoryQuerier()
	return q, handlers.NewEmailTemplateHandler(emailtemplate.NewService(q, generator))
}

func createTemplate(t *testing.T, h *handlers.EmailTemplateHandler) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"key":"pv-first-offer","name":"PV Erstangebot","subject":"Ihr Angebot","body":"Hallo {{contact_name}}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/email-templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	return rec
}

func TestEmailTemplateHandlerCreateDefaultsTheLocale(t *testing.T) {
	_, h := newTemplateHandler(t, nil)

	rec := createTemplate(t, h)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["locale"] != "de-DE" {
		t.Fatalf("an omitted locale must be defaulted to de-DE, got %v", body["locale"])
	}
}

func TestEmailTemplateHandlerRejectsAMalformedTemplate(t *testing.T) {
	q, h := newTemplateHandler(t, nil)
	ctx := context.Background()

	for name, body := range map[string]string{
		"bad json": `{`,
		"no key":   `{"name":"A","subject":"B","body":"C"}`,
		"no name":  `{"key":"k","subject":"B","body":"C"}`,
		"no body":  `{"key":"k","name":"A","subject":"B"}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/email-templates", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.Create(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}

	list, _ := q.ListEmailTemplates(ctx)
	if len(list) != 0 {
		t.Fatalf("a refused create must not write anything, got %d rows", len(list))
	}
}

func TestEmailTemplateHandlerUpdateAndDeleteNeedAnExistingKey(t *testing.T) {
	_, h := newTemplateHandler(t, nil)

	withKey := func(method, key, body string) *httptest.ResponseRecorder {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("key", key)
		req := httptest.NewRequest(method, "/api/v1/email-templates/"+key, strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		if method == http.MethodPut {
			h.Update(rec, req)
		} else {
			h.Delete(rec, req)
		}
		return rec
	}

	if rec := withKey(http.MethodPut, "nope", `{"name":"A","subject":"B","body":"C"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("updating a missing key must be 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	// Delete is an :exec with no affected-row count, so the handler must not
	// report a deletion that did not happen.
	if rec := withKey(http.MethodDelete, "nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("deleting a missing key must be 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec := withKey(http.MethodDelete, "", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("an empty key must be 400, got %d", rec.Code)
	}
}

func TestEmailTemplateHandlerDoesNotClaimAConflictItNeverSaw(t *testing.T) {
	_, h := newTemplateHandler(t, nil)

	if rec := createTemplate(t, h); rec.Code != http.StatusCreated {
		t.Fatalf("first create must succeed, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	rec := createTemplate(t, h)
	if rec.Code == http.StatusConflict {
		t.Fatalf("the in-memory store raises no SQLSTATE 23505, so a 409 would be a claim it cannot back: %s", rec.Body.String())
	}
	if rec.Code < 400 {
		t.Fatalf("a duplicate key must fail, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("the real cause must stay visible, got %s", rec.Body.String())
	}
}

func TestEmailTemplateHandlerGenerateReturnsADraftAndNeverCannedText(t *testing.T) {
	t.Run("real provider answer", func(t *testing.T) {
		q, h := newTemplateHandler(t, handlerGenerator{answer: `{"name":"A","subject":"B","body":"C"}`})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/email-templates/generate",
			strings.NewReader(`{"prompt":"Nachfass-Mail"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Generate(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		generated, _ := body["generated"].(map[string]any)
		if generated == nil || generated["name"] != "A" {
			t.Fatalf("expected the model answer, got %v", body)
		}
		if body["persisted"] != false {
			t.Fatalf("generation must state that nothing was persisted, got %v", body["persisted"])
		}
		list, _ := q.ListEmailTemplates(context.Background())
		if len(list) != 0 {
			t.Fatalf("Generate must not write a row, got %d", len(list))
		}
	})

	t.Run("upstream failure is a 502", func(t *testing.T) {
		_, h := newTemplateHandler(t, handlerGenerator{err: errors.New("connection refused")})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/email-templates/generate",
			strings.NewReader(`{"prompt":"Nachfass-Mail"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Generate(rec, req)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("an unreachable provider must be a 502, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "{{contact_name}}") {
			t.Fatalf("a failed generation must not return a canned template: %s", rec.Body.String())
		}
	})

	t.Run("non-JSON answer is refused", func(t *testing.T) {
		_, h := newTemplateHandler(t, handlerGenerator{answer: "Hier ist Ihre Vorlage: ..."})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/email-templates/generate",
			strings.NewReader(`{"prompt":"Nachfass-Mail"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Generate(rec, req)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("a non-JSON answer must be a 502, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty prompt is a 400", func(t *testing.T) {
		gen := handlerGenerator{answer: `{"name":"A","subject":"B","body":"C"}`}
		_, h := newTemplateHandler(t, gen)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/email-templates/generate",
			strings.NewReader(`{"prompt":"  "}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Generate(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("an empty prompt must be a 400, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestEmailTemplateHandlerListIsAnEmptyArrayNotNull(t *testing.T) {
	_, h := newTemplateHandler(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/email-templates", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("an empty list must serialize as [], got %s", rec.Body.String())
	}
}

func TestEmailTemplateHandlerWithoutADatabaseIsHonest(t *testing.T) {
	h := handlers.NewEmailTemplateHandler(emailtemplate.NewService(nil, nil))

	for _, tc := range []struct {
		name string
		call func() *httptest.ResponseRecorder
		want int
	}{
		{"list", func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/email-templates", nil)
			rec := httptest.NewRecorder()
			h.List(rec, req)
			return rec
		}, http.StatusNotImplemented},
		{"create", func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/email-templates",
				strings.NewReader(`{"key":"k","name":"A","subject":"B","body":"C"}`))
			rec := httptest.NewRecorder()
			h.Create(rec, req)
			return rec
		}, http.StatusNotImplemented},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := tc.call()
			if rec.Code != tc.want {
				t.Fatalf("expected %d, got %d (body: %s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}
