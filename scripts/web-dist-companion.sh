#!/usr/bin/env bash
#
# web-dist-companion.sh — automate the "rebuild web/dist" companion PR (issue #116).
#
# WHY THIS EXISTS
#   web/dist is committed and embedded into the Go binary via //go:embed (repo
#   invariant #5), and the ci.yml step "Freshness Gate — committed web/dist must
#   match a fresh build" fails every branch whose bundle is stale. That step runs
#   BEFORE the Playwright steps, so a stale dist also costs the E2E evidence
#   (measured on 2026-11-06: 5 runs, 2 skipped steps each). A Dependabot bump of a
#   *shipped* web dependency cannot carry its own dist rebuild — GitHub forbids
#   writing to Dependabot branches (maintainerCanModify=false) — so the manual
#   workaround was PR #114: one commit holding the rebuilt bundle. This script
#   turns that pattern into machinery.
#
#   It is called by .github/workflows/web-dist-rebuild.yml right after
#   `cd web && pnpm install --frozen-lockfile && pnpm build`. The script never
#   builds anything: it (a) compares the result with the committed tree using the
#   *same* condition ci.yml uses, and (b) moves that result into exactly one
#   long-lived companion PR.
#
# SUBCOMMANDS
#   check    Compare built web/dist with committed web/dist. Exit 0 either way,
#            export drift=true|false. No network, no gh, no writes.
#   probe    Is an open companion PR already there? Export has_open_pr/pr_number.
#            Read-only; degrades to "false" if gh/the token is unusable.
#   body     Print the PR body publish() would file (preview, no side effects).
#   publish  Drifted  -> commit ONLY web/dist on top of main, force-update the
#                        automation branch, open or update ONE companion PR.
#            Not drifted -> close a companion PR that has become pointless.
#
# NO-SPAM RULES (hard requirements from #116)
#   * One PR only, identified by head branch name — never "open another one".
#   * If the tree we would push is already the branch's tree: no push, no body
#     edit, no comment. A schedule tick must never add noise.
#   * A companion PR a human closed without merging silences new PRs for
#     COOLDOWN_DAYS — that close was a decision, not an oversight.
#   * Staging is pathspec-limited to web/dist, and anything changed outside
#     web/dist aborts the run instead of being smuggled into the bundle PR.
#
# ENV
#   GITHUB_REPOSITORY / GITHUB_OUTPUT / GITHUB_STEP_SUMMARY / GITHUB_RUN_ID /
#   GITHUB_SERVER_URL / GITHUB_TOKEN (probe+publish only)
#   BRANCH, BASE_BRANCH, WEB_DIR, DIST_DIR, COOLDOWN_DAYS, GH  (all have defaults)
#
set -euo pipefail

BRANCH="${BRANCH:-automation/web-dist-rebuild}"
BASE_BRANCH="${BASE_BRANCH:-main}"
WEB_DIR="${WEB_DIR:-web}"
DIST_DIR="${DIST_DIR:-${WEB_DIR}/dist}"
LOCK_FILE="${WEB_DIR}/pnpm-lock.yaml"
PKG_JSON="${WEB_DIR}/package.json"
COOLDOWN_DAYS="${COOLDOWN_DAYS:-7}"
GH="${GH:-gh}"
PR_MARKER="openlocalcrm:web-dist-rebuild-companion"
COMMIT_AUTHOR_NAME="${COMMIT_AUTHOR_NAME:-github-actions[bot]}"
COMMIT_AUTHOR_EMAIL="${COMMIT_AUTHOR_EMAIL:-41898282+github-actions[bot]@users.noreply.github.com}"

REPO=""
BUILD_REF="" # tip the bundle was built from (main) — captured before we commit
BASE_REV=""  # last commit that touched web/dist before BUILD_REF = the drift window start
DRIFT=""     # porcelain file list of the stale bundle, captured before we commit

log() { printf '%s\n' "$*"; }

