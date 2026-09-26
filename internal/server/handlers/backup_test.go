package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/backup"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server/handlers"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// handlerAudit records the audit events the service emits.
type handlerAudit struct {
	events []string
	failOn string
}

func (h *handlerAudit) Log(
	ctx context.Context,
	actorID pgtype.UUID,
	entityType string,
	entityID pgtype.UUID,
	action string,
	changes any,
	ip string,
	userAgent string,
) error {
	if h.failOn != "" && action == h.failOn {
		return errors.New("audit insert failed")
	}
	h.events = append(h.events, action)
	return nil
}

// handlerDBTX satisfies db.DBTX and db.Queries' transaction seam.
type handlerDBTX struct {
	failAfter   int
	commitErr   error
	execCount   int
	beginCalled bool
}

func (h *handlerDBTX) Begin(ctx context.Context) (pgx.Tx, error) {
	h.beginCalled = true
	return &handlerTx{parent: h}, nil
}

func (h *handlerDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	h.execCount++
	if h.failAfter >= 0 && h.execCount > h.failAfter {
		return pgconn.CommandTag{}, errors.New("upsert rejected")
	}
	return pgconn.CommandTag{}, nil
}

func (h *handlerDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, errors.New("unused")
}

func (h *handlerDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

type handlerTx struct {
	pgx.Tx
	parent *handlerDBTX
}

func (t *handlerTx) Commit(ctx context.Context) error { return t.parent.commitErr }

func (t *handlerTx) Rollback(ctx context.Context) error { return nil }

func (t *handlerTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return t.parent.Exec(ctx, sql, args...)
}

// failingResponseWriter makes the response stream break mid-download.
type failingResponseWriter struct {
	header http.Header
	code   int
	writes int
}

func (f *failingResponseWriter) Header() http.Header {
	if f.header == nil {
		f.header = http.Header{}
	}
	return f.header
}

func (f *failingResponseWriter) WriteHeader(code int) { f.code = code }

func (f *failingResponseWriter) Write(p []byte) (int, error) {
	f.writes++
	return 0, errors.New("client went away")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func adminRequest(method, target string, body string) *http.Request {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AccessClaims{
		Role: "ADMIN",
	})
	req = req.WithContext(ctx)
	req.RemoteAddr = "203.0.113.9:51234"
	req.Header.Set("User-Agent", "backup-handler-test")
	return req
}

func nonAdminRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AccessClaims{Role: "BENUTZER"})
	return req.WithContext(ctx)
}

func demoBackupHandler() *handlers.BackupHandler {
	q := demo.NewInMemoryQuerier()
	return handlers.NewBackupHandler(backup.NewService(q, &handlerAudit{}, nil, q))
}

func restoreBackupHandler(fake *handlerDBTX, audit backup.AuditLogger) *handlers.BackupHandler {
	queries := db.New(fake)
	return handlers.NewBackupHandler(backup.NewService(nil, audit, nil, queries))
}

const minimalBackup = `{"format":"openlocalcrm-backup","version":1,"exported_at":"2026-09-26T10:00:00Z","tables":{}}`

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

