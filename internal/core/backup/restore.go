package backup

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// tableRange records where a table's row array starts inside the spooled
// envelope, so the import pass can seek straight to it and run the tables in
// foreign-key-safe order no matter how the file was ordered.
type tableRange struct {
	offset int64
}

// Restore imports an uploaded backup envelope.
//
// Merge is the default: every row is upserted in the 24-entry foreign-key-safe
// order. Replace requires the explicit mode, truncates every backup table in
// reverse order first, and then imports everything inside the same
// transaction. The envelope is validated in full before any transaction work
// happens, and the spool file is deleted on every path.
//
// The completion audit is written only after the transaction committed, because
// a replace-mode import of audit_logs would otherwise delete the very event that
// documents the restore. A failure after that commit is reported as
// ErrRestoreAuditFailed with Committed set to true, so the caller can tell an
// administrative gap apart from a lost restore.
func (s *Service) Restore(
	ctx context.Context,
	body io.Reader,
	mode RestoreMode,
	meta RequestMeta,
) (result RestoreResult, err error) {
	result = RestoreResult{Mode: mode, Counts: make(map[string]int64, len(tableSpecs))}

	if mode != RestoreModeMerge && mode != RestoreModeReplace {
		return result, fmt.Errorf("%w: unsupported restore mode %q", ErrInvalidBackup, mode)
	}

	spool, err := spoolBackup(body)
	if err != nil {
		return result, err
	}
	// The spool file never outlives the call, whatever happens inside it.
	defer func() {
		_ = spool.Close()
		_ = os.Remove(spool.Name())
	}()

	// Pass one: validate the envelope, the table allowlist, the row shape and
	// every column name. Nothing touches the database until this succeeds.
	ranges, err := validateBackupFile(spool)
	if err != nil {
		return result, err
	}

	// Replace truncates the whole inventory before importing. An envelope that
	// carries only some tables would therefore delete the live rows of the rest
	// and still report success, so it is rejected here, before a transaction
	// exists. Merge never truncates, so a subset is a legitimate partial
	// import and stays allowed.
	if mode == RestoreModeReplace {
		if missing := missingTables(ranges); len(missing) > 0 {
			return result, &MissingTablesError{Tables: missing}
		}
	}

	beginner, ok := s.queries.(interface {
		Begin(context.Context) (pgx.Tx, error)
	})
	if !ok {
		return result, ErrBackupUnavailable
	}

	// A failed pre-transaction audit is reported but must not abort the
	// restore: the administrator asked for real work.
	_ = s.logAudit(ctx, meta, "BACKUP_RESTORE_STARTED", map[string]any{"mode": string(mode)})

	defer func() {
		if err != nil && !result.Committed {
			_ = s.logAudit(ctx, meta, "BACKUP_RESTORE_FAILED", map[string]any{
				"mode":  string(mode),
				"error": err.Error(),
			})
		}
	}()

	tx, err := beginner.Begin(ctx)
	if err != nil {
		err = fmt.Errorf("%w: %v", ErrRestoreRolledBack, err)
		return result, err
	}

	committed := false
	defer func() {
		if !committed {
			// A cancelled request must still release the transaction.
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	if err = applyBackupTables(ctx, spool, ranges, db.New(tx), mode, &result); err != nil {
		err = fmt.Errorf("%w: %v", ErrRestoreRolledBack, err)
		return result, err
	}

	if err = tx.Commit(ctx); err != nil {
		err = fmt.Errorf("%w: %v", ErrRestoreRolledBack, err)
		return result, err
	}
	committed = true
	result.Committed = true

	if auditErr := s.logAudit(ctx, meta, "BACKUP_RESTORE_COMPLETED", map[string]any{
		"mode":   string(mode),
		"counts": result.Counts,
	}); auditErr != nil {
		err = fmt.Errorf("%w: %v", ErrRestoreAuditFailed, auditErr)
		return result, err
	}

	return result, nil
}

// applyBackupTables runs the truncate (replace only) and then the upserts in
// registry order. Replace is not idempotent, so it is never retried.
//
// Every registry table is guaranteed to be present in ranges for replace mode:
// Restore rejects an incomplete envelope before opening a transaction, so
// skipping an absent table here can only ever happen in merge mode, where not
// truncating is exactly why the omission is harmless.
func applyBackupTables(
	ctx context.Context,
	spool *os.File,
	ranges map[string]tableRange,
	queries db.Querier,
	mode RestoreMode,
	result *RestoreResult,
) error {
	if mode == RestoreModeReplace {
		if err := queries.TruncateBackupTables(ctx); err != nil {
			return fmt.Errorf("could not truncate the backup tables: %w", err)
		}
	}

	for i := range tableSpecs {
		spec := &tableSpecs[i]
		rng, ok := ranges[spec.Name]
		if !ok {
			continue
		}
		count, err := applyTableRows(ctx, spool, spec, rng, queries)
		if err != nil {
			return err
		}
		result.Counts[spec.Name] = count
	}
	return nil
}

func applyTableRows(
	ctx context.Context,
	spool *os.File,
	spec *tableSpec,
	rng tableRange,
	queries db.Querier,
) (int64, error) {
	// The recorded offset points just past the table's '['. A fresh decoder
	// only tracks array state for delimiters it consumed itself, so step back
	// onto the '[' and read it as a token; otherwise the separator after the
	// first row is rejected as an invalid value.
	if rng.offset < 1 {
		return 0, fmt.Errorf("table %s: invalid recorded row offset", spec.Name)
	}
	if _, err := spool.Seek(rng.offset-1, io.SeekStart); err != nil {
		return 0, fmt.Errorf("table %s: could not seek to the uploaded rows: %w", spec.Name, err)
	}
	decoder := json.NewDecoder(bufio.NewReader(spool))
	if token, err := decoder.Token(); err != nil {
		return 0, fmt.Errorf("table %s: could not read the uploaded rows: %w", spec.Name, err)
	} else if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return 0, fmt.Errorf("table %s: the uploaded rows do not start with an array", spec.Name)
	}

	var count int64
	for decoder.More() {
		var row json.RawMessage
		if err := decoder.Decode(&row); err != nil {
			return count, fmt.Errorf("table %s: could not read an uploaded row: %w", spec.Name, err)
		}
		if err := spec.Upsert(ctx, queries, row); err != nil {
			return count, fmt.Errorf("table %s: could not import a row: %w", spec.Name, err)
		}
		count++
	}
	// A truncated array must never look like a successful import.
	if token, err := decoder.Token(); err != nil {
		return count, fmt.Errorf("table %s: the uploaded rows are truncated: %w", spec.Name, err)
	} else if delim, ok := token.(json.Delim); !ok || delim != ']' {
		return count, fmt.Errorf("table %s: the uploaded rows do not end with an array close", spec.Name)
	}
	return count, nil
}

// validateBackupFile walks the spooled envelope with the token API so no single
// value is ever buffered as a whole. It returns the byte offset of every table
// array for the import pass.
func validateBackupFile(spool *os.File) (map[string]tableRange, error) {
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("backup: could not rewind the uploaded envelope: %w", err)
	}
	decoder := json.NewDecoder(bufio.NewReaderSize(spool, 64*1024))

	format, version, ranges, err := readEnvelopeHeader(decoder)
	if err != nil {
		return nil, err
	}
	if format != backupFormat {
		return nil, fmt.Errorf("%w: format %q is not an %s backup", ErrInvalidBackup, format, backupFormat)
	}
	if version != backupVersion {
		return nil, fmt.Errorf("%w: version %d is not supported, expected %d", ErrInvalidBackup, version, backupVersion)
	}
	if ranges == nil {
		return nil, fmt.Errorf("%w: the envelope has no tables object", ErrInvalidBackup)
	}
	return ranges, nil
}

