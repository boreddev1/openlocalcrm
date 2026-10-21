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
GitHub Dependabot alerts API (`dependabot/alerts?state=open`, via `gh api`) and prints,
with a query timestamp, the advisory ids (GHSA) seen by **each** source plus their
difference:

- Alerts that GitHub reports but `pnpm audit` does not yet know are printed as a
  visible, **non-blocking `WARNING`** — the source lag made them unavoidable, and a
  second blocking source would tie CI to external database sync.
- **Blocking behaviour is unchanged: only the pnpm source (part of the lockfile
  decision) and the exception file itself can fail the gate.** If the cross-check
  cannot run, the gate prints `COULD NOT CHECK (<reason>)` and exits per the old
  rules — a network hiccup never turns CI red.

Set `WEB_AUDIT_OFFLINE=1` to skip all network calls (offline tests, air-gapped work);
the gate then prints the skipped cross-check as a warning and still evaluates the old
rules.

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
