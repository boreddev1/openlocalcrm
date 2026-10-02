package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fake row source
// ---------------------------------------------------------------------------

// fakeRowSource implements RowSource without any database. Every method records
// the table it was asked for and emits `rowsPerTable` minimal JSON objects.
type fakeRowSource struct {
	rowsPerTable int
	calls        []string
	failTable    string
}

func newFakeRowSource(rowsPerTable int) *fakeRowSource {
	return &fakeRowSource{rowsPerTable: rowsPerTable}
}

func (f *fakeRowSource) emit(table string, sink RowSink) error {
	f.calls = append(f.calls, table)
	if f.failTable == table {
		return fmt.Errorf("scan failed for %s", table)
	}
	// A deterministic per-table index keeps the emitted UUIDs unique.
	var tableIndex int64
	for i, name := range tableNames() {
		if name == table {
			tableIndex = int64(i + 1)
			break
		}
	}
	for i := 0; i < f.rowsPerTable; i++ {
		row := fmt.Sprintf(
			`{"id":"00000000-0000-0000-0000-%012d","custom_fields":{"seq":%d}}`,
			tableIndex*1000+int64(i)+1,
			i,
		)
		if err := sink(context.Background(), json.RawMessage(row)); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeRowSource) ScanBackupUsers(ctx context.Context, sink RowSink) error {
	return f.emit("users", sink)
}

func (f *fakeRowSource) ScanBackupCompanies(ctx context.Context, sink RowSink) error {
	return f.emit("companies", sink)
}

func (f *fakeRowSource) ScanBackupWorkflows(ctx context.Context, sink RowSink) error {
	return f.emit("workflows", sink)
}

func (f *fakeRowSource) ScanBackupContacts(ctx context.Context, sink RowSink) error {
	return f.emit("contacts", sink)
}

func (f *fakeRowSource) ScanBackupDeals(ctx context.Context, sink RowSink) error {
	return f.emit("deals", sink)
}

func (f *fakeRowSource) ScanBackupEmailAccounts(ctx context.Context, sink RowSink) error {
	return f.emit("email_accounts", sink)
}

func (f *fakeRowSource) ScanBackupTodos(ctx context.Context, sink RowSink) error {
	return f.emit("todos", sink)
}

func (f *fakeRowSource) ScanBackupRefreshTokens(ctx context.Context, sink RowSink) error {
	return f.emit("refresh_tokens", sink)
}

func (f *fakeRowSource) ScanBackupAuditLogs(ctx context.Context, sink RowSink) error {
	return f.emit("audit_logs", sink)
}

func (f *fakeRowSource) ScanBackupNotifications(ctx context.Context, sink RowSink) error {
	return f.emit("notifications", sink)
}

func (f *fakeRowSource) ScanBackupEmailMessages(ctx context.Context, sink RowSink) error {
	return f.emit("email_messages", sink)
}

func (f *fakeRowSource) ScanBackupEmailAttachments(ctx context.Context, sink RowSink) error {
	return f.emit("email_attachments", sink)
}

func (f *fakeRowSource) ScanBackupAppointments(ctx context.Context, sink RowSink) error {
	return f.emit("appointments", sink)
}

func (f *fakeRowSource) ScanBackupCallActivities(ctx context.Context, sink RowSink) error {
	return f.emit("call_activities", sink)
}

func (f *fakeRowSource) ScanBackupCompanyResearch(ctx context.Context, sink RowSink) error {
	return f.emit("company_research", sink)
}

func (f *fakeRowSource) ScanBackupWorkflowRuns(ctx context.Context, sink RowSink) error {
	return f.emit("workflow_runs", sink)
}

func (f *fakeRowSource) ScanBackupWorkflowSteps(ctx context.Context, sink RowSink) error {
	return f.emit("workflow_steps", sink)
}

func (f *fakeRowSource) ScanBackupNotes(ctx context.Context, sink RowSink) error {
	return f.emit("notes", sink)
}

func (f *fakeRowSource) ScanBackupIntakeForms(ctx context.Context, sink RowSink) error {
	return f.emit("intake_forms", sink)
}

func (f *fakeRowSource) ScanBackupUserInvites(ctx context.Context, sink RowSink) error {
	return f.emit("user_invites", sink)
}

func (f *fakeRowSource) ScanBackupEmailTemplates(ctx context.Context, sink RowSink) error {
	return f.emit("email_templates", sink)
}

func (f *fakeRowSource) ScanBackupKnowledgeBaseArticles(ctx context.Context, sink RowSink) error {
	return f.emit("knowledge_base_articles", sink)
}

func (f *fakeRowSource) ScanBackupAIResearchJobs(ctx context.Context, sink RowSink) error {
	return f.emit("ai_research_jobs", sink)
}

func (f *fakeRowSource) ScanBackupAIAuditLogs(ctx context.Context, sink RowSink) error {
	return f.emit("ai_audit_logs", sink)
}

var _ RowSource = (*fakeRowSource)(nil)

// ---------------------------------------------------------------------------
// Fake audit logger
// ---------------------------------------------------------------------------

type auditEvent struct {
	ActorID   pgtype.UUID
	Entity    string
	EntityID  pgtype.UUID
	Action    string
	Changes   any
	IPAddress string
	UserAgent string
}

type recordingAudit struct {
	events []auditEvent
	failOn string
}

func (r *recordingAudit) Log(
	ctx context.Context,
	actorID pgtype.UUID,
	entityType string,
	entityID pgtype.UUID,
	action string,
	changes any,
	ip string,
	userAgent string,
) error {
	if r.failOn != "" && action == r.failOn {
		return errors.New("audit insert failed")
	}
	r.events = append(r.events, auditEvent{
		ActorID:   actorID,
		Entity:    entityType,
		EntityID:  entityID,
		Action:    action,
		Changes:   changes,
		IPAddress: ip,
		UserAgent: userAgent,
	})
	return nil
}

func (r *recordingAudit) actions() []string {
	out := make([]string, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e.Action)
	}
	return out
}

