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
 * Source-lag cross-check (its findings are non-blocking; on strict runs its FAILURE
 * to run is blocking — see the degradation policy below): `pnpm audit` only knows
 * what npm's advisory source has already synced. Measured on 2026-10-20: GitHub
 * carried the vite/launch-editor advisory for ~33 minutes before `pnpm audit`
 * reported it. So every run also queries the GitHub Dependabot alerts API
 * (`dependabot/alerts?state=open`,
 * via `gh api --paginate` — plain `--paginate`, never `--slurp`: gh builds older than
 * the flag's release reject it with "unknown flag: --slurp" and did silently degrade
 * this cross-check to a no-op until 2026-11-06; the pages it does print are parsed as
 * concatenated JSON documents instead), prints the advisory ids seen by EACH source
 * with a timestamp, and prints the difference (GitHub-only alerts) as a visible
 * WARNING — classified against `web/pnpm-lock.yaml`: a GitHub-only alert that IS in
 * the tree is the pnpm source lagging behind, one whose package is NOT in the
 * lockfile is an alert about something this gate cannot see (stale alert, another
 * manifest, or a lockfile mismatch) and says so. Degradation policy (issue #107 —
 * a security gate must not silently pass because it could not verify, but a gate
 * that goes red on every API blip gets muted within a week and is worse overall):
 *
 *   LOCAL / restricted-token runs (fork PR, dependabot PR): a network hiccup, a
 *     missing `gh`, or an unusable token degrades loudly — "COULD NOT CHECK —
 *     UNVERIFIED" on the per-source line AND as a WARNING — and exits on the old
 *     rules. The exit code stays decided by the pnpm source (part of the lockfile
 *     decision); the unverified state must never read as "no open alerts".
 *   STRICT runs (ci.yml sets WEB_AUDIT_REQUIRE_CROSSCHECK for push-to-main and
 *     same-repo PRs — the runs whose workflow token can authenticate): an
 *     unverified cross-check is BLOCKING. The failure names exactly what was not
 *     verified and what to do. Transient failures (HTTP 408/429/5xx, network-class
 *     errors) are retried first: at most 3 attempts, fixed 2s/4s backoff, never an
 *     unbounded wait; 401/403 and missing-tool failures are configuration errors
 *     and fail fast without retrying. `WEB_AUDIT_OFFLINE=1` contradicts strict
 *     mode and is itself a hard error.
 *
 * Running with live data needs a gh the step can authenticate: local keyring login,
 * or an exported GH_TOKEN/GITHUB_TOKEN whose permission covers the alerts API
 * (ci.yml passes `secrets.GITHUB_TOKEN` as GH_TOKEN and grants the job
 * security-events: write + issues: read — measured minimums for a workflow token;
 * the runner never exports the token on its own).
 *
 * The linked-issue check (ORPHANED) is the one exception that needs GitHub: an
 * acceptance whose issue is closed or does not exist blocks, because a closed issue
 * is not a paper trail (AGENTS.md: acceptances are "issue-linked"). When GitHub
 * cannot be reached the state is unverifiable — printed as a warning, never a
 * failure: an unreachable issue API says nothing about the acceptance, and this
 * check deliberately stays out of the strict blocking path so one transient blip
 * on one issue lookup cannot red-face an otherwise-verified run.
 *
 * Set WEB_AUDIT_OFFLINE=1 to skip every network call (offline tests, air-gapped
 * dev). Strict runs reject this combination outright — offline is not a way to
 * pass a run that must verify.
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
// Strict mode (issue #107): set by ci.yml on runs whose workflow token can
// authenticate (push-to-main, same-repo PRs). Locally it is unset, so local
// behaviour is unchanged. GitHub expressions render booleans as "true"/"false".
const requireCrossCheck = ["1", "true"].includes(
  (process.env.WEB_AUDIT_REQUIRE_CROSSCHECK ?? "").toLowerCase(),
);

const warnings = [];

function fail(lines) {
  process.stdout.write("\n");
  for (const line of lines) process.stdout.write(`  ${line}\n`);
  process.stdout.write("\nweb audit: FAILED\n");
  process.exit(1);
}