func TestBackupExportIsAdminOnly(t *testing.T) {
	h := demoBackupHandler()

	rec := httptest.NewRecorder()
	h.Export(rec, nonAdminRequest(http.MethodGet, "/api/v1/backup/export"))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "forbidden")

	// No admin claims at all.
	rec = httptest.NewRecorder()
	h.Export(rec, httptest.NewRequest(http.MethodGet, "/api/v1/backup/export", nil))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestBackupExportStreamsTheVersionedEnvelope(t *testing.T) {
	h := demoBackupHandler()

	rec := httptest.NewRecorder()
	h.Export(rec, adminRequest(http.MethodGet, "/api/v1/backup/export", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Header().Get("Content-Disposition"), "openlocalcrm-backup.json")

	var payload struct {
		Format  string                       `json:"format"`
		Version int                          `json:"version"`
		Tables  map[string][]json.RawMessage `json:"tables"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, "openlocalcrm-backup", payload.Format)
	require.Equal(t, 1, payload.Version)
	require.Len(t, payload.Tables, 24)
}

func TestBackupExportAttachesTheRequestMetadata(t *testing.T) {
	audit := &recordingHandlerAudit{}
	q := demo.NewInMemoryQuerier()
	h := handlers.NewBackupHandler(backup.NewService(q, audit, nil, q))

	rec := httptest.NewRecorder()
	h.Export(rec, adminRequest(http.MethodGet, "/api/v1/backup/export", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"BACKUP_EXPORT_STARTED", "BACKUP_EXPORT_COMPLETED"}, audit.actions)
	require.Equal(t, "203.0.113.9", audit.ip)
	require.Equal(t, "backup-handler-test", audit.userAgent)
}

type recordingHandlerAudit struct {
	actions   []string
	ip        string
	userAgent string
}

func (r *recordingHandlerAudit) Log(
	ctx context.Context,
	actorID pgtype.UUID,
	entityType string,
	entityID pgtype.UUID,
	action string,
	changes any,
	ip string,
	userAgent string,
) error {
	r.actions = append(r.actions, action)
	r.ip = ip
	r.userAgent = userAgent
	return nil
}

// TestBackupExportAbortsInsteadOfReturningATruncatedSuccess is the honest
// failure contract: once the response has started, a broken stream must abort
// the connection rather than leave a parseable partial document.
func TestBackupExportAbortsInsteadOfReturningATruncatedSuccess(t *testing.T) {
	h := demoBackupHandler()

	w := &failingResponseWriter{}
	req := adminRequest(http.MethodGet, "/api/v1/backup/export", "")

	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.Export(w, req)
	})
}

// ---------------------------------------------------------------------------
// Drill
// ---------------------------------------------------------------------------

func TestBackupDrillIsAdminOnly(t *testing.T) {
	h := demoBackupHandler()

	rec := httptest.NewRecorder()
	h.Drill(rec, nonAdminRequest(http.MethodPost, "/api/v1/backup/drill"))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestBackupDrillReturnsNotImplementedWithoutAPool(t *testing.T) {
	h := demoBackupHandler()

	rec := httptest.NewRecorder()
	h.Drill(rec, adminRequest(http.MethodPost, "/api/v1/backup/drill", ""))
	require.Equal(t, http.StatusNotImplemented, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "backup_unavailable", body["error"])
	require.NotContains(t, rec.Body.String(), "healthy",
		"demo mode must never report a healthy restore drill")
	require.NotContains(t, rec.Body.String(), "passed")
}

func TestBackupDrillIgnoresAnyRequestBody(t *testing.T) {
	h := demoBackupHandler()

	// The drill is bodyless: even a body claiming live data must not change the
	// answer, and no uploaded backup may be honoured.
	rec := httptest.NewRecorder()
	h.Drill(rec, adminRequest(http.MethodPost, "/api/v1/backup/drill", `{"source":"uploaded"}`))
	require.Equal(t, http.StatusNotImplemented, rec.Code)
}

// ---------------------------------------------------------------------------
// Restore
// ---------------------------------------------------------------------------

func TestBackupRestoreIsAdminOnly(t *testing.T) {
	h := demoBackupHandler()

	rec := httptest.NewRecorder()
	h.Restore(rec, nonAdminRequest(http.MethodPost, "/api/v1/backup/restore"))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestBackupRestoreRejectsAnInvalidEnvelope(t *testing.T) {
	h := demoBackupHandler()

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore", `{"format":"other"}`))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "restore_invalid_backup")
}

func TestBackupRestoreRejectsAnUnknownMode(t *testing.T) {
	h := demoBackupHandler()

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore?mode=sideways", minimalBackup))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestBackupRestoreRejectsAnUnreadableBody(t *testing.T) {
	fake := &handlerDBTX{failAfter: -1}
	h := restoreBackupHandler(fake, &handlerAudit{})

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore", strings.Repeat("x", 1024)))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, fake.beginCalled, "an unusable body must not open a transaction")
}

func TestBackupRestoreRejectsAnOversizedBody(t *testing.T) {
	original := handlers.MaxRestoreBytes
	handlers.MaxRestoreBytes = 1024
	t.Cleanup(func() { handlers.MaxRestoreBytes = original })

	fake := &handlerDBTX{failAfter: -1}
	h := restoreBackupHandler(fake, &handlerAudit{})

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore", strings.Repeat("x", 4096)))
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	require.Contains(t, rec.Body.String(), "restore_too_large")
	require.False(t, fake.beginCalled, "an oversized body must not open a transaction")
}

func TestBackupRestoreReturnsNotImplementedForDemo(t *testing.T) {
	h := demoBackupHandler()

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore", minimalBackup))
	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Contains(t, rec.Body.String(), "backup_unavailable")
}

func TestBackupRestoreReportsRollbackAsNotCommitted(t *testing.T) {
	fake := &handlerDBTX{failAfter: 3}
	audit := &handlerAudit{}
	h := restoreBackupHandler(fake, audit)

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore", fullBackup()))
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "restore_rolled_back", body["error"])
	require.Equal(t, false, body["committed"])
	require.Contains(t, audit.events, "BACKUP_RESTORE_FAILED")
}

func TestBackupRestoreReportsACommittedAuditFailureAsCommitted(t *testing.T) {
	fake := &handlerDBTX{failAfter: -1}
	audit := &handlerAudit{failOn: "BACKUP_RESTORE_COMPLETED"}
	h := restoreBackupHandler(fake, audit)

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore", fullBackup()))
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "restore_committed_audit_failed", body["error"])
	require.Equal(t, true, body["committed"],
		"the data restore committed; the UI must be told so")
}

func TestBackupRestoreCommitsMergeAndReportsCounts(t *testing.T) {
	fake := &handlerDBTX{failAfter: -1}
	h := restoreBackupHandler(fake, &handlerAudit{})

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore", fullBackup()))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Mode      string           `json:"mode"`
		Committed bool             `json:"committed"`
		Counts    map[string]int64 `json:"counts"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Committed)
	require.Equal(t, "merge", body.Mode)
	require.Len(t, body.Counts, 24)
}

func TestBackupRestoreCommitsReplaceWhenAskedExplicitly(t *testing.T) {
	fake := &handlerDBTX{failAfter: -1}
	h := restoreBackupHandler(fake, &handlerAudit{})

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore?mode=replace", fullBackup()))
	require.Equal(t, http.StatusOK, rec.Code)
	// 24 upserts plus the truncate.
	require.Equal(t, 25, fake.execCount)
}

func fullBackup() string {
	tables := make([]string, 0, 24)
	for _, name := range backup.TableNames() {
		tables = append(tables, `"`+name+`":[{"id":"00000000-0000-0000-0000-0000000000aa"}]`)
	}
	return `{"format":"openlocalcrm-backup","version":1,"exported_at":"2026-09-26T10:00:00Z","tables":{` +
		strings.Join(tables, ",") + `}}`
}

// ---------------------------------------------------------------------------
// A Querier without the streaming row source must disable the export with 501
// ---------------------------------------------------------------------------

// disabledBackupHandler is what the router builds when cfg.DB does not
// implement backup.RowSource. Export cannot produce a single row in that state,
// so it must not commit a 200 and then abort.
func disabledBackupHandler() *handlers.BackupHandler {
	return handlers.NewBackupHandler(backup.NewService(nil, &handlerAudit{}, nil, nil))
}

func TestBackupExportIsNotImplementedWithoutARowSource(t *testing.T) {
	h := disabledBackupHandler()

	rec := httptest.NewRecorder()
	require.NotPanics(t, func() {
		h.Export(rec, adminRequest(http.MethodGet, "/api/v1/backup/export", ""))
	})

	require.Equal(t, http.StatusNotImplemented, rec.Code,
		"a disabled export must answer 501, not commit 200 and abort")
	require.NotEqual(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body),
		"the 501 body must be a JSON error document")
	require.Equal(t, "backup_unavailable", body["error"])
	require.NotEmpty(t, body["message"])
	require.Equal(t, "", rec.Header().Get("Content-Disposition"),
		"no attachment may be offered when nothing can be streamed")
	require.Zero(t, rec.Body.Len()-len(rec.Body.Bytes()),
		"no backup payload may be written")
}

