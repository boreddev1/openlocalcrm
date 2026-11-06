// Package backup owns backup serialization, restore validation and ordering, the
// scratch-schema restore drill, and the audit boundaries around all three.
//
// The export path never materializes a whole table: every table is streamed one
// row at a time through the RowSource seam in package db, which walks a
// `rows.Next()` loop over the generated `row_to_json` selectors. The restore
// path validates the uploaded envelope completely before it opens a
// transaction, and the drill reads live public data through one connection
// while writing into an isolated scratch schema through another.
package backup

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// RowSink receives one exported row at a time as raw JSON.
//
// It is an alias of db.RowSink on purpose. The handwritten scanners in package
// db and the RowSource interface below must share one identical function type;
// a defined type would not satisfy the interface method set, and package db
// cannot import this package without creating an import cycle.
type RowSink = db.RowSink

// RowSource is the only read boundary for the export and the drill. Both
// *db.Queries (PostgreSQL) and *demo.InMemoryQuerier (demo mode) implement it,
// so callers never need to know which one they hold.
type RowSource = db.BackupRowSource

var _ RowSource = (*db.Queries)(nil)

// RestoreMode selects how a restore reconciles the uploaded rows with the
// live database.
type RestoreMode string

const (
	// RestoreModeMerge upserts the uploaded rows on top of the existing data.
	// It is the default because it is idempotent.
	RestoreModeMerge RestoreMode = "merge"
	// RestoreModeReplace truncates every backup table in reverse foreign-key
	// order first and then imports everything in one transaction.
	RestoreModeReplace RestoreMode = "replace"
)

// RestoreResult reports what a restore actually did.
type RestoreResult struct {
	Mode      RestoreMode
	Counts    map[string]int64
	Committed bool
}

// TableDrillResult is the per-table evidence of one drill run. SourceDigest and
// TargetDigest are SHA-256 digests over the row_to_json output in primary-key
// order, so equal digests mean every column of every row is byte-identical.
type TableDrillResult struct {
	Name         string `json:"name"`
	SourceRows   int64  `json:"source_rows"`
	TargetRows   int64  `json:"target_rows"`
	SourceDigest string `json:"source_digest"`
	TargetDigest string `json:"target_digest"`
	Verified     bool   `json:"verified"`
}

// DrillResult reports the full scratch-schema round trip.
type DrillResult struct {
	Schema      string             `json:"schema"`
	Passed      bool               `json:"passed"`
	TotalRows   int64              `json:"total_rows"`
	ForeignKeys fkRewriteStats     `json:"foreign_keys"`
	Tables      []TableDrillResult `json:"tables"`
}

var (
	// ErrRestoreRolledBack means no data was committed. The whole restore was
	// undone and the response must say committed:false.
	ErrRestoreRolledBack = errors.New("backup restore rolled back")
	// ErrRestoreAuditFailed means the data restore committed but the
	// post-commit audit insert did not. The response must say committed:true
	// so nobody re-runs a non-idempotent replace.
	ErrRestoreAuditFailed = errors.New("backup committed but audit insert failed")
	// ErrBackupUnavailable means the operation needs a real PostgreSQL pool
	// and there is none, which is the case in demo mode.
	ErrBackupUnavailable = errors.New("PostgreSQL backup operations are unavailable")
	// ErrInvalidBackup means the uploaded envelope failed validation. It is
	// always reported before any transaction work happens.
	ErrInvalidBackup = errors.New("backup envelope is invalid")
	// ErrIncompleteBackup means a replace-mode envelope is missing tables. It
	// is reported before any transaction work happens, so it is deliberately
	// not ErrRestoreRolledBack: nothing was ever opened, let alone rolled back.
	ErrIncompleteBackup = errors.New("backup envelope does not carry every table")
)

// MissingTablesError names the tables an envelope omits. Replace mode truncates
// the whole inventory first, so an envelope that carries only some tables would
// silently delete the live rows of the rest while still reporting success.
type MissingTablesError struct {
	Tables []string
}

func (e *MissingTablesError) Error() string {
	return fmt.Sprintf(
		"backup restore cannot replace: the envelope is missing %d of %d tables: %s",
		len(e.Tables), InventorySize(), strings.Join(e.Tables, ", "))
}

// Is makes errors.Is(err, ErrIncompleteBackup) true without folding the
// condition into ErrInvalidBackup, which means a malformed document.
func (e *MissingTablesError) Is(target error) bool { return target == ErrIncompleteBackup }

// missingTables returns the registry tables an envelope does not carry, in
// registry order.
func missingTables(present map[string]tableRange) []string {
	var missing []string
	for i := range tableSpecs {
		if _, ok := present[tableSpecs[i].Name]; !ok {
			missing = append(missing, tableSpecs[i].Name)
		}
	}
	return missing
}

// RequestMeta carries the real request identity into the audit trail.
type RequestMeta struct {
	ActorID   pgtype.UUID
	IPAddress string
	UserAgent string
}

// AuditLogger is the subset of audit.Service the backup flow needs. Keeping it
// an interface lets the unit tests assert exactly which events were written.
type AuditLogger interface {
	Log(ctx context.Context, userID pgtype.UUID, entityType string, entityID pgtype.UUID,
		action string, changes any, ip string, userAgent string) error
}

// backupEntityID is the synthetic entity id used for every backup audit event;
// a backup is not a row in any audited table.
var backupEntityID = pgtype.UUID{Bytes: uuid.UUID{}, Valid: true}

