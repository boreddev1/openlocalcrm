#!/usr/bin/env node
/**
 * Web dependency audit with teeth.
 *
 * Replaces a bare `pnpm audit`, which is red for every finding forever and therefore
 * trains everyone to ignore red. That is not "tracked", that is noise that gets muted.
 *
 * Instead this check distinguishes six things and fails loudly on five of them:
 *
 *   NEW       an advisory with no matching exception            -> blocking
 *   STALE     an exception expected no patch, but one exists    -> blocking
 *   DEAD      an exception that matches nothing anymore         -> blocking
 *   OVERDUE   an exception past its `reviewBy` date             -> blocking
 *   ORPHANED  an exception whose linked issue is closed/gone    -> blocking
 *   KNOWN     a still-unpatchable, accepted, in-date advisory   -> printed, not blocking
 *
 * Every KNOWN finding is re-printed on every run, so nothing is ever silently
 * swallowed: the acceptance is visible in the build log, dated, attributed, and
 * linked to an issue. An unusable audit result is an error, never a pass.
 *
 * Source-lag cross-check (non-blocking): `pnpm audit` only knows what npm's advisory
 * source has already synced. Measured on 2026-10-20: GitHub carried the vite/
 * launch-editor advisory for ~33 minutes before `pnpm audit` reported it. So every
 * run also queries the GitHub Dependabot alerts API (`dependabot/alerts?state=open`,
 * via `gh api`), prints the advisory ids seen by EACH source with a timestamp, and
 * prints the difference (GitHub-only alerts) as a visible WARNING. Blocking behaviour
 * is unchanged: only the pnpm source (part of the lockfile decision) can fail the
 * gate. A network hiccup, a missing `gh`, or a token without `security-events: read`
 * degrades to a printed "could not check" and exits per the old rules — a check that
 * cannot reach GitHub must never turn CI red.
 *
 * The linked-issue check (ORPHANED) is the one exception that needs GitHub: an
 * acceptance whose issue is closed or does not exist blocks, because a closed issue
 * is not a paper trail (AGENTS.md: acceptances are "issue-linked"). When GitHub
 * cannot be reached the state is unverifiable — printed as a warning, never a
 * failure, for the same reason as above.
 *
 * Set WEB_AUDIT_OFFLINE=1 to skip every network call (offline tests, air-gapped dev).
 *
 * Run from the `web/` directory:  cd web && node ../scripts/check-web-audit.mjs
 */
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const exceptionsPath = join(repoRoot, "web", "audit-exceptions.json");
const offlineMode = process.env.WEB_AUDIT_OFFLINE === "1";

const warnings = [];

function fail(lines) {
  process.stdout.write("\n");
  for (const line of lines) process.stdout.write(`  ${line}\n`);
  process.stdout.write("\nweb audit: FAILED\n");
  process.exit(1);
}

/** Key that binds an exception to exactly one advisory, never to a whole package. */
const keyOf = (packageName, vulnerableVersions) => `${packageName}|${vulnerableVersions}`;

function readAudit() {
  const run = spawnSync("pnpm", ["audit", "--json"], {
    cwd: process.cwd(),
    encoding: "utf8",
    maxBuffer: 32 * 1024 * 1024,
  });
  if (run.error !== undefined || typeof run.stdout !== "string") {
    fail([
      "cannot run `pnpm audit --json`.",
      `  reason: ${run.error?.message ?? run.stderr?.slice(0, 200) ?? "no output"}`,
      "  A check that did not run must not report success.",
    ]);
  }
  try {
    return JSON.parse(run.stdout);
  } catch {
    fail([
      "`pnpm audit --json` produced unparsable output.",
      "  A check that cannot read its input must not report success.",
    ]);
  }
}

// ---------------------------------------------------------------------------
// GitHub side. Every function here is safe to call: failure means a printed
// note, never an exit code change.
// ---------------------------------------------------------------------------

