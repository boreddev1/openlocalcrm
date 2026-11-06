package demo

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// The methods in this file satisfy the generated db.Querier contract for the
// backup queries. They are the demo-mode mirror of the canonical SQL in
// internal/db/queries/backup.sql: same table set, same primary-key ordering,
// same ON CONFLICT behavior. The bounded-memory export never uses them: it
// goes through the ScanBackup* methods in backup_stream.go, which stream one
// row at a time.

var errDemoTableNotStored = errors.New("demo mode does not store this table")

// demoExportRows marshals a demo table in primary-key order, matching the
// `ORDER BY t.id` of the canonical selector.
func demoExportRows[V any](rows map[string]V) ([][]byte, error) {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([][]byte, 0, len(keys))
	for _, key := range keys {
		raw, err := json.Marshal(rows[key])
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

// demoUpsertRow applies one restore row to a demo table, mirroring
// `ON CONFLICT (pk) DO UPDATE`.
func demoUpsertRow[V any](raw []byte, keyOf func(V) (string, bool), rows map[string]V) error {
	var value V
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	key, ok := keyOf(value)
	if !ok || key == "" {
		return errors.New("backup row is missing its primary key")
	}
	rows[key] = value
	return nil
}

// Conflict keys per table. UUID-keyed tables conflict on their primary key;
// workflows and workflow_runs keep their text primary key. email_templates
// conflicts on `key` and is handled by its own method in querier.go.

func keyUser(v db.User) (string, bool)                 { return uuidToStr(v.ID), v.ID.Valid }
func keyCompany(v db.Company) (string, bool)           { return uuidToStr(v.ID), v.ID.Valid }
func keyContact(v db.Contact) (string, bool)           { return uuidToStr(v.ID), v.ID.Valid }
func keyDeal(v db.Deal) (string, bool)                 { return uuidToStr(v.ID), v.ID.Valid }
func keyEmailAccount(v db.EmailAccount) (string, bool) { return uuidToStr(v.ID), v.ID.Valid }
func keyTodo(v db.Todo) (string, bool)                 { return uuidToStr(v.ID), v.ID.Valid }
func keyRefreshToken(v db.RefreshToken) (string, bool) { return uuidToStr(v.ID), v.ID.Valid }
func keyNotification(v db.Notification) (string, bool) { return uuidToStr(v.ID), v.ID.Valid }
func keyEmailMessage(v db.EmailMessage) (string, bool) { return uuidToStr(v.ID), v.ID.Valid }
func keyEmailAttachment(v db.EmailAttachment) (string, bool) {
	return uuidToStr(v.ID), v.ID.Valid
}
func keyAppointment(v db.Appointment) (string, bool)   { return uuidToStr(v.ID), v.ID.Valid }
func keyCallActivity(v db.CallActivity) (string, bool) { return uuidToStr(v.ID), v.ID.Valid }
func keyNote(v db.Note) (string, bool)                 { return uuidToStr(v.ID), v.ID.Valid }
func keyArticle(v db.KnowledgeBaseArticle) (string, bool) {
	return uuidToStr(v.ID), v.ID.Valid
}
func keyResearchJob(v db.AiResearchJob) (string, bool) { return uuidToStr(v.ID), v.ID.Valid }
func keyWorkflow(v db.Workflow) (string, bool)         { return v.ID, v.ID != "" }
func keyWorkflowRun(v db.WorkflowRun) (string, bool)   { return v.ID, v.ID != "" }

// --- users ---
func (q *InMemoryQuerier) ExportUsersRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.users)
}

func (q *InMemoryQuerier) UpsertUsersRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyUser, q.users)
}

// --- companies ---
func (q *InMemoryQuerier) ExportCompaniesRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.companies)
}

func (q *InMemoryQuerier) UpsertCompaniesRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyCompany, q.companies)
}

// --- workflows (text primary key) ---
func (q *InMemoryQuerier) ExportWorkflowsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.workflows)
}

func (q *InMemoryQuerier) UpsertWorkflowsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyWorkflow, q.workflows)
}

// --- contacts ---
func (q *InMemoryQuerier) ExportContactsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.contacts)
}

func (q *InMemoryQuerier) UpsertContactsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyContact, q.contacts)
}

// --- deals ---
func (q *InMemoryQuerier) ExportDealsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.deals)
}

func (q *InMemoryQuerier) UpsertDealsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyDeal, q.deals)
}

// --- email accounts ---
func (q *InMemoryQuerier) ExportEmailAccountsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.emailAccounts)
}

func (q *InMemoryQuerier) UpsertEmailAccountsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyEmailAccount, q.emailAccounts)
}

// --- todos ---
func (q *InMemoryQuerier) ExportTodosRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.todos)
}

func (q *InMemoryQuerier) UpsertTodosRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyTodo, q.todos)
}

// --- refresh tokens ---
func (q *InMemoryQuerier) ExportRefreshTokensRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.refreshTokens)
}

func (q *InMemoryQuerier) UpsertRefreshTokensRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyRefreshToken, q.refreshTokens)
}

