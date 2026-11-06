# Web Dependency Audit Policy

**Why this exists.** `pnpm audit` in CI reports every advisory forever. A check that is
permanently red is not a tracked risk, it is noise that everyone mutes — and a muted check
is how `--admin` merges over red CI become normal. This repo has already had to fix that
pattern once.

So the web audit is split into **"unaccepted"** and **"accepted, dated, visible"**.

## The rule

`scripts/check-web-audit.mjs` replaces `pnpm audit` in CI and fails on:

| Label | Meaning | Why it blocks |
| --- | --- | --- |
| `NEW` | an advisory with no matching acceptance | nothing was ever decided about it |
| `STALE` | the acceptance said "no patch exists", and one now does | the reason for accepting it is gone |
| `DEAD` | an acceptance that matches nothing anymore | the file is rotting |
| `OVERDUE` | the acceptance is past its `reviewBy` date | silence is not a decision |
| `ORPHANED` | the acceptance's linked issue is closed or does not exist | a dead reference is not a paper trail |

An accepted, still-unpatchable, in-date finding **passes** — and is printed in full on
every run: package, severity, who accepted it, when, until when, which issue, why.

A `pnpm audit` run that cannot be executed or parsed also fails. A check that did not
look must not report success.

The `ORPHANED` check queries the linked issue's state via `gh api`. If GitHub cannot be
reached (no `gh`, no network, no permission), the state is *unverifiable* — printed as a
`WARNING`, never a failure: a network hiccup must not turn CI red.

## The coverage boundary — what a green gate does NOT mean

