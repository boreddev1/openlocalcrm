package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fake DBTX / transaction
// ---------------------------------------------------------------------------

type execCall struct {
	sql  string
	args []any
}

type fakeDBTX struct {
	beginCalls int
	beginErr   error

	calls     []execCall
	failAfter int // -1 disables; otherwise fail once len(calls) reaches this
	commitErr error

	pending   map[string]map[string][]byte
	committed map[string]map[string][]byte
	// tables is the set of table names the first upsert of each statement
	// observed, used to make sure every registry table is exercised.
	seenTables map[string]int

	tx *fakeTx
}

func newFakeDBTX() *fakeDBTX {
	return &fakeDBTX{
		failAfter:  -1,
		pending:    make(map[string]map[string][]byte),
		committed:  make(map[string]map[string][]byte),
		seenTables: make(map[string]int),
	}
}

func (f *fakeDBTX) Begin(ctx context.Context) (pgx.Tx, error) {
	f.beginCalls++
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	f.tx = &fakeTx{parent: f}
	return f.tx, nil
}

func (f *fakeDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, execCall{sql: sql, args: args})
	if f.failAfter >= 0 && len(f.calls) > f.failAfter {
		return pgconn.CommandTag{}, errors.New("upsert rejected by database")
	}

	// A row upsert carries exactly one jsonb argument.
	if len(args) == 1 {
		raw, ok := args[0].([]byte)
		if !ok {
			return pgconn.CommandTag{}, nil
		}
		key := statementKey(sql)
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err != nil {
			return pgconn.CommandTag{}, err
		}
		f.seenTables[key]++
		bucket := f.pending[key]
		if bucket == nil {
			bucket = make(map[string][]byte)
			f.pending[key] = bucket
		}
		bucket[fmt.Sprintf("%v", row["id"])] = raw
	}
	return pgconn.CommandTag{}, nil
}

func (f *fakeDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, errors.New("restore must not issue streaming queries")
}

func (f *fakeDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func (f *fakeDBTX) commit() {
	for key, rows := range f.pending {
		if f.committed[key] == nil {
			f.committed[key] = make(map[string][]byte)
		}
		for id, raw := range rows {
			f.committed[key][id] = raw
		}
	}
	f.pending = make(map[string]map[string][]byte)
}

func (f *fakeDBTX) rollback() {
	f.pending = make(map[string]map[string][]byte)
}

func statementKey(sql string) string {
	for _, line := range strings.Split(sql, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "INSERT INTO ") {
			rest := strings.TrimPrefix(line, "INSERT INTO ")
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				return fields[0]
			}
		}
		if strings.HasPrefix(line, "TRUNCATE") {
			return "TRUNCATE"
		}
	}
	return "UNKNOWN"
}

type fakeTx struct {
	pgx.Tx
	parent     *fakeDBTX
	committed  bool
	rolledBack bool
}

func (t *fakeTx) Commit(ctx context.Context) error {
	if t.parent.commitErr != nil {
		return t.parent.commitErr
	}
	t.committed = true
	t.parent.commit()
	return nil
}

func (t *fakeTx) Rollback(ctx context.Context) error {
	t.rolledBack = true
	t.parent.rollback()
	return nil
}

func (t *fakeTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return t.parent.Exec(ctx, sql, args...)
}

// newRestoreService wires a real *db.Queries over the fake DBTX, so the restore
// path exercises the generated upsert statements and the package-db Begin helper.
func newRestoreService(t *testing.T, fake *fakeDBTX, audit *recordingAudit) *Service {
	t.Helper()
	queries := db.New(fake)
	source := newFakeRowSource(0)
	return NewService(source, audit, nil, queries)
}

