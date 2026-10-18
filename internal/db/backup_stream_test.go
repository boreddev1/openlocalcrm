package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// fakeRows serves row_to_json values the way PostgreSQL would.
type fakeRows struct {
	pgx.Rows
	raws  [][]byte
	pos   int
	err   error
	scanN int
}

func (r *fakeRows) Next() bool { return r.pos < len(r.raws) }

func (r *fakeRows) Scan(dest ...any) error {
	if r.pos >= len(r.raws) {
		return pgx.ErrNoRows
	}
	raw := r.raws[r.pos]
	r.pos++
	r.scanN++
	*(dest[0].(*[]byte)) = append([]byte(nil), raw...)
	return nil
}

func (r *fakeRows) Close()     {}
func (r *fakeRows) Err() error { return r.err }

// noTransactions is a DBTX that cannot open a transaction, like the demo store.
type noTransactions struct{}

func (n *noTransactions) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (n *noTransactions) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unused")
}

func (n *noTransactions) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

type noTransactionsWithTx struct {
	noTransactions
	tx pgx.Tx
}

func (n *noTransactionsWithTx) Begin(context.Context) (pgx.Tx, error) { return n.tx, nil }

type fakeDBTX struct {
	statements []string
	queried    []string
	rows       *fakeRows
	queryErr   error
	rowsErr    error
}

func (f *fakeDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.statements = append(f.statements, sql)
	return pgconn.CommandTag{}, nil
}

func (f *fakeDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.queried = append(f.queried, sql)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return &fakeRows{raws: f.rows.raws, err: f.rowsErr}, nil
}

