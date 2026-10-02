package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fake scratch executor (DDL) and fake target runner (import + verification)
// ---------------------------------------------------------------------------

type scratchScript struct {
	failOnContains string
}

type fakeScratch struct {
	script scratchScript

	statements []string

	// catalogRows is what the live public schema declares.
	catalogRows []catalogFK
	// copiedRows is what the clone actually left in the scratch schema.
	// PostgreSQL 16 copies primary keys but not foreign keys, so the default
	// is empty: the drill has to add the scratch constraints itself.
	copiedRows []catalogFK
}

type catalogFK struct {
	constraint   string
	child        string
	parent       string
	parentSchema string
	definition   string
}

func (f *fakeScratch) record(stmt string) error {
	f.statements = append(f.statements, stmt)
	if f.script.failOnContains != "" && strings.Contains(stmt, f.script.failOnContains) {
		return fmt.Errorf("scratch ddl failed on %q", f.script.failOnContains)
	}
	return nil
}

func (f *fakeScratch) CreateSchema(ctx context.Context, schema string) error {
	return f.record("CREATE SCHEMA " + pgx.Identifier{schema}.Sanitize())
}

func (f *fakeScratch) CloneTables(ctx context.Context, schema string, names []string) error {
	for _, name := range names {
		if err := f.record(fmt.Sprintf("CREATE TABLE %s (LIKE public.%s INCLUDING ALL)",
			pgx.Identifier{schema, name}.Sanitize(), pgx.Identifier{name}.Sanitize())); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeScratch) DropSchema(ctx context.Context, schema string) error {
	return f.record("DROP SCHEMA IF EXISTS " + pgx.Identifier{schema}.Sanitize() + " CASCADE")
}

func (f *fakeScratch) RewriteForeignKeys(ctx context.Context, schema string) (fkRewriteStats, error) {
	stats := fkRewriteStats{Public: len(f.catalogRows)}

	// Inspect what the clone really copied, then drop whatever still points at
	// public before any replacement is added.
	stats.CopiedInspected = len(f.copiedRows)
	for _, fk := range f.copiedRows {
		if fk.parentSchema != "public" {
			continue
		}
		if err := f.record(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s",
			pgx.Identifier{schema, fk.child}.Sanitize(),
			pgx.Identifier{fk.constraint}.Sanitize())); err != nil {
			return stats, err
		}
		stats.CopiedDropped++
	}

	for _, fk := range f.catalogRows {
		rewritten, err := rewriteForeignKeyDefinition(fk.definition, schema, fk.parent)
		if err != nil {
			return stats, fmt.Errorf("backup drill: %s: %w", fk.constraint, err)
		}
		if err := f.record(fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s",
			pgx.Identifier{schema, fk.child}.Sanitize(),
			pgx.Identifier{fk.constraint}.Sanitize(), rewritten)); err != nil {
			return stats, err
		}
		stats.ScratchAdded++
	}
	return stats, nil
}

func (f *fakeScratch) matching(fragment string) []string {
	var out []string
	for _, stmt := range f.statements {
		if strings.Contains(stmt, fragment) {
			out = append(out, stmt)
		}
	}
	return out
}

func defaultCatalog() []catalogFK {
	return []catalogFK{
		{
			constraint:   "contacts_company_id_fkey",
			child:        "contacts",
			parent:       "companies",
			parentSchema: "public",
			definition:   "FOREIGN KEY (company_id) REFERENCES public.companies(id) ON DELETE SET NULL",
		},
		{
			constraint:   "workflow_runs_workflow_id_fkey",
			child:        "workflow_runs",
			parent:       "workflows",
			parentSchema: "public",
			definition:   "FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE",
		},
	}
}

type fakeTargetRunner struct {
	// existing lists the scratch tables that are actually present. Empty means
	// "all 24 exist".
	existing []string
	// publicRefs is the number of scratch constraints that still reference
	// public after the rewrite.
	publicRefs int
	// scratchRefs is the number of scratch-scoped constraints.
	scratchRefs int
	// source supplies the live rows. nil means one row per table.
	source *fakeRowSource

	rows          map[string][][]byte
	imports       int
	failImport    string
	failRead      string
	committed     bool
	rolledBack    bool
	prepareErr    error
	constraintErr error
	enforced      bool
	schema        string
	order         []string
	released      bool
}