# Diagnostics go to stderr: find_open_pr()'s stdout is parsed, so a stray log
# line there would be read as a pull request number.
warn() { printf '::warning::web-dist-companion: %s\n' "$*" >&2; }

die() {
  printf '::error::web-dist-companion: %s\n' "$*" >&2
  exit 1
}

init() {
  cd "$(git rev-parse --show-toplevel)"
  case "${DIST_DIR}" in
    /*) die "DIST_DIR must be repo-relative, got ${DIST_DIR}" ;;
  esac
  [ -d "$DIST_DIR" ] || die "no ${DIST_DIR} directory — run 'cd ${WEB_DIR} && pnpm build' first"
  REPO="${GITHUB_REPOSITORY:-}"
  if [ -z "$REPO" ]; then # local run: derive owner/repo from the origin URL
    REPO="$(git remote get-url origin 2>/dev/null |
      sed -E 's#^https?://[^/]+/##; s#^git@github\.com:##; s#\.git$##; s#^/##' || true)"
  fi
  BUILD_REF="$(git rev-parse HEAD)"
  BASE_REV="$(rebuild_base)"
}

out() { # GitHub Actions output channel; harmless when run locally
  [ -n "${GITHUB_OUTPUT:-}" ] || return 0
  printf '%s=%s\n' "$1" "$2" >>"$GITHUB_OUTPUT"
}

summary() { # Markdown on the run page ("Job summary"). %b, because callers pass
  # \n escapes that have to become real line breaks in the rendered summary.
  [ -n "${GITHUB_STEP_SUMMARY:-}" ] || return 0
  printf '%b\n' "$*" >>"$GITHUB_STEP_SUMMARY"
}

# --- detection -------------------------------------------------------------

# The exact condition of ci.yml's freshness gate, so the two can never disagree.
# --untracked-files=all so a NEW chunk file counts as drift, too.
drift_lines() { git status --porcelain --untracked-files=all -- "$DIST_DIR"; }

# Anything modified/created outside web/dist means the working tree is not just a
# rebuild (a different pnpm touching the lockfile, a stray file).
#   check/body -> warn only: detection must keep working, and a red nightly job
#                 over a harmless stray file teaches people to ignore the job.
#   publish    -> hard stop: that mode is about to commit and push.
other_changes() {
  git status --porcelain --untracked-files=all -- . ":(exclude)${DIST_DIR}"
}

assert_nothing_else_changed() {
  local other
  other="$(other_changes)"
  [ -z "$other" ] && return 0
  printf 'Unexpected changes outside %s:\n%s\n' "$DIST_DIR" "$other" >&2
  die "the bundle PR must contain nothing but ${DIST_DIR}; investigate the changes above (pnpm/node version drift is the usual cause)"
}

warn_if_other_changes() {
  local other
  other="$(other_changes)"
  [ -z "$other" ] && return 0
  warn "working tree changed outside ${DIST_DIR} (detection continues, publish would stop here): $(printf '%s' "$other" | tr '\n' ' ')"
}

# --- drift explanation -----------------------------------------------------

# Resolved versions of the direct dependencies of the "." importer of the
# lockfile at a given revision -> "name<TAB>version<TAB>runtime|dev".
# Parsed structurally (lockfileVersion 9 has fixed indentation) instead of with a
# YAML library, because adding an npm dependency just to format a PR body would
# violate the repo's dependency policy.
lock_versions() {
  local rev="$1"
  git show "${rev}:${LOCK_FILE}" 2>/dev/null |
    awk '
      BEGIN { imp = ""; sec = ""; name = "" }
      /^importers:[ ]*$/         { inimp = 1; next }
      inimp && /^[^ ]/           { inimp = 0 }
      inimp && /^  [^ ].*:[ ]*$/ { imp = $0; sub(/^  /, "", imp); sub(/:[ ]*$/, "", imp); gsub("\047", "", imp); next }
      inimp && imp == "." {
        if ($0 ~ /^    (dependencies|devDependencies):[ ]*$/) {
          sec = ($0 ~ /devDependencies/) ? "dev" : "runtime"; next
        }
        if ($0 ~ /^      [^ ].*:[ ]*$/) {
          name = $0; sub(/^      /, "", name); sub(/:[ ]*$/, "", name); gsub("\047", "", name); next
        }
        if ($0 ~ /^        version:[ ]*/ && name != "") {
          v = $0; sub(/^ *version: */, "", v); sub(/\(.*/, "", v)
          printf "%s\t%s\t%s\n", name, v, sec; name = ""
        }
      }
    '
}