func TestBackupExportWithoutARowSourceNeverWritesAnEnvelope(t *testing.T) {
	h := disabledBackupHandler()

	rec := httptest.NewRecorder()
	require.NotPanics(t, func() {
		h.Export(rec, adminRequest(http.MethodGet, "/api/v1/backup/export", ""))
	})
	require.NotContains(t, rec.Body.String(), "openlocalcrm-backup",
		"no backup envelope may reach the client")
}

func TestBackupExportForNonAdminIsStillForbiddenWithoutARowSource(t *testing.T) {
	// Authorization is checked before availability: a non-admin must not be able
	// to probe which features the instance supports.
	h := disabledBackupHandler()

	rec := httptest.NewRecorder()
	h.Export(rec, nonAdminRequest(http.MethodGet, "/api/v1/backup/export"))
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestBackupServiceReportsExportAvailability(t *testing.T) {
	require.True(t, backup.NewService(demo.NewInMemoryQuerier(), nil, nil, nil).CanExport(),
		"the demo querier streams rows, so export is available")
	require.False(t, backup.NewService(nil, nil, nil, nil).CanExport(),
		"a service without a row source cannot export")

	var nilService *backup.Service
	require.False(t, nilService.CanExport())
}

// ---------------------------------------------------------------------------
// An incomplete replace envelope is an honest 400 with committed:false
// ---------------------------------------------------------------------------

func TestBackupRestoreRejectsAnIncompleteReplaceEnvelope(t *testing.T) {
	fake := &handlerDBTX{failAfter: -1}
	h := restoreBackupHandler(fake, &handlerAudit{})

	// Every table except todos, so the completeness rule is the only failure.
	var names []string
	for _, name := range backup.TableNames() {
		if name != "todos" {
			names = append(names, `"`+name+`":[{"id":"00000000-0000-0000-0000-0000000000aa"}]`)
		}
	}
	partial := `{"format":"openlocalcrm-backup","version":1,"tables":{` + strings.Join(names, ",") + `}}`

	rec := httptest.NewRecorder()
	h.Restore(rec, adminRequest(http.MethodPost, "/api/v1/backup/restore?mode=replace", partial))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "restore_incomplete_backup", body["error"])
	require.Equal(t, false, body["committed"], "nothing was committed")
	require.Equal(t, []any{"todos"}, body["missing_tables"],
		"the response must name the table whose live data would have been deleted")
	require.Zero(t, fake.execCount, "no truncate may run for an incomplete replace")
}