// Service owns the export, restore and drill orchestration. The PostgreSQL
// pool is used only for the scratch-schema lifecycle; ordinary statements go
// through the Querier and the RowSource.
type Service struct {
	source  RowSource
	audit   AuditLogger
	pool    *pgxpool.Pool
	queries db.Querier

	// scratch and targets are test seams. In production they default to the
	// real pool-backed implementations; the scratch drill and the real
	// PostgreSQL round-trip test substitute their own.
	scratch scratchManager
	targets targetRunner
	// drillHook runs inside the target transaction at the two interesting
	// points of a drill. It exists so the integration test can observe the
	// scratch schema and remove a scratch table after validation.
	drillHook func(ctx context.Context, phase drillPhase, schema string, tx pgx.Tx) error
}

// NewService wires the backup service. queries may be nil for read-only export
// use, but restore and drill need a real pool and report ErrBackupUnavailable
// without one.
func NewService(source RowSource, audit AuditLogger, pool *pgxpool.Pool, queries db.Querier) *Service {
	return &Service{
		source:  source,
		audit:   audit,
		pool:    pool,
		queries: queries,
	}
}

// TableNames returns the backup inventory in foreign-key-safe insertion order.
func TableNames() []string {
	return tableNames()
}

// InventorySize is the number of tables a complete backup carries.
//
// Every operator-facing message that quotes the inventory size must read it
// from here. A literal is a snapshot of a registry that a future migration can
// grow, and a stale number in an error message is worse than no number at all:
// the operator is being told how much data the operation would delete.
func InventorySize() int {
	return len(tableSpecs)
}

// CanExport reports whether a streaming export is possible. It is false when
// the service has no row source, which is the case for a custom db.Querier that
// does not implement the backup row source. The HTTP layer must answer 501
// rather than commit a 200 it cannot fill.
func (s *Service) CanExport() bool {
	return s != nil && s.source != nil
}

func (s *Service) logAudit(ctx context.Context, meta RequestMeta, action string, changes any) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.Log(ctx, meta.ActorID, "BACKUP", backupEntityID, action, changes,
		meta.IPAddress, meta.UserAgent)
}

// Export streams the whole backup as a versioned JSON envelope. It writes the
// metadata, then each table boundary, then each row straight to dst. No
// payload struct and no row slice is ever built, so memory stays flat.
//
// A failure after the response has started is returned to the caller, which
// aborts the HTTP response instead of completing a success document. The
// buffered writer is deliberately flushed only after the completion audit
// succeeds, so an unaudited export never reaches the client as a valid file.
func (s *Service) Export(ctx context.Context, dst io.Writer, meta RequestMeta) error {
	if s.source == nil {
		return errors.New("backup: export requires a row source")
	}

	// A failed pre-flight audit must not block the export itself.
	_ = s.logAudit(ctx, meta, "BACKUP_EXPORT_STARTED", map[string]any{"tables": len(tableSpecs)})

	buffered := bufio.NewWriter(dst)
	encoder := json.NewEncoder(buffered)

	if err := exportWriteEnvelopeHeader(buffered, encoder, meta); err != nil {
		return s.exportFailed(ctx, meta, err)
	}

	for i := range tableSpecs {
		spec := &tableSpecs[i]
		if i > 0 {
			if _, err := buffered.WriteString(","); err != nil {
				return s.exportFailed(ctx, meta, err)
			}
		}
		if err := encoder.Encode(spec.Name); err != nil {
			return s.exportFailed(ctx, meta, err)
		}
		if _, err := buffered.WriteString(":["); err != nil {
			return s.exportFailed(ctx, meta, err)
		}
		first := true
		if err := spec.Stream(ctx, s.source, func(ctx context.Context, row json.RawMessage) error {
			if !first {
				if err := buffered.WriteByte(','); err != nil {
					return err
				}
			}
			first = false
			return encoder.Encode(row)
		}); err != nil {
			return s.exportFailed(ctx, meta, err)
		}
		if _, err := buffered.WriteString("]"); err != nil {
			return s.exportFailed(ctx, meta, err)
		}
	}

	if _, err := buffered.WriteString("}}"); err != nil {
		return s.exportFailed(ctx, meta, err)
	}

	if err := s.logAudit(ctx, meta, "BACKUP_EXPORT_COMPLETED", map[string]any{"tables": len(tableSpecs)}); err != nil {
		return fmt.Errorf("backup: export finished but the audit insert failed: %w", err)
	}
	return buffered.Flush()
}

func exportWriteEnvelopeHeader(w *bufio.Writer, encoder *json.Encoder, _ RequestMeta) error {
	if _, err := w.WriteString(`{"format":"openlocalcrm-backup","version":1,"exported_at":`); err != nil {
		return err
	}
	if err := encoder.Encode(time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	_, err := w.WriteString(`,"tables":{`)
	return err
}

func (s *Service) exportFailed(ctx context.Context, meta RequestMeta, cause error) error {
	_ = s.logAudit(ctx, meta, "BACKUP_EXPORT_FAILED", map[string]any{"error": cause.Error()})
	return cause
}

// spoolBackup streams the uploaded envelope into a temporary file so the body
// is never held in memory. The caller must remove the file; Restore does so on
// every path.
func spoolBackup(body io.Reader) (*os.File, error) {
	file, err := os.CreateTemp("", "openlocalcrm-restore-*.json")
	if err != nil {
		return nil, fmt.Errorf("backup: could not create the restore spool file: %w", err)
	}
	if _, err := io.Copy(file, body); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, fmt.Errorf("backup: could not read the uploaded envelope: %w", err)
	}
	return file, nil
}