func (f *fakeTargetRunner) Prepare(ctx context.Context, schema string) (RowSource, pgx.Tx, error) {
	f.schema = schema
	if f.prepareErr != nil {
		return nil, nil, f.prepareErr
	}
	if f.rows == nil {
		f.rows = make(map[string][][]byte)
	}
	source := f.source
	if source == nil {
		source = newFakeRowSource(1)
	}
	return source, &fakeDrillTx{}, nil
}

// newFakeTarget is the common case: a scratch schema that ends up with as many
// scratch-scoped foreign keys as the live schema declares.
func newFakeTarget() *fakeTargetRunner {
	return &fakeTargetRunner{scratchRefs: len(defaultCatalog())}
}

func (f *fakeTargetRunner) Release(ctx context.Context) { f.released = true }

func (f *fakeTargetRunner) VerifyTables(ctx context.Context, tx pgx.Tx, names []string) error {
	if len(f.existing) == 0 {
		return nil
	}
	have := make(map[string]bool, len(f.existing))
	for _, name := range f.existing {
		have[name] = true
	}
	var missing []string
	for _, name := range names {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("backup drill: %d of %d scratch tables are missing: %s",
			len(missing), len(names), strings.Join(missing, ", "))
	}
	return nil
}

func (f *fakeTargetRunner) EnforceConstraints(ctx context.Context, tx pgx.Tx) error {
	if f.constraintErr != nil {
		return f.constraintErr
	}
	f.enforced = true
	return nil
}

func (f *fakeTargetRunner) Import(ctx context.Context, tx pgx.Tx, spec *tableSpec, row json.RawMessage) error {
	f.imports++
	f.order = append(f.order, spec.Name)
	if f.failImport == spec.Name {
		return fmt.Errorf("scratch import rejected for %s", spec.Name)
	}
	f.rows[spec.Name] = append(f.rows[spec.Name], row)
	return nil
}

func (f *fakeTargetRunner) VerifyForeignKeys(ctx context.Context, tx pgx.Tx, expected int) error {
	if f.publicRefs > 0 {
		return fmt.Errorf(
			"backup drill: %d scratch foreign key(s) still reference public, so imported rows would be validated against live data",
			f.publicRefs)
	}
	if f.scratchRefs < expected {
		return fmt.Errorf(
			"backup drill: the scratch schema carries %d foreign key(s) but the live schema declares %d",
			f.scratchRefs, expected)
	}
	return nil
}

func (f *fakeTargetRunner) ReadBack(ctx context.Context, tx pgx.Tx, table string) (int64, string, error) {
	if f.failRead == table {
		return 0, "", fmt.Errorf("relation %q does not exist", table)
	}
	rows := f.rows[table]
	count, digest := db.DigestRows(rows)
	return count, digest, nil
}

func (f *fakeTargetRunner) Finish(ctx context.Context, tx pgx.Tx, commit bool) error {
	if commit {
		f.committed = true
	} else {
		f.rolledBack = true
	}
	return nil
}

// fakeDrillTx is the placeholder transaction the fake target runner hands out.
type fakeDrillTx struct {
	pgx.Tx
}

func newDrillService(scratch scratchManager, target targetRunner, audit *recordingAudit) *Service {
	svc := NewService(newFakeRowSource(0), audit, nil, nil)
	svc.scratch = scratch
	svc.targets = target
	return svc
}

// ---------------------------------------------------------------------------
// Schema lifecycle
// ---------------------------------------------------------------------------

