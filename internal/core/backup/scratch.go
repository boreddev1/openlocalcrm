package backup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

// scratchSchemaPrefix and scratchSchemaPattern constrain the generated schema
// name. The name is never taken from a request, but it is still validated
// before it is ever interpolated into DDL.
const scratchSchemaPrefix = "backup_drill_"

var scratchSchemaPattern = regexp.MustCompile(`^backup_drill_[0-9a-f]{16}$`)

// scratchCleanupTimeout bounds the guaranteed DROP SCHEMA so a cancelled drill
// still releases its schema instead of leaking it.
const scratchCleanupTimeout = 30 * time.Second

// scratchManager is the DDL surface the drill needs. The pool-backed
// implementation and the unit-test double both satisfy it, which is how the
// schema lifecycle (create, clone, rewrite, guaranteed drop) is asserted without
// a database.
type scratchManager interface {
	CreateSchema(ctx context.Context, schema string) error
	CloneTables(ctx context.Context, schema string, names []string) error
	RewriteForeignKeys(ctx context.Context, schema string) (fkRewriteStats, error)
	DropSchema(ctx context.Context, schema string) error
}

// targetRunner owns the two handles the drill must keep strictly apart: a
// source connection whose effective schema is public, and a target transaction
// whose search_path is the scratch schema only.
type targetRunner interface {
	Prepare(ctx context.Context, schema string) (RowSource, pgx.Tx, error)
	Release(ctx context.Context)
	VerifyTables(ctx context.Context, tx pgx.Tx, names []string) error
	EnforceConstraints(ctx context.Context, tx pgx.Tx) error
	Import(ctx context.Context, tx pgx.Tx, spec *tableSpec, row json.RawMessage) error
	VerifyForeignKeys(ctx context.Context, tx pgx.Tx, expected int) error
	ReadBack(ctx context.Context, tx pgx.Tx, table string) (int64, string, error)
	Finish(ctx context.Context, tx pgx.Tx, commit bool) error
}

// drillPhase names the two points inside the target transaction where the
// integration test needs to observe or perturb the scratch schema.
type drillPhase string

const (
	drillPhaseAfterValidation drillPhase = "after_validation"
	drillPhaseAfterImport     drillPhase = "after_import"
)

func newScratchSchemaName() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("backup: could not generate a scratch schema name: %w", err)
	}
	return scratchSchemaPrefix + hex.EncodeToString(buf), nil
}

func validateScratchSchemaName(name string) error {
	if !scratchSchemaPattern.MatchString(name) {
		return fmt.Errorf("backup: refusing to use the scratch schema name %q", name)
	}
	return nil
}

