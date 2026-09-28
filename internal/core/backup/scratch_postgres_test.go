package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/stretchr/testify/require"
)

// drillFixture holds the values seeded into `public` so the test can compare
// them against what the drill actually imported into the scratch schema.
type drillFixture struct {
	userID            uuid.UUID
	companyID         uuid.UUID
	contactID         uuid.UUID
	dealID            uuid.UUID
	researchID        uuid.UUID
	articleID         uuid.UUID
	tags              []string
	industryKeywords  []string
	embeddingLiteral  string
	embeddingElements int
}

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; run ./scripts/test-backup-integration.sh")
	}
	return url
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDatabaseURL(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(context.Background()))
	return pool
}

func vectorLiteral(dimensions int) string {
	parts := make([]string, 0, dimensions)
	for i := 0; i < dimensions; i++ {
		parts = append(parts, fmt.Sprintf("%d.5", i%10))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func seedFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) drillFixture {
	t.Helper()
	f := drillFixture{
		userID:            uuid.New(),
		companyID:         uuid.New(),
		contactID:         uuid.New(),
		dealID:            uuid.New(),
		researchID:        uuid.New(),
		articleID:         uuid.New(),
		tags:              []string{"pv", "speicher", "dachanlage", "wallbox"},
		industryKeywords:  []string{"erneuerbare-energie", "photovoltaik", "solartechnik"},
		embeddingLiteral:  vectorLiteral(1024),
		embeddingElements: 1024,
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, first_name, last_name, role, status)
		VALUES ($1, $2, 'hash', 'Sabine', 'Mustermann', 'ADMIN', 'ACTIVE')`,
		f.userID, f.userID.String()+"@backup-drill.example")
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO companies (id, name, domain, custom_fields)
		VALUES ($1, 'Backup Drill GmbH', 'backup-drill.example', '{"probe": true}')`, f.companyID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO contacts (id, company_id, first_name, last_name, custom_fields, tags)
		VALUES ($1, $2, 'Ralf', 'Rundfunk', '{"consent": true}', '["lead","pv"]')`,
		f.contactID, f.companyID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO deals (id, title, company_id, contact_id, value, assigned_to, tags)
		VALUES ($1, 'PV-Anlage 12 kWp', $2, $3, 18500.00, $4, '["photovoltaik"]')`,
		f.dealID, f.companyID, f.contactID, f.userID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO company_research (id, company_id, domain, title, industry_keywords)
		VALUES ($1, $2, 'backup-drill.example', 'Backup Drill GmbH', $3)`,
		f.researchID, f.companyID, f.industryKeywords)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO knowledge_base_articles (id, title, category, content, tags, embedding, embedding_model)
		VALUES ($1, 'Drill-Artikel', 'drill', 'Roundtrip', $2, $3::public.vector(1024), 'qwen3-embedding:0.6b')`,
		f.articleID, f.tags, f.embeddingLiteral)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM knowledge_base_articles WHERE id = $1`, f.articleID)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM company_research WHERE id = $1`, f.researchID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM deals WHERE id = $1`, f.dealID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM contacts WHERE id = $1`, f.contactID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM companies WHERE id = $1`, f.companyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, f.userID)
	})

	return f
}

func publicCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64, len(tableSpecs))
	for _, spec := range tableSpecs {
		var count int64
		require.NoError(t, pool.QueryRow(ctx,
			fmt.Sprintf("SELECT count(*) FROM public.%s", pgx.Identifier{spec.Name}.Sanitize())).Scan(&count))
		counts[spec.Name] = count
	}
	return counts
}

func publicRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) [][]byte {
	t.Helper()
	rows, err := db.ReadBackupTable(ctx, db.New(pool), table)
	require.NoError(t, err)
	require.NotEmpty(t, rows, "the fixture must produce a non-empty public table for %s", table)
	return rows
}

func scratchSchemaExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema string) bool {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_namespace WHERE nspname = $1`, schema).Scan(&count))
	return count > 0
}

// summarizeVector shortens a vector literal for the test log while keeping the
// element count visible.
func summarizeVector(literal string) string {
	parts := strings.Split(strings.Trim(literal, "[]"), ",")
	if len(parts) <= 6 {
		return literal
	}
	return fmt.Sprintf("[%s,...,%s] (%d dimensions)",
		strings.Join(parts[:3], ","), strings.Join(parts[len(parts)-3:], ","), len(parts))
}

func newDrillServiceForPool(pool *pgxpool.Pool, audit AuditLogger) *Service {
	queries := db.New(pool)
	return NewService(queries, audit, pool, queries)
}