var _ AuditLogger = (*recordingAudit)(nil)

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

func TestExportStreamsEveryTableWithoutAccumulator(t *testing.T) {
	source := newFakeRowSource(2)
	var out bytes.Buffer
	svc := NewService(source, nil, nil, nil)
	err := svc.Export(context.Background(), &out, RequestMeta{})
	require.NoError(t, err)

	var payload struct {
		Format     string                       `json:"format"`
		Version    int                          `json:"version"`
		ExportedAt string                       `json:"exported_at"`
		Tables     map[string][]json.RawMessage `json:"tables"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &payload))
	require.Equal(t, "openlocalcrm-backup", payload.Format)
	require.Equal(t, 1, payload.Version)
	require.NotEmpty(t, payload.ExportedAt)
	require.Len(t, payload.Tables, 24)
	require.Len(t, payload.Tables["contacts"], 2)
	require.NotContains(t, out.String(), `"custom_fields":"e30="`)
}

func TestExportKeepsJSONBColumnsAsJSON(t *testing.T) {
	source := newFakeRowSource(1)
	var out bytes.Buffer
	svc := NewService(source, nil, nil, nil)
	require.NoError(t, svc.Export(context.Background(), &out, RequestMeta{}))

	require.Contains(t, out.String(), `"custom_fields":{"seq":0}`)
	// A base64 encoding of {"seq":0} would be eyJvcXVlbmNlIjoyfQ==.
	require.NotContains(t, out.String(), "eyJvcXVlbmNlIjoyfQ==")
}

func TestExportEmitsEveryTableOnceInForeignKeySafeOrder(t *testing.T) {
	source := newFakeRowSource(1)
	svc := NewService(source, nil, nil, nil)
	require.NoError(t, svc.Export(context.Background(), &bytes.Buffer{}, RequestMeta{}))

	require.Equal(t, tableNames(), source.calls, "each table must be streamed exactly once in FK-safe order")

	seen := make(map[string]int, len(source.calls))
	for _, name := range source.calls {
		seen[name]++
	}
	for _, name := range tableNames() {
		require.Equal(t, 1, seen[name], "table %s must be streamed once", name)
	}
}

func TestExportAbortsWithoutAValidDocumentWhenStreamingFails(t *testing.T) {
	source := newFakeRowSource(2)
	source.failTable = "deals"
	var out bytes.Buffer
	svc := NewService(source, nil, nil, nil)

	err := svc.Export(context.Background(), &out, RequestMeta{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "scan failed for deals")

	var payload map[string]any
	require.Error(t, json.Unmarshal(out.Bytes(), &payload),
		"a failed export must never leave a parseable success document")
}

func TestExportRequiresARowSource(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	err := svc.Export(context.Background(), &bytes.Buffer{}, RequestMeta{})
	require.Error(t, err)
}

func TestExportAuditsStartedAndCompletedWithRequestMetadata(t *testing.T) {
	source := newFakeRowSource(1)
	audit := &recordingAudit{}
	svc := NewService(source, audit, nil, nil)

	meta := RequestMeta{
		IPAddress: "203.0.113.7",
		UserAgent: "backup-test-agent",
	}
	require.NoError(t, svc.Export(context.Background(), &bytes.Buffer{}, meta))

	require.Equal(t, []string{"BACKUP_EXPORT_STARTED", "BACKUP_EXPORT_COMPLETED"}, audit.actions())
	require.Equal(t, "BACKUP", audit.events[0].Entity)
	require.Equal(t, "203.0.113.7", audit.events[0].IPAddress)
	require.Equal(t, "backup-test-agent", audit.events[0].UserAgent)

	started, ok := audit.events[0].Changes.(map[string]any)
	require.True(t, ok)
	require.Equal(t, 24, started["tables"])

	completed, ok := audit.events[1].Changes.(map[string]any)
	require.True(t, ok)
	require.Equal(t, 24, completed["tables"])
}

func TestExportAuditsFailureAndNeverReportsSuccess(t *testing.T) {
	source := newFakeRowSource(1)
	source.failTable = "contacts"
	audit := &recordingAudit{}
	svc := NewService(source, audit, nil, nil)

	err := svc.Export(context.Background(), &bytes.Buffer{}, RequestMeta{})
	require.Error(t, err)
	require.Equal(t, []string{"BACKUP_EXPORT_STARTED", "BACKUP_EXPORT_FAILED"}, audit.actions())
	require.NotContains(t, audit.actions(), "BACKUP_EXPORT_COMPLETED")
}

func TestExportAuditFailureIsNotSwallowed(t *testing.T) {
	source := newFakeRowSource(1)
	audit := &recordingAudit{failOn: "BACKUP_EXPORT_COMPLETED"}
	svc := NewService(source, audit, nil, nil)

	var out bytes.Buffer
	err := svc.Export(context.Background(), &out, RequestMeta{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "audit insert failed")
}

func TestExportToleratesASinkError(t *testing.T) {
	// A destination that fails mid-write must abort the stream.
	source := newFakeRowSource(3)
	svc := NewService(source, nil, nil, nil)
	err := svc.Export(context.Background(), failingWriter{}, RequestMeta{})
	require.Error(t, err)
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return 0, errors.New("destination closed")
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

func TestTableRegistryCoversTheApprovedInventory(t *testing.T) {
	expected := []string{
		"users",
		"companies",
		"workflows",
		"contacts",
		"deals",
		"email_accounts",
		"todos",
		"refresh_tokens",
		"audit_logs",
		"notifications",
		"email_messages",
		"email_attachments",
		"appointments",
		"call_activities",
		"company_research",
		"workflow_runs",
		"workflow_steps",
		"notes",
		"intake_forms",
		"user_invites",
		"email_templates",
		"knowledge_base_articles",
		"ai_research_jobs",
		"ai_audit_logs",
	}
	require.Equal(t, expected, tableNames())
	require.NotContains(t, tableNames(), "schema_migrations",
		"schema_migrations is infrastructure metadata, not application data")

	seen := make(map[string]bool, len(tableSpecs))
	for _, spec := range tableSpecs {
		require.False(t, seen[spec.Name], "duplicate table %s", spec.Name)
		seen[spec.Name] = true
		require.NotNil(t, spec.Stream, "table %s needs a stream function", spec.Name)
		require.NotNil(t, spec.Upsert, "table %s needs an upsert function", spec.Name)
		require.NotEmpty(t, spec.PK, "table %s needs a primary key", spec.Name)
		require.NotEmpty(t, spec.Columns, "table %s needs a column allowlist", spec.Name)
	}
}

func TestTableRegistryColumnAllowlistRejectsUnknownColumns(t *testing.T) {
	for _, spec := range tableSpecs {
		allow := make(map[string]bool, len(spec.Columns))
		for _, col := range spec.Columns {
			require.False(t, allow[col], "table %s lists column %s twice", spec.Name, col)
			allow[col] = true
		}
		require.True(t, allow[spec.PK], "table %s primary key %s must be allowlisted", spec.Name, spec.PK)
	}
}

func TestTextPrimaryKeyTablesAreNotCastToUUID(t *testing.T) {
	// workflows, workflow_runs and workflow_steps keep TEXT identity columns.
	for _, name := range []string{"workflows", "workflow_runs", "workflow_steps"} {
		idx := indexOfTable(name)
		require.GreaterOrEqual(t, idx, 0)
	}
}

// ---------------------------------------------------------------------------
// Static guarantee: the package never calls generated List* / Export*Rows
// ---------------------------------------------------------------------------

func TestBackupPackageNeverCallsAccumulatorHelpers(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, parseErr)
		files[name] = file
	}
	require.NotEmpty(t, files, "the backup package must contain non-test Go sources")

	for path, file := range files {
		pkgName := file.Name.Name
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			recv, ok := sel.X.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := recv.X.(*ast.Ident)
			if !ok || ident.Name != "s" {
				return true
			}
			if recv.Sel.Name != "queries" && recv.Sel.Name != "source" {
				return true
			}
			if strings.HasPrefix(sel.Sel.Name, "List") {
				t.Errorf("%s: %s.%s.%s calls an accumulator helper; RowSource is the only read boundary",
					filepath.Base(path), pkgName, recv.Sel.Name, sel.Sel.Name)
			}
			if strings.HasPrefix(sel.Sel.Name, "Export") && strings.HasSuffix(sel.Sel.Name, "Rows") {
				t.Errorf("%s: %s.%s.%s materializes [][]byte; use the handwritten scanner",
					filepath.Base(path), pkgName, recv.Sel.Name, sel.Sel.Name)
			}
			return true
		})
	}
}