**A green gate does not mean "no known vulnerabilities", it means "none known to pnpm's
source".** `pnpm audit` only sees advisories npm's registry mirror has already synced,
and that source lags GitHub's advisory database. Measured on 2026-10-20: GitHub carried
the vite/`launch-editor` advisory as Dependabot alert #6 for ~33 minutes before
`pnpm audit` reported it — during that window the gate printed `OK` while the Security
tab had an open alert. (The inverse also happens: the source catches up overnight and
`main` goes red through no one's merge.)

To make that gap visible instead of structural, every run of the gate also queries the
GitHub Dependabot alerts API (`dependabot/alerts?state=open`, via `gh api --paginate`)
and prints, with a query timestamp, the advisory ids (GHSA) seen by **each** source
plus their difference. The paginated call is parsed as a sequence of concatenated
JSON documents — one array per page, back to back. Deliberately **without** `gh api
--slurp`: the flag does not exist in every gh 2.x build (gh 2.46.0 fails it with
`unknown flag: --slurp`), and when it is unsupported the whole cross-check used to die
on that CLI error while the gate still exited on the old rules alone — silently
disarmed from the day PR #78 merged until 2026-11-06.

Each GitHub-only alert is classified against `web/pnpm-lock.yaml` before it is
printed, so "GitHub-only" never means one uniform reassurance:

- The package **is** in the locked tree → the pnpm advisory source is lagging. Printed
  as a visible, **non-blocking `WARNING`** ("source lag, not absence") — a second
  blocking source would tie CI to external database sync.
- The package is **not** in `web/pnpm-lock.yaml` → a stale alert, a different
  manifest, or a tree mismatch. Printed as `UNVERIFIED, not accepted`: the
  cross-check explicitly cannot rule it out, and the fix is to close the alert in the
  Security tab with a reason or explain it in an issue — never to shrug.
- The lockfile itself cannot be read → also `UNVERIFIED`, with the unreadable
  lockfile named.
- An **empty** alert list (`none (0)`) is verified-and-empty, distinct from "could not
  check" — the two must never be conflated.

## The degradation policy — strict where the token can work, loud elsewhere

Before issue #107 was fixed, the cross-check **could never authenticate on CI**:
`ci.yml` granted the workflow no `permissions:` and the audit step no token (the
runner does not export `GITHUB_TOKEN` into step environments on its own). Every CI
run therefore took the degraded path — which the old policy said must never go red.
The gate "passed" while its GitHub half was structurally dead. That is the bug class:
unverified silently equalled verified.

The fix has two halves, and the policy now distinguishes **unverified** from
**verified-clean** by run type:

- **Strict runs** — `push` to `main` and same-repo `pull_request`s, i.e. the runs
  whose workflow token can be granted the alerts API. `ci.yml` sets
  `WEB_AUDIT_REQUIRE_CROSSCHECK` for exactly these. A cross-check that cannot verify
  GitHub's open Dependabot alerts **fails the step**, naming what was not verified
  and what to do. The failure message points at the two legitimate causes: a GitHub
  API outage (re-run once <https://www.githubstatus.com> is green) or a broken
  token/permission setup in `ci.yml`.
- **Non-strict runs** — local dev, fork PRs, and Dependabot-authored PRs, where the
  token is read-only and non-elevatable by design. There, an unverified cross-check
  keeps the old loud behaviour: `COULD NOT CHECK — UNVERIFIED (<reason>)` plus a
  `WARNING`, exit decided by the pnpm source and the exception file alone. A gate
  that goes red on every transient blip — or permanently red on every fork PR —
  gets muted within a week and is worse overall; that is why strict mode is scoped
  to the runs where it can actually hold the promise.
- **Bounded retries absorb blips.** Transient failures (HTTP 408/429/5xx,
  network-class errors) are retried up to 3 attempts with fixed 2s/4s backoff —
  at most ~6s of added waiting, fully logged, never an unbounded loop (runtime
  budget culture, AGENTS.md). 401/403 and missing-`gh` are configuration errors and
  fail fast without retrying.
- **`WEB_AUDIT_OFFLINE=1` + strict is a hard error.** Offline mode is for local
  offline tests and air-gapped work; a run that is required to verify may not
  declare itself offline. Outside strict mode it still just skips the network calls
  and prints the loud unverified warning.

The CI wiring that makes strict mode possible lives in `ci.yml`: the
`frontend-and-e2e` job declares `permissions: {contents: read, security-events:
read, issues: read}` (least privilege — the alerts API needs `security-events`, the
ORPHANED issue check needs `issues: read`), and the audit step passes
`GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}`. If GitHub ever tightens the alerts API
further and `read` is not sufficient, the minimum replacement is the narrowest
working scope on this job — never `write-all`, never dropping the env/permissions
to silence a red run. Deleting or relaxing any of that is itself the #107 bug
class, not a fix.

The `ORPHANED` linked-issue lookup deliberately stays outside the strict path: one
transient failure on an individual issue lookup must not red-face an otherwise
fully-verified run; it warns, and the cross-check result above it is what strict
mode protects.

Set `WEB_AUDIT_OFFLINE=1` (non-strict runs only, see above) to skip all network
calls; the gate then prints the skipped cross-check as `COULD NOT CHECK —
UNVERIFIED` and a `WARNING` and still evaluates the old rules.

## What an acceptance must contain

`web/audit-exceptions.json`, one entry per advisory:

```json
{
  "package": "braces",
  "vulnerableVersions": "<=3.0.3",
  "severity": "high",
  "patchedVersionsExpected": null,
  "issue": "https://github.com/boreddev1/openlocalcrm/issues/57",
  "reason": "…why this cannot be fixed here and now…",
  "acceptedBy": "…who decided…",
  "acceptedOn": "2026-10-20",
  "reviewBy": "2026-11-20"
}
```

`patchedVersionsExpected: null` is the mechanism that makes the entry self-destructing:
the moment upstream ships a patch, the check turns `STALE` and forces a decision instead
of letting the acceptance age quietly.

Matching is on **package + vulnerable range**, not on package name alone — an acceptance
for `<=3.0.3` does not cover a different range of the same package.

The `issue` link must point to an **open** issue for as long as the acceptance is
carried. Closing it is a decision that the matter is settled; if the exception is still
live, that decision is false and the gate says so (`ORPHANED`). Reopen the issue or
remove the acceptance — do not keep both.

## Forbidden

- `--audit-level`, `--ignore`, or dropping the CI step — these hide findings instead of tracking them.
- Removing `GH_TOKEN`, `WEB_AUDIT_REQUIRE_CROSSCHECK`, or the `security-events` /
  `issues` permissions from the audit job in `ci.yml` to silence a strict failure —
  that recreates the exact #107 bug: a gate that cannot look while reporting no problem.
- Setting `WEB_AUDIT_OFFLINE=1` in CI — strict runs reject the combination outright.
- Acceptances without `issue`, `reason`, `acceptedBy`, `acceptedOn`, `reviewBy`.
- Extending `reviewBy` without a new `reason`. Re-accepting is allowed; drifting is not.
- Closing the linked issue while the acceptance is still live (the gate fails `ORPHANED`).
- Silencing the cross-check outside `WEB_AUDIT_OFFLINE=1` for offline tests — the
  GitHub-vs-pnpm difference is the point; hiding it recreates the #76 blind spot.

## Currently accepted

| Package | Severity | Range | Issue | Accepted | Review by | Status upstream |
| --- | --- | --- | --- | --- | --- | --- |
| `braces` | high | `<=3.0.3` | [#57](https://github.com/boreddev1/openlocalcrm/issues/57) | 2026-10-20 | 2026-11-20 | no patch published (`patched_versions: null`) |

`braces` is transitive through `tailwindcss` v3. The real fix is the Tailwind v4
migration, which changes `web/dist` and therefore the `//go:embed`-ed binary content — a
project of its own, deliberately not smuggled into a dependency bump. Tracked in #57.

## Verifying locally

```bash
cd web && pnpm install --frozen-lockfile && node ../scripts/check-web-audit.mjs

# fully offline — skips the Dependabot cross-check and the issue-state check, prints
# them as unverified warnings, still evaluates NEW/STALE/DEAD/OVERDUE/ORPHANED-data rules:
cd web && WEB_AUDIT_OFFLINE=1 node ../scripts/check-web-audit.mjs
```

Exit `0` with the accepted findings and the per-source advisory ids printed, or a
non-zero exit naming one of `NEW` / `STALE` / `DEAD` / `OVERDUE` / `ORPHANED`.