# Markdown table rows for the direct dependency versions that moved between two
# revisions. Empty output means the lockfile resolution did not change at all.
version_table() {
  local old new
  old="$(mktemp)"; new="$(mktemp)"
  lock_versions "$1" | awk -F'\t' '{ printf "%s\t%s\t%s\tOLD\n", $1, $2, $3 }' >"$old"
  lock_versions "$2" | awk -F'\t' '{ printf "%s\t%s\t%s\tNEW\n", $1, $2, $3 }' >"$new"
  awk -F'\t' '
    $4 == "OLD" { o[$1] = $2; k[$1] = $3; seen[$1] = 1; next }
    $4 == "NEW" { n[$1] = $2; k[$1] = $3; seen[$1] = 1; next }
    END {
      for (p in seen) {
        if ((p in o) && (p in n)) { if (o[p] != n[p]) printf "| `%s` | %s | %s | %s |\n", p, o[p], n[p], k[p] }
        else if (p in n) { printf "| `%s` | (neu im Lockfile) | %s | %s |\n", p, n[p], k[p] }
        else { printf "| `%s` | %s | (entfernt) | %s |\n", p, o[p], k[p] }
      }
    }
  ' "$old" "$new" | sort -f
  rm -f "$old" "$new"
}

web_commits() { git log --format='  * `%h` %s' "$1..$2" -- "$WEB_DIR" 2>/dev/null | head -n 25; }

# The commit the *committed* bundle still represents: the last one that touched
# web/dist. Everything after it is the drift window. If HEAD itself is that
# commit, step back one, because then the window has no useful start.
rebuild_base() {
  local b head
  head="$(git rev-parse HEAD)"
  b="$(git log -1 --format='%H' -- "$DIST_DIR" 2>/dev/null || true)"
  if [ "$b" = "$head" ]; then
    b="$(git log -1 --format='%H' HEAD~1 -- "$DIST_DIR" 2>/dev/null || true)"
  fi
  printf '%s' "${b:-$head}"
}

run_link() {
  local server="${GITHUB_SERVER_URL:-https://github.com}"
  if [ -n "${GITHUB_REPOSITORY:-}" ] && [ -n "${GITHUB_RUN_ID:-}" ]; then
    printf '%s/%s/actions/runs/%s' "$server" "$GITHUB_REPOSITORY" "$GITHUB_RUN_ID"
  elif [ -n "$REPO" ]; then
    printf '%s/%s/actions' "$server" "$REPO"
  else
    printf '(lokal ausgeführt — kein Workflow-Run)'
  fi
}

# --- pull request lookups --------------------------------------------------

# Open companion PR for this branch, as "number url" ("" when there is none).
find_open_pr() {
  "$GH" pr list --repo "$REPO" --state open --base "$BASE_BRANCH" --head "$BRANCH" \
    --json number,url --jq '.[] | "\(.number) \(.url)"' 2>/dev/null ||
    warn "could not query pull requests (gh unavailable, or the token lacks pull-requests:read); assuming no companion PR is open"
}

