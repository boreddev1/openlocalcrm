#!/usr/bin/env bash
# ==============================================================================
# OpenLocalCRM — Disposable PostgreSQL/pgvector gate for backup and invite SQL
# ==============================================================================
#
# The unit tests can prove that the restore and the scratch drill call the right
# statements, but only a real database can prove the parts that actually break in
# practice: the vector(1024) cast, the TEXT[] reconstruction, the enum casts, the
# foreign-key rewrite and the search_path isolation. The same is true for the
# invite statements, where the one-time-token guarantee rests on a row lock that
# an in-memory store can only imitate.
#
# This script owns that evidence:
#
#   * By default it starts a disposable pgvector/pgvector:pg16 container, applies
#     migrations 00001-00009, records 00010_foreign_key_indexes.sql in
#     schema_migrations as already applied, applies 00011 onward, runs the tests
#     and removes the container.
#   * It honours an exported TEST_DATABASE_URL, but only with
#     TEST_DATABASE_URL_ALLOW_EXISTING=1. The live tests write to their target
#     (they create users and invites, accept invites, and delete the rows they
#     seeded), so an existing database is opt-in: the script refuses to point at
#     one it did not provision unless the operator says so explicitly.
#
# Two groups of tests run against it: the backup scratch-drill round trip and the
# invite statements (issuance CTE plus the FOR UPDATE acceptance).
#
# 00010 is NOT executed and NOT edited. It indexes notes(contact_id), a column
# that no migration ever creates, so replaying the historical migration set from
# an empty database cannot work. Recording it as pre-applied reproduces the
# schema a live installation actually has. The only thing the fixture omits is
# that migration's performance indexes, not any application column.
# ==============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

IMAGE="${BACKUP_TEST_IMAGE:-pgvector/pgvector:pg16}"
CONTAINER_NAME="${BACKUP_TEST_CONTAINER:-openlocalcrm-backup-it-$$}"
SKIP_MIGRATION="00010_foreign_key_indexes.sql"
CONTAINER_STARTED=""

MIGRATION_DIR="internal/db/migrations"

log() { printf '%s\n' "$*"; }
fail() { printf '❌ %s\n' "$*" >&2; exit 1; }

cleanup() {
    local status=$?
    if [[ -n "${CONTAINER_STARTED}" ]]; then
        log "🧹 Removing disposable container ${CONTAINER_NAME}"
        docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
    fi
    exit "${status}"
}
trap cleanup EXIT INT TERM

wait_for_postgres() {
    local attempts=0
    until docker exec "${CONTAINER_NAME}" pg_isready -U backup_it -d backup_it >/dev/null 2>&1; do
        attempts=$((attempts + 1))
        if [[ "${attempts}" -gt 60 ]]; then
            docker logs "${CONTAINER_NAME}" || true
            fail "PostgreSQL did not become ready in time"
        fi
        sleep 1
    done
}

psql_in_container() {
    docker exec -i "${CONTAINER_NAME}" psql -v ON_ERROR_STOP=1 -U backup_it -d backup_it "$@"
}

apply_migration() {
    local file="$1"
    local name
    name="$(basename "${file}")"
    log "  → applying ${name}"
    # Strip the goose annotations: only the Up section is applied, and psql runs
    # it as one ON_ERROR_STOP transaction.
    {
        printf 'BEGIN;\n'
        awk '/^-- \+goose Down/{exit} {print}' "${file}"
        printf '\nINSERT INTO schema_migrations (version) VALUES (%s);\n' "'${name}'"
        printf 'COMMIT;\n'
    } | psql_in_container >/dev/null
}

migrations_up_to() {
    local prefix="$1"
    local found=0
    while IFS= read -r file; do
        local name
        name="$(basename "${file}")"
        [[ "${name}" == "${prefix}"* ]] || continue
        if [[ "${name}" == "${SKIP_MIGRATION}" ]]; then
            log "  → recording ${name} as already applied (never executed, never edited)"
            # </dev/null matters: docker exec -i would otherwise swallow the
            # remaining file list from the loop's own stdin.
            psql_in_container -c "INSERT INTO schema_migrations (version, applied_at)
                VALUES ('${name}', NOW()) ON CONFLICT DO NOTHING;" </dev/null >/dev/null
        else
            apply_migration "${file}"
        fi
        found=1
    done < <(find "${MIGRATION_DIR}" -maxdepth 1 -name '*.sql' | sort)
    [[ "${found}" -eq 1 ]] || fail "no migrations found in ${MIGRATION_DIR}"
}