func (f *fakeDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func newStreamTestQueries() (*Queries, *fakeDBTX) {
	fake := &fakeDBTX{rows: &fakeRows{raws: [][]byte{
		[]byte(`{"id":"00000000-0000-0000-0000-000000000001","custom_fields":{"a":1}}`),
		[]byte(`{"id":"00000000-0000-0000-0000-000000000002","custom_fields":{"a":2}}`),
	}}}
	return New(fake), fake
}

// everyScanner pairs each public scanner with the table it must select.
func everyScanner() map[string]struct {
	scan   func(*Queries, context.Context, RowSink) error
	table  string
	stream string
} {
	return map[string]struct {
		scan   func(*Queries, context.Context, RowSink) error
		table  string
		stream string
	}{
		"ScanBackupUsers":                 {(*Queries).ScanBackupUsers, "users", "FROM users t"},
		"ScanBackupCompanies":             {(*Queries).ScanBackupCompanies, "companies", "FROM companies t"},
		"ScanBackupWorkflows":             {(*Queries).ScanBackupWorkflows, "workflows", "FROM workflows t"},
		"ScanBackupContacts":              {(*Queries).ScanBackupContacts, "contacts", "FROM contacts t"},
		"ScanBackupDeals":                 {(*Queries).ScanBackupDeals, "deals", "FROM deals t"},
		"ScanBackupEmailAccounts":         {(*Queries).ScanBackupEmailAccounts, "email_accounts", "FROM email_accounts t"},
		"ScanBackupTodos":                 {(*Queries).ScanBackupTodos, "todos", "FROM todos t"},
		"ScanBackupRefreshTokens":         {(*Queries).ScanBackupRefreshTokens, "refresh_tokens", "FROM refresh_tokens t"},
		"ScanBackupAuditLogs":             {(*Queries).ScanBackupAuditLogs, "audit_logs", "FROM audit_logs t"},
		"ScanBackupNotifications":         {(*Queries).ScanBackupNotifications, "notifications", "FROM notifications t"},
		"ScanBackupEmailMessages":         {(*Queries).ScanBackupEmailMessages, "email_messages", "FROM email_messages t"},
		"ScanBackupEmailAttachments":      {(*Queries).ScanBackupEmailAttachments, "email_attachments", "FROM email_attachments t"},
		"ScanBackupAppointments":          {(*Queries).ScanBackupAppointments, "appointments", "FROM appointments t"},
		"ScanBackupCallActivities":        {(*Queries).ScanBackupCallActivities, "call_activities", "FROM call_activities t"},
		"ScanBackupCompanyResearch":       {(*Queries).ScanBackupCompanyResearch, "company_research", "FROM company_research t"},
		"ScanBackupWorkflowRuns":          {(*Queries).ScanBackupWorkflowRuns, "workflow_runs", "FROM workflow_runs t"},
		"ScanBackupWorkflowSteps":         {(*Queries).ScanBackupWorkflowSteps, "workflow_steps", "FROM workflow_steps t"},
		"ScanBackupNotes":                 {(*Queries).ScanBackupNotes, "notes", "FROM notes t"},
		"ScanBackupIntakeForms":           {(*Queries).ScanBackupIntakeForms, "intake_forms", "FROM intake_forms t"},
		"ScanBackupUserInvites":           {(*Queries).ScanBackupUserInvites, "user_invites", "FROM user_invites t"},
		"ScanBackupEmailTemplates":        {(*Queries).ScanBackupEmailTemplates, "email_templates", "FROM email_templates t"},
		"ScanBackupKnowledgeBaseArticles": {(*Queries).ScanBackupKnowledgeBaseArticles, "knowledge_base_articles", "FROM knowledge_base_articles t"},
		"ScanBackupAIResearchJobs":        {(*Queries).ScanBackupAIResearchJobs, "ai_research_jobs", "FROM ai_research_jobs t"},
		"ScanBackupAIAuditLogs":           {(*Queries).ScanBackupAIAuditLogs, "ai_audit_logs", "FROM ai_audit_logs t"},
	}
}

func TestScanBackupScannersStreamEveryTableOneRowAtATime(t *testing.T) {
	scanners := everyScanner()
	require.Len(t, scanners, 24)
	require.Len(t, BackupTableStatements, 24)

	for name, entry := range scanners {
		t.Run(name, func(t *testing.T) {
			queries, fake := newStreamTestQueries()

			var rows []json.RawMessage
			err := entry.scan(queries, context.Background(), func(_ context.Context, row json.RawMessage) error {
				// The sink must receive its own copy: the scanner is allowed to
				// hand out a reused buffer.
				rows = append(rows, row)
				return nil
			})
			require.NoError(t, err)
			require.Len(t, rows, 2)
			require.JSONEq(t, `{"a":1}`, string(mustField(t, rows[0], "custom_fields")))

			require.Len(t, fake.queried, 1)
			require.Contains(t, fake.queried[0], entry.stream)
			require.Contains(t, fake.queried[0], "row_to_json")
			require.Contains(t, fake.queried[0], "ORDER BY")

			statement, ok := BackupTableStatements[entry.table]
			require.True(t, ok, "table %s must be in the inventory", entry.table)
			require.Equal(t, statement, fake.queried[0],
				"the scanner must use the generated statement constant, not an ad-hoc query")
		})
	}
}

func mustField(t *testing.T, row json.RawMessage, key string) json.RawMessage {
	t.Helper()
	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(row, &decoded))
	value, ok := decoded[key]
	require.True(t, ok, "row must contain %s", key)
	return value
}

func TestScanBackupScannersRejectANilSink(t *testing.T) {
	queries, fake := newStreamTestQueries()
	for name, entry := range everyScanner() {
		t.Run(name, func(t *testing.T) {
			before := len(fake.queried)
			require.ErrorIs(t, entry.scan(queries, context.Background(), nil), ErrNilBackupRowSink)
			require.Len(t, fake.queried, before, "a nil sink must not hit the database")
		})
	}
}

