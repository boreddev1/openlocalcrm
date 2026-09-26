package demo_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/stretchr/testify/require"
)

// demoScanner pairs each streaming method with the table it must stream.
type demoScanner struct {
	table string
	scan  func(*demo.InMemoryQuerier, context.Context, db.RowSink) error
}

func demoScanners() []demoScanner {
	return []demoScanner{
		{"users", (*demo.InMemoryQuerier).ScanBackupUsers},
		{"companies", (*demo.InMemoryQuerier).ScanBackupCompanies},
		{"workflows", (*demo.InMemoryQuerier).ScanBackupWorkflows},
		{"contacts", (*demo.InMemoryQuerier).ScanBackupContacts},
		{"deals", (*demo.InMemoryQuerier).ScanBackupDeals},
		{"email_accounts", (*demo.InMemoryQuerier).ScanBackupEmailAccounts},
		{"todos", (*demo.InMemoryQuerier).ScanBackupTodos},
		{"refresh_tokens", (*demo.InMemoryQuerier).ScanBackupRefreshTokens},
		{"audit_logs", (*demo.InMemoryQuerier).ScanBackupAuditLogs},
		{"notifications", (*demo.InMemoryQuerier).ScanBackupNotifications},
		{"email_messages", (*demo.InMemoryQuerier).ScanBackupEmailMessages},
		{"email_attachments", (*demo.InMemoryQuerier).ScanBackupEmailAttachments},
		{"appointments", (*demo.InMemoryQuerier).ScanBackupAppointments},
		{"call_activities", (*demo.InMemoryQuerier).ScanBackupCallActivities},
		{"company_research", (*demo.InMemoryQuerier).ScanBackupCompanyResearch},
		{"workflow_runs", (*demo.InMemoryQuerier).ScanBackupWorkflowRuns},
		{"workflow_steps", (*demo.InMemoryQuerier).ScanBackupWorkflowSteps},
		{"notes", (*demo.InMemoryQuerier).ScanBackupNotes},
		{"intake_forms", (*demo.InMemoryQuerier).ScanBackupIntakeForms},
		{"user_invites", (*demo.InMemoryQuerier).ScanBackupUserInvites},
		{"email_templates", (*demo.InMemoryQuerier).ScanBackupEmailTemplates},
		{"knowledge_base_articles", (*demo.InMemoryQuerier).ScanBackupKnowledgeBaseArticles},
		{"ai_research_jobs", (*demo.InMemoryQuerier).ScanBackupAIResearchJobs},
		{"ai_audit_logs", (*demo.InMemoryQuerier).ScanBackupAIAuditLogs},
	}
}

// demoTable pairs each table with its generated export and upsert methods.
type demoTable struct {
	table  string
	export func(*demo.InMemoryQuerier, context.Context) ([][]byte, error)
	upsert func(*demo.InMemoryQuerier, context.Context, []byte) error
	stored bool
}