start_disposable_database() {
    log "🐘 Starting disposable ${IMAGE} as ${CONTAINER_NAME}"
    docker run -d --rm \
        --name "${CONTAINER_NAME}" \
        -p 127.0.0.1::5432 \
        -e POSTGRES_USER=backup_it \
        -e POSTGRES_PASSWORD=backup_it \
        -e POSTGRES_DB=backup_it \
        "${IMAGE}" >/dev/null
    CONTAINER_STARTED="${CONTAINER_NAME}"
    wait_for_postgres
    # The migrator owns schema_migrations; the fixture recreates the same table.
    psql_in_container -c "CREATE TABLE IF NOT EXISTS schema_migrations (
        version VARCHAR(255) PRIMARY KEY,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW());" </dev/null >/dev/null

    local host_port
    host_port="$(docker port "${CONTAINER_NAME}" 5432/tcp | head -1 | sed 's/.*://')"
    [[ -n "${host_port}" ]] || fail "could not determine the mapped PostgreSQL port"
    export TEST_DATABASE_URL="postgres://backup_it:backup_it@127.0.0.1:${host_port}/backup_it?sslmode=disable"
}

if [[ -n "${TEST_DATABASE_URL:-}" ]]; then
    # The live tests write to whatever they are pointed at: they insert users and
    # invites, accept invites, and (since this round) remove what they seeded.
    # An operator who exports TEST_DATABASE_URL by accident — or who keeps a
    # development install running — must not have rows appear in it, so an
    # existing database is opt-in only.
    if [[ "${TEST_DATABASE_URL_ALLOW_EXISTING:-0}" != "1" ]]; then
        fail "TEST_DATABASE_URL is set, so the live tests would write to a database this script did not provision.

  The backup scratch-drill tests and the invite statement tests insert rows into
  the target and remove the rows they seeded. They do not provision it and they
  do not migrate it.

  Either:
    * unset TEST_DATABASE_URL and let this script start a disposable
      pgvector/pgvector:pg16 container (the default, and what make check does), or
    * set TEST_DATABASE_URL_ALLOW_EXISTING=1 to confirm the target is a
      throwaway database, for example:

        TEST_DATABASE_URL=postgres://user:pass@127.0.0.1:5432/throwaway \\
        TEST_DATABASE_URL_ALLOW_EXISTING=1 ./scripts/test-backup-integration.sh

  The target database is expected to be migrated already."
    fi
    log "🔗 Using TEST_DATABASE_URL=${TEST_DATABASE_URL}"
    log "   (the target database is expected to be migrated already)"
    log "⚠️  WARNING: TEST_DATABASE_URL_ALLOW_EXISTING=1 — this database was NOT provisioned by this script."
    log "⚠️  WARNING: the live tests WILL WRITE TO IT: they create users and invites, accept invites, and"
    log "⚠️  WARNING: delete the rows they seeded. Point this only at a throwaway database."
else
    start_disposable_database
    log "🧱 Applying the migration set to the disposable database"
    migrations_up_to ""
    log "🔗 Using TEST_DATABASE_URL=${TEST_DATABASE_URL}"
fi

log "🧪 Running the scratch-drill round-trip test against a real pgvector database"
TEST_DATABASE_URL="${TEST_DATABASE_URL}" go test ./internal/core/backup \
    -run 'TestScratchDrill' -count=1 -v

# The invite statements carry their own security property: a one-time token must
# activate an account exactly once, and a concurrent second acceptance must be
# serialised by the FOR UPDATE lock rather than allowed to overwrite the
# password. No unit test can prove that, and the demo querier can only
# approximate it, so the statements run against the same disposable database.
log "🧪 Running the invite statement test against a real pgvector database"
TEST_DATABASE_URL="${TEST_DATABASE_URL}" go test ./internal/core/invite \
    -run 'TestInvite.*Postgres' -count=1 -v

log "✅ Real PostgreSQL backup and invite statements verified"
