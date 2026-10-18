package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// RowSink receives one exported row at a time as raw JSON. The backup export
// calls it straight from a `rows.Next()` loop, so no table is ever accumulated
// as a []T or a [][]byte slice.
type RowSink func(ctx context.Context, row json.RawMessage) error

// ErrNilBackupRowSink is returned when a caller passes no sink to a scanner.
var ErrNilBackupRowSink = errors.New("db: backup row sink is nil")

// ErrUnknownBackupTable is returned for a table name outside the inventory.
// Identifiers are never assembled from caller input.
var ErrUnknownBackupTable = errors.New("db: unknown backup table")

// BackupTableStatements maps each backup table to its canonical
// `row_to_json` selector. The statement constants are package-private and
// sqlc-owned, so this map is how callers reach them without the generated
// `:many` methods, which materialize every row into a slice.
var BackupTableStatements = map[string]string{
	"users":                   exportUsersRows,
	"companies":               exportCompaniesRows,
	"workflows":               exportWorkflowsRows,
	"contacts":                exportContactsRows,
	"deals":                   exportDealsRows,
	"email_accounts":          exportEmailAccountsRows,
	"todos":                   exportTodosRows,
	"refresh_tokens":          exportRefreshTokensRows,
	"audit_logs":              exportAuditLogsRows,
	"notifications":           exportNotificationsRows,
	"email_messages":          exportEmailMessagesRows,
	"email_attachments":       exportEmailAttachmentsRows,
	"appointments":            exportAppointmentsRows,
	"call_activities":         exportCallActivitiesRows,
	"company_research":        exportCompanyResearchRows,
	"workflow_runs":           exportWorkflowRunsRows,
	"workflow_steps":          exportWorkflowStepsRows,
	"notes":                   exportNotesRows,
	"intake_forms":            exportIntakeFormsRows,
	"user_invites":            exportUserInvitesRows,
	"email_templates":         exportEmailTemplatesRows,
	"knowledge_base_articles": exportKnowledgeBaseArticlesRows,
	"ai_research_jobs":        exportAIResearchJobsRows,
	"ai_audit_logs":           exportAIAuditLogsRows,
}

// scanBackupRows streams one `row_to_json` result per row into sink. It never
// appends to a slice, so memory stays flat no matter how large the table is.
func scanBackupRows(ctx context.Context, q *Queries, statement string, sink RowSink) error {
	if sink == nil {
		return ErrNilBackupRowSink
	}
	rows, err := q.db.Query(ctx, statement)
	if err != nil {
		return fmt.Errorf("db: backup scan failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return fmt.Errorf("db: backup scan failed: %w", err)
		}
		// The scanner may reuse the buffer, so each row gets its own copy.
		if err := sink(ctx, json.RawMessage(append([]byte(nil), raw...))); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("db: backup scan failed: %w", err)
	}
	return nil
}

// ScanBackupTable streams one table by name through its canonical selector.
// Unknown table names are rejected: identifiers are never built from input.
func (q *Queries) ScanBackupTable(ctx context.Context, table string, sink RowSink) error {
	statement, ok := BackupTableStatements[table]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownBackupTable, table)
	}
	return scanBackupRows(ctx, q, statement, sink)
}

// ReadBackupTable materializes a single table's rows. It exists for
// verification and diagnostics only; the export path uses ScanBackupTable so it
// never holds a whole table in memory.
func ReadBackupTable(ctx context.Context, q *Queries, table string) ([][]byte, error) {
	statement, ok := BackupTableStatements[table]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownBackupTable, table)
	}
	rows, err := q.db.Query(ctx, statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out [][]byte
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, append([]byte(nil), raw...))
	}
	return out, rows.Err()
}

// BackupTableNames returns every table name in the backup inventory, sorted.
func BackupTableNames() []string {
	names := make([]string, 0, len(BackupTableStatements))
	for name := range BackupTableStatements {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (q *Queries) ScanBackupUsers(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportUsersRows, sink)
}

func (q *Queries) ScanBackupCompanies(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportCompaniesRows, sink)
}

func (q *Queries) ScanBackupWorkflows(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportWorkflowsRows, sink)
}

func (q *Queries) ScanBackupContacts(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportContactsRows, sink)
}

func (q *Queries) ScanBackupDeals(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportDealsRows, sink)
}

func (q *Queries) ScanBackupEmailAccounts(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportEmailAccountsRows, sink)
}

func (q *Queries) ScanBackupTodos(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportTodosRows, sink)
}

func (q *Queries) ScanBackupRefreshTokens(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportRefreshTokensRows, sink)
}

func (q *Queries) ScanBackupAuditLogs(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportAuditLogsRows, sink)
}

func (q *Queries) ScanBackupNotifications(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportNotificationsRows, sink)
}

func (q *Queries) ScanBackupEmailMessages(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportEmailMessagesRows, sink)
}

func (q *Queries) ScanBackupEmailAttachments(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportEmailAttachmentsRows, sink)
}

func (q *Queries) ScanBackupAppointments(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportAppointmentsRows, sink)
}

func (q *Queries) ScanBackupCallActivities(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportCallActivitiesRows, sink)
}

func (q *Queries) ScanBackupCompanyResearch(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportCompanyResearchRows, sink)
}

func (q *Queries) ScanBackupWorkflowRuns(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportWorkflowRunsRows, sink)
}

func (q *Queries) ScanBackupWorkflowSteps(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportWorkflowStepsRows, sink)
}

func (q *Queries) ScanBackupNotes(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportNotesRows, sink)
}

func (q *Queries) ScanBackupIntakeForms(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportIntakeFormsRows, sink)
}

func (q *Queries) ScanBackupUserInvites(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportUserInvitesRows, sink)
}

func (q *Queries) ScanBackupEmailTemplates(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportEmailTemplatesRows, sink)
}

func (q *Queries) ScanBackupKnowledgeBaseArticles(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportKnowledgeBaseArticlesRows, sink)
}

func (q *Queries) ScanBackupAIResearchJobs(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportAIResearchJobsRows, sink)
}

func (q *Queries) ScanBackupAIAuditLogs(ctx context.Context, sink RowSink) error {
	return scanBackupRows(ctx, q, exportAIAuditLogsRows, sink)
}