// Drill performs a real restore round trip: it clones the live public tables
// into an isolated scratch schema, rewrites the copied foreign keys so they
// point at scratch instead of public, streams every live row from public through
// the same static upserts the restore uses, and then reads the scratch rows back
// and compares them row for row.
//
// The read and the write use separate handles on purpose. One shared
// search_path would make the unqualified row selectors read the empty scratch
// tables and produce a false 0 = 0 pass, and a target whose search_path still
// included public would let a missing scratch table silently write live data.
// The target search_path is therefore pinned to the scratch schema only, and
// all 24 scratch tables are verified before the first write.
func (s *Service) Drill(ctx context.Context, meta RequestMeta) (DrillResult, error) {
	scratch := s.scratchManager()
	target := s.targetRunner()
	if scratch == nil || target == nil {
		return DrillResult{}, ErrBackupUnavailable
	}

	schema, err := newScratchSchemaName()
	if err != nil {
		return DrillResult{}, err
	}
	if err = validateScratchSchemaName(schema); err != nil {
		return DrillResult{}, err
	}

	result := DrillResult{Schema: schema}

	cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), scratchCleanupTimeout)
	defer cancelCleanup()
	// The scratch schema is dropped on every path, including every failure and
	// a cancelled request. A failed drop is not fatal: the schema is namespaced
	// and never read by the application.
	defer func() {
		_ = scratch.DropSchema(cleanupCtx, schema)
	}()

	_ = s.logAudit(ctx, meta, "BACKUP_RESTORE_DRILL_STARTED", map[string]any{"schema": schema})

	var tx pgx.Tx
	finished := false
	abort := func(cause error) (DrillResult, error) {
		if tx != nil && !finished {
			_ = target.Finish(context.WithoutCancel(ctx), tx, false)
		}
		_ = s.logAudit(ctx, meta, "BACKUP_RESTORE_DRILL_FAILED", map[string]any{
			"schema": schema,
			"error":  cause.Error(),
		})
		return result, cause
	}

	if err = scratch.CreateSchema(ctx, schema); err != nil {
		return abort(err)
	}
	if err = scratch.CloneTables(ctx, schema, tableNames()); err != nil {
		return abort(err)
	}
	fkStats, err := scratch.RewriteForeignKeys(ctx, schema)
	if err != nil {
		return abort(err)
	}
	result.ForeignKeys = fkStats

	source, preparedTx, err := target.Prepare(ctx, schema)
	if err != nil {
		return abort(err)
	}
	tx = preparedTx
	defer target.Release(ctx)

	if err := target.VerifyTables(ctx, tx, tableNames()); err != nil {
		return abort(err)
	}
	if s.drillHook != nil {
		if err := s.drillHook(ctx, drillPhaseAfterValidation, schema, tx); err != nil {
			return abort(err)
		}
	}

	sourceDigests := make(map[string]db.RowDigest, len(tableSpecs))
	for i := range tableSpecs {
		spec := &tableSpecs[i]
		digest := db.NewRowDigest()
		streamErr := spec.Stream(ctx, source, func(ctx context.Context, row json.RawMessage) error {
			if err := target.Import(ctx, tx, spec, row); err != nil {
				return err
			}
			digest.Add(row)
			return nil
		})
		if streamErr != nil {
			return abort(fmt.Errorf("backup drill: could not import %s into the scratch schema: %w", spec.Name, streamErr))
		}
		sourceDigests[spec.Name] = *digest
	}

	if s.drillHook != nil {
		if err := s.drillHook(ctx, drillPhaseAfterImport, schema, tx); err != nil {
			return abort(err)
		}
	}

	// Enforce the scratch foreign keys now rather than at commit time, so a
	// broken round trip is reported as a drill failure and not as a commit
	// error after the fact.
	if err := target.EnforceConstraints(ctx, tx); err != nil {
		return abort(err)
	}
	if err := target.VerifyForeignKeys(ctx, tx, fkStats.ScratchAdded); err != nil {
		return abort(err)
	}

	for i := range tableSpecs {
		spec := &tableSpecs[i]
		sourceDigest := sourceDigests[spec.Name]
		targetRows, targetDigest, err := target.ReadBack(ctx, tx, spec.Name)
		if err != nil {
			return abort(fmt.Errorf("backup drill: could not read back %s from the scratch schema: %w", spec.Name, err))
		}
		entry := TableDrillResult{
			Name:         spec.Name,
			SourceRows:   sourceDigest.Count(),
			TargetRows:   targetRows,
			SourceDigest: sourceDigest.Sum(),
			TargetDigest: targetDigest,
		}
		entry.Verified = entry.SourceRows == targetRows && entry.SourceDigest == targetDigest
		result.Tables = append(result.Tables, entry)
		result.TotalRows += targetRows

		if !entry.Verified {
			return abort(fmt.Errorf(
				"backup drill: %s did not survive the round trip (%d source rows vs %d scratch rows)",
				spec.Name, entry.SourceRows, entry.TargetRows))
		}
	}

	if result.TotalRows == 0 {
		return abort(errors.New(
			"backup drill: the scratch import contained no rows, so a row comparison would be a false 0 = 0 pass"))
	}

	if err := target.Finish(ctx, tx, true); err != nil {
		return abort(fmt.Errorf("backup drill: could not commit the scratch import: %w", err))
	}
	finished = true

	result.Passed = true
	_ = s.logAudit(ctx, meta, "BACKUP_RESTORE_DRILL_COMPLETED", map[string]any{
		"schema": schema,
		"tables": len(result.Tables),
		"rows":   result.TotalRows,
	})
	return result, nil
}