/** Run a `gh` CLI call. Returns { ok, stdout } or { ok: false, status, reason }. */
function ghRun(args) {
  if (offlineMode) return { ok: false, reason: "skipped — WEB_AUDIT_OFFLINE=1" };
  const run = spawnSync("gh", args, {
    encoding: "utf8",
    timeout: 30_000,
    maxBuffer: 16 * 1024 * 1024,
    env: { ...process.env, GH_PROMPT_DISABLED: "1", GH_PAGER: "cat", NO_COLOR: "1" },
  });
  if (run.error !== undefined) {
    const reason =
      run.error.code === "ENOENT" ? "gh CLI not installed" : run.error.message ?? String(run.error);
    return { ok: false, reason };
  }
  if (run.status !== 0 || run.signal !== null) {
    const status = /HTTP (\d{3})/.exec(run.stderr ?? "");
    const firstLine = (run.stderr ?? "").trim().split("\n")[0] ?? "no output";
    return { ok: false, status: status ? Number(status[1]) : undefined, reason: firstLine.slice(0, 180) };
  }
  return { ok: true, stdout: run.stdout };
}

/** owner/repo from the local git remote. No network. */
function repoSlug() {
  const run = spawnSync("git", ["config", "--get", "remote.origin.url"], {
    encoding: "utf8",
    timeout: 5_000,
  });
  if (run.status !== 0 || typeof run.stdout !== "string") return null;
  const m = /github\.com[:/]([^/]+)\/([^/]+?)(?:\.git)?\/?\s*$/.exec(run.stdout.trim());
  return m ? `${m[1]}/${m[2]}` : null;
}

/** Open Dependabot alerts for the npm ecosystem. Returns { ok, alerts } or { ok: false, reason }. */
function fetchOpenNpmAlerts(slug) {
  const run = ghRun([
    "api",
    `repos/${slug}/dependabot/alerts?state=open&per_page=100`,
    "--paginate",
    "--slurp",
  ]);
  if (!run.ok) return run;
  let alerts = [];
  try {
    // `--paginate --slurp` prints one JSON array per page; accept both a single
    // array and the page-per-line form.
    const text = run.stdout.trim();
    if (text !== "") {
      try {
        const parsed = JSON.parse(text);
        alerts = Array.isArray(parsed) ? parsed : [parsed];
      } catch {
        for (const line of text.split("\n")) {
          if (line.trim() !== "") alerts.push(...JSON.parse(line));
        }
      }
    }
  } catch {
    return { ok: false, reason: "dependabot alerts response was not parsable JSON" };
  }
  return {
    ok: true,
    alerts: alerts
      .filter((a) => a?.security_vulnerability?.package?.ecosystem === "npm")
      .map((a) => ({
        number: a.number,
        ghsa: a.security_advisory?.ghsa_id ?? "?",
        pkg: a.security_vulnerability?.package?.name ?? "?",
        range: a.security_vulnerability?.vulnerable_version_range ?? "?",
        severity: a.security_vulnerability?.severity ?? a.security_advisory?.severity ?? "?",
      })),
  };
}

const issueStateCache = new Map();