// An escape hatch is not a degradation policy: if this run must verify the
// Dependabot state, it must not also declare itself offline.
if (offlineMode && requireCrossCheck) {
  fail([
    "WEB_AUDIT_OFFLINE=1 contradicts WEB_AUDIT_REQUIRE_CROSSCHECK on this run.",
    "  Strict mode demands a verified Dependabot cross-check; offline mode skips",
    "  every network call. On CI WEB_AUDIT_OFFLINE must never be set.",
    "  See docs/WEB_AUDIT_POLICY.md — do not resolve this by relaxing the gate.",
  ]);
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
    return { ok: false, reason, code: run.error.code };
  }
  if (run.status !== 0 || run.signal !== null) {
    const status = /HTTP (\d{3})/.exec(run.stderr ?? "");
    const firstLine = (run.stderr ?? "").trim().split("\n")[0] ?? "no output";
    return { ok: false, status: status ? Number(status[1]) : undefined, reason: firstLine.slice(0, 180) };
  }
  return { ok: true, stdout: run.stdout };
}

// ---------------------------------------------------------------------------
// Bounded retry for the alerts cross-check. Transient failures only (HTTP
// 408/429/5xx, network-class spawn/CLI errors): at most ALERT_ATTEMPTS tries
// with fixed 2s/4s backoff, so the worst-case added wall time is bounded and
// predictable — this repo budgets short runtimes, and an unbounded retry loop
// is how a "temporarily red" gate becomes a hung one. 401/403, a missing gh,
// and a missing remote are configuration errors: retrying them changes nothing.
// ---------------------------------------------------------------------------
const ALERT_ATTEMPTS = 3;
const ALERT_BACKOFF_MS = [2000, 4000];

