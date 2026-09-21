package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

func setupAIHandler() *handlers.AIHandler {
	gw := ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaModel:     "gemma2:12b",
	})
	obs := ai.NewObservabilityService()
	triageSvc := ai.NewTriageService(gw)
	querier := demo.NewInMemoryQuerier()
	chatSvc := ai.NewChatService(gw, obs, querier)
	researchSvc := ai.NewResearchService(gw, obs)

	return handlers.NewAIHandler(triageSvc, chatSvc, researchSvc, obs, gw, querier)
}

func TestAIHandler_TriageAndChat(t *testing.T) {
	h := setupAIHandler()

	t.Run("TriageEmail valid", func(t *testing.T) {
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
	})

	t.Run("TriageEmail invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/ai/triage", bytes.NewReader([]byte("{bad-json")))
		rec := httptest.NewRecorder()

		h.TriageEmail(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Chat greeting", func(t *testing.T) {
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
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		require.Contains(t, resp.Reply, "Vertriebs-Copilot")
		require.NotContains(t, resp.Reply, "106.700 €")
	})

	t.Run("Chat unknown input dd", func(t *testing.T) {
		reqBody := ai.ChatRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: "dd"},
			},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.Chat(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp ai.ChatResponse
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		require.Contains(t, resp.Reply, "nicht genau verstanden")
		require.NotContains(t, resp.Reply, "106.700 €")
	})

	t.Run("Chat pipeline summary with action card", func(t *testing.T) {
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
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Reply)
		require.NotNil(t, resp.ActionCard)
		require.Equal(t, "/deals", resp.ActionCard.Route)
	})

	t.Run("Chat invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader([]byte("{bad-json")))
		rec := httptest.NewRecorder()

		h.Chat(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestAIHandler_ObservabilityAndBill(t *testing.T) {
	h := setupAIHandler()

	t.Run("GetObservability", func(t *testing.T) {
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

	t.Run("ParseBill with customer name", func(t *testing.T) {
		body := []byte(`{"customer_name":"Familie Schmidt"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/ai/parse-bill", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		h.ParseBill(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "Familie Schmidt")
	})

	t.Run("ParseBill default fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/ai/parse-bill", bytes.NewReader([]byte("{}")))
		rec := httptest.NewRecorder()

		h.ParseBill(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "Familie Müller")
	})
}

func TestAIHandler_KnowledgeBaseAndResearch(t *testing.T) {
	h := setupAIHandler()

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

		createBody := []byte(`{"domain":"solar-mueller.de","depth":"DEEP","category":"Photovoltaik"}`)
		createReq := httptest.NewRequest(http.MethodPost, "/api/ai/research", bytes.NewReader(createBody))
		createRec := httptest.NewRecorder()

		h.CreateResearchJob(createRec, createReq)
		require.Equal(t, http.StatusCreated, createRec.Code)
	})
}