# Most recently closed-but-unmerged companion PR, as "number closedAtIso" ("" if none).
last_unmerged_pr() {
  "$GH" pr list --repo "$REPO" --state closed --base "$BASE_BRANCH" --head "$BRANCH" \
    --limit 20 --json number,closedAt,mergedAt \
    --jq '.[] | select(.mergedAt == null) | "\(.closedAt) \(.number)"' 2>/dev/null |
    sort -r | head -n 1 | awk '{ print $2, $1 }'
}

# A PR a human closed without merging is a decision. Stay quiet for COOLDOWN_DAYS.
pr_create_allowed() {
  local last closed_at epoch age_days
  last="$(last_unmerged_pr || true)"
  [ -z "$last" ] || {
    closed_at="${last#* }"
    epoch="$(date -u -d "$closed_at" +%s 2>/dev/null || echo '')"
    [ -n "$epoch" ] || return 0
    age_days=$(( ($(date -u +%s) - epoch) / 86400 ))
    if [ "$age_days" -lt "$COOLDOWN_DAYS" ]; then
      log "PR #${last%% *} was closed without merging ${age_days} day(s) ago (< ${COOLDOWN_DAYS}d cooldown) — not opening a new companion PR."
      return 1
    fi
  }
  return 0
}

# --- the PR body -----------------------------------------------------------