func validEnvelope(t *testing.T, overrides map[string]any) []byte {
	t.Helper()
	tables := make(map[string]any, len(tableSpecs))
	for _, spec := range tableSpecs {
		tables[spec.Name] = []any{
			map[string]any{spec.PK: "00000000-0000-0000-0000-0000000000aa"},
		}
	}
	for key, value := range overrides {
		if value == nil {
			delete(tables, key)
			continue
		}
		tables[key] = value
	}
	payload := map[string]any{
		"format":      "openlocalcrm-backup",
		"version":     1,
		"exported_at": "2026-09-26T10:00:00Z",
		"tables":      tables,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return raw
}

// ---------------------------------------------------------------------------
// Validation happens before any transaction work
// ---------------------------------------------------------------------------

func TestRestoreRejectsMalformedInputBeforeOpeningATransaction(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		want    string
	}{
		{
			name:    "unknown format",
			payload: []byte(`{"format":"something-else","version":1,"tables":{}}`),
			want:    "format",
		},
		{
			name:    "unsupported version",
			payload: []byte(`{"format":"openlocalcrm-backup","version":2,"tables":{}}`),
			want:    "version",
		},
		{
			name:    "missing tables object",
			payload: []byte(`{"format":"openlocalcrm-backup","version":1}`),
			want:    "tables",
		},
		{
			name:    "unknown table key",
			payload: []byte(`{"format":"openlocalcrm-backup","version":1,"tables":{"secrets":[]}}`),
			want:    "secrets",
		},
		{
			name:    "row is not an object",
			payload: []byte(`{"format":"openlocalcrm-backup","version":1,"tables":{"users":["nope"]}}`),
			want:    "users",
		},
		{
			name:    "unknown column",
			payload: []byte(`{"format":"openlocalcrm-backup","version":1,"tables":{"users":[{"id":"00000000-0000-0000-0000-000000000001","evil":"1"}]}}`),
			want:    "evil",
		},
		{
			name:    "missing primary key",
			payload: []byte(`{"format":"openlocalcrm-backup","version":1,"tables":{"users":[{"email":"a@example.de"}]}}`),
			want:    "id",
		},
		{
			name:    "truncated json",
			payload: []byte(`{"format":"openlocalcrm-backup","version":1,"tables":`),
			want:    "tables",
		},
		{
			name:    "not json at all",
			payload: []byte(`this is not a backup`),
			want:    "backup",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeDBTX()
			svc := newRestoreService(t, fake, &recordingAudit{})

			result, err := svc.Restore(context.Background(), strings.NewReader(string(tc.payload)),
				RestoreModeMerge, RequestMeta{})
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tc.want))
			require.False(t, result.Committed)
			require.Zero(t, fake.beginCalls, "validation must fail before a transaction starts")
			require.Empty(t, fake.calls)
		})
	}
}

func TestRestoreMergeUpsertsInRegistryOrderAndIsIdempotent(t *testing.T) {
	payload := validEnvelope(t, nil)

	fake := newFakeDBTX()
	svc := newRestoreService(t, fake, &recordingAudit{})

	first, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeMerge, RequestMeta{})
	require.NoError(t, err)
	require.True(t, first.Committed)
	require.Equal(t, RestoreModeMerge, first.Mode)
	require.Len(t, first.Counts, 24)
	for name, count := range first.Counts {
		require.Equal(t, int64(1), count, "table %s", name)
	}
	require.Equal(t, 1, fake.beginCalls)

	inserted := make([]string, 0, len(fake.calls))
	for _, call := range fake.calls {
		inserted = append(inserted, statementKey(call.sql))
	}
	require.Equal(t, tableNames(), inserted,
		"merge must upsert the 24 tables in FK-safe order without truncating")
	require.Equal(t, 1, fake.seenTables["users"])

	// Replaying the same payload is a no-op: every primary key already exists.
	fake.calls = nil
	second, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeMerge, RequestMeta{})
	require.NoError(t, err)
	require.True(t, second.Committed)
	require.Equal(t, first.Counts, second.Counts)
	require.Len(t, fake.committed["users"], 1, "merge upserts are idempotent")
}

func TestRestoreReplaceTruncatesFirstInReverseOrder(t *testing.T) {
	payload := validEnvelope(t, nil)

	fake := newFakeDBTX()
	svc := newRestoreService(t, fake, &recordingAudit{})

	result, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeReplace, RequestMeta{})
	require.NoError(t, err)
	require.True(t, result.Committed)
	require.Equal(t, RestoreModeReplace, result.Mode)

	require.Len(t, fake.calls, 25)
	require.Equal(t, "TRUNCATE", statementKey(fake.calls[0].sql))
	require.Contains(t, fake.calls[0].sql, "RESTART IDENTITY CASCADE")

	// The truncate list must be the exact reverse of the insertion order.
	truncated := truncateOrder(t, fake.calls[0].sql)
	require.Equal(t, reversedTableNames(), truncated)

	upserted := make([]string, 0, 24)
	for _, call := range fake.calls[1:] {
		upserted = append(upserted, statementKey(call.sql))
	}
	require.Equal(t, tableNames(), upserted)
}