// readEnvelopeHeader reads format, version and the tables object. Unknown
// top-level keys are skipped token by token so a future field cannot be used to
// smuggle a large payload past validation.
func readEnvelopeHeader(decoder *json.Decoder) (string, int, map[string]tableRange, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", 0, nil, fmt.Errorf("%w: the envelope is not a JSON object", ErrInvalidBackup)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return "", 0, nil, fmt.Errorf("%w: the envelope must be a JSON object", ErrInvalidBackup)
	}

	var format string
	var version int
	var ranges map[string]tableRange

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return "", 0, nil, fmt.Errorf("%w: a top-level key is unreadable: %v", ErrInvalidBackup, err)
		}
		key, _ := keyToken.(string)
		switch key {
		case "format":
			if err = decoder.Decode(&format); err != nil {
				return "", 0, nil, envelopeFieldError(key, err)
			}
		case "version":
			if err = decoder.Decode(&version); err != nil {
				return "", 0, nil, envelopeFieldError(key, err)
			}
		case "tables":
			ranges, err = readTablesObject(decoder)
			if err != nil {
				return "", 0, nil, err
			}
		default:
			if err := skipJSONValue(decoder); err != nil {
				return "", 0, nil, envelopeFieldError(key, err)
			}
		}
	}

	if _, err := decoder.Token(); err != nil {
		return "", 0, nil, fmt.Errorf("%w: the envelope is truncated", ErrInvalidBackup)
	}
	return format, version, ranges, nil
}

