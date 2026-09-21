package demo_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
)

func vecLiteral(vals ...float64) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestDemoKBEmbeddingCreateListAndSearch(t *testing.T) {
	ctx := context.Background()
	q := demo.NewEmptyInMemoryQuerier()

	model := pgtype.Text{String: "qwen3-embedding:0.6b", Valid: true}
	a1, err := q.CreateKBArticle(ctx, db.CreateKBArticleParams{
		Title:          "PV Handbuch",
		Category:       "Solar",
		Content:        "Photovoltaik Montage",
		Tags:           []string{"PV"},
		Author:         "System",
		Embedding:      vecLiteral(1, 0, 0),
		EmbeddingModel: model,
	})
	if err != nil {
		t.Fatalf("create kb article failed: %v", err)
	}
	if !a1.Indexed {
		t.Fatal("expected created article to be indexed when an embedding is stored")
	}
	if a1.EmbeddingModel.String != "qwen3-embedding:0.6b" {
		t.Fatalf("expected real embedding model, got %q", a1.EmbeddingModel.String)
	}

	a2, err := q.CreateKBArticle(ctx, db.CreateKBArticleParams{
		Title:          "EEG Gesetz",
		Category:       "Recht",
		Content:        "Einspeisevergütung",
		Tags:           []string{"EEG"},
		Author:         "System",
		Embedding:      vecLiteral(0, 1, 0),
		EmbeddingModel: model,
	})
	if err != nil {
		t.Fatalf("create kb article failed: %v", err)
	}

	a3, err := q.CreateKBArticle(ctx, db.CreateKBArticleParams{
		Title:    "Unindexierter Entwurf",
		Category: "Entwurf",
		Content:  "Noch kein Vektor",
		Tags:     []string{},
		Author:   "System",
	})
	if err != nil {
		t.Fatalf("create kb article failed: %v", err)
	}
	if a3.Indexed {
		t.Fatal("expected article without embedding to be reported as not indexed")
	}

	list, err := q.ListKBArticles(ctx)
	if err != nil {
		t.Fatalf("list kb articles failed: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 articles, got %d", len(list))
	}
	indexedCount := 0
	for _, a := range list {
		if a.Indexed {
			indexedCount++
		}
	}
	if indexedCount != 2 {
		t.Fatalf("expected 2 indexed articles, got %d", indexedCount)
	}

	res, err := q.SearchKBArticlesByEmbedding(ctx, db.SearchKBArticlesByEmbeddingParams{
		Embedding:  vecLiteral(1, 0, 0),
		LimitCount: 10,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 search results (only indexed articles), got %d", len(res))
	}
	if res[0].ID != a1.ID || res[1].ID != a2.ID {
		t.Fatalf("expected a1 ranked before a2, got order %v then %v", res[0].ID, res[1].ID)
	}
	if res[0].Distance > res[1].Distance {
		t.Fatalf("expected ascending cosine distance, got %v then %v", res[0].Distance, res[1].Distance)
	}
}

func TestDemoKBUpdateEmbedding(t *testing.T) {
	ctx := context.Background()
	q := demo.NewEmptyInMemoryQuerier()

	a, err := q.CreateKBArticle(ctx, db.CreateKBArticleParams{
		Title: "Entwurf", Category: "Entwurf", Content: "text", Tags: []string{}, Author: "System",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if a.Indexed {
		t.Fatal("expected not indexed before embedding update")
	}

	if err := q.UpdateKBArticleEmbedding(ctx, db.UpdateKBArticleEmbeddingParams{
		ID:             a.ID,
		Embedding:      vecLiteral(0, 0, 1),
		EmbeddingModel: pgtype.Text{String: "qwen3-embedding:0.6b", Valid: true},
	}); err != nil {
		t.Fatalf("update embedding failed: %v", err)
	}

	got, err := q.GetKBArticleByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !got.Indexed {
		t.Fatal("expected article to be indexed after embedding update")
	}
	if got.EmbeddingModel.String != "qwen3-embedding:0.6b" {
		t.Fatalf("expected embedding model persisted, got %q", got.EmbeddingModel.String)
	}
}
