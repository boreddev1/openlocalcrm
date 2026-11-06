#!/usr/bin/env bash
#
# check-dist-freshness.sh — compare the web/dist tree ON DISK against the
# web/dist tree IN THE GIT INDEX, file by file, by content hash.
#
# WHY THIS EXISTS (issue #130)
#   The Go binary embeds web/dist from the filesystem (web/embed.go,
#   //go:embed dist/*), while the old freshness condition — ci.yml's
#   "Freshness Gate" step and scripts/web-dist-companion.sh — asked git:
#       git status --porcelain --untracked-files=all -- web/dist
#   git status never looks at files an ignore rule hides, and the root
#   .gitignore ignores repo-wide patterns that match real bundle output:
#   `*.png` (.gitignore:54) today, and `*.log`, `*.swp`, `node_modules/`,
#   `.DS_Store`, … as tomorrow's vite plugin could emit them just as easily.
#   Such a file is embedded into the shipped binary yet invisible to git,
#   to the freshness gate, and to the companion automation. Comparing the
#   actual trees instead of git's view of them closes that blind spot:
#   this check is a strict SUPERSET of the old condition — every change
#   `git status` reported is reported here, plus ignored-but-present files.
#
# MODES
#   build (default)  Do a fresh production build
#                    (cd web && pnpm install --frozen-lockfile && pnpm build),
#                    then run the comparison.
#   compare          Only run the comparison — for callers that have just
#                    built (used by scripts/web-dist-companion.sh check).
#
# OUTPUT
#   stdout: one line per offender, machine-readable, "<KIND>\t<path>\t[detail]"
#     NOT-IN-GIT       on disk, not tracked in the index. The detail names the
#                      .gitignore rule when git would hide the file — that is
#                      the #130 blind-spot class.
#     CONTENT-DIFFERS  bytes on disk differ from the index blob (sha256)
#     MISSING-ON-DISK  tracked in the index, absent from the build output
#   stderr: progress/human messages. Exit codes for CI and hooks:
#     0  no drift — disk == index, a fresh build is byte-identical to git
#     1  drift  — offenders listed on stdout
#     2  the check could not be performed (no git repo, no ${DIST_DIR},
#        pnpm missing, build failed) — never silently "clean"
#
# ENV (same knobs as scripts/web-dist-companion.sh)
#   WEB_DIR (default: web)  DIST_DIR (default: ${WEB_DIR}/dist)
#
set -euo pipefail

WEB_DIR="${WEB_DIR:-web}"
DIST_DIR="${DIST_DIR:-${WEB_DIR}/dist}"

log() { printf '%s\n' "$*" >&2; }
die() { log "check-dist-freshness: $*"; exit 2; }

usage() {
  cat <<EOF
Usage: $(basename "$0") [build|compare]

  build    cd ${WEB_DIR} && pnpm install --frozen-lockfile && pnpm build, then
           compare ${DIST_DIR} on disk with ${DIST_DIR} in the git index
  compare  only the hash comparison (the caller already built)

Exit 0 identical · 1 drift (offenders on stdout as KIND\tpath[\tdetail]) · 2 error.
EOF
}

mode="${1:-build}"
case "$mode" in
  -h | --help | help)
    usage
    exit 0
    ;;
  build | compare) ;;
  *)
    printf 'unknown mode: %s\n\n' "$mode" >&2
    usage >&2
    exit 2
    ;;
esac

git rev-parse --show-toplevel >/dev/null 2>&1 || die "not inside a git repository"
cd "$(git rev-parse --show-toplevel)"

# sha256 of stdin (Linux: sha256sum; macOS dev boxes: shasum -a 256).
if command -v sha256sum >/dev/null 2>&1; then
  hash_stdin() { sha256sum | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  hash_stdin() { shasum -a 256 | cut -d' ' -f1; }
else
  die "need sha256sum or shasum to hash files"
fi

case "$DIST_DIR" in
  /*) die "DIST_DIR must be repo-relative, got ${DIST_DIR}" ;;
esac

# --- a) fresh build (mode: build) --------------------------------------------

if [ "$mode" = build ]; then
  [ -d "$WEB_DIR" ] || die "no ${WEB_DIR} directory"
  command -v pnpm >/dev/null 2>&1 || die "pnpm is required for mode 'build' (use mode 'compare' after building yourself)"
  log "Building a fresh bundle: cd ${WEB_DIR} && pnpm install --frozen-lockfile && pnpm build"
  (cd "$WEB_DIR" && pnpm install --frozen-lockfile && pnpm build) || die "fresh web build failed — cannot judge freshness"
fi

[ -d "$DIST_DIR" ] || die "no ${DIST_DIR} directory — run 'cd ${WEB_DIR} && pnpm install --frozen-lockfile && pnpm build' first (or use mode 'build')"

# --- b) disk vs. index, both directions, by content hash ---------------------
# Deliberately NOT using `git status`: ignored files must be comparable, too.
# Paths with newlines are not representable in the one-line-per-offender
# format; a bundler does not emit those (vite hashes to URL-safe names).

tmp_disk="$(mktemp)" tmp_idx="$(mktemp)"
trap 'rm -f "$tmp_disk" "$tmp_idx"' EXIT

while IFS= read -r -d '' f; do
  printf '%s\t%s\n' "$(hash_stdin <"$f")" "$f"
done < <(find "$DIST_DIR" -type f -print0 | sort -z) >"$tmp_disk"

while IFS= read -r -d '' f; do
  # ":path" reads the index blob — exactly what would be committed, and what
  # //go:embed would ship if the file were ignored instead. A blob that cannot
  # be read gets an unmistakable sentinel hash, so a broken index entry can
  # never accidentally hash-match an empty file on disk.
  if git cat-file -e ":$f" 2>/dev/null; then
    h="$(git cat-file blob ":$f" | hash_stdin)"
  else
    h="UNREADABLE-INDEX-BLOB"
  fi
  printf '%s\t%s\n' "$h" "$f"
done < <(git ls-files -z -- "$DIST_DIR" | sort -z) >"$tmp_idx"

offenders="$(
  awk -F'\t' '
    NR == FNR { disk[$2] = $1; next }
    { idx[$2] = $1 }
    END {
      for (p in disk) {
        if (!(p in idx)) print "NOT-IN-GIT\t" p
        else if (disk[p] != idx[p]) print "CONTENT-DIFFERS\t" p
      }
      for (p in idx) if (!(p in disk)) print "MISSING-ON-DISK\t" p
    }
  ' "$tmp_disk" "$tmp_idx" | sort
)"

# --- c) annotated, named report ----------------------------------------------

if [ -z "$offenders" ]; then
  log "OK: every file under ${DIST_DIR} on disk is tracked with identical bytes, and every tracked file exists on disk — including files .gitignore would hide."
  exit 0
fi

while IFS=$'\t' read -r kind path; do
  if [ "$kind" = "NOT-IN-GIT" ] && rule="$(git check-ignore -v -- "$path" 2>/dev/null)"; then
    # Blind-spot class: present on disk (=> embedded by //go:embed), invisible to git.
    printf 'NOT-IN-GIT\t%s\tgitignored via %s — embedded by go:embed but git cannot see it (issue #130)\n' \
      "$path" "${rule%%$'\t'*}"
  else
    printf '%s\t%s\n' "$kind" "$path"
  fi
done <<<"$offenders"

exit 1