func demoTables() []demoTable {
	return []demoTable{
		{"users", (*demo.InMemoryQuerier).ExportUsersRows, (*demo.InMemoryQuerier).UpsertUsersRow, true},
		{"companies", (*demo.InMemoryQuerier).ExportCompaniesRows, (*demo.InMemoryQuerier).UpsertCompaniesRow, true},
		{"workflows", (*demo.InMemoryQuerier).ExportWorkflowsRows, (*demo.InMemoryQuerier).UpsertWorkflowsRow, true},
		{"contacts", (*demo.InMemoryQuerier).ExportContactsRows, (*demo.InMemoryQuerier).UpsertContactsRow, true},
		{"deals", (*demo.InMemoryQuerier).ExportDealsRows, (*demo.InMemoryQuerier).UpsertDealsRow, true},
		{"email_accounts", (*demo.InMemoryQuerier).ExportEmailAccountsRows, (*demo.InMemoryQuerier).UpsertEmailAccountsRow, true},
		{"todos", (*demo.InMemoryQuerier).ExportTodosRows, (*demo.InMemoryQuerier).UpsertTodosRow, true},
		{"refresh_tokens", (*demo.InMemoryQuerier).ExportRefreshTokensRows, (*demo.InMemoryQuerier).UpsertRefreshTokensRow, true},
		{"audit_logs", (*demo.InMemoryQuerier).ExportAuditLogsRows, (*demo.InMemoryQuerier).UpsertAuditLogsRow, true},
		{"notifications", (*demo.InMemoryQuerier).ExportNotificationsRows, (*demo.InMemoryQuerier).UpsertNotificationsRow, true},
		{"email_messages", (*demo.InMemoryQuerier).ExportEmailMessagesRows, (*demo.InMemoryQuerier).UpsertEmailMessagesRow, true},
		{"email_attachments", (*demo.InMemoryQuerier).ExportEmailAttachmentsRows, (*demo.InMemoryQuerier).UpsertEmailAttachmentsRow, true},
		{"appointments", (*demo.InMemoryQuerier).ExportAppointmentsRows, (*demo.InMemoryQuerier).UpsertAppointmentsRow, true},
		{"call_activities", (*demo.InMemoryQuerier).ExportCallActivitiesRows, (*demo.InMemoryQuerier).UpsertCallActivitiesRow, true},
		{"workflow_runs", (*demo.InMemoryQuerier).ExportWorkflowRunsRows, (*demo.InMemoryQuerier).UpsertWorkflowRunsRow, true},
		{"notes", (*demo.InMemoryQuerier).ExportNotesRows, (*demo.InMemoryQuerier).UpsertNotesRow, true},
		{"knowledge_base_articles", (*demo.InMemoryQuerier).ExportKnowledgeBaseArticlesRows, (*demo.InMemoryQuerier).UpsertKnowledgeBaseArticlesRow, true},
		{"ai_research_jobs", (*demo.InMemoryQuerier).ExportAIResearchJobsRows, (*demo.InMemoryQuerier).UpsertAIResearchJobsRow, true},
		{"company_research", (*demo.InMemoryQuerier).ExportCompanyResearchRows, (*demo.InMemoryQuerier).UpsertCompanyResearchRow, false},
		{"workflow_steps", (*demo.InMemoryQuerier).ExportWorkflowStepsRows, (*demo.InMemoryQuerier).UpsertWorkflowStepsRow, false},
		{"intake_forms", (*demo.InMemoryQuerier).ExportIntakeFormsRows, (*demo.InMemoryQuerier).UpsertIntakeFormsRow, false},
		{"ai_audit_logs", (*demo.InMemoryQuerier).ExportAIAuditLogsRows, (*demo.InMemoryQuerier).UpsertAIAuditLogsRow, false},
		{"user_invites", (*demo.InMemoryQuerier).ExportUserInvitesRows, (*demo.InMemoryQuerier).UpsertUserInvitesRow, true},
		{"email_templates", (*demo.InMemoryQuerier).ExportEmailTemplatesRows, (*demo.InMemoryQuerier).UpsertEmailTemplatesRow, true},
	}
}

func TestDemoBackupStreamCoversTheWholeInventory(t *testing.T) {
	scanners := demoScanners()
	require.Len(t, scanners, 24)
	require.Len(t, db.BackupTableStatements, 24)

	seen := make(map[string]bool, len(scanners))
	for _, entry := range scanners {
		require.False(t, seen[entry.table], "table %s streamed twice", entry.table)
		seen[entry.table] = true
		_, ok := db.BackupTableStatements[entry.table]
		require.True(t, ok, "table %s must exist in the SQL inventory", entry.table)
	}

	var source db.BackupRowSource = demo.NewInMemoryQuerier()
	require.NotNil(t, source)
}

