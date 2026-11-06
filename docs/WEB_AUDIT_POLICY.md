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

## The degradation policy — fail-closed exactly where verification is possible

Before issue #107 was fixed, the cross-check **could never authenticate on CI**:
`ci.yml` granted the workflow no `permissions:` and the audit step no token (the
runner does not export `GITHUB_TOKEN` into step environments on its own). Every CI
run therefore took the degraded path — which the old policy said must never go red.
The gate "passed" while its GitHub half was structurally dead. That is the bug class:
unverified silently equalled verified.

### What the platform actually allows (measured on PR #119, 2026-11-06)

The Dependabot alerts API is **not reachable with the workflow `GITHUB_TOKEN` at
any permission level**. The evidence chain, from real runs on this repo:

| attempt | grant (visible in runner log) | alerts API answer |
| --- | --- | --- |
| CI run 1 | `security-events: read` | **HTTP 403** "Resource not accessible by integration" |
| CI run 2 | `SecurityEvents: write` (granted!) | **HTTP 403** — same |
| local | classic PAT (`repo` scope), gh keyring | works — `GitHub dependabot open: none (0)` |

The app that mints `GITHUB_TOKEN` has no *Dependabot alerts* permission, and the
`security-events` scope maps only to code/secret scanning — a workflow-file
permission block cannot add it (GitHub community discussion
[orgs/community#60612](https://github.com/orgs/community/discussions/60612)).
The smallest working credential is a **fine-grained PAT with "Dependabot alerts:
read"** (its owner needs write access to the repo) or a GitHub App with the
Dependabot-alerts permission. The `issues` API, in contrast, **is** reachable by
the `GITHUB_TOKEN` with `issues: read` — so the `ORPHANED` linked-issue check is
genuinely fixed by this PR's workflow wiring.

### The policy

- **Alerts cross-check, strict mode.** `ci.yml` sets `WEB_AUDIT_REQUIRE_CROSSCHECK`
  for `push` to `main` and same-repo PRs (not fork PRs, not Dependabot-authored
  PRs — their tokens are non-elevatable by GitHub's rules), and the audit step
  passes `WEB_AUDIT_ALERTS_TOKEN: ${{ secrets.WEB_AUDIT_ALERTS_TOKEN }}`. The
  script enforces strict **exactly when the promise can be kept**: on CI, once an
  alerts-capable token is present; locally, on explicit opt-in. Then an
  unverified cross-check **fails the step**, naming what was not verified, the
  reason with the HTTP status, and what to do.
  - Until the secret exists, forcing it would be a *permanent false red* — a gate
    that is always red for reasons no contributor can fix gets muted within a
    week and is worse overall. So until then the run degrades loudly: `COULD NOT
    CHECK — UNVERIFIED (HTTP 403 …)` plus a `WARNING` that prints the exact
    unblock (create repo secret `WEB_AUDIT_ALERTS_TOKEN`, fine-grained PAT,
    "Dependabot alerts: read"). **Adding that one secret flips the gate to
    fail-closed with zero further code or workflow changes.**
- **Bounded retries absorb blips.** Transient failures (HTTP 408/429/5xx,
  network-class errors) are retried up to 3 attempts with fixed 2s/4s backoff —
  at most ~6s of added waiting, fully logged, never an unbounded loop (runtime
  budget culture, AGENTS.md). 401/403 and missing-`gh` are configuration errors
  and fail fast without retrying.
- **`WEB_AUDIT_OFFLINE=1` + strict is a hard error.** Offline mode is for local
  offline tests and air-gapped work; a run that is required to verify may not
  declare itself offline.
- **`ORPHANED` issue lookup stays warning-only** even in strict runs: it now
  authenticates on CI (GITHUB_TOKEN + `issues: read`), and a *closed/missing*
  issue still blocks; but an unreachable issues API says nothing about the
  acceptance itself, so a transient blip there warns rather than reddening an
  otherwise-verified run.

`ci.yml` deliberately grants **no `security-events` scope**: measured above that it
buys nothing for this workflow, keeping it would be misleading attack surface, not
capability. Least privilege means the smallest scope that *works* — here that is
`contents: read` (whole workflow) + `issues: read` (the one job), with the alerts
capability carried by the dedicated secret, not the token.

Set `WEB_AUDIT_OFFLINE=1` (never on CI — rejected outright when strict is active)
to skip all network calls; the gate then prints the skipped cross-check as
`COULD NOT CHECK — UNVERIFIED` and a `WARNING` and still evaluates the old rules.

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
- Removing `GH_TOKEN` / `WEB_AUDIT_ALERTS_TOKEN` / `WEB_AUDIT_REQUIRE_CROSSCHECK`,
  or the `issues: read` job permission from the audit step in `ci.yml` to silence a
  strict failure — that recreates the exact #107 bug: a gate that cannot look while
  reporting no problem. If a strict red is a false one, fix the credential, not the
  wiring.
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