func TestDrillRecordsTheFullScratchLifecycle(t *testing.T) {
	scratch := &fakeScratch{catalogRows: defaultCatalog()}
	target := newFakeTarget()

	result, err := newDrillService(scratch, target, &recordingAudit{}).Drill(context.Background(), RequestMeta{})
	require.NoError(t, err)
	require.True(t, result.Passed)

	creates := scratch.matching("CREATE SCHEMA")
	require.Len(t, creates, 1)
	require.Contains(t, creates[0], "backup_drill_")

	clones := scratch.matching("CREATE TABLE")
	require.Len(t, clones, 24, "every backup table must be cloned")
	schema := scratchCreates(scratch)
	require.NotEmpty(t, schema)
	for _, name := range tableNames() {
		require.Len(t, scratch.matching(`LIKE public."`+name+`" INCLUDING ALL`), 1,
			"table %s must be cloned once", name)
	}

	// Every live foreign key gets a scratch-scoped replacement.
	for _, fk := range defaultCatalog() {
		adds := scratch.matching(`ADD CONSTRAINT "` + fk.constraint + `"`)
		require.Len(t, adds, 1)
		require.Contains(t, adds[0], "REFERENCES "+schema+"."+fk.parent+"(")
		require.NotContains(t, adds[0], "REFERENCES public.")
	}
	// PostgreSQL 16 copies primary keys but not foreign keys, so nothing needs
	// dropping here; the drill inspects rather than assuming.
	require.Empty(t, scratch.matching("DROP CONSTRAINT"))

	drops := scratch.matching("DROP SCHEMA")
	require.Len(t, drops, 1)
	require.Contains(t, drops[0], "CASCADE")

	require.True(t, target.committed)
	require.False(t, target.rolledBack)
	require.True(t, target.enforced, "the scratch constraints must be enforced before the comparison")
	require.True(t, target.released)
	require.Equal(t, schema, target.schema)
}

func scratchCreates(scratch *fakeScratch) string {
	for _, stmt := range scratch.statements {
		if strings.HasPrefix(stmt, "CREATE SCHEMA ") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(stmt, "CREATE SCHEMA ")), `"`)
		}
	}
	return ""
}

func indexOfStatement(statements []string, fragment string) int {
	for i, stmt := range statements {
		if strings.Contains(stmt, fragment) {
			return i
		}
	}
	return -1
}

func TestDrillDropsCopiedConstraintsThatStillReferencePublic(t *testing.T) {
	// On a server that does copy foreign keys, every copied constraint must be
	// dropped before the scratch replacement is added. Leaving one in place
	// would validate imported rows against live data.
	copied := []catalogFK{
		{
			constraint:   "contacts_company_id_fkey",
			child:        "contacts",
			parent:       "companies",
			parentSchema: "public",
			definition:   "FOREIGN KEY (company_id) REFERENCES public.companies(id) ON DELETE SET NULL",
		},
	}
	scratch := &fakeScratch{catalogRows: defaultCatalog(), copiedRows: copied}
	result, err := newDrillService(scratch, newFakeTarget(), &recordingAudit{}).
		Drill(context.Background(), RequestMeta{})
	require.NoError(t, err)
	require.True(t, result.Passed)
	require.Equal(t, 1, result.ForeignKeys.CopiedInspected)
	require.Equal(t, 1, result.ForeignKeys.CopiedDropped)
	require.Equal(t, len(defaultCatalog()), result.ForeignKeys.ScratchAdded)

	dropIdx := indexOfStatement(scratch.statements, `DROP CONSTRAINT "contacts_company_id_fkey"`)
	addIdx := indexOfStatement(scratch.statements, `ADD CONSTRAINT "contacts_company_id_fkey"`)
	require.Positive(t, dropIdx)
	require.Less(t, dropIdx, addIdx,
		"the copied constraint must be dropped before the replacement is added")
}

func TestDrillRewritesUnqualifiedAndQualifiedForeignKeys(t *testing.T) {
	catalog := []catalogFK{
		{
			constraint:   "contacts_company_id_fkey",
			child:        "contacts",
			parent:       "companies",
			parentSchema: "public",
			definition:   "FOREIGN KEY (company_id) REFERENCES public.companies(id) ON DELETE SET NULL",
		},
		{
			constraint:   "workflow_steps_run_id_fkey",
			child:        "workflow_steps",
			parent:       "workflow_runs",
			parentSchema: "public",
			definition:   "FOREIGN KEY (run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE",
		},
	}
	scratch := &fakeScratch{catalogRows: catalog}

	_, err := newDrillService(scratch, newFakeTarget(), &recordingAudit{}).Drill(context.Background(), RequestMeta{})
	require.NoError(t, err)

	schema := scratchCreates(scratch)
	adds := scratch.matching("ADD CONSTRAINT")
	require.Len(t, adds, 2)
	require.Contains(t, adds[0], "REFERENCES "+schema+".companies(id) ON DELETE SET NULL")
	require.Contains(t, adds[1], "REFERENCES "+schema+".workflow_runs(id) ON DELETE CASCADE")
}