func TestDemoBackupStreamAgreesWithTheGeneratedExportSurface(t *testing.T) {
	// A table the demo does not seed must stream zero rows, exactly like an
	// empty PostgreSQL table. A seeded table must stream exactly the rows the
	// generated Export*Rows method reports, in primary-key order.
	ctx := context.Background()
	q := demo.NewInMemoryQuerier()
	seedInvitesAndTemplates(t, ctx, q)

	exports := make(map[string]func(*demo.InMemoryQuerier, context.Context) ([][]byte, error), len(demoTables()))
	for _, entry := range demoTables() {
		exports[entry.table] = entry.export
	}

	for _, entry := range demoScanners() {
		t.Run(entry.table, func(t *testing.T) {
			expected, err := exports[entry.table](q, ctx)
			require.NoError(t, err)

			var streamed [][]byte
			require.NoError(t, entry.scan(q, ctx, func(_ context.Context, row json.RawMessage) error {
				streamed = append(streamed, []byte(row))
				return nil
			}))

			require.Len(t, streamed, len(expected),
				"the streaming scanner and the generated export must see the same rows")
			for i := range streamed {
				require.Equal(t, string(expected[i]), string(streamed[i]),
					"row %d of %s must be byte-identical and in the same order", i, entry.table)
				var decoded map[string]any
				require.NoError(t, json.Unmarshal(streamed[i], &decoded))
				require.NotEmpty(t, decoded["id"], "every backup row must carry its primary key")
			}
		})
	}
}

// seedInvitesAndTemplates gives the two SP7 tables a real row so they are not
// compared as two empty lists.
func seedInvitesAndTemplates(t *testing.T, ctx context.Context, q *demo.InMemoryQuerier) {
	t.Helper()
	userID := parseUUIDForBackupTest(t, "11111111-1111-1111-1111-111111111111")
	_, err := q.CreateUserInvite(ctx, db.CreateUserInviteParams{
		UserID:    userID,
		TokenHash: strings.Repeat("a", 64),
		ExpiresAt: timestamptzForBackupTest(time.Now().UTC().Add(72 * time.Hour)),
	})
	require.NoError(t, err)
	_, err = q.CreateEmailTemplate(ctx, db.CreateEmailTemplateParams{
		Key:      "backup_drill_template",
		Name:     "Backup-Drill",
		Subject:  "Backup",
		Body:     "Text",
		Category: "system",
		Locale:   "de-DE",
	})
	require.NoError(t, err)
}