func (s *Service) scratchManager() scratchManager {
	if s.scratch != nil {
		return s.scratch
	}
	if s.pool == nil {
		return nil
	}
	return poolScratch{exec: s.pool}
}

func (s *Service) targetRunner() targetRunner {
	if s.targets != nil {
		return s.targets
	}
	if s.pool == nil {
		return nil
	}
	return &poolTarget{pool: s.pool}
}

// ---------------------------------------------------------------------------
// Pool-backed implementations
// ---------------------------------------------------------------------------

// scratchSQL is the statement surface poolScratch needs. *pgxpool.Pool satisfies
// it, and so does the recording double the unit tests use, which is how the DDL
// and the foreign-key rewrite are asserted without a database.
type scratchSQL interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type poolScratch struct {
	exec scratchSQL
}

func (p poolScratch) CreateSchema(ctx context.Context, schema string) error {
	if err := validateScratchSchemaName(schema); err != nil {
		return err
	}
	_, err := p.exec.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	return err
}

// CloneTables copies each application table with INCLUDING ALL. Whether that
// also copies the foreign key constraints is server-version dependent, so the
// rewrite step inspects the result instead of assuming it.
func (p poolScratch) CloneTables(ctx context.Context, schema string, names []string) error {
	if err := validateScratchSchemaName(schema); err != nil {
		return err
	}
	for _, name := range names {
		stmt := fmt.Sprintf("CREATE TABLE %s (LIKE public.%s INCLUDING ALL)",
			pgx.Identifier{schema, name}.Sanitize(), pgx.Identifier{name}.Sanitize())
		if _, err := p.exec.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("backup drill: could not clone %s: %w", name, err)
		}
	}
	return nil
}

func (p poolScratch) DropSchema(ctx context.Context, schema string) error {
	if err := validateScratchSchemaName(schema); err != nil {
		return err
	}
	_, err := p.exec.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
	return err
}

// foreignKey is one catalog foreign key: the child table that carries it, the
// parent table it points at, and the server-rendered definition.
type foreignKey struct {
	constraint   string
	child        string
	parent       string
	parentSchema string
	definition   string
}

// fkRewriteStats records what the rewrite actually did, so a drill that silently
// skipped it cannot pass.
type fkRewriteStats struct {
	// CopiedInspected is how many foreign keys the scratch schema actually had
	// before the rewrite.
	CopiedInspected int
	// CopiedDropped is how many of them still pointed at public and were
	// dropped. Leaving one in place would validate imported rows against live
	// data instead of scratch data.
	CopiedDropped int
	// ScratchAdded is how many scratch-scoped constraints were added.
	ScratchAdded int
	// Public is how many foreign keys the public schema declares.
	Public int
}

const foreignKeyCatalogColumns = `
		SELECT c.conname,
		       ch.relname,
		       pg_get_constraintdef(c.oid),
		       pa.relname,
		       npa.nspname
		FROM pg_constraint c
		JOIN pg_class ch ON ch.oid = c.conrelid
		JOIN pg_namespace nch ON nch.oid = ch.relnamespace
		JOIN pg_class pa ON pa.oid = c.confrelid
		JOIN pg_namespace npa ON npa.oid = pa.relnamespace
		WHERE c.contype = 'f'
		  AND nch.nspname = $1`