func TestDrillFailsWhenAForeignKeyDefinitionCannotBeRewritten(t *testing.T) {
	scratch := &fakeScratch{catalogRows: []catalogFK{{
		constraint:   "mystery_fkey",
		child:        "contacts",
		parent:       "companies",
		parentSchema: "public",
		definition:   "FOREIGN KEY (company_id) REFERENCES somewhere_else(id)",
	}}}

	_, err := newDrillService(scratch, newFakeTarget(), &recordingAudit{}).Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mystery_fkey")
	require.Empty(t, scratch.matching("ADD CONSTRAINT"), "an unrewritable FK must never be added")
}

func TestDrillAlwaysDropsTheScratchSchemaOnFailure(t *testing.T) {
	scratch := &fakeScratch{catalogRows: defaultCatalog(), script: scratchScript{failOnContains: "CREATE TABLE"}}
	target := newFakeTarget()

	_, err := newDrillService(scratch, target, &recordingAudit{}).Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.Len(t, scratch.matching("DROP SCHEMA"), 1, "cleanup must run on the failure path too")
	require.False(t, target.committed)
}

func TestDrillRollsBackTheScratchTransactionOnFailure(t *testing.T) {
	// A scratch table that vanishes after validation must abort the whole
	// transaction instead of falling back to the live public table.
	scratch := &fakeScratch{catalogRows: defaultCatalog()}
	target := &fakeTargetRunner{existing: []string{"users"}}

	_, err := newDrillService(scratch, target, &recordingAudit{}).Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.True(t, target.rolledBack)
	require.False(t, target.committed)
	require.True(t, target.released, "the source connection must be released on the failure path")
	require.Len(t, scratch.matching("DROP SCHEMA"), 1)
}

func TestDrillValidatesEveryScratchTableBeforeWriting(t *testing.T) {
	scratch := &fakeScratch{catalogRows: defaultCatalog()}
	target := &fakeTargetRunner{existing: []string{"users", "todos"}}

	_, err := newDrillService(scratch, target, &recordingAudit{}).Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "companies", "the first missing scratch table must be named")
	require.Contains(t, err.Error(), "ai_audit_logs", "every missing scratch table must be named")
	require.NotContains(t, err.Error(), " todos,", "an existing scratch table must not be reported missing")
	require.Zero(t, target.imports, "no row may be written before all 24 scratch tables exist")
	require.Len(t, scratch.matching("DROP SCHEMA"), 1)
}

func TestDrillRejectsScratchForeignKeysThatStillReferencePublic(t *testing.T) {
	target := &fakeTargetRunner{publicRefs: 3, scratchRefs: len(defaultCatalog())}

	_, err := newDrillService(&fakeScratch{catalogRows: defaultCatalog()}, target, &recordingAudit{}).
		Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "public")
}

func TestDrillRejectsAnEmptyScratchImport(t *testing.T) {
	// A 0 = 0 pass is exactly the false positive the drill must never report.
	target := &fakeTargetRunner{source: newFakeRowSource(0), scratchRefs: len(defaultCatalog())}
	svc := NewService(newFakeRowSource(0), &recordingAudit{}, nil, nil)
	svc.scratch = &fakeScratch{catalogRows: defaultCatalog()}
	svc.targets = target

	result, err := svc.Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no rows")
	require.False(t, result.Passed)
}

func TestDrillFailsWhenTheScratchConstraintsCannotBeEnforced(t *testing.T) {
	target := &fakeTargetRunner{constraintErr: errors.New("cannot enforce constraints")}

	_, err := newDrillService(&fakeScratch{catalogRows: defaultCatalog()}, target, &recordingAudit{}).
		Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.True(t, target.rolledBack)
	require.False(t, target.committed)
}

func TestDrillFailsWhenTheImportIsRejected(t *testing.T) {
	target := &fakeTargetRunner{failImport: "todos", scratchRefs: len(defaultCatalog())}

	_, err := newDrillService(&fakeScratch{catalogRows: defaultCatalog()}, target, &recordingAudit{}).
		Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "todos")
	require.True(t, target.rolledBack)
	require.False(t, target.committed)
}