func TestScanBackupScannersPropagateASinkError(t *testing.T) {
	queries, _ := newStreamTestQueries()
	sentinel := errors.New("client went away")

	var seen int
	err := queries.ScanBackupContacts(context.Background(), func(context.Context, json.RawMessage) error {
		seen++
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, 1, seen, "the stream must stop at the first sink error")
}

func TestScanBackupScannersReportQueryAndRowErrors(t *testing.T) {
	queries, fake := newStreamTestQueries()
	fake.queryErr = errors.New("connection reset")
	require.Error(t, queries.ScanBackupUsers(context.Background(), func(context.Context, json.RawMessage) error {
		return nil
	}))

	queries, fake = newStreamTestQueries()
	fake.rowsErr = errors.New("rows stream failed")
	err := queries.ScanBackupUsers(context.Background(), func(context.Context, json.RawMessage) error {
		return nil
	})
	require.ErrorContains(t, err, "rows stream failed")
}

func TestScanBackupTableRejectsUnknownTables(t *testing.T) {
	queries, fake := newStreamTestQueries()
	err := queries.ScanBackupTable(context.Background(), "users; DROP TABLE users", func(context.Context, json.RawMessage) error {
		return nil
	})
	require.ErrorIs(t, err, ErrUnknownBackupTable)
	require.Empty(t, fake.queried, "an unknown table must not reach the database")
}

func TestScanBackupTableUsesTheInventorySelector(t *testing.T) {
	queries, fake := newStreamTestQueries()
	count := 0
	require.NoError(t, queries.ScanBackupTable(context.Background(), "todos",
		func(context.Context, json.RawMessage) error {
			count++
			return nil
		}))
	require.Equal(t, 2, count)
	require.Equal(t, BackupTableStatements["todos"], fake.queried[0])
}

func TestReadBackupTableRejectsUnknownTables(t *testing.T) {
	queries, _ := newStreamTestQueries()
	_, err := ReadBackupTable(context.Background(), queries, "nope")
	require.ErrorIs(t, err, ErrUnknownBackupTable)
}

func TestReadBackupTableCopiesEveryRow(t *testing.T) {
	queries, _ := newStreamTestQueries()
	rows, err := ReadBackupTable(context.Background(), queries, "users")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Contains(t, string(rows[0]), `"custom_fields":{"a":1}`)
}

func TestBackupTableNamesIsSortedAndComplete(t *testing.T) {
	names := BackupTableNames()
	require.Len(t, names, 24)
	for i := 1; i < len(names); i++ {
		require.Less(t, names[i-1], names[i], "table names must be sorted")
	}
	require.Contains(t, names, "users")
	require.Contains(t, names, "ai_audit_logs")
	require.NotContains(t, names, "schema_migrations")
}

func TestRowDigestMatchesTheStreamedDigest(t *testing.T) {
	rows := [][]byte{[]byte(`{"id":"1"}`), []byte(`{"id":"2"}`)}
	count, digest := DigestRows(rows)
	require.Equal(t, int64(2), count)

	queries, _ := newStreamTestQueries()
	fake := queries.db.(*fakeDBTX)
	fake.rows = &fakeRows{raws: rows}

	streamedCount, streamedDigest, err := BackupTableDigest(context.Background(), queries, "users")
	require.NoError(t, err)
	require.Equal(t, count, streamedCount)
	require.Equal(t, digest, streamedDigest)
}

func TestRowDigestSeparatesRows(t *testing.T) {
	// Without a separator, ["ab"] and ["a","b"] would collide.
	_, joined := DigestRows([][]byte{[]byte("ab")})
	_, split := DigestRows([][]byte{[]byte("a"), []byte("b")})
	require.NotEqual(t, joined, split)

	empty := NewRowDigest()
	require.Equal(t, int64(0), empty.Count())
	require.Len(t, empty.Sum(), 64)
}

func TestBackupTableDigestRejectsUnknownTables(t *testing.T) {
	queries, _ := newStreamTestQueries()
	_, _, err := BackupTableDigest(context.Background(), queries, "nope")
	require.ErrorIs(t, err, ErrUnknownBackupTable)
}

func TestBackupTableDigestPropagatesStreamErrors(t *testing.T) {
	queries, fake := newStreamTestQueries()
	fake.rowsErr = errors.New("stream broke")
	_, _, err := BackupTableDigest(context.Background(), queries, "users")
	require.ErrorContains(t, err, "stream broke")
}

func TestBeginReportsAnUnsupportedHandle(t *testing.T) {
	_, err := New(&noTransactions{}).Begin(context.Background())
	require.ErrorIs(t, err, ErrTransactionsUnsupported)
}

func TestBeginDelegatesToTheHandle(t *testing.T) {
	handle := &noTransactionsWithTx{tx: &fakeTx{}}
	tx, err := New(handle).Begin(context.Background())
	require.NoError(t, err)
	require.NotNil(t, tx)
}

func TestQueriesImplementsTheBackupRowSource(t *testing.T) {
	var source BackupRowSource = New(&fakeDBTX{})
	require.NotNil(t, source)
}

// fakeTx is a placeholder transaction; the restore and drill tests use the
// richer double in internal/core/backup.
type fakeTx struct {
	pgx.Tx
}
