package demo

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// The demo backup row source streams the in-memory fixtures one row at a time.
// It never builds a []json.RawMessage, so the demo mode has the same bounded
// memory profile as the PostgreSQL path. Tables without a demo map (company
// research, intake forms, AI audit logs, workflow steps) emit zero rows, which
// is what an empty table would produce from PostgreSQL as well.

// scanDemoRows streams each entry in primary-key order. The map is copied by
// the caller under the read lock, so one row is marshaled at a time and no
// []json.RawMessage is ever built.
func scanDemoRows(ctx context.Context, key string, rows map[string]any, sink db.RowSink) error {
	if sink == nil {
		return db.ErrNilBackupRowSink
	}
	keys := make([]string, 0, len(rows))
	for k := range rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		raw, err := json.Marshal(rows[k])
		if err != nil {
			return err
		}
		if err := sink(ctx, json.RawMessage(raw)); err != nil {
			return err
		}
	}
	return nil
}

func (q *InMemoryQuerier) ScanBackupUsers(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.users))
	for k, v := range q.users {
		rows[k] = v
	}
	return scanDemoRows(ctx, "users", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupCompanies(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.companies))
	for k, v := range q.companies {
		rows[k] = v
	}
	return scanDemoRows(ctx, "companies", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupWorkflows(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.workflows))
	for k, v := range q.workflows {
		rows[k] = v
	}
	return scanDemoRows(ctx, "workflows", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupContacts(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.contacts))
	for k, v := range q.contacts {
		rows[k] = v
	}
	return scanDemoRows(ctx, "contacts", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupDeals(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.deals))
	for k, v := range q.deals {
		rows[k] = v
	}
	return scanDemoRows(ctx, "deals", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupEmailAccounts(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.emailAccounts))
	for k, v := range q.emailAccounts {
		rows[k] = v
	}
	return scanDemoRows(ctx, "email_accounts", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupTodos(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.todos))
	for k, v := range q.todos {
		rows[k] = v
	}
	return scanDemoRows(ctx, "todos", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupRefreshTokens(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.refreshTokens))
	for k, v := range q.refreshTokens {
		rows[k] = v
	}
	return scanDemoRows(ctx, "refresh_tokens", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupAuditLogs(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.auditLogs))
	for i, v := range q.auditLogs {
		rows[string(rune(i))] = v
	}
	return scanDemoRows(ctx, "audit_logs", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupNotifications(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.notifications))
	for k, v := range q.notifications {
		rows[k] = v
	}
	return scanDemoRows(ctx, "notifications", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupEmailMessages(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.emailMessages))
	for k, v := range q.emailMessages {
		rows[k] = v
	}
	return scanDemoRows(ctx, "email_messages", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupEmailAttachments(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.emailAttachs))
	for k, v := range q.emailAttachs {
		rows[k] = v
	}
	return scanDemoRows(ctx, "email_attachments", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupAppointments(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.appointments))
	for k, v := range q.appointments {
		rows[k] = v
	}
	return scanDemoRows(ctx, "appointments", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupCallActivities(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.callActivities))
	for k, v := range q.callActivities {
		rows[k] = v
	}
	return scanDemoRows(ctx, "call_activities", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupCompanyResearch(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// No demo fixture for company research: an empty table is the honest answer.
	return scanDemoRows(ctx, "company_research", nil, sink)
}

func (q *InMemoryQuerier) ScanBackupWorkflowRuns(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.workflowRuns))
	for k, v := range q.workflowRuns {
		rows[k] = v
	}
	return scanDemoRows(ctx, "workflow_runs", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupWorkflowSteps(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Workflow steps are not persisted by the demo fixtures.
	return scanDemoRows(ctx, "workflow_steps", nil, sink)
}

func (q *InMemoryQuerier) ScanBackupNotes(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.notes))
	for k, v := range q.notes {
		rows[k] = v
	}
	return scanDemoRows(ctx, "notes", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupIntakeForms(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Intake forms are not persisted by the demo fixtures.
	return scanDemoRows(ctx, "intake_forms", nil, sink)
}

func (q *InMemoryQuerier) ScanBackupUserInvites(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.userInvites))
	for k, v := range q.userInvites {
		rows[k] = v
	}
	return scanDemoRows(ctx, "user_invites", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupEmailTemplates(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.emailTemplates))
	for k, v := range q.emailTemplates {
		rows[k] = v
	}
	return scanDemoRows(ctx, "email_templates", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupKnowledgeBaseArticles(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.kbArticles))
	for k, v := range q.kbArticles {
		rows[k] = v
	}
	return scanDemoRows(ctx, "knowledge_base_articles", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupAIResearchJobs(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows := make(map[string]any, len(q.aiResearchJobs))
	for k, v := range q.aiResearchJobs {
		rows[k] = v
	}
	return scanDemoRows(ctx, "ai_research_jobs", rows, sink)
}

func (q *InMemoryQuerier) ScanBackupAIAuditLogs(ctx context.Context, sink db.RowSink) error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// AI audit logs are not persisted by the demo fixtures.
	return scanDemoRows(ctx, "ai_audit_logs", nil, sink)
}

var _ db.BackupRowSource = (*InMemoryQuerier)(nil)