func parseUUIDForBackupTest(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	parsed, err := uuid.Parse(s)
	require.NoError(t, err)
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

func timestamptzForBackupTest(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// TestDemoBackupStreamSeedsEveryStoredTable proves the row source really does
// stream data, so the agreement test above is not comparing two empty lists for
// every table.
func TestDemoBackupStreamSeedsEveryStoredTable(t *testing.T) {
	ctx := context.Background()
	q := demo.NewInMemoryQuerier()

	var populated int
	for _, entry := range demoTables() {
		if !entry.stored {
			continue
		}
		rows, err := entry.export(q, ctx)
		require.NoError(t, err)
		if len(rows) > 0 {
			populated++
		}
	}
	require.Greater(t, populated, 10, "the demo fixtures must populate most backup tables")
}

func TestDemoBackupStreamRejectsANilSink(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	for _, entry := range demoScanners() {
		require.ErrorIs(t, entry.scan(q, context.Background(), nil), db.ErrNilBackupRowSink, entry.table)
	}
}

func TestDemoBackupStreamPropagatesASinkError(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	err := q.ScanBackupUsers(context.Background(), func(context.Context, json.RawMessage) error {
		return context.Canceled
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestDemoBackupStreamKeepsJSONBColumnsAsJSON(t *testing.T) {
	q := demo.NewInMemoryQuerier()
	var found bool
	require.NoError(t, q.ScanBackupContacts(context.Background(), func(_ context.Context, row json.RawMessage) error {
		if strings.Contains(string(row), `"custom_fields":"`) {
			t.Errorf("custom_fields must stay JSON, got %s", row)
		}
		if strings.Contains(string(row), "custom_fields") {
			found = true
		}
		return nil
	}))
	require.True(t, found, "contacts must actually carry custom_fields")
}

func TestDemoBackupRowsRoundTripThroughExportAndUpsert(t *testing.T) {
	ctx := context.Background()

	for _, entry := range demoTables() {
		t.Run(entry.table, func(t *testing.T) {
			q := demo.NewInMemoryQuerier()
			seedInvitesAndTemplates(t, ctx, q)
			before, err := entry.export(q, ctx)
			require.NoError(t, err)

			if !entry.stored {
				require.Empty(t, before, "a table the demo does not store exports zero rows")
				err := entry.upsert(q, ctx, []byte(`{"id":"11111111-1111-1111-1111-111111111111"}`))
				require.Error(t, err,
					"the demo must not silently accept a row for a table it does not store")
				return
			}

			// Replaying every exported row is an ON CONFLICT upsert, so the row
			// count must not change. An empty table is a legitimate starting
			// state and must stay empty.
			for _, row := range before {
				require.NoError(t, entry.upsert(q, ctx, row))
			}
			after, err := entry.export(q, ctx)
			require.NoError(t, err)
			require.Len(t, after, len(before), "upsert must be idempotent")
		})
	}
}

func TestDemoBackupRowsRejectMalformedInput(t *testing.T) {
	ctx := context.Background()
	q := demo.NewInMemoryQuerier()

	require.Error(t, q.UpsertUsersRow(ctx, []byte(`not json`)))
	require.Error(t, q.UpsertUsersRow(ctx, []byte(`{"email":"nobody@example.de"}`)))
	require.Error(t, q.UpsertWorkflowsRow(ctx, []byte(`{"name":"no id"}`)))
	require.Error(t, q.UpsertAuditLogsRow(ctx, []byte(`{"action":"no id"}`)))
	require.Error(t, q.UpsertWorkflowRunsRow(ctx, []byte(`{"status":"no id"}`)))
}

func TestDemoBackupRowsPreserveTheConflictKeyOnReplay(t *testing.T) {
	ctx := context.Background()
	q := demo.NewInMemoryQuerier()

	// A company row replayed with a different id must update the existing
	// company, mirroring ON CONFLICT (id) DO UPDATE.
	rows, err := q.ExportCompaniesRows(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	var original map[string]any
	require.NoError(t, json.Unmarshal(rows[0], &original))
	original["id"] = "99999999-9999-9999-9999-999999999999"
	original["name"] = "Umbenannt"

	mutated, err := json.Marshal(original)
	require.NoError(t, err)
	require.NoError(t, q.UpsertCompaniesRow(ctx, mutated))

	after, err := q.ExportCompaniesRows(ctx)
	require.NoError(t, err)
	require.Len(t, after, len(rows)+1, "a different primary key is a new row")
}

// TestDemoBackupRowsAcceptMinimalRowsForEveryStoredTable exercises the upsert
// path of every stored table. Several demo tables have no fixtures, so their
// upsert would otherwise never run.
func TestDemoBackupRowsAcceptMinimalRowsForEveryStoredTable(t *testing.T) {
	ctx := context.Background()
	q := demo.NewInMemoryQuerier()

	const id = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	minimal := map[string]string{
		"users":                   `{"id":"` + id + `","email":"minimal@example.de","password_hash":"h","first_name":"A","last_name":"B","role":"ADMIN","status":"ACTIVE","totp_enabled":false,"totp_last_used_step":0}`,
		"companies":               `{"id":"` + id + `","name":"Minimal"}`,
		"workflows":               `{"id":"minimal-wf","name":"WF","trigger_type":"MANUAL","target_type":"CONTACT"}`,
		"contacts":                `{"id":"` + id + `","first_name":"A","last_name":"B"}`,
		"deals":                   `{"id":"` + id + `","title":"T"}`,
		"email_accounts":          `{"id":"` + id + `","name":"EA","email_address":"ea@example.de"}`,
		"todos":                   `{"id":"` + id + `","title":"T"}`,
		"refresh_tokens":          `{"id":"` + id + `","user_id":"` + id + `","token_hash":"h"}`,
		"audit_logs":              `{"id":"` + id + `","entity_type":"USER","entity_id":"` + id + `","action":"A"}`,
		"notifications":           `{"id":"` + id + `","type":"SYSTEM","title":"t","message":"m"}`,
		"email_messages":          `{"id":"` + id + `","account_id":"` + id + `","thread_id":"t","message_id":"m","sender_email":"a@example.de","subject":"s"}`,
		"email_attachments":       `{"id":"` + id + `","message_id":"` + id + `","filename":"f","content_type":"text/plain"}`,
		"appointments":            `{"id":"` + id + `","title":"T","ics_uid":"u"}`,
		"call_activities":         `{"id":"` + id + `","disposition":"REACHED"}`,
		"workflow_runs":           `{"id":"minimal-run","workflow_id":"minimal-wf","target_id":"x","target_type":"CONTACT"}`,
		"notes":                   `{"id":"` + id + `","entity_type":"CONTACT","content":"c"}`,
		"knowledge_base_articles": `{"id":"` + id + `","title":"T","category":"c","content":"c"}`,
		"ai_research_jobs":        `{"id":"` + id + `","company_name":"C"}`,
		"user_invites":            `{"id":"` + id + `","user_id":"` + id + `","token_hash":"` + strings.Repeat("b", 64) + `","expires_at":"2027-01-01T00:00:00Z"}`,
		"email_templates":         `{"id":"` + id + `","key":"minimal_template","name":"N","subject":"S","body":"B"}`,
	}

	for _, entry := range demoTables() {
		row, ok := minimal[entry.table]
		if !ok {
			require.False(t, entry.stored, "%s has no stored row fixture", entry.table)
			continue
		}
		require.True(t, entry.stored, entry.table)
		require.NoError(t, entry.upsert(q, ctx, []byte(row)), entry.table)
	}

	// The minimal rows must be readable straight back out of the demo store.
	for table, want := range map[string]int{
		"users": 1, "companies": 1, "workflows": 2, "contacts": 1, "deals": 1,
		"email_accounts": 1, "todos": 1, "refresh_tokens": 1, "audit_logs": 1,
		"notifications": 1, "email_messages": 1, "email_attachments": 1,
		"appointments": 1, "call_activities": 1, "workflow_runs": 1, "notes": 1,
		"knowledge_base_articles": 1, "ai_research_jobs": 1, "user_invites": 1,
		"email_templates": 1,
	} {
		found := false
		for _, entry := range demoTables() {
			if entry.table != table {
				continue
			}
			rows, err := entry.export(q, ctx)
			require.NoError(t, err)
			// workflows is seeded by the demo fixtures, so it holds more than
			// the row added here.
			require.GreaterOrEqual(t, len(rows), want, table)
			found = true
		}
		require.True(t, found, "no export method registered for %s", table)
	}
}

func TestDemoTruncateBackupTablesClearsEveryStoredTable(t *testing.T) {
	ctx := context.Background()
	q := demo.NewInMemoryQuerier()
	seedInvitesAndTemplates(t, ctx, q)

	// Only the tables the demo actually seeds can be non-empty beforehand; a
	// table the demo never populates is trivially empty after a truncate.
	var seeded int
	for _, entry := range demoTables() {
		if !entry.stored {
			continue
		}
		before, err := entry.export(q, ctx)
		require.NoError(t, err)
		if len(before) > 0 {
			seeded++
		}
	}
	require.Greater(t, seeded, 10)

	require.NoError(t, q.TruncateBackupTables(ctx))

	for _, entry := range demoTables() {
		after, err := entry.export(q, ctx)
		require.NoError(t, err)
		require.Empty(t, after, "truncate must clear %s", entry.table)
	}
}