// --- audit logs (append-only slice, not a keyed map) ---
func (q *InMemoryQuerier) ExportAuditLogsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	sorted := make([]db.AuditLog, len(q.auditLogs))
	copy(sorted, q.auditLogs)
	sort.Slice(sorted, func(i, j int) bool {
		return uuidToStr(sorted[i].ID) < uuidToStr(sorted[j].ID)
	})

	out := make([][]byte, 0, len(sorted))
	for _, entry := range sorted {
		raw, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

func (q *InMemoryQuerier) UpsertAuditLogsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	var entry db.AuditLog
	if err := json.Unmarshal(row, &entry); err != nil {
		return err
	}
	id := uuidToStr(entry.ID)
	if id == "" {
		return errors.New("backup row is missing its primary key")
	}
	for i := range q.auditLogs {
		if uuidToStr(q.auditLogs[i].ID) == id {
			// ON CONFLICT (id) DO UPDATE
			q.auditLogs[i] = entry
			return nil
		}
	}
	q.auditLogs = append(q.auditLogs, entry)
	return nil
}

// --- notifications ---
func (q *InMemoryQuerier) ExportNotificationsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.notifications)
}

func (q *InMemoryQuerier) UpsertNotificationsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyNotification, q.notifications)
}

// --- email messages ---
func (q *InMemoryQuerier) ExportEmailMessagesRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.emailMessages)
}

func (q *InMemoryQuerier) UpsertEmailMessagesRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyEmailMessage, q.emailMessages)
}

// --- email attachments ---
func (q *InMemoryQuerier) ExportEmailAttachmentsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.emailAttachs)
}

func (q *InMemoryQuerier) UpsertEmailAttachmentsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyEmailAttachment, q.emailAttachs)
}

// --- appointments ---
func (q *InMemoryQuerier) ExportAppointmentsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.appointments)
}

func (q *InMemoryQuerier) UpsertAppointmentsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyAppointment, q.appointments)
}

// --- call activities ---
func (q *InMemoryQuerier) ExportCallActivitiesRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.callActivities)
}

func (q *InMemoryQuerier) UpsertCallActivitiesRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyCallActivity, q.callActivities)
}

// --- company research: no demo fixture ---
func (q *InMemoryQuerier) ExportCompanyResearchRows(ctx context.Context) ([][]byte, error) {
	return [][]byte{}, nil
}

func (q *InMemoryQuerier) UpsertCompanyResearchRow(ctx context.Context, row []byte) error {
	return errDemoTableNotStored
}

// --- workflow runs (text primary key) ---
func (q *InMemoryQuerier) ExportWorkflowRunsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.workflowRuns)
}

func (q *InMemoryQuerier) UpsertWorkflowRunsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyWorkflowRun, q.workflowRuns)
}

// --- workflow steps: no demo fixture ---
func (q *InMemoryQuerier) ExportWorkflowStepsRows(ctx context.Context) ([][]byte, error) {
	return [][]byte{}, nil
}

func (q *InMemoryQuerier) UpsertWorkflowStepsRow(ctx context.Context, row []byte) error {
	return errDemoTableNotStored
}

// --- notes ---
func (q *InMemoryQuerier) ExportNotesRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.notes)
}

func (q *InMemoryQuerier) UpsertNotesRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyNote, q.notes)
}

// --- intake forms: no demo fixture ---
func (q *InMemoryQuerier) ExportIntakeFormsRows(ctx context.Context) ([][]byte, error) {
	return [][]byte{}, nil
}

func (q *InMemoryQuerier) UpsertIntakeFormsRow(ctx context.Context, row []byte) error {
	return errDemoTableNotStored
}

// --- knowledge base articles ---
func (q *InMemoryQuerier) ExportKnowledgeBaseArticlesRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.kbArticles)
}

func (q *InMemoryQuerier) UpsertKnowledgeBaseArticlesRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyArticle, q.kbArticles)
}

// --- AI research jobs ---
func (q *InMemoryQuerier) ExportAIResearchJobsRows(ctx context.Context) ([][]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return demoExportRows(q.aiResearchJobs)
}

func (q *InMemoryQuerier) UpsertAIResearchJobsRow(ctx context.Context, row []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return demoUpsertRow(row, keyResearchJob, q.aiResearchJobs)
}

// --- AI audit logs: no demo fixture ---
func (q *InMemoryQuerier) ExportAIAuditLogsRows(ctx context.Context) ([][]byte, error) {
	return [][]byte{}, nil
}

func (q *InMemoryQuerier) UpsertAIAuditLogsRow(ctx context.Context, row []byte) error {
	return errDemoTableNotStored
}

// TruncateBackupTables clears every demo table, mirroring the replace-mode
// TRUNCATE ... RESTART IDENTITY CASCADE.
func (q *InMemoryQuerier) TruncateBackupTables(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	clear(q.users)
	clear(q.companies)
	clear(q.workflows)
	clear(q.contacts)
	clear(q.deals)
	clear(q.emailAccounts)
	clear(q.todos)
	clear(q.refreshTokens)
	q.auditLogs = q.auditLogs[:0]
	clear(q.notifications)
	clear(q.emailMessages)
	clear(q.emailAttachs)
	clear(q.appointments)
	clear(q.callActivities)
	clear(q.workflowRuns)
	clear(q.notes)
	clear(q.kbArticles)
	clear(q.aiResearchJobs)
	clear(q.userInvites)
	clear(q.emailTemplates)
	return nil
}
