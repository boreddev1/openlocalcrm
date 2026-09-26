package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
)

// BackupRowSource is the streaming read boundary for the backup export and the
// restore drill. It exists in package db because the handwritten scanners that
// implement it need access to the package-private sqlc statement constants,
// which package db cannot expose any other way without materializing rows.
//
// Neither *Queries nor the demo querier implement it with a generated :many
// method: every method walks a `rows.Next()` loop and hands one row to the sink.
type BackupRowSource interface {
	ScanBackupUsers(ctx context.Context, sink RowSink) error
	ScanBackupCompanies(ctx context.Context, sink RowSink) error
	ScanBackupWorkflows(ctx context.Context, sink RowSink) error
	ScanBackupContacts(ctx context.Context, sink RowSink) error
	ScanBackupDeals(ctx context.Context, sink RowSink) error
	ScanBackupEmailAccounts(ctx context.Context, sink RowSink) error
	ScanBackupTodos(ctx context.Context, sink RowSink) error
	ScanBackupRefreshTokens(ctx context.Context, sink RowSink) error
	ScanBackupAuditLogs(ctx context.Context, sink RowSink) error
	ScanBackupNotifications(ctx context.Context, sink RowSink) error
	ScanBackupEmailMessages(ctx context.Context, sink RowSink) error
	ScanBackupEmailAttachments(ctx context.Context, sink RowSink) error
	ScanBackupAppointments(ctx context.Context, sink RowSink) error
	ScanBackupCallActivities(ctx context.Context, sink RowSink) error
	ScanBackupCompanyResearch(ctx context.Context, sink RowSink) error
	ScanBackupWorkflowRuns(ctx context.Context, sink RowSink) error
	ScanBackupWorkflowSteps(ctx context.Context, sink RowSink) error
	ScanBackupNotes(ctx context.Context, sink RowSink) error
	ScanBackupIntakeForms(ctx context.Context, sink RowSink) error
	ScanBackupUserInvites(ctx context.Context, sink RowSink) error
	ScanBackupEmailTemplates(ctx context.Context, sink RowSink) error
	ScanBackupKnowledgeBaseArticles(ctx context.Context, sink RowSink) error
	ScanBackupAIResearchJobs(ctx context.Context, sink RowSink) error
	ScanBackupAIAuditLogs(ctx context.Context, sink RowSink) error
}

var _ BackupRowSource = (*Queries)(nil)

// RowDigest accumulates a SHA-256 digest and a row count over row_to_json
// output, without retaining the rows. Two tables with the same digest contain
// the same rows in the same order, byte for byte, which is what makes it a
// usable equality proof for a backup round trip.
type RowDigest struct {
	hash  hash.Hash
	count int64
}

// NewRowDigest returns an empty digest accumulator.
func NewRowDigest() *RowDigest {
	return &RowDigest{hash: sha256.New()}
}

// Add folds one row into the digest. Rows must be added in the order the
// selector produced them.
func (d *RowDigest) Add(row []byte) {
	d.count++
	// A separator keeps ["a","b"] from colliding with ["ab"].
	d.hash.Write(row)
	d.hash.Write([]byte{'\n'})
}

// Count returns how many rows were added.
func (d *RowDigest) Count() int64 { return d.count }

// Sum returns the hex-encoded digest.
func (d *RowDigest) Sum() string { return hex.EncodeToString(d.hash.Sum(nil)) }

// DigestRows digests a materialised row slice with the same algorithm, so a
// caller can predict the digest of a streamed table.
func DigestRows(rows [][]byte) (int64, string) {
	digest := NewRowDigest()
	for _, row := range rows {
		digest.Add(row)
	}
	return digest.Count(), digest.Sum()
}

// BackupTableDigest streams one table through its canonical selector and
// returns its row count and digest. It never holds a whole table in memory.
func BackupTableDigest(ctx context.Context, q *Queries, table string) (int64, string, error) {
	statement, ok := BackupTableStatements[table]
	if !ok {
		return 0, "", fmt.Errorf("%w: %q", ErrUnknownBackupTable, table)
	}
	digest := NewRowDigest()
	if err := scanBackupRows(ctx, q, statement, func(_ context.Context, row json.RawMessage) error {
		digest.Add(row)
		return nil
	}); err != nil {
		return 0, "", err
	}
	return digest.Count(), digest.Sum(), nil
}
