package backup

import (
	"context"
	"encoding/json"

	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

const (
	backupFormat  = "openlocalcrm-backup"
	backupVersion = 1
)

// UpsertFunc writes one validated row through the static sqlc upsert for its
// table. The statement is fixed SQL, so no identifier is ever taken from the
// uploaded file.
type UpsertFunc func(ctx context.Context, queries db.Querier, row json.RawMessage) error

// StreamFunc streams one table's rows from a RowSource.
type StreamFunc func(ctx context.Context, source RowSource, sink RowSink) error

// tableSpec is the single registry entry per backup table. It carries the
// foreign-key-safe position in the inventory, the read and write seams, the
// primary key, and the column allowlist derived from the static upsert's
// jsonb_to_record fields.
type tableSpec struct {
	Name    string
	PK      string
	Columns []string
	Stream  StreamFunc
	Upsert  UpsertFunc
}

func (s *tableSpec) allows(column string) bool {
	for _, allowed := range s.Columns {
		if allowed == column {
			return true
		}
	}
	return false
}

// tableSpecs is the FK-safe insertion order from the approved design. The
// reverse of this order is the replace-mode truncation order.
//
// schema_migrations is deliberately absent: it is infrastructure metadata, not
// application data, and restoring it would lie about which migrations have run.
var tableSpecs = []tableSpec{
	{
		Name:    "users",
		PK:      "id",
		Columns: userColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupUsers(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertUsersRow(ctx, row)
		},
	},
	{
		Name:    "companies",
		PK:      "id",
		Columns: companyColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupCompanies(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertCompaniesRow(ctx, row)
		},
	},
	{
		Name:    "workflows",
		PK:      "id",
		Columns: workflowColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupWorkflows(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertWorkflowsRow(ctx, row)
		},
	},
	{
		Name:    "contacts",
		PK:      "id",
		Columns: contactColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupContacts(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertContactsRow(ctx, row)
		},
	},
	{
		Name:    "deals",
		PK:      "id",
		Columns: dealColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupDeals(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertDealsRow(ctx, row)
		},
	},
	{
		Name:    "email_accounts",
		PK:      "id",
		Columns: emailAccountColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupEmailAccounts(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertEmailAccountsRow(ctx, row)
		},
	},
	{
		Name:    "todos",
		PK:      "id",
		Columns: todoColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupTodos(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertTodosRow(ctx, row)
		},
	},
	{
		Name:    "refresh_tokens",
		PK:      "id",
		Columns: refreshTokenColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupRefreshTokens(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertRefreshTokensRow(ctx, row)
		},
	},
	{
		Name:    "audit_logs",
		PK:      "id",
		Columns: auditLogColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupAuditLogs(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertAuditLogsRow(ctx, row)
		},
	},
	{
		Name:    "notifications",
		PK:      "id",
		Columns: notificationColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupNotifications(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertNotificationsRow(ctx, row)
		},
	},
	{
		Name:    "email_messages",
		PK:      "id",
		Columns: emailMessageColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupEmailMessages(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertEmailMessagesRow(ctx, row)
		},
	},
	{
		Name:    "email_attachments",
		PK:      "id",
		Columns: emailAttachmentColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupEmailAttachments(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertEmailAttachmentsRow(ctx, row)
		},
	},
	{
		Name:    "appointments",
		PK:      "id",
		Columns: appointmentColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupAppointments(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertAppointmentsRow(ctx, row)
		},
	},
	{
		Name:    "call_activities",
		PK:      "id",
		Columns: callActivityColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupCallActivities(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertCallActivitiesRow(ctx, row)
		},
	},
	{
		Name:    "company_research",
		PK:      "id",
		Columns: companyResearchColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupCompanyResearch(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertCompanyResearchRow(ctx, row)
		},
	},
	{
		Name:    "workflow_runs",
		PK:      "id",
		Columns: workflowRunColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupWorkflowRuns(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertWorkflowRunsRow(ctx, row)
		},
	},
	{
		Name:    "workflow_steps",
		PK:      "id",
		Columns: workflowStepColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupWorkflowSteps(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertWorkflowStepsRow(ctx, row)
		},
	},
	{
		Name:    "notes",
		PK:      "id",
		Columns: noteColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupNotes(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertNotesRow(ctx, row)
		},
	},
	{
		Name:    "intake_forms",
		PK:      "id",
		Columns: intakeFormColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupIntakeForms(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertIntakeFormsRow(ctx, row)
		},
	},
	{
		Name:    "user_invites",
		PK:      "id",
		Columns: userInviteColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupUserInvites(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertUserInvitesRow(ctx, row)
		},
	},
	{
		Name:    "email_templates",
		PK:      "id",
		Columns: emailTemplateColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupEmailTemplates(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertEmailTemplatesRow(ctx, row)
		},
	},
	{
		Name:    "knowledge_base_articles",
		PK:      "id",
		Columns: knowledgeBaseArticleColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupKnowledgeBaseArticles(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertKnowledgeBaseArticlesRow(ctx, row)
		},
	},
	{
		Name:    "ai_research_jobs",
		PK:      "id",
		Columns: aiResearchJobColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupAIResearchJobs(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertAIResearchJobsRow(ctx, row)
		},
	},
	{
		Name:    "ai_audit_logs",
		PK:      "id",
		Columns: aiAuditLogColumns,
		Stream: func(ctx context.Context, source RowSource, sink RowSink) error {
			return source.ScanBackupAIAuditLogs(ctx, sink)
		},
		Upsert: func(ctx context.Context, q db.Querier, row json.RawMessage) error {
			return q.UpsertAIAuditLogsRow(ctx, row)
		},
	},
}

// TableColumns returns the restore allow-list for one backup table.
func TableColumns(table string) []string {
	for _, spec := range tableSpecs {
		if spec.Name == table {
			return spec.Columns
		}
	}
	return nil
}

// The column allowlists below mirror the jsonb_to_record fields of the matching
// Upsert*Row statement in internal/db/queries/backup.sql, invites.sql and
// email_templates.sql. They are the restore validation contract: a column the
// static statement would silently drop is rejected up front instead of being
// reported as imported.

var userColumns = []string{
	"id", "email", "password_hash", "first_name", "last_name", "role", "status",
	"totp_secret_encrypted", "totp_enabled", "last_login_at", "created_at", "updated_at",
	"totp_last_used_step", "token_version", "must_change_password",
}

var companyColumns = []string{
	"id", "name", "domain", "phone", "email", "address_street", "address_zip",
	"address_city", "address_country", "custom_fields", "created_at", "updated_at",
	"owner_user_id",
}

var workflowColumns = []string{
	"id", "name", "description", "trigger_type", "target_type", "is_active",
	"steps_json", "created_at", "updated_at",
}

var contactColumns = []string{
	"id", "company_id", "first_name", "last_name", "email", "phone", "mobile", "position",
	"lead_source", "address_street", "address_zip", "address_city", "latitude", "longitude",
	"custom_fields", "created_at", "updated_at", "consent_phone", "consent_email",
	"consent_updated_at", "stromverbrauch_kwh", "gasverbrauch_kwh", "zaehlernummer",
	"eigentuemer_status", "tags", "owner_user_id",
}

var dealColumns = []string{
	"id", "title", "company_id", "contact_id", "value", "currency", "stage", "probability",
	"assigned_to", "closed_at", "custom_fields", "created_at", "updated_at", "widerrufen_at",
	"widerruf_grund", "tags",
}

var emailAccountColumns = []string{
	"id", "name", "email_address", "provider", "imap_host", "imap_port", "smtp_host",
	"smtp_port", "username", "password_encrypted", "is_active", "last_synced_at", "created_at",
	"updated_at", "account_type", "owner_user_id", "last_sync_at", "last_uid",
}

var todoColumns = []string{
	"id", "title", "description", "due_date", "status", "priority", "assigned_to",
	"contact_id", "deal_id", "created_at", "updated_at",
}

var refreshTokenColumns = []string{
	"id", "user_id", "token_hash", "user_agent", "ip_address", "expires_at", "created_at",
	"family_id", "used_at",
}

var auditLogColumns = []string{
	"id", "user_id", "entity_type", "entity_id", "action", "changes", "ip_address",
	"user_agent", "created_at", "old_values", "new_values",
}

var notificationColumns = []string{
	"id", "user_id", "type", "title", "message", "link", "is_read", "created_at",
}

var emailMessageColumns = []string{
	"id", "account_id", "thread_id", "message_id", "in_reply_to", "direction", "sender_email",
	"sender_name", "recipient_emails", "subject", "body_text", "body_html", "received_at",
	"is_read", "contact_id", "deal_id", "created_at", "tags", "is_draft",
}

var emailAttachmentColumns = []string{
	"id", "message_id", "filename", "content_type", "size_bytes", "storage_path", "created_at",
}

var appointmentColumns = []string{
	"id", "title", "contact_id", "company_id", "deal_id", "start_time", "end_time", "location",
	"notes", "ics_uid", "created_at", "assigned_to", "type", "is_external", "provider",
	"is_private", "is_pushed", "updated_at", "user_id",
}

var callActivityColumns = []string{
	"id", "contact_id", "duration_seconds", "disposition", "notes", "created_at",
}

var companyResearchColumns = []string{
	"id", "company_id", "domain", "title", "meta_description", "summary", "industry_keywords",
	"researched_at",
}

var workflowRunColumns = []string{
	"id", "workflow_id", "target_id", "target_type", "target_name", "status", "current_step",
	"snapshot_json", "started_at", "completed_at",
}

var workflowStepColumns = []string{
	"id", "run_id", "step_number", "title", "action_type", "payload_json", "status",
	"prepared_at", "executed_at",
}

var noteColumns = []string{
	"id", "entity_type", "entity_id", "type", "author", "content", "created_at", "updated_at",
	"user_id",
}

var intakeFormColumns = []string{
	"id", "name", "slug", "fields_json", "is_active", "created_at",
}

var userInviteColumns = []string{
	"id", "user_id", "token_hash", "created_by", "expires_at", "consumed_at", "revoked_at",
	"created_at",
}

var emailTemplateColumns = []string{
	"id", "key", "name", "subject", "body", "category", "locale", "created_at", "updated_at",
}

var knowledgeBaseArticleColumns = []string{
	"id", "title", "category", "content", "tags", "author", "created_at", "updated_at",
	"embedding", "embedding_model",
}

var aiResearchJobColumns = []string{
	"id", "company_name", "domain", "status", "result_json", "error_message", "created_at",
	"completed_at",
}

var aiAuditLogColumns = []string{
	"id", "interaction_type", "model_name", "provider", "prompt_tokens", "completion_tokens",
	"latency_ms", "pii_filter_triggered", "pii_redactions_count", "human_approved", "created_at",
}

func tableNames() []string {
	names := make([]string, 0, len(tableSpecs))
	for i := range tableSpecs {
		names = append(names, tableSpecs[i].Name)
	}
	return names
}

func tableByName(name string) *tableSpec {
	for i := range tableSpecs {
		if tableSpecs[i].Name == name {
			return &tableSpecs[i]
		}
	}
	return nil
}

func indexOfTable(name string) int {
	for i := range tableSpecs {
		if tableSpecs[i].Name == name {
			return i
		}
	}
	return -1
}
