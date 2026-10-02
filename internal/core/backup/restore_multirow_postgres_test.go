package backup

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// seedMultiRow inserts three or more rows into several tables, including audit
// rows with hash-chain columns, so a real export carries multi-row arrays.
func seedMultiRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (userEmail string, contactID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM deals WHERE title LIKE 'Deal %' AND value = 100.50`)
		_, _ = pool.Exec(bg, `DELETE FROM contacts WHERE tags = '["multirow"]'::jsonb`)
		_, _ = pool.Exec(bg, `DELETE FROM companies WHERE name LIKE 'Multirow % GmbH'`)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE email LIKE '%@multirow.example'`)
		_, _ = pool.Exec(bg, `DELETE FROM audit_logs WHERE prev_hash = $1 AND entity_type = 'contact' AND action = 'CREATE'`,
			strings.Repeat("a", 64))
	})
	var userIDs, companyIDs, contactIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		id := uuid.New()
		userIDs = append(userIDs, id)
		_, err := pool.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, first_name, last_name, role, status)
			VALUES ($1, $2, 'hash', 'User', $3, 'ADMIN', 'ACTIVE')`,
			id, fmt.Sprintf("%s@multirow.example", id), fmt.Sprintf("N%d", i))
		require.NoError(t, err)
		cid := uuid.New()
		companyIDs = append(companyIDs, cid)
		_, err = pool.Exec(ctx, `INSERT INTO companies (id, name, domain) VALUES ($1, $2, $3)`,
			cid, fmt.Sprintf("Multirow %d GmbH", i), fmt.Sprintf("multirow%d-%s.example", i, cid))
		require.NoError(t, err)
	}
	for i := 0; i < 4; i++ {
		id := uuid.New()
		contactIDs = append(contactIDs, id)
		_, err := pool.Exec(ctx, `
			INSERT INTO contacts (id, company_id, first_name, last_name, tags)
			VALUES ($1, $2, 'Contact', $3, '["multirow"]')`,
			id, companyIDs[i%3], fmt.Sprintf("Last%d", i))
		require.NoError(t, err)
	}
	for i := 0; i < 3; i++ {
		_, err := pool.Exec(ctx, `
			INSERT INTO deals (id, title, company_id, contact_id, value, assigned_to)
			VALUES ($1, $2, $3, $4, 100.50, $5)`,
			uuid.New(), fmt.Sprintf("Deal %d", i), companyIDs[i], contactIDs[i], userIDs[i])
		require.NoError(t, err)
	}
	for i := 0; i < 3; i++ {
		id := uuid.New()
		_, err := pool.Exec(ctx, `
			INSERT INTO audit_logs (id, entity_type, entity_id, action, prev_hash, hash)
			VALUES ($1, 'contact', $2, 'CREATE', $3, $4)`,
			id, uuid.New(), strings.Repeat("a", 64), strings.Repeat(fmt.Sprintf("%d", i), 64))
		require.NoError(t, err)
	}
	return fmt.Sprintf("%s@multirow.example", userIDs[1]), contactIDs[2]
}

func TestRestoreRoundTripsMultiRowTablesFromARealExport(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	_, contactID := seedMultiRow(t, ctx, pool)
	svc := newDrillServiceForPool(pool, &recordingAudit{})

	var buf bytes.Buffer
	require.NoError(t, svc.Export(ctx, &buf, RequestMeta{}))
	source := publicCounts(t, ctx, pool)
	require.GreaterOrEqual(t, source["users"], int64(3))
	require.GreaterOrEqual(t, source["contacts"], int64(4))
	require.GreaterOrEqual(t, source["audit_logs"], int64(3))

	for _, mode := range []RestoreMode{RestoreModeMerge, RestoreModeReplace} {
		result, err := svc.Restore(ctx, bytes.NewReader(buf.Bytes()), mode, RequestMeta{})
		require.NoError(t, err, "mode %s", mode)
		require.True(t, result.Committed)
		for _, name := range []string{"users", "companies", "contacts", "deals", "audit_logs"} {
			require.Equal(t, source[name], result.Counts[name], "mode %s result count for %s", mode, name)
		}
		after := publicCounts(t, ctx, pool)
		for name, want := range source {
			// Restore itself appends audit events; every other table must match.
			if name == "audit_logs" {
				require.GreaterOrEqual(t, after[name], want, "mode %s audit_logs", mode)
				continue
			}
			require.Equal(t, want, after[name], "mode %s count for %s", mode, name)
		}

		var lastName string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT last_name FROM contacts WHERE id = $1`, contactID).Scan(&lastName))
		require.Equal(t, "Last2", lastName, "mode %s", mode)
		var hashes int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM audit_logs WHERE hash = $1`, strings.Repeat("2", 64)).Scan(&hashes))
		require.GreaterOrEqual(t, hashes, 1, "mode %s audit hash survives", mode)
	}
}

func TestDrillPassesWithMultiRowTables(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	seedMultiRow(t, ctx, pool)
	result, err := newDrillServiceForPool(pool, &recordingAudit{}).Drill(ctx, RequestMeta{})
	require.NoError(t, err)
	require.True(t, result.Passed)
}