function sleepSync(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

/** Would retrying this gh failure plausibly help? Only for transport-class faults. */
function isTransientGhFailure(run) {
  if (run.status === 408 || run.status === 429) return true;
  if (typeof run.status === "number" && run.status >= 500) return true;
  if (typeof run.status !== "number") {
    return /ETIMEDOUT|ECONNRESET|ECONNREFUSED|EAI_AGAIN|ENETUNREACH|ENOTFOUND|timeout/i.test(
      `${run.code ?? ""} ${run.reason ?? ""}`,
    );
  }
  return /dial tcp|could not resolve|connection reset|broken pipe|TLS|EOF/i.test(run.reason ?? "");
}

/** `ghRun` with the bounded retry above; prints every retry so the log tells the story. */
function ghRunRetried(args, label) {
  let run;
  for (let attempt = 1; attempt <= ALERT_ATTEMPTS; attempt++) {
    run = ghRun(args);
    if (run.ok) {
      if (attempt > 1) {
        process.stdout.write(`    ${label}: recovered on attempt ${attempt}/${ALERT_ATTEMPTS}\n`);
      }
      return run;
    }
    if (!isTransientGhFailure(run) || attempt === ALERT_ATTEMPTS) return run;
    const backoff = ALERT_BACKOFF_MS[attempt - 1];
    process.stdout.write(
      `    ${label}: transient failure (${String(run.reason).slice(0, 90)}) — retrying in ` +
        `${backoff / 1000}s (attempt ${attempt + 1}/${ALERT_ATTEMPTS})\n`,
    );
    sleepSync(backoff);
  }
  return run;
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

/** Split a string at the characters that carry meaning inside a regex, for dynamic patterns. */
function escapeRegExp(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Cut the concatenated JSON documents that `gh api --paginate` prints (one full
 * array per page, back to back) into parsed values. Line- or whole-buffer
 * `JSON.parse` both break on that form, and this must not depend on how a page
 * happens to be framed — minified, pretty-printed, one page or many. Scans for
 * balanced top-level `{}`/`[]` values, honouring strings and escapes; anything
 * else throws and the caller degrades loudly, never silently.
 */
function parseJsonDocuments(text) {
  const docs = [];
  let depth = 0;
  let start = -1;
  let inString = false;
  let escaped = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (inString) {
      if (escaped) escaped = false;
      else if (ch === "\\") escaped = true;
      else if (ch === '"') inString = false;
      continue;
    }
    if (start === -1) {
      if (ch === "{" || ch === "[") {
        start = i;
        depth = 1;
      }
      continue;
    }
    if (ch === '"') inString = true;
    else if (ch === "{" || ch === "[") depth++;
    else if (ch === "}" || ch === "]") {
      depth--;
      if (depth === 0) {
        docs.push(JSON.parse(text.slice(start, i + 1)));
        start = -1;
      }
    }
  }
  if (start !== -1) throw new Error("truncated JSON");
  return docs;
}

let lockfileText; // undefined = not read yet, null = unreadable
function readLockfile() {
  if (lockfileText === undefined) {
    try {
      lockfileText = readFileSync(join(repoRoot, "web", "pnpm-lock.yaml"), "utf8");
    } catch {
      lockfileText = null;
    }
  }
  return lockfileText;
}

/** Is this package name part of the locked dependency tree? true | false | null (lockfile unreadable). */
function lockfileHasPackage(pkg) {
  const text = readLockfile();
  if (text === null) return null;
  return new RegExp(`(?:^|\\n)[ \\t]*'?/?${escapeRegExp(pkg)}@`).test(text);
}

/** Explain a failed alerts call by the class of failure, not just the first stderr line. */
function alertFailureReason(run) {
  const reason = run.reason ?? "no output";
  if (run.status === 401 || run.status === 403) {
    // Measured on this repo's runner (PR #119, 2026-11-06): `security-events: read`
    // answers HTTP 403 "Resource not accessible by integration" — for a workflow
    // token the alerts API's minimum WORKING grant is `security-events: write`
    // (the endpoint has never exposed a read-only mode to GITHUB_TOKEN).
    return `${reason}; the token lacks permission — on a workflow token the alerts API needs security-events: write (read is verifiably insufficient; see ci.yml comment)`;
  }
  if (/GH_TOKEN|auth/i.test(reason)) {
    return `${reason}; gh needs a local login or an exported GH_TOKEN/GITHUB_TOKEN`;
  }
  if (run.status !== undefined) {
    // gh's own stderr usually already ends in "(HTTP nnn)" — don't print it twice.
    return reason.includes(`HTTP ${run.status}`) ? reason : `${reason} (HTTP ${run.status})`;
  }
  return reason;
}

/** Open Dependabot alerts for the npm ecosystem. Returns { ok, alerts } or { ok: false, reason }. */
function fetchOpenNpmAlerts(slug) {
  // Plain `--paginate`, NOT `--slurp`: the flag does not exist in every gh 2.x
  // build (gh 2.46.0 fails the call with "unknown flag: --slurp"), and as a
  // hard-to-read CLI error it silently disarmed this entire cross-check.
  const run = ghRunRetried(
    [
      "api",
      `repos/${slug}/dependabot/alerts?state=open&per_page=100`,
      "--paginate",
    ],
    "dependabot alerts",
  );
  if (!run.ok) return { ok: false, reason: alertFailureReason(run) };
  if ((run.stdout ?? "").trim() === "") {
    // A verified-empty list still prints `[]` — a successful call with NO body is
    // a broken response, and must not be readable as "zero open alerts".
    return { ok: false, reason: "dependabot alerts call returned no output at all" };
  }
  let alerts = [];
  try {
    // One JSON array per page, back to back; a single document is the one-page
    // special case. An empty list parses to `[]` — that is "verified, none open",
    // deliberately not lumped in with the cannot-check path.
    for (const doc of parseJsonDocuments(run.stdout ?? "")) {
      if (Array.isArray(doc)) alerts.push(...doc);
      else if (doc !== null && typeof doc === "object") alerts.push(doc);
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
const pnpmNames = new Set(advisories.map((a) => a.module_name).filter(Boolean));
const pnpmIdList = advisories.length === 0
  ? "none"
  : [...pnpmIds].join(", ") + (pnpmIds.size === 0 ? " (no GHSA ids)" : ` (${pnpmIds.size})`);

process.stdout.write(`  sources queried ${new Date().toISOString()}:\n`);
process.stdout.write(`    pnpm audit:              ${pnpmIdList}\n`);

let ghAlertsKnown = false;
let strictCrossCheckFailure = null;
const ghOnly = [];
const ghCheck = slug === null ? { ok: false, reason: "no github.com remote resolved" } : fetchOpenNpmAlerts(slug);
if (!ghCheck.ok) {
  process.stdout.write(
    `    GitHub dependabot open:  COULD NOT CHECK — UNVERIFIED (${ghCheck.reason})\n` +
    (requireCrossCheck
      ? `                             strict mode (issue #107): this alone FAILS the gate below.\n`
      : `                             the exit code is decided by the old rules alone.\n`),
  );
  // Loud degradation, in the WARNING block too: "could not check" must never be
  // readable as "no open alerts". Outside strict runs fail-closed stays on the
  // pnpm side; the Dependabot state is explicitly unverified here, not silently
  // accepted. On strict runs the unverified state itself blocks (see below).
  if (requireCrossCheck) strictCrossCheckFailure = ghCheck.reason;
  warnings.push(
    `Dependabot cross-check did not run (${ghCheck.reason}) — GitHub's open alerts are ` +
    `UNVERIFIED this run. Do not read the gate result as "no open alerts".`,
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
      // A GHSA can be GitHub-only because pnpm's SOURCE lags, because the alert
      // is about a package this lockfile does not contain at all (stale alert,
      // a different manifest, or an alert GitHub has not re-evaluated), or
      // because the lockfile itself cannot be read to decide. Only the first is
      // provably "not absence, just lag" — say which one it is, never blanket-
      // reassure. All three stay non-blocking but UNVERIFIED in the WARNING block.
      const inLockfile = pnpmNames.has(alert.pkg) || lockfileHasPackage(alert.pkg);
      const head = `GitHub alert #${alert.number} ${alert.ghsa} — ${alert.pkg} (${alert.severity}, ${alert.range})`;
      if (inLockfile === true) {
        warnings.push(
          `${head} is NOT yet visible to \`pnpm audit\`. Source lag, not absence: the gate ` +
          `stays green until pnpm's source catches up, then this becomes NEW and blocking. ` +
          `Fix it while it is still cheap.`,
        );
      } else if (inLockfile === false) {
        warnings.push(
          `${head} is NOT in web/pnpm-lock.yaml. The cross-check cannot rule it out — a stale ` +
          `alert, a different manifest, or a tree mismatch. UNVERIFIED, not accepted: close it ` +
          `in the Security tab with a reason or explain it in an issue.`,
        );
      } else {
        warnings.push(
          `${head} could not be matched against web/pnpm-lock.yaml (unreadable from this run). ` +
          `UNVERIFIED, not accepted — check the Security tab manually.`,
        );
      }
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

// Strict mode (issue #107): on the runs where the workflow token MUST be able to
// authenticate, a cross-check that did not verify is itself a finding — a gate
// that green-lights because it could not look is exactly the bug that was filed.
if (strictCrossCheckFailure !== null) {
  blocking.push(
    `UNVERIFIED  the Dependabot cross-check did not run and this run is STRICT\n` +
      `            (ci.yml sets WEB_AUDIT_REQUIRE_CROSSCHECK for push-to-main and same-repo PRs).\n` +
      `            NOT VERIFIED: GitHub's open Dependabot alerts for ${slug ?? "the repository"}.\n` +
      `            This does NOT mean the Security tab is clean — it means this gate could not look.\n` +
      `            reason: ${strictCrossCheckFailure}\n` +
      `            retries: transient faults get up to ${ALERT_ATTEMPTS} attempts with fixed 2s/4s\n` +
      `            backoff — see the retry lines in the log above; auth/config faults fail fast\n` +
      `            What to do:\n` +
      `            - API outage: re-run this job once https://www.githubstatus.com is green.\n` +
      `            - HTTP 401/403 or auth errors: the token setup is broken. ci.yml must keep the\n` +
      `              job-level security-events + issues permissions AND the GH_TOKEN step env.\n` +
      `            - Never silence this by deleting those from ci.yml, by setting\n` +
      `              WEB_AUDIT_OFFLINE=1, or by relaxing the script — that is the bug class of\n` +
      `              issue #107. See docs/WEB_AUDIT_POLICY.md.`,
  );
}

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
  ? "the GitHub cross-check did not run, so Dependabot's open alerts are UNVERIFIED (see WARNING)."
  : ghOnly.length === 0
    ? "GitHub currently reports nothing beyond pnpm's view."
    : `GitHub reports ${ghOnly.length} alert(s) beyond pnpm's view — see WARNING above.`;

process.stdout.write(
  `\nweb audit: OK — ${known.length} accepted finding(s) shown above, 0 unaccepted.\n` +
  `           Accepted findings are printed on every run on purpose.\n` +
  `           "OK" means: nothing is unaccepted AS FAR AS PNPM'S SOURCE CAN SEE. It does not mean\n` +
  `           "no known vulnerabilities" — ${gapNote}\n\n`,
);
