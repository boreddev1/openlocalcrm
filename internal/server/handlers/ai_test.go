package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

func newFakeAIGateway(t *testing.T, response string) *ai.Gateway {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/api/embeddings") {
			_ = json.NewEncoder(w).Encode(map[string]any{"embedding": unitVector(768)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"response": response})
	}))
	t.Cleanup(srv.Close)
	return ai.NewGateway(ai.GatewayConfig{
		DefaultProvider:      ai.ProviderOllama,
		OllamaBaseURL:        srv.URL,
		OllamaModel:          "test-model",
		OllamaEmbeddingModel: "nomic-embed-text",
	})
}

func newFailingAIGateway(t *testing.T, status int) *ai.Gateway {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failure", status)
	}))
	t.Cleanup(srv.Close)
	return ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})
}

func newNonJSONAIGateway(t *testing.T) *ai.Gateway {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("this is not json"))
	}))
	t.Cleanup(srv.Close)
	return ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		OllamaModel:     "test-model",
	})
}

func setupAIHandlerWithGateway(t *testing.T, gw *ai.Gateway) *handlers.AIHandler {
	t.Helper()
	obs := ai.NewObservabilityService()
	triageSvc := ai.NewTriageService(gw, obs)
	querier := demo.NewInMemoryQuerier()
	researchSvc := ai.NewResearchService(gw, obs)
	chatSvc := ai.NewChatService(gw, obs, researchSvc, querier)
	return handlers.NewAIHandler(triageSvc, chatSvc, researchSvc, obs, gw, querier)
}

const validTriageJSON = `{"category":"ANFRAGE","sentiment":"POSITIVE","priority":"HIGH","summary":"Anfrage erfasst.","draft_reply":"Vielen Dank für Ihre Anfrage."}`