func TestRestoreRollsBackEverythingOnAnUpsertError(t *testing.T) {
	payload := validEnvelope(t, nil)

	fake := newFakeDBTX()
	fake.failAfter = 12
	svc := newRestoreService(t, fake, &recordingAudit{})

	result, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeReplace, RequestMeta{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRestoreRolledBack)
	require.False(t, result.Committed)
	require.False(t, fake.tx.committed, "a failed restore must never commit")
	require.True(t, fake.tx.rolledBack, "a failed restore must roll back")
	require.Empty(t, fake.committed, "rollback must leave the original rows untouched")
}

func TestRestoreReportsCommittedOnlyAfterTheCommitSucceeds(t *testing.T) {
	payload := validEnvelope(t, nil)

	fake := newFakeDBTX()
	fake.commitErr = errors.New("commit failed")
	svc := newRestoreService(t, fake, &recordingAudit{})

	result, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeMerge, RequestMeta{})
	require.ErrorIs(t, err, ErrRestoreRolledBack)
	require.False(t, result.Committed)
}

func TestRestoreAuditFailureAfterCommitIsDistinct(t *testing.T) {
	payload := validEnvelope(t, nil)

	fake := newFakeDBTX()
	audit := &recordingAudit{failOn: "BACKUP_RESTORE_COMPLETED"}
	svc := newRestoreService(t, fake, audit)

	result, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeMerge, RequestMeta{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRestoreAuditFailed)
	require.NotErrorIs(t, err, ErrRestoreRolledBack)
	require.True(t, result.Committed, "the data restore did commit; the audit insert is what failed")
	require.True(t, fake.tx.committed)

	// A replace-mode completion audit must never land inside the import
	// transaction: replacing audit_logs would delete the event.
	require.Equal(t, []string{"BACKUP_RESTORE_STARTED"}, audit.actions())
}

func TestRestoreAuditsFailureAfterRollback(t *testing.T) {
	payload := validEnvelope(t, nil)

	fake := newFakeDBTX()
	fake.failAfter = 3
	audit := &recordingAudit{}
	svc := newRestoreService(t, fake, audit)

	_, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeMerge, RequestMeta{})
	require.ErrorIs(t, err, ErrRestoreRolledBack)
	require.Equal(t, []string{"BACKUP_RESTORE_STARTED", "BACKUP_RESTORE_FAILED"}, audit.actions())
}

func TestRestoreAuditFailureBeforeTransactionIsNotFatal(t *testing.T) {
	payload := validEnvelope(t, nil)
	fake := newFakeDBTX()
	audit := &recordingAudit{failOn: "BACKUP_RESTORE_STARTED"}
	svc := newRestoreService(t, fake, audit)

	result, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeMerge, RequestMeta{})
	require.NoError(t, err, "a failed pre-transaction audit must not abort the restore")
	require.True(t, result.Committed)
}

func TestRestoreRejectsAnUnsupportedMode(t *testing.T) {
	fake := newFakeDBTX()
	svc := newRestoreService(t, fake, &recordingAudit{})

	_, err := svc.Restore(context.Background(), strings.NewReader(string(validEnvelope(t, nil))),
		RestoreMode("sideways"), RequestMeta{})
	require.Error(t, err)
	require.Zero(t, fake.beginCalls)
}

func TestRestoreReturnsUnavailableWithoutATransactionCapableQuerier(t *testing.T) {
	// The demo querier has no pool and cannot open a transaction.
	svc := NewService(demo.NewInMemoryQuerier(), &recordingAudit{}, nil, demo.NewInMemoryQuerier())

	result, err := svc.Restore(context.Background(), strings.NewReader(string(validEnvelope(t, nil))),
		RestoreModeMerge, RequestMeta{})
	require.ErrorIs(t, err, ErrBackupUnavailable)
	require.False(t, result.Committed)
}

func TestRestoreFailsBeforeATransactionWhenTheBodyCannotBeRead(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	fake := newFakeDBTX()
	svc := newRestoreService(t, fake, &recordingAudit{})

	_, err := svc.Restore(context.Background(), failingReader{}, RestoreModeMerge, RequestMeta{})
	require.Error(t, err)
	require.Zero(t, fake.beginCalls)
	assertNoSpoolLeftovers(t)
}

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	return 0, errors.New("connection reset while uploading")
}