pr_body() {
  local drift tbl commits
  # Captured before the rebuild commit: after committing, the tree is clean again
  # by definition, and the body would have nothing to show.
  drift="${DRIFT:-$(drift_lines)}"
  tbl="$(version_table "$BASE_REV" "$BUILD_REF")"
  commits="$(web_commits "$BASE_REV" "$BUILD_REF")"

  cat <<EOF
<!-- ${PR_MARKER} -->
**Automatischer Begleit-PR** — geöffnet von \`.github/workflows/web-dist-rebuild.yml\` (Schedule oder \`workflow_dispatch\`), nicht von einem Menschen. Es geht um Issue #116: Jeder Bump einer *ausgelieferten* web-Dependency ändert die Chunk-Hashes in \`web/dist\`, und das Frische-Gate in \`ci.yml\` fällt genau deshalb rot — auf Dependabot-Branches lässt sich der nötige Rebuild nicht nachreichen (\`maintainerCanModify=false\`). Dieser PR ist die Automatisierung des Musters aus #114.

Enthalten sind **ausschließlich** die neu gebauten Dateien unter \`${DIST_DIR}\` — kein \`web/src\`, kein \`${PKG_JSON}\`, kein \`web/pnpm-workspace.yaml\`.

## Was hat sich seit dem letzten Bundle-Commit geändert?

Verglichen wird \`${LOCK_FILE}\` zwischen dem letzten Commit, der \`${DIST_DIR}\` berührt hat (\`${BASE_REV:0:7}\`), und dem Build-Stand \`${BUILD_REF:0:7}\` (= \`$BASE_BRANCH\`). Aufgelöste Versionen der direkten Abhängigkeiten; \`${PKG_JSON}\` selbst wird hier nicht angefasst.

| Paket | vorher | jetzt | Typ |
| --- | --- | --- | --- |
${tbl:-_Keine Versionsänderung in den direkten Abhängigkeiten. Die Abweichung stammt aus \`web/src\` oder aus dem Build-Output selbst._}

Commits, die seitdem \`web/\` berührt haben:

${commits:-_Keine — der committete Bundle-Stand passt zu keinem aktuellen \`web/\`-Stand._}

## Was ist anders am Bundle?

\`\`\`
${drift}
\`\`\`

## Status der Checks auf diesem PR (wichtig, und nicht schön)

Dieser PR wurde mit dem eingebauten \`GITHUB_TOKEN\` aus einem Workflow heraus geöffnet.
GitHub unterdrückt deshalb den gewöhnlichen Workflow-Start — sinngemäß aus den
Actions-Docs („When \`GITHUB_TOKEN\` triggers workflow runs“): _„events triggered by the
\`GITHUB_TOKEN\` will not create a new workflow run“_ — mit einer ausdrücklich
dokumentierten Ausnahme für Automation: _„when a workflow using \`GITHUB_TOKEN\` creates or
updates a pull request, the resulting \`pull_request\` event creates workflow runs in an
**approval-required** state“_, also _„still allowing CI workflows to run on pull requests
created by automation“_. Praktisch:

1. **Ein Klick mit write-Rechten genügt:** im Checks-Bereich **„Approve and run“**. Dann
   laufen die beiden required Checks \`Web Frontend Build & Playwright E2E\` und
   \`Go Backend Tests & License Check\` genau gegen diesen Bundle-Stand — inklusive dem
   Frische-Gate **und** der Playwright-Suite, die laut #116 bei den betroffenen Bumps gar
   nicht erst gelaufen ist. Das ist der vorgesehene Weg.
2. Alternativ: ein Mensch pusht einen (leeren) Commit auf diesen Branch. Benutzer-Events
   starten die PR-Checks ohne Freigabe.
3. Nur als Rückfall: \`main\` hat Branch-Protection mit \`enforce_admins=false\`; Admins
   können übersteuern. Damit ist die Bundle-Änderung aber gerade *nicht* browsergeprüft —
   bitte nicht als Normalzustand.

Gewollt **kein** \`workflow_dispatch\`-Trick: \`ci.yml\` hat (noch) keinen
\`workflow_dispatch\`-Trigger, und Läufe aus \`workflow_dispatch\` erfüllen laut
GitHub-Docs keine required status check („check the event that triggered the workflow
run“). Ein zweiter Workflow mit \`actions: write\`, der \`ci.yml\` selbst startet, wäre mehr
Berechtigung, als dieser Automation zusteht.

## Was der erzeugende Run geprüft hat

Der Workflow hat exakt die Schritte von \`ci.yml\` ausgeführt und danach verglichen —
Protokoll: $(run_link)

\`\`\`sh
cd web && pnpm install --frozen-lockfile && pnpm build   # pnpm 12.4.1, node 22
git status --porcelain --untracked-files=all -- ${DIST_DIR}   # nicht leer => dieser PR
\`\`\`

Nach dem Merge ist das Frische-Gate auf \`$BASE_BRANCH\` wieder grün; der nächste
Scheduled-Run meldet „keine Abweichung“ und öffnet nichts Neues.

## Wiederholungen / Branch-Eigentum

\`$BRANCH\` gehört dieser Automation und wird bei jedem Lauf auf \`$BASE_BRANCH\` + genau
diesen einen Bundle-Commit **zurückgesetzt** (\`--force-with-lease\`). Eigene Commits auf
diesem Branch gehen dabei verloren — Änderungen bitte woanders einpflegen und diesen PR
mergen oder schließen. Ein geschlossener, nicht gemergter Begleit-PR unterdrückt weitere
für ${COOLDOWN_DAYS} Tage.
EOF
}

# --- subcommands -----------------------------------------------------------

cmd_check() {
  local drift
  warn_if_other_changes
  drift="$(drift_lines)"
  if [ -z "$drift" ]; then
    out drift "false"
    log "OK: committed ${DIST_DIR} is byte-identical to a fresh production build."
    summary "### ${DIST_DIR} ist frisch\n\`cd ${WEB_DIR} && pnpm build\` (pnpm 12.4.1 / node 22) liefert exakt den Stand, der auf \`$BASE_BRANCH\` committet ist. Kein Begleit-PR nötig."
    return 0
  fi
  out drift "true"
  out drift_count "$(printf '%s\n' "$drift" | grep -c .)"
  log "::warning::committed ${DIST_DIR} is STALE — a fresh 'cd ${WEB_DIR} && pnpm build' produces different output (same condition as ci.yml's freshness gate)."
  log "Differing files:"
  printf '%s\n' "$drift"
  summary "### ${DIST_DIR} ist STALE\nDer frische Build von \`$BASE_BRANCH\` unterscheidet sich vom committeten Bundle; das Frische-Gate in \`ci.yml\` fällt auf \`$BASE_BRANCH\` rot und überspringt dadurch die Playwright-Steps. Es wird genau EIN Begleit-PR mit dem neuen Bundle geöffnet bzw. aktualisiert.\n\n\`\`\`\n${drift}\n\`\`\`"
}

cmd_probe() {
  local found
  found="$(find_open_pr || true)"
  if [ -n "$found" ]; then
    out has_open_pr "true"
    out pr_number "${found%% *}"
    log "Open companion PR already exists: ${found}"
  else
    out has_open_pr "false"
    out pr_number ""
    log "No open companion PR on branch ${BRANCH}."
  fi
}

cmd_publish() {
  command -v "$GH" >/dev/null 2>&1 || die "gh is required for publish"
  [ -n "$REPO" ] || die "cannot determine owner/repo (set GITHUB_REPOSITORY)"
  assert_nothing_else_changed

  local drift
  drift="$(drift_lines)"
  DRIFT="$drift"

  # No drift: the only useful write left is retiring a companion PR that became empty.
  if [ -z "$drift" ]; then
    local found num url
    found="$(find_open_pr || true)"
    if [ -n "$found" ]; then
      num="${found%% *}"; url="${found#* }"
      log "No drift anymore — closing the now-pointless companion PR #${num} (${url})."
      "$GH" pr close "$num" --repo "$REPO" \
        --comment "Kein Drift mehr zwischen frischem Build und committetem \`${DIST_DIR}\`: dieser Begleit-PR hätte keine Änderung mehr. Geschlossen von \`.github/workflows/web-dist-rebuild.yml\`." ||
        die "failed to close PR #${num}"
      summary "### Begleit-PR #${num} geschlossen\nDer Bundle-Stand auf \`$BASE_BRANCH\` ist wieder identisch mit dem frischen Build; dieser Begleit-PR hätte nichts mehr geändert."
    else
      log "No drift and no open companion PR — nothing to do."
    fi
    return 0
  fi

  # 1. Decide first, write second: a companion PR a human closed without merging
  #    must not even leave a stray branch push behind.
  local found
  found="$(find_open_pr || true)"
  if [ -z "$found" ] && ! pr_create_allowed; then
    log "Drift detected, but no companion PR is open and the cooldown after a human-closed one is still running — reporting only."
    summary "### Kein neuer Begleit-PR\nDrift erkannt, aber ein Mensch hat zuletzt einen Begleit-PR ohne Merge geschlossen; Cooldown von ${COOLDOWN_DAYS} Tagen läuft noch. Branch und PR bleiben unberührt, der Drift steht als Warnung auf diesem Run."
    return 0
  fi

  # 2. One commit, only web/dist, on top of the base branch we just built from.
  git config user.name "$COMMIT_AUTHOR_NAME"
  git config user.email "$COMMIT_AUTHOR_EMAIL"
  git add -A -- "$DIST_DIR"
  git diff --cached --quiet && die "drift was detected but nothing under ${DIST_DIR} staged"
  local subject body_file num url remote_sha pushed
  subject="chore(web): rebuild embedded dist from ${BASE_BRANCH}@$(git rev-parse --short "$BUILD_REF")"
  git commit -q -m "$subject" -m "Automatisch gebaut von .github/workflows/web-dist-rebuild.yml
(cd ${WEB_DIR} && pnpm install --frozen-lockfile && pnpm build; pnpm 12.4.1 / node 22)
gegen ${BASE_BRANCH} @ $(git rev-parse --short "$BUILD_REF"). Der committete Bundle-Stand
war nicht mehr byte-identisch zu diesem Build — Frische-Gate in ci.yml. Siehe Issue #116.

Run: $(run_link)"

  # Belt and braces: this commit must not contain anything but the bundle.
  local unexpected
  unexpected="$(git show --name-only --format= HEAD | grep -v "^${DIST_DIR}/" || true)"
  [ -z "$unexpected" ] || die "the rebuild commit touches files outside ${DIST_DIR}: ${unexpected}"

  # 3. Update the single automation branch, or create it.
  remote_sha=""
  if git ls-remote --exit-code --heads origin "$BRANCH" >/dev/null 2>&1; then
    git fetch -q origin "+refs/heads/${BRANCH}:refs/remotes/origin/${BRANCH}" ||
      die "could not fetch ${BRANCH} from origin"
    remote_sha="$(git rev-parse "refs/remotes/origin/${BRANCH}")"
  fi

  pushed="yes"
  if [ -n "$remote_sha" ] &&
    [ "$(git rev-parse 'HEAD^{tree}')" = "$(git rev-parse "${remote_sha}^{tree}")" ]; then
    pushed="no"
    log "Branch ${BRANCH} already carries exactly this ${DIST_DIR} tree (${remote_sha}) — no push."
  elif [ -n "$remote_sha" ]; then
    log "Resetting ${BRANCH}: ${remote_sha} -> $(git rev-parse --short HEAD) (bundle tree differs)."
    git push -q --force-with-lease="refs/heads/${BRANCH}:${remote_sha}" \
      origin "HEAD:refs/heads/${BRANCH}" ||
      die "force-with-lease push to ${BRANCH} failed (branch moved during the run? the next tick retries)"
  else
    log "Creating branch ${BRANCH} at $(git rev-parse --short HEAD)."
    git push -q origin "HEAD:refs/heads/${BRANCH}" ||
      die "push of new branch ${BRANCH} failed"
  fi

  # 4. Exactly one PR: update the open one, else create the one this branch gets.
  body_file="$(mktemp)"
  pr_body >"$body_file"

  if [ -n "$found" ]; then
    num="${found%% *}"; url="${found#* }"
    if [ "$pushed" = "yes" ]; then
      "$GH" pr edit "$num" --repo "$REPO" --body-file "$body_file" ||
        die "failed to update the body of PR #${num}"
      log "Companion PR #${num} updated: ${url}"
      summary "### Begleit-PR #${num} aktualisiert\n${url}"
    else
      log "Companion PR #${num} is already up to date — no push, no body edit, no comment."
      summary "### Begleit-PR #${num} bleibt, wie er ist\nDer Bundle-Stand auf dem Branch ist identisch; dieser Run ändert nichts (kein Kommentar, keine Aktualisierung)."
    fi
  else
    # No open companion PR, and pr_create_allowed() already cleared the human
    # cooldown in step 1.
    num="$("$GH" pr create --repo "$REPO" --base "$BASE_BRANCH" --head "$BRANCH" \
      --title "$subject" --body-file "$body_file")" ||
      die "gh pr create failed"
    log "Opened companion PR: ${num}"
    summary "### Neuer Begleit-PR\n${num}"
  fi
  rm -f "$body_file"
}

usage() {
  cat <<EOF
Usage: $(basename "$0") <check|probe|body|publish>

  check    compare built ${DIST_DIR} with committed ${DIST_DIR}; export drift=true|false
  probe    report whether an open companion PR exists (read-only)
  body     print the companion PR body that publish would file (read-only)
  publish  commit the rebuilt bundle and open/update exactly one companion PR

Run from anywhere in the repo after: cd ${WEB_DIR} && pnpm install --frozen-lockfile && pnpm build
EOF
}

main() {
  local mode="${1:-}"
  case "$mode" in
    check | probe | body | publish) ;;
    -h | --help | help | '')
      usage
      exit 2
      ;;
    *)
      printf 'unknown mode: %s\n\n' "$mode" >&2
      usage >&2
      exit 2
      ;;
  esac
  init
  case "$mode" in
    check) cmd_check ;;
    probe) cmd_probe ;;
    body) pr_body ;;
    publish) cmd_publish ;;
  esac
}

main "$@"
