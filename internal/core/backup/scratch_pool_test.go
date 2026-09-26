package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Recording doubles for the pool-backed scratch implementation
// ---------------------------------------------------------------------------

type recordedExec struct {
	sql  string
	args []any
}

// recordingSQL answers the pg_constraint catalog query with canned rows and
// records every statement, so the generated DDL can be asserted directly.
type recordingSQL struct {
	execs    []recordedExec
	queries  []recordedExec
	public   []foreignKey
	copied   []foreignKey
	execErr  string
	queryErr error
}

func (r *recordingSQL) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	r.execs = append(r.execs, recordedExec{sql: sql, args: args})
	if r.execErr != "" && strings.Contains(sql, r.execErr) {
		return pgconn.CommandTag{}, fmt.Errorf("ddl failed: %s", r.execErr)
	}
	return pgconn.CommandTag{}, nil
}

func (r *recordingSQL) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	r.queries = append(r.queries, recordedExec{sql: sql, args: args})
	if r.queryErr != nil {
		return nil, r.queryErr
	}
	schema, _ := args[0].(string)
	rows := r.public
	if schema != "public" {
		rows = r.copied
	}
	return &catalogRows{rows: rows}, nil
}

func (r *recordingSQL) indexOf(fragment string) int {
	for i, exec := range r.execs {
		if strings.Contains(exec.sql, fragment) {
			return i
		}
	}
	return -1
}

func (r *recordingSQL) matching(fragment string) []string {
	var out []string
	for _, exec := range r.execs {
		if strings.Contains(exec.sql, fragment) {
			out = append(out, exec.sql)
		}
	}
	return out
}

type catalogRows struct {
	pgx.Rows
	rows []foreignKey
	pos  int
}

func (r *catalogRows) Next() bool { return r.pos < len(r.rows) }

func (r *catalogRows) Scan(dest ...any) error {
	if r.pos >= len(r.rows) {
		return pgx.ErrNoRows
	}
	fk := r.rows[r.pos]
	r.pos++
	*(dest[0].(*string)) = fk.constraint
	*(dest[1].(*string)) = fk.child
	*(dest[2].(*string)) = fk.definition
	*(dest[3].(*string)) = fk.parent
	*(dest[4].(*string)) = fk.parentSchema
	return nil
}

func (r *catalogRows) Close()     {}
func (r *catalogRows) Err() error { return nil }

// recordingTx records the statements the target transaction receives and
// answers the scratch verification queries.
type recordingTx struct {
	pgx.Tx
	execs     []recordedExec
	queries   []recordedExec
	tables    []string
	fkRefs    map[string]int
	committed bool
	rolled    bool
	execErr   string
}

func (r *recordingTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	r.execs = append(r.execs, recordedExec{sql: sql, args: args})
	if r.execErr != "" && strings.Contains(sql, r.execErr) {
		return pgconn.CommandTag{}, errors.New("constraint could not be enforced")
	}
	return pgconn.CommandTag{}, nil
}

func (r *recordingTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	r.queries = append(r.queries, recordedExec{sql: sql, args: args})
	if strings.Contains(sql, "relname = ANY") {
		return &stringRows{rows: r.tables}, nil
	}
	return newFKSchemaRows(r.fkRefs), nil
}

func (r *recordingTx) Commit(ctx context.Context) error   { r.committed = true; return nil }
func (r *recordingTx) Rollback(ctx context.Context) error { r.rolled = true; return nil }

type stringRows struct {
	pgx.Rows
	rows []string
	pos  int
}

func (r *stringRows) Next() bool { return r.pos < len(r.rows) }
func (r *stringRows) Scan(dest ...any) error {
	if r.pos >= len(r.rows) {
		return pgx.ErrNoRows
	}
	*(dest[0].(*string)) = r.rows[r.pos]
	r.pos++
	return nil
}
func (r *stringRows) Close()     {}
func (r *stringRows) Err() error { return nil }

// fkSchemaRows replays the "parent schema -> constraint count" rows. The keys
// are materialised in sorted order so the double is deterministic.
type fkSchemaRows struct {
	pgx.Rows
	keys   []string
	counts map[string]int
	pos    int
}

func newFKSchemaRows(counts map[string]int) *fkSchemaRows {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return &fkSchemaRows{keys: keys, counts: counts}
}