func TestRestoreDeletesTheSpoolFileOnEveryPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	t.Run("success", func(t *testing.T) {
		fake := newFakeDBTX()
		svc := newRestoreService(t, fake, &recordingAudit{})
		_, err := svc.Restore(context.Background(), strings.NewReader(string(validEnvelope(t, nil))),
			RestoreModeMerge, RequestMeta{})
		require.NoError(t, err)
		assertNoSpoolLeftovers(t)
	})

	t.Run("invalid envelope", func(t *testing.T) {
		fake := newFakeDBTX()
		svc := newRestoreService(t, fake, &recordingAudit{})
		_, err := svc.Restore(context.Background(), strings.NewReader(`{"format":"nope"}`),
			RestoreModeMerge, RequestMeta{})
		require.Error(t, err)
		assertNoSpoolLeftovers(t)
	})

	t.Run("database error", func(t *testing.T) {
		fake := newFakeDBTX()
		fake.failAfter = 1
		svc := newRestoreService(t, fake, &recordingAudit{})
		_, err := svc.Restore(context.Background(), strings.NewReader(string(validEnvelope(t, nil))),
			RestoreModeMerge, RequestMeta{})
		require.Error(t, err)
		assertNoSpoolLeftovers(t)
	})
}

// truncateOrder extracts the table list from the generated TRUNCATE statement.
func truncateOrder(t *testing.T, sql string) []string {
	t.Helper()
	start := strings.Index(sql, "TRUNCATE TABLE")
	require.GreaterOrEqual(t, start, 0, "statement must be a TRUNCATE")
	body := sql[start+len("TRUNCATE TABLE"):]
	body, _, _ = strings.Cut(body, "RESTART IDENTITY")

	var out []string
	for _, part := range strings.Split(body, ",") {
		name := strings.TrimSpace(part)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func reversedTableNames() []string {
	names := tableNames()
	out := make([]string, 0, len(names))
	for i := len(names) - 1; i >= 0; i-- {
		out = append(out, names[i])
	}
	return out
}

func assertNoSpoolLeftovers(t *testing.T) {
	t.Helper()
	dir := os.Getenv("TMPDIR")
	entries, err := os.ReadDir(dir)
	if err != nil {
		require.NoError(t, err)
	}
	require.Empty(t, entries, "the restore spool file must be deleted on every path")
}

func TestRestoreDoesNotRetryAutomatically(t *testing.T) {
	fake := newFakeDBTX()
	fake.failAfter = 2
	svc := newRestoreService(t, fake, &recordingAudit{})

	_, err := svc.Restore(context.Background(), strings.NewReader(string(validEnvelope(t, nil))),
		RestoreModeMerge, RequestMeta{})
	require.ErrorIs(t, err, ErrRestoreRolledBack)
	require.Equal(t, 1, fake.beginCalls, "replace is not idempotent; the service must never auto-retry")
}

// ---------------------------------------------------------------------------
// Replace mode must not truncate tables the envelope does not carry
// ---------------------------------------------------------------------------

// subsetEnvelope returns a valid envelope with the named tables removed.
func subsetEnvelope(t *testing.T, missing ...string) []byte {
	t.Helper()
	overrides := make(map[string]any, len(missing))
	for _, name := range missing {
		overrides[name] = nil
	}
	return validEnvelope(t, overrides)
}

func TestRestoreRejectsAnIncompleteReplaceEnvelope(t *testing.T) {
	// Replace truncates all 24 tables first. An envelope that omits one would
	// therefore delete that table's live data and still report success.
	payload := subsetEnvelope(t, "todos", "notes", "ai_audit_logs")

	fake := newFakeDBTX()
	svc := newRestoreService(t, fake, &recordingAudit{})

	result, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeReplace, RequestMeta{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrIncompleteBackup)
	require.NotErrorIs(t, err, ErrRestoreRolledBack,
		"an incomplete envelope is rejected before a transaction exists, so nothing was rolled back")
	require.False(t, result.Committed)
	require.Zero(t, fake.beginCalls, "no transaction may be opened for an incomplete replace")
	require.Empty(t, fake.calls, "no statement may run, least of all the truncate")

	// The operator must be told exactly what is missing, in the same
	// foreign-key-safe order the truncate would have used.
	var missing *MissingTablesError
	require.ErrorAs(t, err, &missing)
	require.Equal(t, []string{"todos", "notes", "ai_audit_logs"}, missing.Tables)
	for _, name := range []string{"todos", "notes", "ai_audit_logs"} {
		require.ErrorContains(t, err, name)
	}
}

func TestRestoreRejectsAnEmptyReplaceEnvelope(t *testing.T) {
	// The worst case: an envelope with no tables at all would wipe everything.
	payload := []byte(`{"format":"openlocalcrm-backup","version":1,"tables":{}}`)

	fake := newFakeDBTX()
	svc := newRestoreService(t, fake, &recordingAudit{})

	result, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeReplace, RequestMeta{})
	require.ErrorIs(t, err, ErrIncompleteBackup)
	require.NotErrorIs(t, err, ErrRestoreRolledBack)
	require.False(t, result.Committed)
	require.Zero(t, fake.beginCalls)
	require.Empty(t, fake.calls)

	var missing *MissingTablesError
	require.ErrorAs(t, err, &missing)
	require.Len(t, missing.Tables, 24)
}

func TestRestoreAcceptsACompleteReplaceEnvelope(t *testing.T) {
	payload := validEnvelope(t, nil)

	fake := newFakeDBTX()
	svc := newRestoreService(t, fake, &recordingAudit{})

	result, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeReplace, RequestMeta{})
	require.NoError(t, err)
	require.True(t, result.Committed)
	require.Len(t, result.Counts, 24)
	require.Len(t, fake.calls, 25, "the truncate plus 24 upserts")
	require.Equal(t, "TRUNCATE", statementKey(fake.calls[0].sql))
}