/** Classify an acceptance's linked issue: open | closed | missing | unverifiable. */
function verifyLinkedIssue(slug, issueUrl) {
  if (!slug) return { kind: "unverifiable", note: "no github.com remote resolved" };
  if (offlineMode) return { kind: "unverifiable", note: "WEB_AUDIT_OFFLINE=1" };
  const m = /\/issues\/(\d+)/.exec(issueUrl ?? "");
  if (m === null) return { kind: "unverifiable", note: "no parseable issue link in the exception" };
  const num = m[1];
  if (issueStateCache.has(num)) return issueStateCache.get(num);
  const run = ghRun(["api", `repos/${slug}/issues/${num}`, "--jq", ".state"]);
  let verdict;
  if (!run.ok) {
    verdict =
      run.status === 404
        ? { kind: "missing", num }
        : { kind: "unverifiable", note: run.reason };
  } else {
    const state = run.stdout.trim();
    verdict =
      state === "open"
        ? { kind: "open", num }
        : state === "closed"
          ? { kind: "closed", num }
          : { kind: "unverifiable", note: `unexpected issue state '${state}'` };
  }
  issueStateCache.set(num, verdict);
  return verdict;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

let exceptions;
try {
  exceptions = JSON.parse(readFileSync(exceptionsPath, "utf8")).advisories ?? [];
} catch (error) {
  fail([`cannot read ${exceptionsPath.replace(repoRoot + "/", "")}: ${error.message}`]);
}

const audit = readAudit();
const advisories = Object.values(audit.advisories ?? {});
const summary = audit.metadata?.vulnerabilities ?? {};

process.stdout.write(`\nweb audit — ${advisories.length} advisory finding(s), ` +
  `severity totals: high=${summary.high ?? 0} critical=${summary.critical ?? 0} ` +
  `moderate=${summary.moderate ?? 0} low=${summary.low ?? 0}\n\n`);

// --- source-lag cross-check: print what EACH source knows, then the difference ---
const slug = repoSlug();
const pnpmIds = new Set(advisories.map((a) => a.github_advisory_id).filter(Boolean));
const pnpmIdList = advisories.length === 0
  ? "none"
  : [...pnpmIds].join(", ") + (pnpmIds.size === 0 ? " (no GHSA ids)" : ` (${pnpmIds.size})`);

process.stdout.write(`  sources queried ${new Date().toISOString()}:\n`);
process.stdout.write(`    pnpm audit:              ${pnpmIdList}\n`);

let ghAlertsKnown = false;
const ghOnly = [];
const ghCheck = slug === null ? { ok: false, reason: "no github.com remote resolved" } : fetchOpenNpmAlerts(slug);
if (!ghCheck.ok) {
  process.stdout.write(
    `    GitHub dependabot open:  COULD NOT CHECK (${ghCheck.reason})\n` +
    `                             the exit code is decided by the old rules alone.\n`,
  );
} else {
  ghAlertsKnown = true;
  process.stdout.write(
    `    GitHub dependabot open:  ${ghCheck.alerts.length === 0 ? "none" : ghCheck.alerts.map((a) => `${a.ghsa} [${a.pkg}]`).join(", ")} (${ghCheck.alerts.length})\n`,
  );
  for (const alert of ghCheck.alerts) {
    if (!pnpmIds.has(alert.ghsa)) ghOnly.push(alert);
  }
  if (ghOnly.length === 0) {
    process.stdout.write(`    difference (GitHub-only, pnpm source behind): none\n\n`);
  } else {
    process.stdout.write(
      `    difference (GitHub-only, pnpm source behind): ` +
      `${ghOnly.map((a) => a.ghsa).join(", ")}\n\n`,
    );
    for (const alert of ghOnly) {
      warnings.push(
        `GitHub alert #${alert.number} ${alert.ghsa} — ${alert.pkg} (${alert.severity}, ${alert.range}) ` +
        `is NOT yet visible to \`pnpm audit\`. Source lag, not absence: the gate stays green until ` +
        `pnpm's source catches up, then this becomes NEW and blocking. Fix it while it is still cheap.`,
      );
    }
  }
}

if (exceptions.length === 0 && advisories.length > 0) {
  fail([
    `${advisories.length} advisory finding(s) and no exceptions file entries.`,
    "  Either fix the dependency or record an explicit, dated, attributed acceptance",
    "  in web/audit-exceptions.json. Never lower the bar by deleting the check.",
  ]);
}

const blocking = [];
const known = [];
const usedKeys = new Set();

for (const advisory of advisories) {
  const name = advisory.module_name ?? "unknown";
  const vulnerable = advisory.vulnerable_versions ?? "";
  const key = keyOf(name, vulnerable);
  const match = exceptions.find((e) => keyOf(e.package, e.vulnerableVersions) === key);

  if (match === undefined) {
    blocking.push(
      `NEW       ${name} (${advisory.severity}) vulnerable ${vulnerable || "?"}` +
        ` — ${advisory.title ?? "no title"}\n` +
        `          no acceptance exists. Upgrade, or add a dated entry to web/audit-exceptions.json.`,
    );
    continue;
  }

  usedKeys.add(key);

  // STALE: the acceptance was granted because upstream had no patch. If a patch now
  // exists, the reason for the acceptance is gone and the exception must die.
  if (match.patchedVersionsExpected === null && advisory.patched_versions !== null &&
      advisory.patched_versions !== undefined && advisory.patched_versions !== "") {
    blocking.push(
      `STALE     ${name}: acceptance says "no patch exists", but ${advisory.patched_versions} ` +
        `is available now (issue ${match.issue ?? "?"}).\n` +
        `          Upgrade and delete this exception — the reason for accepting it no longer holds.`,
    );
    continue;
  }

  // OVERDUE: every acceptance expires on its review date. Silence is not a decision.
  const reviewBy = Date.parse(`${match.reviewBy}T23:59:59Z`);
  if (Number.isNaN(reviewBy)) {
    blocking.push(`BAD ENTRY ${name}: exception has no valid \`reviewBy\` date — every acceptance must expire.`);
    continue;
  }
  if (Date.now() > reviewBy) {
    blocking.push(
      `OVERDUE   ${name}: accepted on ${match.acceptedOn ?? "?"} for review by ${match.reviewBy}, ` +
        `which has passed (issue ${match.issue ?? "?"}).\n` +
        `          Re-decide: fix it, or re-accept with a new date and a reason. Do not just extend blindly.`,
    );
    continue;
  }

  // ORPHANED: the paper trail must stay live. AGENTS.md requires an "issue-linked"
  // acceptance; a closed or deleted issue is a reference nobody can lean on while
  // the exception still carries a real, unpatched advisory.
  const issueCheck = verifyLinkedIssue(slug, match.issue);
  if (issueCheck.kind === "closed" || issueCheck.kind === "missing") {
    blocking.push(
      `ORPHANED  ${name}: the exception links ${match.issue}, but issue #${issueCheck.num} is ` +
        `${issueCheck.kind === "closed" ? "CLOSED" : "MISSING (HTTP 404)"} while this acceptance is still live.\n` +
        `          A closed issue is not a paper trail. Reopen the issue as long as the exception is\n` +
        `          carried, or upgrade and delete the exception.`,
    );
    continue;
  }
  if (issueCheck.kind === "unverifiable") {
    warnings.push(
      `linked issue of exception ${name} (${match.issue ?? "no link"}): GitHub state could not be ` +
      `checked (${issueCheck.note}) — unverified, so not treated as a failure.`,
    );
  }

  known.push({ name, advisory, match });
}

for (const exception of exceptions) {
  const key = keyOf(exception.package, exception.vulnerableVersions);
  if (!usedKeys.has(key)) {
    blocking.push(
      `DEAD      exception for ${exception.package} (${exception.vulnerableVersions}) matches nothing.\n` +
        `          The advisory is gone or the range changed — delete the entry instead of letting it rot.`,
    );
  }
}

for (const entry of known) {
  process.stdout.write(
    `  KNOWN     ${entry.name} (${entry.advisory.severity}) vulnerable ${entry.advisory.vulnerable_versions}\n` +
    `            accepted ${entry.match.acceptedOn ?? "?"} by ${entry.match.acceptedBy ?? "?"}, ` +
    `re-review by ${entry.match.reviewBy}\n` +
    `            issue: ${entry.match.issue ?? "?"}\n` +
    `            reason: ${entry.match.reason ?? "-"}\n`,
  );
}

if (warnings.length > 0) {
  process.stdout.write("\n");
  for (const warning of warnings) {
    process.stdout.write(`  WARNING   ${warning}\n`);
  }
}

if (blocking.length > 0) {
  process.stdout.write("\n");
  fail(blocking);
}

const gapNote = !ghAlertsKnown
  ? "GitHub cross-check did not run — pnpm's view alone decided nothing today."
  : ghOnly.length === 0
    ? "GitHub currently reports nothing beyond pnpm's view."
    : `GitHub reports ${ghOnly.length} alert(s) beyond pnpm's view — see WARNING above.`;

process.stdout.write(
  `\nweb audit: OK — ${known.length} accepted finding(s) shown above, 0 unaccepted.\n` +
  `           Accepted findings are printed on every run on purpose.\n` +
  `           "OK" means: nothing is unaccepted AS FAR AS PNPM'S SOURCE CAN SEE. It does not mean\n` +
  `           "no known vulnerabilities" — ${gapNote}\n\n`,
);