func scanForeignKeys(rows pgx.Rows) ([]foreignKey, error) {
	defer rows.Close()

	var out []foreignKey
	for rows.Next() {
		var fk foreignKey
		if err := rows.Scan(&fk.constraint, &fk.child, &fk.definition, &fk.parent, &fk.parentSchema); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}

// publicForeignKeys lists every foreign key the live schema declares.
func (p poolScratch) publicForeignKeys(ctx context.Context) ([]foreignKey, error) {
	rows, err := p.exec.Query(ctx, foreignKeyCatalogColumns+" ORDER BY ch.relname, c.conname", "public")
	if err != nil {
		return nil, fmt.Errorf("backup drill: could not read the public foreign keys: %w", err)
	}
	return scanForeignKeys(rows)
}

// scratchForeignKeys lists the foreign keys that really exist inside the scratch
// schema after the clone. This is the inspection the design requires; assuming
// what INCLUDING ALL copied would be wrong on PostgreSQL 16, where it copies
// primary keys but not foreign keys.
func (p poolScratch) scratchForeignKeys(ctx context.Context, schema string) ([]foreignKey, error) {
	rows, err := p.exec.Query(ctx, foreignKeyCatalogColumns+" ORDER BY ch.relname, c.conname", schema)
	if err != nil {
		return nil, fmt.Errorf("backup drill: could not inspect the copied constraints: %w", err)
	}
	return scanForeignKeys(rows)
}

// RewriteForeignKeys drops every constraint the clone left pointing at public
// and then adds the equivalent constraint against the scratch parent table, so
// the drill really exercises foreign-key integrity instead of trusting the
// live data.
func (p poolScratch) RewriteForeignKeys(ctx context.Context, schema string) (fkRewriteStats, error) {
	stats := fkRewriteStats{}
	if err := validateScratchSchemaName(schema); err != nil {
		return stats, err
	}

	public, err := p.publicForeignKeys(ctx)
	if err != nil {
		return stats, err
	}
	stats.Public = len(public)

	copied, err := p.scratchForeignKeys(ctx, schema)
	if err != nil {
		return stats, err
	}
	stats.CopiedInspected = len(copied)

	for _, fk := range copied {
		if fk.parentSchema != "public" {
			continue
		}
		drop := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s",
			pgx.Identifier{schema, fk.child}.Sanitize(), pgx.Identifier{fk.constraint}.Sanitize())
		if _, err := p.exec.Exec(ctx, drop); err != nil {
			return stats, fmt.Errorf("backup drill: could not drop the copied constraint %s: %w", fk.constraint, err)
		}
		stats.CopiedDropped++
	}

	for _, fk := range public {
		rewritten, err := rewriteForeignKeyDefinition(fk.definition, schema, fk.parent)
		if err != nil {
			return stats, fmt.Errorf("backup drill: %s: %w", fk.constraint, err)
		}
		add := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s",
			pgx.Identifier{schema, fk.child}.Sanitize(),
			pgx.Identifier{fk.constraint}.Sanitize(), rewritten)
		if _, err := p.exec.Exec(ctx, add); err != nil {
			return stats, fmt.Errorf("backup drill: could not add the scratch constraint %s: %w", fk.constraint, err)
		}
		stats.ScratchAdded++
	}
	return stats, nil
}

// rewriteForeignKeyDefinition retargets a copied foreign key at the scratch
// schema. pg_get_constraintdef only qualifies the parent table when public is
// not in the search path, so both spellings are handled and anything else is
// rejected rather than guessed at.
func rewriteForeignKeyDefinition(definition, schema, parentTable string) (string, error) {
	for _, candidate := range []string{
		"REFERENCES public." + parentTable + "(",
		"REFERENCES " + parentTable + "(",
	} {
		if strings.Contains(definition, candidate) {
			return strings.Replace(definition, candidate,
				"REFERENCES "+schema+"."+parentTable+"(", 1), nil
		}
	}
	return "", fmt.Errorf("cannot retarget %q at the scratch schema", definition)
}

type poolTarget struct {
	pool   *pgxpool.Pool
	source *pgxpool.Conn
}

func (p *poolTarget) Prepare(ctx context.Context, schema string) (RowSource, pgx.Tx, error) {
	if err := validateScratchSchemaName(schema); err != nil {
		return nil, nil, err
	}

	source, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("backup drill: could not acquire the source connection: %w", err)
	}
	// The source handle is pinned to public so the unqualified row selectors
	// read the live tables even while a scratch schema exists.
	if _, err = source.Exec(ctx, "SET search_path TO public"); err != nil {
		source.Release()
		return nil, nil, fmt.Errorf("backup drill: could not pin the source schema: %w", err)
	}
	p.source = source

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		p.releaseSource()
		return nil, nil, fmt.Errorf("backup drill: could not begin the scratch transaction: %w", err)
	}
	// Scratch ONLY. A missing scratch table must fail; it must never fall back
	// to the live public table.
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		_ = tx.Rollback(ctx)
		p.releaseSource()
		return nil, nil, fmt.Errorf("backup drill: could not pin the scratch schema: %w", err)
	}
	return db.New(source), tx, nil
}