func TestRestoreMergeAcceptsAnIncompleteEnvelope(t *testing.T) {
	// Merge never truncates, so a subset is a legitimate partial import and
	// must not be rejected by the replace-only completeness rule.
	payload := subsetEnvelope(t, "todos", "notes")

	fake := newFakeDBTX()
	svc := newRestoreService(t, fake, &recordingAudit{})

	result, err := svc.Restore(context.Background(), strings.NewReader(string(payload)),
		RestoreModeMerge, RequestMeta{})
	require.NoError(t, err)
	require.True(t, result.Committed)
	require.Len(t, result.Counts, 22, "only the tables actually present are reported")
	require.NotContains(t, result.Counts, "todos")
	require.NotContains(t, result.Counts, "notes")
	require.Equal(t, 22, len(fake.calls), "merge must not truncate anything")
	for _, call := range fake.calls {
		require.NotEqual(t, "TRUNCATE", statementKey(call.sql))
	}
}

func TestMissingTablesErrorNamesTheInventorySize(t *testing.T) {
	err := &MissingTablesError{Tables: []string{"users", "todos"}}
	require.ErrorContains(t, err, "2 of 24 tables")
	require.ErrorContains(t, err, "users, todos")
	require.ErrorIs(t, err, ErrIncompleteBackup)
	require.NotErrorIs(t, err, ErrRestoreRolledBack)
	require.NotErrorIs(t, err, ErrInvalidBackup,
		"an incomplete replace is its own condition, not a malformed envelope")
}

func TestRestoreRejectsATruncatedTableArray(t *testing.T) {
	// Three rows, then the envelope ends before the closing ']'.
	raw := `{"format":"openlocalcrm-backup","version":1,"tables":{"users":[` +
		`{"id":"00000000-0000-0000-0000-0000000000a1"},` +
		`{"id":"00000000-0000-0000-0000-0000000000a2"},` +
		`{"id":"00000000-0000-0000-0000-0000000000a3"}`
	fake := newFakeDBTX()
	svc := newRestoreService(t, fake, &recordingAudit{})
	_, err := svc.Restore(context.Background(), strings.NewReader(raw), RestoreModeMerge, RequestMeta{})
	require.Error(t, err)
}

func TestApplyTableRowsReadsEveryRowAndDetectsTruncation(t *testing.T) {
	spec := tableByName("users")
	require.NotNil(t, spec)
	body := `{"t":[{"id":"a"},{"id":"b"},{"id":"c"}]}`
	spool, err := os.CreateTemp(t.TempDir(), "spool")
	require.NoError(t, err)
	defer spool.Close()
	_, err = spool.WriteString(body)
	require.NoError(t, err)
	offset := int64(strings.Index(body, "[") + 1)

	fake := newFakeDBTX()
	count, err := applyTableRows(context.Background(), spool, spec, tableRange{offset: offset}, db.New(fake))
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	// Truncated after the second row: the array never closes.
	require.NoError(t, spool.Truncate(int64(strings.Index(body, `,{"id":"c"}`))))
	_, err = applyTableRows(context.Background(), spool, spec, tableRange{offset: offset}, db.New(fake))
	require.Error(t, err)
}
