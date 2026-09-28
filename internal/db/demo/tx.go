package demo

import (
	"context"
	"maps"
	"slices"

	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// TxRunner is the transactional capability a service can depend on. It is
// declared in both packages so either can be satisfied and asserted at compile
// time, and so internal/auth depends on neither pgx nor the demo package.
type TxRunner interface {
	RunTx(ctx context.Context, fn func(db.Querier) error) error
}

var _ TxRunner = (*InMemoryQuerier)(nil)

// RunTx gives the in-memory querier all-or-nothing semantics without a pool.
//
// It deliberately does not call fn(q). q.mu is a sync.RWMutex and every method
// takes it, so re-entering it from inside the callback would deadlock. Instead
// the callback runs against a shadow clone with its own mutex and its own copy
// of every map and slice. The original is untouched until the callback
// succeeds, so rollback is exact and needs no restore step, and no concurrent
// reader can observe a half-applied transaction.
func (q *InMemoryQuerier) RunTx(ctx context.Context, fn func(db.Querier) error) error {
	// Held for the whole callback so the transaction is serialised against
	// other writers and against itself; never re-entered, because fn works on
	// the shadow.
	q.mu.Lock()
	defer q.mu.Unlock()

	shadow := q.cloneForTransaction()
	if err := fn(shadow); err != nil {
		// q was never written, so there is nothing to restore.
		return err
	}
	q.adoptFrom(shadow)
	return nil
}

// cloneForTransaction builds the shadow. It constructs a new value rather than
// copying *q, because copying the struct would copy the mutex and trip
// go vet's copylocks check.
func (q *InMemoryQuerier) cloneForTransaction() *InMemoryQuerier {
	return &InMemoryQuerier{
		users:          maps.Clone(q.users),
		companies:      maps.Clone(q.companies),
		contacts:       maps.Clone(q.contacts),
		deals:          maps.Clone(q.deals),
		todos:          maps.Clone(q.todos),
		notifications:  maps.Clone(q.notifications),
		emailAccounts:  maps.Clone(q.emailAccounts),
		emailMessages:  maps.Clone(q.emailMessages),
		emailAttachs:   maps.Clone(q.emailAttachs),
		refreshTokens:  maps.Clone(q.refreshTokens),
		appointments:   maps.Clone(q.appointments),
		notes:          maps.Clone(q.notes),
		callActivities: maps.Clone(q.callActivities),
		kbArticles:     maps.Clone(q.kbArticles),
		aiResearchJobs: maps.Clone(q.aiResearchJobs),
		workflows:      maps.Clone(q.workflows),
		workflowRuns:   maps.Clone(q.workflowRuns),
		userInvites:    maps.Clone(q.userInvites),
		emailTemplates: maps.Clone(q.emailTemplates),
		// auditLogs is a slice, not a map: a map-only snapshot would leave every
		// audit row written inside an aborted transaction in place.
		auditLogs: slices.Clone(q.auditLogs),
	}
}

// adoptFrom copies the shadow's contents into q. The caller holds q.mu.
func (q *InMemoryQuerier) adoptFrom(s *InMemoryQuerier) {
	q.users = s.users
	q.companies = s.companies
	q.contacts = s.contacts
	q.deals = s.deals
	q.todos = s.todos
	q.notifications = s.notifications
	q.emailAccounts = s.emailAccounts
	q.emailMessages = s.emailMessages
	q.emailAttachs = s.emailAttachs
	q.refreshTokens = s.refreshTokens
	q.appointments = s.appointments
	q.notes = s.notes
	q.callActivities = s.callActivities
	q.kbArticles = s.kbArticles
	q.aiResearchJobs = s.aiResearchJobs
	q.workflows = s.workflows
	q.workflowRuns = s.workflowRuns
	q.userInvites = s.userInvites
	q.emailTemplates = s.emailTemplates
	q.auditLogs = s.auditLogs
}

// TransactionCollectionFields returns the names of the reference-typed fields
// that cloneForTransaction and adoptFrom must both cover. It exists for the
// coverage test; keep it in step with the two functions above.
func TransactionCollectionFields() []string {
	return []string{
		"users", "companies", "contacts", "deals", "todos", "notifications",
		"emailAccounts", "emailMessages", "emailAttachs", "refreshTokens",
		"appointments", "notes", "callActivities", "kbArticles",
		"aiResearchJobs", "workflows", "workflowRuns", "userInvites",
		"emailTemplates", "auditLogs",
	}
}

// AuditLogsForTest returns a copy of auditLogs for testing rollback.
func (q *InMemoryQuerier) AuditLogsForTest() []db.AuditLog {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return slices.Clone(q.auditLogs)
}