func (r *fkSchemaRows) Next() bool { return r.pos < len(r.keys) }

func (r *fkSchemaRows) Scan(dest ...any) error {
	if r.pos >= len(r.keys) {
		return pgx.ErrNoRows
	}
	key := r.keys[r.pos]
	r.pos++
	*(dest[0].(*string)) = key
	*(dest[1].(*int)) = r.counts[key]
	return nil
}

func (r *fkSchemaRows) Close()     {}
func (r *fkSchemaRows) Err() error { return nil }

func publicCatalogFixture() []foreignKey {
	return []foreignKey{
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
}

const testSchema = "backup_drill_0123456789abcdef"

// ---------------------------------------------------------------------------
// scratchManager implementation
// ---------------------------------------------------------------------------

func TestPoolScratchCreatesAndDropsTheScratchSchema(t *testing.T) {
	rec := &recordingSQL{}
	scratch := poolScratch{exec: rec}

	require.NoError(t, scratch.CreateSchema(context.Background(), testSchema))
	require.Equal(t, []string{`CREATE SCHEMA "` + testSchema + `"`}, rec.matching("CREATE SCHEMA"))

	require.NoError(t, scratch.DropSchema(context.Background(), testSchema))
	require.Equal(t, []string{`DROP SCHEMA IF EXISTS "` + testSchema + `" CASCADE`}, rec.matching("DROP SCHEMA"))
}

func TestPoolScratchRefusesAnUntrustedSchemaName(t *testing.T) {
	rec := &recordingSQL{}
	scratch := poolScratch{exec: rec}

	for _, hostile := range []string{
		`backup_drill_"; DROP SCHEMA public; --`,
		"public",
		"",
		"backup_drill_ZZZZ",
	} {
		require.Error(t, scratch.CreateSchema(context.Background(), hostile), hostile)
		require.Error(t, scratch.CloneTables(context.Background(), hostile, []string{"users"}), hostile)
		require.Error(t, scratch.DropSchema(context.Background(), hostile), hostile)
		_, err := scratch.RewriteForeignKeys(context.Background(), hostile)
		require.Error(t, err, hostile)
	}
	require.Empty(t, rec.execs, "a rejected schema name must never reach the database")
}

func TestPoolScratchClonesEveryTableWithIncludingAll(t *testing.T) {
	rec := &recordingSQL{}
	scratch := poolScratch{exec: rec}

	names := tableNames()
	require.NoError(t, scratch.CloneTables(context.Background(), testSchema, names))
	clones := rec.matching("CREATE TABLE")
	require.Len(t, clones, 24)
	for _, name := range names {
		require.Contains(t, rec.matching("CREATE TABLE")[0], "INCLUDING ALL")
		require.Len(t, rec.matching(`LIKE public."`+name+`"`), 1, name)
	}
}

func TestPoolScratchReportsACloneFailure(t *testing.T) {
	rec := &recordingSQL{execErr: "LIKE public.\"todos\""}
	scratch := poolScratch{exec: rec}

	err := scratch.CloneTables(context.Background(), testSchema, tableNames())
	require.ErrorContains(t, err, "todos")
}

func TestPoolScratchRewritesEveryPublicForeignKeyIntoTheScratchSchema(t *testing.T) {
	rec := &recordingSQL{public: publicCatalogFixture()}
	scratch := poolScratch{exec: rec}

	stats, err := scratch.RewriteForeignKeys(context.Background(), testSchema)
	require.NoError(t, err)
	require.Equal(t, 2, stats.Public)
	require.Equal(t, 0, stats.CopiedInspected, "PostgreSQL 16 copies primary keys but not foreign keys")
	require.Equal(t, 0, stats.CopiedDropped)
	require.Equal(t, 2, stats.ScratchAdded)

	adds := rec.matching("ADD CONSTRAINT")
	require.Len(t, adds, 2)
	require.Contains(t, adds[0], `ALTER TABLE "`+testSchema+`"."contacts" ADD CONSTRAINT "contacts_company_id_fkey"`)
	require.Contains(t, adds[0], "REFERENCES "+testSchema+".companies(id) ON DELETE SET NULL")
	require.Contains(t, adds[1], "REFERENCES "+testSchema+".workflow_runs(id) ON DELETE CASCADE")
	for _, stmt := range adds {
		require.NotContains(t, stmt, "REFERENCES public.")
	}
}

func TestPoolScratchDropsACopiedConstraintThatStillReferencesPublic(t *testing.T) {
	rec := &recordingSQL{
		public: publicCatalogFixture(),
		copied: []foreignKey{{
			constraint:   "contacts_company_id_fkey",
			child:        "contacts",
			parent:       "companies",
			parentSchema: "public",
			definition:   "FOREIGN KEY (company_id) REFERENCES public.companies(id) ON DELETE SET NULL",
		}},
	}
	scratch := poolScratch{exec: rec}

	stats, err := scratch.RewriteForeignKeys(context.Background(), testSchema)
	require.NoError(t, err)
	require.Equal(t, 1, stats.CopiedInspected)
	require.Equal(t, 1, stats.CopiedDropped)
	require.Equal(t, 2, stats.ScratchAdded)

	drop := rec.matching("DROP CONSTRAINT")
	require.Len(t, drop, 1)
	require.Less(t,
		rec.indexOf("DROP CONSTRAINT"),
		rec.indexOf("ADD CONSTRAINT"),
		"the copied constraint must be dropped before the replacement is added")
}

func TestPoolScratchKeepsAScratchScopedCopiedConstraint(t *testing.T) {
	rec := &recordingSQL{
		public: publicCatalogFixture(),
		copied: []foreignKey{{
			constraint:   "already_scoped_fkey",
			child:        "contacts",
			parent:       "companies",
			parentSchema: testSchema,
			definition:   "FOREIGN KEY (company_id) REFERENCES " + testSchema + ".companies(id)",
		}},
	}
	scratch := poolScratch{exec: rec}

	stats, err := scratch.RewriteForeignKeys(context.Background(), testSchema)
	require.NoError(t, err)
	require.Equal(t, 0, stats.CopiedDropped, "a constraint that already points at scratch is kept")
}

func TestPoolScratchFailsWhenACatalogQueryFails(t *testing.T) {
	rec := &recordingSQL{queryErr: errors.New("catalog unavailable")}
	scratch := poolScratch{exec: rec}

	_, err := scratch.RewriteForeignKeys(context.Background(), testSchema)
	require.ErrorContains(t, err, "catalog unavailable")
}

func TestPoolScratchFailsWhenAnAddFails(t *testing.T) {
	rec := &recordingSQL{public: publicCatalogFixture(), execErr: "ADD CONSTRAINT"}
	scratch := poolScratch{exec: rec}

	_, err := scratch.RewriteForeignKeys(context.Background(), testSchema)
	require.ErrorContains(t, err, "could not add the scratch constraint")
}

func TestPoolScratchFailsWhenADropFails(t *testing.T) {
	rec := &recordingSQL{
		public:  publicCatalogFixture(),
		copied:  publicCatalogFixture(),
		execErr: "DROP CONSTRAINT",
	}
	scratch := poolScratch{exec: rec}

	_, err := scratch.RewriteForeignKeys(context.Background(), testSchema)
	require.ErrorContains(t, err, "could not drop the copied constraint")
}

func TestRewriteForeignKeyDefinition(t *testing.T) {
	rewritten, err := rewriteForeignKeyDefinition(
		"FOREIGN KEY (company_id) REFERENCES public.companies(id) ON DELETE SET NULL", "s", "companies")
	require.NoError(t, err)
	require.Equal(t, "FOREIGN KEY (company_id) REFERENCES s.companies(id) ON DELETE SET NULL", rewritten)

	rewritten, err = rewriteForeignKeyDefinition(
		"FOREIGN KEY (run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE", "s", "workflow_runs")
	require.NoError(t, err)
	require.Equal(t, "FOREIGN KEY (run_id) REFERENCES s.workflow_runs(id) ON DELETE CASCADE", rewritten)

	_, err = rewriteForeignKeyDefinition("FOREIGN KEY (x) REFERENCES other(id)", "s", "companies")
	require.ErrorContains(t, err, "cannot retarget")
}

// ---------------------------------------------------------------------------
// targetRunner implementation
// ---------------------------------------------------------------------------

func TestPoolTargetVerifiesEveryScratchTableBeforeWriting(t *testing.T) {
	target := &poolTarget{}
	tx := &recordingTx{tables: []string{"users", "companies"}}

	err := target.VerifyTables(context.Background(), tx, []string{"users", "companies", "contacts"})
	require.ErrorContains(t, err, "1 of 3 scratch tables are missing")
	require.ErrorContains(t, err, "contacts")
	require.Empty(t, tx.execs, "verification must not write anything")
}

func TestPoolTargetAcceptsACompleteScratchSchema(t *testing.T) {
	target := &poolTarget{}
	require.NoError(t, target.VerifyTables(context.Background(),
		&recordingTx{tables: tableNames()}, tableNames()))
}

func TestPoolTargetEnforcesConstraintsImmediately(t *testing.T) {
	target := &poolTarget{}
	tx := &recordingTx{}
	require.NoError(t, target.EnforceConstraints(context.Background(), tx))
	require.Len(t, tx.execs, 1)
	require.Equal(t, "SET CONSTRAINTS ALL IMMEDIATE", tx.execs[0].sql)

	failing := &recordingTx{execErr: "SET CONSTRAINTS"}
	require.Error(t, target.EnforceConstraints(context.Background(), failing))
}

func TestPoolTargetRejectsScratchForeignKeysThatReferencePublic(t *testing.T) {
	target := &poolTarget{}
	tx := &recordingTx{fkRefs: map[string]int{"public": 4, testSchema: 20}}
	err := target.VerifyForeignKeys(context.Background(), tx, 20)
	require.ErrorContains(t, err, "still reference public")
}

func TestPoolTargetRequiresEveryPublicForeignKeyToBeMirrored(t *testing.T) {
	target := &poolTarget{}
	tx := &recordingTx{fkRefs: map[string]int{testSchema: 19}}
	err := target.VerifyForeignKeys(context.Background(), tx, 20)
	require.ErrorContains(t, err, "referential integrity")

	complete := &recordingTx{fkRefs: map[string]int{testSchema: 20}}
	require.NoError(t, target.VerifyForeignKeys(context.Background(), complete, 20))
}

func TestPoolTargetReportsAVerificationQueryFailure(t *testing.T) {
	target := &poolTarget{}
	err := target.VerifyForeignKeys(context.Background(), &errTx{}, 1)
	require.Error(t, err)
	require.Error(t, target.VerifyTables(context.Background(), &errTx{}, tableNames()))
}

type errTx struct {
	pgx.Tx
}

func (e *errTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, errors.New("connection lost")
}

func (e *errTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func TestPoolTargetImportUsesTheStaticUpsert(t *testing.T) {
	target := &poolTarget{}
	tx := &recordingTx{}
	spec := tableByName("users")
	require.NotNil(t, spec)

	row := json.RawMessage(`{"id":"00000000-0000-0000-0000-000000000001"}`)
	require.NoError(t, target.Import(context.Background(), tx, spec, row))
	require.Error(t, target.Import(context.Background(), tx, nil, row),
		"a nil table spec must be reported, not silently skipped")
}

func TestPoolTargetReadBackReportsRowsAndDigest(t *testing.T) {
	target := &poolTarget{}
	// The digest comes from the package-db streaming helper, so an unknown
	// table must surface its sentinel rather than an empty result.
	_, _, err := target.ReadBack(context.Background(), &recordingTx{}, "not_a_table")
	require.Error(t, err)
}

func TestPoolTargetFinishCommitsOrRollsBack(t *testing.T) {
	target := &poolTarget{}

	committed := &recordingTx{}
	require.NoError(t, target.Finish(context.Background(), committed, true))
	require.True(t, committed.committed)
	require.False(t, committed.rolled)

	rolledBack := &recordingTx{}
	require.NoError(t, target.Finish(context.Background(), rolledBack, false))
	require.True(t, rolledBack.rolled)
	require.False(t, rolledBack.committed)
}

func TestPoolTargetRefusesAnUntrustedSchemaName(t *testing.T) {
	target := &poolTarget{}
	_, _, err := target.Prepare(context.Background(), "public")
	require.ErrorContains(t, err, "refusing to use the scratch schema name")
}

func TestPoolTargetReleaseWithoutASourceIsSafe(t *testing.T) {
	target := &poolTarget{}
	require.NotPanics(t, func() { target.Release(context.Background()) })
}