func envelopeFieldError(key string, err error) error {
	return fmt.Errorf("%w: the value for %q could not be read: %v", ErrInvalidBackup, key, err)
}

func readTablesObject(decoder *json.Decoder) (map[string]tableRange, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, envelopeFieldError("tables", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf(`%w: "tables" must be a JSON object`, ErrInvalidBackup)
	}

	ranges := make(map[string]tableRange, len(tableSpecs))
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf(`%w: "tables" has an unreadable key: %v`, ErrInvalidBackup, err)
		}
		name, _ := keyToken.(string)

		spec := tableByName(name)
		if spec == nil {
			return nil, fmt.Errorf("%w: table %q is not part of the backup inventory", ErrInvalidBackup, name)
		}

		offset, err := validateTableRows(decoder, spec)
		if err != nil {
			return nil, err
		}
		ranges[name] = tableRange{offset: offset}
	}

	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf(`%w: "tables" is truncated: %v`, ErrInvalidBackup, err)
	}
	return ranges, nil
}

// validateTableRows consumes one table's row array and returns the byte offset
// of its first element, so the import pass can seek straight back to it and run
// the tables in foreign-key-safe order regardless of file order.
func validateTableRows(decoder *json.Decoder, spec *tableSpec) (int64, error) {
	token, err := decoder.Token()
	if err != nil {
		return 0, fmt.Errorf("%w: table %q could not be read: %v", ErrInvalidBackup, spec.Name, err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return 0, fmt.Errorf("%w: table %q must be a JSON array of row objects", ErrInvalidBackup, spec.Name)
	}
	offset := decoder.InputOffset()

	for decoder.More() {
		rowToken, err := decoder.Token()
		if err != nil {
			return 0, fmt.Errorf("%w: a row in table %q is unreadable: %v", ErrInvalidBackup, spec.Name, err)
		}
		if delim, ok := rowToken.(json.Delim); !ok || delim != '{' {
			return 0, fmt.Errorf("%w: every row in table %q must be a JSON object", ErrInvalidBackup, spec.Name)
		}
		if err := validateRow(decoder, spec); err != nil {
			return 0, err
		}
	}

	if _, err := decoder.Token(); err != nil {
		return 0, fmt.Errorf("%w: table %q is truncated: %v", ErrInvalidBackup, spec.Name, err)
	}
	return offset, nil
}

func validateRow(decoder *json.Decoder, spec *tableSpec) error {
	seen := make(map[string]bool, len(spec.Columns))
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%w: a column in table %q is unreadable: %v", ErrInvalidBackup, spec.Name, err)
		}
		column, _ := keyToken.(string)
		if !spec.allows(column) {
			return fmt.Errorf("%w: table %q does not have a column %q", ErrInvalidBackup, spec.Name, column)
		}
		if seen[column] {
			return fmt.Errorf("%w: table %q repeats the column %q", ErrInvalidBackup, spec.Name, column)
		}
		seen[column] = true
		if err := skipJSONValue(decoder); err != nil {
			return fmt.Errorf("%w: the column %q in table %q is unreadable: %v", ErrInvalidBackup, column, spec.Name, err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("%w: a row in table %q is truncated: %v", ErrInvalidBackup, spec.Name, err)
	}
	if !seen[spec.PK] {
		return fmt.Errorf("%w: a row in table %q is missing the primary key %q", ErrInvalidBackup, spec.Name, spec.PK)
	}
	return nil
}

// skipJSONValue consumes exactly one JSON value using the token API, so nested
// objects and arrays are never materialized.
func skipJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return fmt.Errorf("unexpected delimiter %q", delim)
	}
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if inner, ok := token.(json.Delim); ok {
			switch inner {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	return nil
}