func TestDrillFailsWhenTheScratchTableCannotBeReadBack(t *testing.T) {
	target := &fakeTargetRunner{failRead: "todos", scratchRefs: len(defaultCatalog())}

	result, err := newDrillService(&fakeScratch{catalogRows: defaultCatalog()}, target, &recordingAudit{}).
		Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "todos")
	require.False(t, result.Passed)
	require.True(t, target.rolledBack)
}

func TestDrillFailsWhenTheScratchSchemaLostItsForeignKeys(t *testing.T) {
	// A drill that quietly skipped the constraint rewrite must not pass.
	target := &fakeTargetRunner{scratchRefs: 0}

	_, err := newDrillService(&fakeScratch{catalogRows: defaultCatalog()}, target, &recordingAudit{}).
		Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "live schema declares")
	require.True(t, target.rolledBack)
}

func TestDrillReturnsUnavailableWithoutAPool(t *testing.T) {
	svc := NewService(newFakeRowSource(0), nil, nil, nil)
	result, err := svc.Drill(context.Background(), RequestMeta{})
	require.ErrorIs(t, err, ErrBackupUnavailable)
	require.False(t, result.Passed)
}

func TestDrillReturnsUnavailableForTheDemoQuerier(t *testing.T) {
	demoQuerier := demo.NewInMemoryQuerier()
	svc := NewService(demoQuerier, nil, nil, demoQuerier)
	_, err := svc.Drill(context.Background(), RequestMeta{})
	require.ErrorIs(t, err, ErrBackupUnavailable)
}

func TestScratchSchemaNamesAreValidIdentifiers(t *testing.T) {
	for i := 0; i < 32; i++ {
		name, err := newScratchSchemaName()
		require.NoError(t, err)
		require.Regexp(t, `^backup_drill_[0-9a-f]{16}$`, name)
		require.NoError(t, validateScratchSchemaName(name))
	}
	require.Error(t, validateScratchSchemaName("public"))
	require.Error(t, validateScratchSchemaName(`evil"; DROP SCHEMA public; --`))
	require.Error(t, validateScratchSchemaName(""))
}

func TestDrillAuditsCompletionAndFailure(t *testing.T) {
	audit := &recordingAudit{}
	_, err := newDrillService(&fakeScratch{catalogRows: defaultCatalog()}, newFakeTarget(), audit).
		Drill(context.Background(), RequestMeta{})
	require.NoError(t, err)
	require.Equal(t, []string{"BACKUP_RESTORE_DRILL_STARTED", "BACKUP_RESTORE_DRILL_COMPLETED"}, audit.actions())

	failing := &recordingAudit{}
	brokenScratch := &fakeScratch{catalogRows: defaultCatalog(), script: scratchScript{failOnContains: "CREATE SCHEMA"}}
	_, err = newDrillService(brokenScratch, newFakeTarget(), failing).Drill(context.Background(), RequestMeta{})
	require.Error(t, err)
	require.Equal(t, []string{"BACKUP_RESTORE_DRILL_STARTED", "BACKUP_RESTORE_DRILL_FAILED"}, failing.actions())
}

func TestDrillReportsPerTableCountsAndDigests(t *testing.T) {
	target := newFakeTarget()
	result, err := newDrillService(&fakeScratch{catalogRows: defaultCatalog()}, target, &recordingAudit{}).
		Drill(context.Background(), RequestMeta{})
	require.NoError(t, err)
	require.Len(t, result.Tables, 24)
	for _, entry := range result.Tables {
		require.True(t, entry.Verified, "table %s must be verified", entry.Name)
		require.NotEmpty(t, entry.SourceDigest)
		require.Equal(t, entry.SourceDigest, entry.TargetDigest)
		require.Equal(t, entry.SourceRows, entry.TargetRows)
	}
}

func TestRowDigestIsOrderSensitiveAndContentSensitive(t *testing.T) {
	a := [][]byte{[]byte(`{"id":"1"}`), []byte(`{"id":"2"}`)}
	b := [][]byte{[]byte(`{"id":"2"}`), []byte(`{"id":"1"}`)}
	require.NotEqual(t, RowDigest(a), RowDigest(b))
	require.Equal(t, RowDigest(a), RowDigest(a))
	require.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", RowDigest(nil))
}

// RowDigest is the exported alias the restore and drill code shares with the
// package-db digest, so a test can predict the digest of a streamed table.
func RowDigest(rows [][]byte) string {
	_, digest := db.DigestRows(rows)
	return digest
}