func (p *poolTarget) releaseSource() {
	if p.source != nil {
		p.source.Release()
		p.source = nil
	}
}

func (p *poolTarget) Release(ctx context.Context) {
	p.releaseSource()
}

// VerifyTables fails unless every backup table really exists in the pinned
// scratch schema, before a single row is written.
func (p *poolTarget) VerifyTables(ctx context.Context, tx pgx.Tx, names []string) error {
	rows, err := tx.Query(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema()
		  AND c.relkind = 'r'
		  AND c.relname = ANY($1)`, names)
	if err != nil {
		return fmt.Errorf("backup drill: could not verify the scratch tables: %w", err)
	}
	defer rows.Close()

	found := make(map[string]bool, len(names))
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("backup drill: could not verify the scratch tables: %w", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backup drill: could not verify the scratch tables: %w", err)
	}

	var missing []string
	for _, name := range names {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("backup drill: %d of %d scratch tables are missing: %s",
			len(missing), len(names), strings.Join(missing, ", "))
	}
	return nil
}

// EnforceConstraints checks the scratch foreign keys immediately instead of
// deferring them to commit.
func (p *poolTarget) EnforceConstraints(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
		return fmt.Errorf("backup drill: could not enforce the scratch constraints: %w", err)
	}
	return nil
}

func (p *poolTarget) Import(ctx context.Context, tx pgx.Tx, spec *tableSpec, row json.RawMessage) error {
	if spec == nil {
		return errors.New("backup drill: cannot import a row without a table specification")
	}
	return spec.Upsert(ctx, db.New(tx), row)
}

// VerifyForeignKeys asserts that no scratch constraint still references public
// and that the scratch schema really carries scratch-scoped foreign keys, so a
// drill cannot pass with foreign-key checking silently disabled.
func (p *poolTarget) VerifyForeignKeys(ctx context.Context, tx pgx.Tx, expected int) error {
	rows, err := tx.Query(ctx, `
		SELECT npa.nspname, count(*)
		FROM pg_constraint c
		JOIN pg_class ch ON ch.oid = c.conrelid
		JOIN pg_namespace nch ON nch.oid = ch.relnamespace
		JOIN pg_class pa ON pa.oid = c.confrelid
		JOIN pg_namespace npa ON npa.oid = pa.relnamespace
		WHERE c.contype = 'f'
		  AND nch.nspname = current_schema()
		GROUP BY npa.nspname`)
	if err != nil {
		return fmt.Errorf("backup drill: could not verify the scratch foreign keys: %w", err)
	}
	defer rows.Close()

	publicRefs, scratchRefs := 0, 0
	for rows.Next() {
		var schema string
		var count int
		if err := rows.Scan(&schema, &count); err != nil {
			return fmt.Errorf("backup drill: could not verify the scratch foreign keys: %w", err)
		}
		if schema == "public" {
			publicRefs = count
		} else {
			scratchRefs += count
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backup drill: could not verify the scratch foreign keys: %w", err)
	}

	if publicRefs > 0 {
		return fmt.Errorf(
			"backup drill: %d scratch foreign key(s) still reference public, so imported rows would be validated against live data",
			publicRefs)
	}
	if scratchRefs < expected {
		return fmt.Errorf(
			"backup drill: the scratch schema carries %d foreign key(s) but the live schema declares %d, so the round trip would not exercise referential integrity",
			scratchRefs, expected)
	}
	return nil
}

func (p *poolTarget) ReadBack(ctx context.Context, tx pgx.Tx, table string) (int64, string, error) {
	return db.BackupTableDigest(ctx, db.New(tx), table)
}

func (p *poolTarget) Finish(ctx context.Context, tx pgx.Tx, commit bool) error {
	if commit {
		return tx.Commit(ctx)
	}
	return tx.Rollback(ctx)
}