func TestAIHandler_TriageAndChat(t *testing.T) {
	t.Run("TriageEmail valid", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, validTriageJSON))
		reqBody := handlers.TriageRequest{
			Sender:  "kunde@solar.de",
			Subject: "Photovoltaik 15kWp",
			Body:    "Wir planen eine Aufdachanlage.",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/triage", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.TriageEmail(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var res ai.TriageResult
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&res))
		require.Equal(t, "ANFRAGE", res.Category)
		require.Equal(t, "Anfrage erfasst.", res.Summary)
	})

	t.Run("TriageEmail invalid json", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, validTriageJSON))
		req := httptest.NewRequest(http.MethodPost, "/api/ai/triage", bytes.NewReader([]byte("{bad-json")))
		rec := httptest.NewRecorder()

		h.TriageEmail(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("TriageEmail upstream failure is 502", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFailingAIGateway(t, http.StatusInternalServerError))
		reqBody := handlers.TriageRequest{Sender: "kunde@solar.de", Subject: "Angebot", Body: "Bitte Angebot."}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/triage", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.TriageEmail(rec, req)
		require.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("TriageEmail non-JSON upstream is 502", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newNonJSONAIGateway(t))
		reqBody := handlers.TriageRequest{Sender: "kunde@solar.de", Subject: "Angebot", Body: "Bitte Angebot."}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/triage", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.TriageEmail(rec, req)
		require.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("Chat returns real model reply", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, "Antwort vom echten Modell"))
		reqBody := ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "hi"},
			},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.Chat(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp ai.ChatResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, "Antwort vom echten Modell", resp.Reply)
		require.False(t, resp.Simulated)
		require.NotContains(t, resp.Reply, "106.700 €")
	})

	t.Run("Chat pipeline summary with action card", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, "Hier ist Ihre Pipeline."))
		reqBody := ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "Fasse die Pipeline zusammen"},
			},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.Chat(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp ai.ChatResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.NotEmpty(t, resp.Reply)
		require.NotNil(t, resp.ActionCard)
		require.Equal(t, "/deals", resp.ActionCard.Route)
	})

	t.Run("Chat upstream failure is 502", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFailingAIGateway(t, http.StatusInternalServerError))
		reqBody := ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "Fasse die Pipeline zusammen"},
			},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.Chat(rec, req)
		require.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("Chat invalid json", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, "Antwort"))
		req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader([]byte("{bad-json")))
		rec := httptest.NewRecorder()

		h.Chat(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestAIHandler_ObservabilityAndBill(t *testing.T) {
	t.Run("GetObservability", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, "Antwort"))
		req := httptest.NewRequest(http.MethodGet, "/api/ai/observability", nil)
		rec := httptest.NewRecorder()

		h.GetObservability(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var res map[string]any
		err := json.NewDecoder(rec.Body).Decode(&res)
		require.NoError(t, err)
		require.Contains(t, res, "stats")
		require.Contains(t, res, "recent_logs")
	})

	t.Run("ParseBill requires document_text", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, "{}"))
		body := []byte(`{"customer_name":"Familie Schmidt"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/parse-bill", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.ParseBill(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.NotContains(t, rec.Body.String(), "Familie Müller")
	})

	t.Run("ParseBill empty body is 400", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, "{}"))
		req := httptest.NewRequest(http.MethodPost, "/api/ai/parse-bill", bytes.NewReader([]byte("{}")))
		rec := httptest.NewRecorder()

		h.ParseBill(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ParseBill extracts real data via gateway", func(t *testing.T) {
		extracted := `{"customer_name":"Familie Schmidt","yearly_consumption":5000,"recommended_kwp":12.5}`
		h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, extracted))
		body := []byte(`{"document_text":"Stromrechnung 5000 kWh, Familie Schmidt"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/parse-bill", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.ParseBill(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var res map[string]any
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&res))
		require.Equal(t, false, res["simulated"])
		require.Equal(t, "ai_ocr_extraction", res["mode"])
		extractedMap, ok := res["extracted"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "Familie Schmidt", extractedMap["customer_name"])
	})

	t.Run("ParseBill upstream failure is 502", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFailingAIGateway(t, http.StatusInternalServerError))
		body := []byte(`{"document_text":"Stromrechnung 5000 kWh"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/parse-bill", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.ParseBill(rec, req)
		require.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func TestAIHandler_KnowledgeBaseAndResearch(t *testing.T) {
	h := setupAIHandlerWithGateway(t, newFakeAIGateway(t, `{"summary":"Recherche","industry_keywords":["Handwerk"]}`))

	t.Run("ListKB", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/ai/kb", nil)
		rec := httptest.NewRecorder()

		h.ListKB(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("CreateKB and DeleteKB", func(t *testing.T) {
		doc := handlers.KBArticle{
			Title:    "Sicherheitsrichtlinie 2026",
			Category: "Compliance",
			Content:  "Zwei-Faktor-Authentifizierung für alle Benutzer obligatorisch.",
		}
		body, _ := json.Marshal(doc)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/kb", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.CreateKB(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var created handlers.KBArticle
		err := json.NewDecoder(rec.Body).Decode(&created)
		require.NoError(t, err)
		require.NotEmpty(t, created.ID)

		// Delete
		delReq := httptest.NewRequest(http.MethodDelete, "/api/ai/kb/"+created.ID, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", created.ID)
		delReq = delReq.WithContext(context.WithValue(delReq.Context(), chi.RouteCtxKey, rctx))
		delRec := httptest.NewRecorder()

		h.DeleteKB(delRec, delReq)
		require.Equal(t, http.StatusOK, delRec.Code)
	})

	t.Run("ListResearchJobs and CreateResearchJob", func(t *testing.T) {
		listReq := httptest.NewRequest(http.MethodGet, "/api/ai/research", nil)
		listRec := httptest.NewRecorder()

		h.ListResearchJobs(listRec, listReq)
		require.Equal(t, http.StatusOK, listRec.Code)

		createBody := []byte(`{"domain":"example.invalid","depth":"DEEP","category":"Photovoltaik"}`)
		createReq := httptest.NewRequest(http.MethodPost, "/api/ai/research", bytes.NewReader(createBody))
		createRec := httptest.NewRecorder()

		h.CreateResearchJob(createRec, createReq)
		require.Equal(t, http.StatusCreated, createRec.Code)
	})

	t.Run("ResearchCompany upstream failure is 502", func(t *testing.T) {
		h := setupAIHandlerWithGateway(t, newFailingAIGateway(t, http.StatusInternalServerError))
		body := []byte(`{"domain":"example.invalid"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/research-company", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.ResearchCompany(rec, req)
		require.Equal(t, http.StatusBadGateway, rec.Code)
	})
}