// TestScratchDrillRoundTripsTypedRowsWithoutFalseZeroPass exercises the real
// pgvector round trip: seeded public rows must be imported into the scratch
// schema with byte-identical JSON, which is what proves vector(1024), TEXT[],
// enums and ordinary foreign keys all survive.
func TestScratchDrillRoundTripsTypedRowsWithoutFalseZeroPass(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	fixture := seedFixture(t, ctx, pool)
	audit := &recordingAudit{}
	svc := newDrillServiceForPool(pool, audit)

	var observedSchema string
	var publicRefsInScratch int
	var scratchRefs int
	svc.drillHook = func(ctx context.Context, phase drillPhase, schema string, tx pgx.Tx) error {
		observedSchema = schema
		if phase != drillPhaseAfterImport {
			return nil
		}

		// No scratch constraint may still reference public.
		require.NoError(t, tx.QueryRow(ctx, `
			SELECT count(*)
			FROM pg_constraint c
			JOIN pg_class ch ON ch.oid = c.conrelid
			JOIN pg_namespace nch ON nch.oid = ch.relnamespace
			JOIN pg_class pa ON pa.oid = c.confrelid
			JOIN pg_namespace npa ON npa.oid = pa.relnamespace
			WHERE c.contype = 'f' AND nch.nspname = $1 AND npa.nspname = 'public'`,
			schema).Scan(&publicRefsInScratch))
		require.NoError(t, tx.QueryRow(ctx, `
			SELECT count(*)
			FROM pg_constraint c
			JOIN pg_class ch ON ch.oid = c.conrelid
			JOIN pg_namespace nch ON nch.oid = ch.relnamespace
			JOIN pg_class pa ON pa.oid = c.confrelid
			JOIN pg_namespace npa ON npa.oid = pa.relnamespace
			WHERE c.contype = 'f' AND nch.nspname = $1 AND npa.nspname = $1`,
			schema).Scan(&scratchRefs))

		// Direct, value-level assertions on what actually landed in scratch.
		var tags []string
		require.NoError(t, tx.QueryRow(ctx,
			`SELECT tags FROM knowledge_base_articles WHERE id = $1`, fixture.articleID).Scan(&tags))
		require.Equal(t, fixture.tags, tags, "TEXT[] tags must round-trip exactly")

		var keywords []string
		require.NoError(t, tx.QueryRow(ctx,
			`SELECT industry_keywords FROM company_research WHERE id = $1`, fixture.researchID).Scan(&keywords))
		require.Equal(t, fixture.industryKeywords, keywords, "TEXT[] industry_keywords must round-trip exactly")

		var embedding string
		require.NoError(t, tx.QueryRow(ctx,
			`SELECT embedding::text FROM knowledge_base_articles WHERE id = $1`, fixture.articleID).Scan(&embedding))
		require.Equal(t, fixture.embeddingLiteral, embedding,
			"the 1024-dimensional vector must round-trip byte for byte")
		require.Len(t, strings.Split(strings.Trim(embedding, "[]"), ","), fixture.embeddingElements)
		t.Logf("scratch knowledge_base_articles.tags          = %v", tags)
		t.Logf("scratch company_research.industry_keywords   = %v", keywords)
		t.Logf("scratch knowledge_base_articles.embedding     = %s", summarizeVector(embedding))

		// A single scratch row really exists, so this is not a 0 = 0 pass.
		var scratchContacts int64
		require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM contacts`).Scan(&scratchContacts))
		require.Positive(t, scratchContacts)
		return nil
	}

	result, err := svc.Drill(ctx, RequestMeta{})
	require.NoError(t, err)
	require.True(t, result.Passed)
	require.NotEmpty(t, observedSchema)

	// Non-zero counts for every seeded table, equal on both sides.
	byName := make(map[string]TableDrillResult, len(result.Tables))
	for _, entry := range result.Tables {
		byName[entry.Name] = entry
	}
	for _, name := range []string{
		"users", "companies", "contacts", "deals", "company_research", "knowledge_base_articles",
	} {
		entry := byName[name]
		require.Greater(t, entry.SourceRows, int64(0), "table %s must be seeded non-empty", name)
		require.Equal(t, entry.SourceRows, entry.TargetRows, "table %s", name)
		require.True(t, entry.Verified, "table %s", name)
		require.Equal(t, entry.SourceDigest, entry.TargetDigest,
			"table %s: scratch rows must be byte-identical to the public rows", name)
	}
	require.Len(t, result.Tables, 24)

	// The drill's own digest must equal the digest the test computes from
	// public, which ties the equality claim to the seeded values.
	for _, name := range []string{"knowledge_base_articles", "company_research", "contacts", "users"} {
		require.Equal(t, RowDigest(publicRows(t, ctx, pool, name)), byName[name].SourceDigest, name)
		require.Equal(t, byName[name].SourceDigest, byName[name].TargetDigest, name)
	}

	for _, name := range []string{"users", "companies", "contacts", "deals", "company_research", "knowledge_base_articles"} {
		entry := byName[name]
		t.Logf("drill %-24s source_rows=%d target_rows=%d verified=%t digest=%s..",
			name, entry.SourceRows, entry.TargetRows, entry.Verified, entry.SourceDigest[:16])
	}

	require.Zero(t, publicRefsInScratch, "no scratch foreign key may reference public")
	require.Positive(t, scratchRefs, "the scratch schema must keep scratch-scoped foreign keys")
	require.Equal(t, result.ForeignKeys.ScratchAdded, scratchRefs,
		"the scratch schema must carry one rewritten constraint per live foreign key")
	require.Equal(t, 0, result.ForeignKeys.CopiedDropped)

	require.Equal(t, []string{"BACKUP_RESTORE_DRILL_STARTED", "BACKUP_RESTORE_DRILL_COMPLETED"}, audit.actions())
	require.False(t, scratchSchemaExists(t, ctx, pool, observedSchema), "the scratch schema must be dropped")
}

// TestScratchDrillRefusesToFallBackToPublicWhenAScratchTableIsMissing is the
// structural guarantee: a scratch table that disappears after the 24-table
// validation must abort the drill with "relation does not exist" instead of
// writing the rows into the live public table, and every public count must be
// left exactly as it was.
func TestScratchDrillRefusesToFallBackToPublicWhenAScratchTableIsMissing(t *testing.T) {
	// intake_forms carries no foreign key, so the only thing its removal can break is
	// the write itself. That isolates the search_path question: the import has
	// to fail with "relation does not exist" rather than quietly create or fill
	// the live public table. todos additionally loses three foreign keys with
	// it, so it is also the case that proves the constraint check is real.
	for _, table := range []string{"intake_forms", "todos"} {
		t.Run(table, func(t *testing.T) {
			ctx := context.Background()
			pool := testPool(t)
			seedFixture(t, ctx, pool)
			before := publicCounts(t, ctx, pool)

			svc := newDrillServiceForPool(pool, &recordingAudit{})
			var observedSchema string
			svc.drillHook = func(ctx context.Context, phase drillPhase, schema string, tx pgx.Tx) error {
				observedSchema = schema
				if phase != drillPhaseAfterValidation {
					return nil
				}
				// Delete the scratch table after validation, which is the only
				// point at which a fallback to public could still happen.
				_, err := tx.Exec(ctx, fmt.Sprintf("DROP TABLE %s",
					pgx.Identifier{schema, table}.Sanitize()))
				return err
			}

			result, err := svc.Drill(ctx, RequestMeta{})
			require.Error(t, err, "a missing scratch table must fail the drill")
			require.False(t, result.Passed)
			require.NotEmpty(t, observedSchema)
			require.NotContains(t, err.Error(), "passed")
			if table == "intake_forms" {
				require.Contains(t, err.Error(), table)
				require.Contains(t, err.Error(), "does not exist",
					"with search_path pinned to the scratch schema the write must fail, not fall back to public")
			} else {
				// Dropping todos also drops its three foreign keys, so the
				// structural check is the first honest failure to surface.
				require.Contains(t, err.Error(), "live schema declares")
			}

			after := publicCounts(t, ctx, pool)
			require.Equal(t, before, after, "the failed drill must not change any public row count")
			require.False(t, scratchSchemaExists(t, ctx, pool, observedSchema))
		})
	}
}

// TestScratchDrillRowToJsonIsNotBase64 pins the wire-format requirement: JSONB
// and array columns must appear as JSON values in the export, never base64.
func TestScratchDrillRowToJsonIsNotBase64(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	seedFixture(t, ctx, pool)

	rows := publicRows(t, ctx, pool, "contacts")
	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rows[0], &decoded))
	require.JSONEq(t, `["lead","pv"]`, string(decoded["tags"]))
	require.JSONEq(t, `{"consent": true}`, string(decoded["custom_fields"]))
	require.NotContains(t, string(rows[0]), "custom_fields\":\"")
}
