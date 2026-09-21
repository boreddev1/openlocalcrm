package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/ai"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

func embeddingVecLiteral(vals ...float64) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func unitVector(dim int) []float32 {
	vec := make([]float32, dim)
	vec[0] = 1
	return vec
}

func paddedVec(dim int, vals ...float64) string {
	all := make([]float64, dim)
	copy(all, vals)
	return embeddingVecLiteral(all...)
}

func newEmbeddingGateway(t *testing.T, vec []float32) *ai.Gateway {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": vec})
	}))
	t.Cleanup(srv.Close)
	return ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		EmbeddingModel:  "qwen3-embedding:0.6b",
	})
}

func newFailingEmbeddingGateway(t *testing.T) *ai.Gateway {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failure", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return ai.NewGateway(ai.GatewayConfig{
		DefaultProvider: ai.ProviderOllama,
		OllamaBaseURL:   srv.URL,
		EmbeddingModel:  "qwen3-embedding:0.6b",
	})
}

func setupKBHandler(t *testing.T, gw *ai.Gateway) (*handlers.AIHandler, *demo.InMemoryQuerier) {
	t.Helper()
	obs := ai.NewObservabilityService()
	triageSvc := ai.NewTriageService(gw, obs)
	querier := demo.NewEmptyInMemoryQuerier()
	researchSvc := ai.NewResearchService(gw, obs)
	chatSvc := ai.NewChatService(gw, obs, researchSvc, querier)
	return handlers.NewAIHandler(triageSvc, chatSvc, researchSvc, obs, gw, querier), querier
}

func TestKBHandler_CreateKBEmbedsAndReportsRealIndex(t *testing.T) {
	h, _ := setupKBHandler(t, newEmbeddingGateway(t, unitVector(1024)))
	doc := handlers.KBArticle{Title: "PV Handbuch", Category: "Solar", Content: "Photovoltaik Montage"}
	body, _ := json.Marshal(doc)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/kb", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateKB(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created handlers.KBArticle
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&created))
	require.True(t, created.Indexed, "expected honestly indexed article")
	require.Equal(t, "qwen3-embedding:0.6b", created.EmbeddingModel)

	listReq := httptest.NewRequest(http.MethodGet, "/api/ai/kb", nil)
	listRec := httptest.NewRecorder()
	h.ListKB(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)
	require.Contains(t, listRec.Body.String(), "qwen3-embedding:0.6b")
}

func TestKBHandler_CreateKBFailsHonestlyWhenEmbeddingUnavailable(t *testing.T) {
	h, querier := setupKBHandler(t, newFailingEmbeddingGateway(t))
	before, err := querier.ListKBArticles(context.Background())
	require.NoError(t, err)

	doc := handlers.KBArticle{Title: "PV Handbuch", Category: "Solar", Content: "Photovoltaik Montage"}
	body, _ := json.Marshal(doc)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/kb", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateKB(rec, req)
	require.Equal(t, http.StatusBadGateway, rec.Code)

	after, err := querier.ListKBArticles(context.Background())
	require.NoError(t, err)
	require.Equal(t, len(before), len(after), "half-created article must not be persisted")
}

func TestKBHandler_ListKBReturnsRealIndexStatusAndNoFabricatedChunks(t *testing.T) {
	gw := newEmbeddingGateway(t, unitVector(1024))
	h, querier := setupKBHandler(t, gw)
	_, err := querier.CreateKBArticle(context.Background(), db.CreateKBArticleParams{
		Title: "Indexiert", Category: "Solar", Content: "Text", Tags: []string{}, Author: "System",
		Embedding:      embeddingVecLiteral(1, 0, 0),
		EmbeddingModel: pgtype.Text{String: "qwen3-embedding:0.6b", Valid: true},
	})
	require.NoError(t, err)
	_, err = querier.CreateKBArticle(context.Background(), db.CreateKBArticleParams{
		Title: "Entwurf", Category: "Entwurf", Content: "Text", Tags: []string{}, Author: "System",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/ai/kb", nil)
	rec := httptest.NewRecorder()
	h.ListKB(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	require.NotContains(t, body, "chunks_count")
	require.NotContains(t, body, `"chunks_count":4`)
	require.Contains(t, body, `"indexed":true`)
	require.Contains(t, body, `"indexed":false`)
	require.Contains(t, body, `"embedding_model":"qwen3-embedding:0.6b"`)
}

func TestKBHandler_SearchKBEmptyQueryIs400(t *testing.T) {
	h, _ := setupKBHandler(t, newEmbeddingGateway(t, unitVector(1024)))

	for _, target := range []string{"/api/ai/kb/search", "/api/ai/kb/search?q=", "/api/ai/kb/search?q=%20%20"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		h.SearchKB(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code, "target %q", target)
	}
}

func TestKBHandler_SearchKBRanksIndexedArticles(t *testing.T) {
	h, querier := setupKBHandler(t, newEmbeddingGateway(t, unitVector(1024)))
	ctx := context.Background()
	_, err := querier.CreateKBArticle(ctx, db.CreateKBArticleParams{
		Title: "Nah", Category: "Solar", Content: "pv", Tags: []string{}, Author: "System",
		Embedding:      paddedVec(1024, 1),
		EmbeddingModel: pgtype.Text{String: "qwen3-embedding:0.6b", Valid: true},
	})
	require.NoError(t, err)
	_, err = querier.CreateKBArticle(ctx, db.CreateKBArticleParams{
		Title: "Fern", Category: "Solar", Content: "eeg", Tags: []string{}, Author: "System",
		Embedding:      paddedVec(1024, 0, 1),
		EmbeddingModel: pgtype.Text{String: "qwen3-embedding:0.6b", Valid: true},
	})
	require.NoError(t, err)
	// not indexed, must be excluded
	_, err = querier.CreateKBArticle(ctx, db.CreateKBArticleParams{
		Title: "Ohne Vektor", Category: "Entwurf", Content: "x", Tags: []string{}, Author: "System",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/ai/kb/search?q=photovoltaik&limit=5", nil)
	rec := httptest.NewRecorder()
	h.SearchKB(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var results []handlers.KBSearchResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&results))
	require.Len(t, results, 2)
	require.Equal(t, "Nah", results[0].Title)
	require.Less(t, results[0].Distance, results[1].Distance)
}

func TestKBHandler_SearchKBFailsHonestlyWhenEmbeddingUnavailable(t *testing.T) {
	h, _ := setupKBHandler(t, newFailingEmbeddingGateway(t))
	req := httptest.NewRequest(http.MethodGet, "/api/ai/kb/search?q=photovoltaik", nil)
	rec := httptest.NewRecorder()

	h.SearchKB(rec, req)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}
