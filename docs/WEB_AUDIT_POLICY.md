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

An accepted, still-unpatchable, in-date finding **passes** — and is printed in full on
every run: package, severity, who accepted it, when, until when, which issue, why.

A `pnpm audit` run that cannot be executed or parsed also fails. A check that did not
look must not report success.

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

## Forbidden

- `--audit-level`, `--ignore`, or dropping the CI step — these hide findings instead of tracking them.
- Acceptances without `issue`, `reason`, `acceptedBy`, `acceptedOn`, `reviewBy`.
- Extending `reviewBy` without a new `reason`. Re-accepting is allowed; drifting is not.

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
```

Exit `0` with the accepted findings printed, or a non-zero exit naming one of `NEW` /
`STALE` / `DEAD` / `OVERDUE`.
