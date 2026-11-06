# Embedded Bundle Repair — the `web/dist` Companion PR Automation

**Why this exists.** `web/dist` is committed and baked into the Go binary by
`//go:embed` (invariant #5), so a change to a *shipped* web dependency is only
complete together with its rebuilt bundle. Dependabot cannot deliver that half:
GitHub refuses writes to Dependabot branches (`maintainerCanModify=false`), so its
PRs fail ci.yml's step **“Freshness Gate — committed web/dist must match a fresh
build”** — and because that step sits *before* the Playwright steps, the bundle in
question never gets browser-validated either. Measured on 2026-11-06: five CI runs
failed on that one step and ten Playwright steps were skipped as a result. The
human fix was PR #114 (bump + rebuilt `web/dist` in one commit). Issue #116 asked
for the pattern to stop being tribal knowledge; this file and
`.github/workflows/web-dist-rebuild.yml` are the answer.

## What runs, and what it decides

A scheduled job (daily 02:30 UTC, plus `workflow_dispatch`) builds `main` with the
same pinned toolchain as ci.yml — `node 22`, `pnpm 12.4.1`,
`pnpm install --frozen-lockfile`, `pnpm build` — and compares the result with the
committed bundle using the *same* condition ci.yml uses:

```sh
git status --porcelain --untracked-files=all -- web/dist   # empty == fresh
```

| Result | Action |
| --- | --- |
| identical | green, nothing is written anywhere. The job that runs every day holds a **read-only** token. |
| different | one companion PR on branch `automation/web-dist-rebuild`, containing **only** the rebuilt `web/dist`, opened or updated in place. |
| different, but an open companion PR already carries exactly that tree | no push, no body edit, no comment. |
| identical again while a companion PR is open (someone else rebuilt) | that PR is closed by the script — it would merge nothing. |

The logic lives in `scripts/web-dist-companion.sh` (`check` / `probe` / `body` /
`publish`), so it is readable and testable outside YAML:

```sh
cd web && pnpm install --frozen-lockfile && pnpm build
../scripts/web-dist-companion.sh check          # drift=true|false, exit 0 either way
../scripts/web-dist-companion.sh body           # the exact PR text it would file
```

## No-spam invariants

* The companion PR is identified by its **head branch name**, never by “open
  another one”. `gh pr list --state open --head automation/web-dist-rebuild` first.
* The branch is owned by the automation and is reset to `main` + one bundle commit
  with `--force-with-lease`. **Anything a human commits to that branch is lost** —
  review and merge the PR, or move the change elsewhere.
* A companion PR that a human **closed without merging** suppresses new companion
  PRs for 7 days (`COOLDOWN_DAYS`); the run reports the drift as a warning instead.
  A close is a decision, not an oversight.
* `git add` is pathspec-limited to `web/dist`, and `publish` refuses outright if the
  working tree changed **outside** `web/dist` (a lockfile touched by a different
  pnpm is the case that matters). The commit is re-checked before pushing.
* No npm dependency and no third-party “create PR” action: the lockfile version
  table in the PR body is parsed with awk, the PR is made with `git` + `gh`.

## Permissions

| Job | `permissions:` | Why |
| --- | --- | --- |
| `detect-drift` | `contents: read`, `pull-requests: read` | the everyday path never needs more |
| `open-companion-pr` | `contents: write`, `pull-requests: write` | push one branch, open/update/close one PR |

The split exists because GitHub scopes `GITHUB_TOKEN` **per job**: with one job,
every nightly tick would mint a write-capable token for a run that writes nothing.
Handing a built bundle from job to job would need an artifact, i.e.
`actions: write` — a wider scope, so the second job rebuilds instead (~2 min, only
on drift days).

## The one thing a reviewer has to do: approve the runs

Read this before you decide the automation is broken.

GitHub documents (`Actions → Security → When GITHUB_TOKEN triggers workflow runs`):

> events triggered by the `GITHUB_TOKEN` will not create a new workflow run …
> `workflow_dispatch` and `repository_dispatch` events always create workflow runs.

and, for exactly this case:

> when a workflow using `GITHUB_TOKEN` creates or updates a pull request, the
> resulting `pull_request` event creates workflow runs in an **approval-required**
> state. … still allowing CI workflows to run on pull requests created by automation.

So the companion PR does get ci.yml — it just waits. **Click “Approve and run”**
(needs write access) and the two required checks — `Web Frontend Build & Playwright
E2E` and `Go Backend Tests & License Check` — run against that exact bundle,
including the Playwright suite that #116 says was never executed for these states.
Alternatives, in descending order of preference: a human pushes any commit to the
branch (user events need no approval); or an admin merges over the pending checks
(`main` protection runs with `enforce_admins=false`) — which is precisely the way
that leaves the bundle unvalidated, so treat it as an exception.

A `workflow_dispatch` trick is deliberately **not** used: `ci.yml` has no such
trigger, and per GitHub's own troubleshooting docs a run started that way reports no
required status check for the PR (“check the event that triggered the workflow run”).
Triggering it anyway would cost an `actions: write` scope this automation does not
need.

## What it does not do

* It does not make a Dependabot PR mergeable. The bump PR still fails the gate by
  design — that half cannot be delivered inside it. What arrives automatically,
  within one schedule tick, is the missing bundle for the state `main` is in after
  that bump landed (by admin merge or any other bypass). This is the repair path,
  not a bypass of the gate.
* It does not touch `ci.yml`, the gate itself, or the Dependabot configuration.
* It does not decide that a stale `main` is acceptable. It files a reviewable PR.

## How main goes stale in the first place

`.githooks/pre-commit` check 6 already compares a fresh build against the *staged*
`web/dist`, so a local commit that forgets the rebuild is normally blocked. The
state this automation repairs is therefore mostly a **merge that bypassed the
gate** — an admin merge of a red Dependabot PR, or a `--no-verify` commit — which is
exactly how #92/#95/#96 ended on 2026-11-06 (replaced by hand through #114).

One measured caveat for whoever adds binary assets to the SPA: the root `.gitignore`
ignores `*.png` repo-wide, so a PNG emitted into `web/dist` would be invisible both
to the freshness gate (`git status` skips ignored files) and to this workflow — the
bundle PR could not carry it. Nothing in `web/dist` is ignored today (checked
2026-11-06: 4 tracked files, `git status --ignored -- web/dist` empty).
