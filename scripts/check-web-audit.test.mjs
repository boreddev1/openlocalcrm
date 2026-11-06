/**
 * Unit harness for the web audit security gate (GitHub issue #110).
 *
 *   node --test scripts/
 *
 * Runs with Node's built-in test runner only — no new npm dependency, no real
 * pnpm, no network, no frozen-clock tricks (time and the GitHub issue lookup
 * are injected into the gate's pure decision core). The golden strings below
 * are copied from the gate's pre-refactor source: they pin the exact wording
 * the build log shows and the blocking behaviour that protects the repo. If a
 * golden fails, either the gate silently changed its mind or its vocabulary —
 * both are review events, never "just update the test".
 *
 * The final describe block runs the gate end-to-end against PATH shims for
 * pnpm/git/gh inside a throwaway temp dir: exit codes, the loud degradation
 * lines, and the `gh api --paginate` flag itself (the --slurp no-op that sat
 * undiscovered until issue #104 is exactly what this block now pins). Shims
 * need a POSIX shell — the suite targets Linux/macOS dev boxes and the
 * ubuntu-latest runners, like the gate itself.
 */
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";

import {
  alertFailureReason,
  auditHeader,
  collectAdvisories,
  evaluateAdvisories,
  escapeRegExp,
  ghErrorReason,
  ghFailureResult,
  ghOnlyDisposition,
  ghOnlyWarning,
  interpretAlertsOutput,
  keyOf,
  lockfileTextHasPackage,
  parseJsonDocuments,
  selectGitHubOnlyAlerts,
} from "./check-web-audit.mjs";

// ---------------------------------------------------------------------------
// Fixtures — one advisory finding, one acceptance, deterministic "now".
// ---------------------------------------------------------------------------

const NOW = Date.parse("2026-11-06T12:00:00.000Z");
const ISSUE_URL = "https://github.com/o/r/issues/42";

const advisory = {
  module_name: "left-pad",
  vulnerable_versions: ">=1.0.0 <1.0.5",
  severity: "high",
  title: "Rendering injection",
  patched_versions: ">=1.0.5",
  github_advisory_id: "GHSA-test0001",
};

const exception = {
  package: "left-pad",
  vulnerableVersions: ">=1.0.0 <1.0.5",
  patchedVersionsExpected: ">=1.0.5",
  acceptedOn: "2026-10-01",
  acceptededBy: "ewen",
  reviewBy: "2026-12-31",
  issue: ISSUE_URL,
  reason: "waiting for upstream",
};

const runEval = (advisories, exceptions, deps = {}) =>
  evaluateAdvisories(advisories, exceptions, {
    resolveIssue: () => ({ kind: "open", num: "42" }),
    nowMs: NOW,
    ...deps,
  });

// ---------------------------------------------------------------------------

describe("parseJsonDocuments — the concatenated-page scanner", () => {
  it("parses a single pretty-printed page", () => {
    const docs = parseJsonDocuments('[\n  { "number": 1 },\n  { "number": 2 }\n]');
    assert.equal(docs.length, 1);
    assert.deepEqual(docs[0], [{ number: 1 }, { number: 2 }]);
  });

  it("parses a single minified page", () => {
    const docs = parseJsonDocuments('[{"number":1},{"number":2}]');
    assert.equal(docs.length, 1);
    assert.deepEqual(docs[0].map((a) => a.number), [1, 2]);
  });

  it("splits multiple concatenated pages, minified AND pretty mixed", () => {
    const text = '[{"number":1}]\n[\n  { "number": 2 }\n][{"number":3}]';
    const docs = parseJsonDocuments(text);
    assert.equal(docs.length, 3);
    assert.deepEqual(docs.flat().map((a) => a.number), [1, 2, 3]);
  });

  it("keeps braces inside strings from splitting documents", () => {
    const docs = parseJsonDocuments('[{"title":"a } b { c] d"}]');
    assert.equal(docs.length, 1);
    assert.equal(docs[0][0].title, "a } b { c] d");
  });

  it("honours escaped quotes and backslashes inside strings", () => {
    // JSON:  ["a\\", "x\"{ ]}"]  — a trailing-backslash string and an
    // escaped quote followed by a closing brace that must NOT end the doc.
    const docs = parseJsonDocuments('["a\\\\", "x\\"{ ]}"]');
    assert.equal(docs.length, 1);
    assert.deepEqual(docs[0], ["a\\", 'x"{ ]}']);
  });

  it("throws on truncated input — never a silent partial parse", () => {
    assert.throws(() => parseJsonDocuments('[{"a":1'), /truncated JSON/);
    assert.throws(() => parseJsonDocuments('[{"a":1}][{"b":'), /truncated JSON/);
  });

  it("ignores surrounding noise; returns [] when there is no JSON at all", () => {
    assert.deepEqual(parseJsonDocuments(""), []);
    assert.deepEqual(parseJsonDocuments("no json here"), []);
    assert.deepEqual(parseJsonDocuments('Page 1: {"a":1}\ntrailing'), [{ a: 1 }]);
  });

  it("handles nested containers and adjacent top-level values", () => {
    const docs = parseJsonDocuments('[]{"a":[{"b":{}}]}[[1]]');
    assert.deepEqual(docs, [[], { a: [{ b: {} }] }, [[1]]]);
  });
});

describe("interpretAlertsOutput — the GitHub degradation paths", () => {
  const rawAlert = (n, pkg, ecosystem = "npm") => ({
    number: n,
    security_advisory: { ghsa_id: `GHSA-${n}` },
    security_vulnerability: {
      package: { name: pkg, ecosystem },
      vulnerable_version_range: "<2.0.0",
      severity: "moderate",
    },
  });

  it("empty body is a broken response, NOT zero alerts", () => {
    for (const empty of ["", "   ", "\n  \t\n"]) {
      assert.deepEqual(interpretAlertsOutput(empty), {
        ok: false,
        reason: "dependabot alerts call returned no output at all",
      });
    }
    assert.deepEqual(interpretAlertsOutput(undefined), {
      ok: false,
      reason: "dependabot alerts call returned no output at all",
    });
  });

  it("`[]` parses as verified-and-empty, deliberately not the cannot-check path", () => {
    assert.deepEqual(interpretAlertsOutput("[]"), { ok: true, alerts: [] });
  });

  it("truncated JSON degrades loudly, never silently", () => {
    assert.deepEqual(interpretAlertsOutput('[{"a":1}][{"b":'), {
      ok: false,
      reason: "dependabot alerts response was not parsable JSON",
    });
  });

  it("concatenates multiple pages, keeps order, filters by npm ecosystem", () => {
    const text =
      JSON.stringify([rawAlert(1, "vite"), rawAlert(2, "requests", "pip")]) +
      "\n" +
      JSON.stringify([rawAlert(3, "left-pad")]);
    const out = interpretAlertsOutput(text);
    assert.equal(out.ok, true);
    assert.deepEqual(out.alerts, [
      { number: 1, ghsa: "GHSA-1", pkg: "vite", range: "<2.0.0", severity: "moderate" },
      { number: 3, ghsa: "GHSA-3", pkg: "left-pad", range: "<2.0.0", severity: "moderate" },
    ]);
  });

  it("wraps a stray single object document", () => {
    const out = interpretAlertsOutput(JSON.stringify(rawAlert(9, "evil")));
    assert.equal(out.ok, true);
    assert.equal(out.alerts.length, 1);
    assert.equal(out.alerts[0].ghsa, "GHSA-9");
  });

  it("maps missing fields to '?' and falls back advisory-severity→GHSA severity", () => {
    const out = interpretAlertsOutput(
      JSON.stringify([
        { number: 4, security_vulnerability: { package: { name: "x", ecosystem: "npm" } },
          security_advisory: { severity: "critical" } },
      ]),
    );
    assert.deepEqual(out.alerts, [
      { number: 4, ghsa: "?", pkg: "x", range: "?", severity: "critical" },
    ]);
  });
});

describe("gh degradation reasons — missing CLI, 401/403, generic failure", () => {
  it("ENOENT reads as 'gh CLI not installed', not as a silent skip", () => {
    assert.equal(ghErrorReason({ code: "ENOENT" }), "gh CLI not installed");
    assert.equal(ghErrorReason({ message: "socket hang up" }), "socket hang up");
    assert.equal(ghErrorReason({}), "[object Object]");
  });

  it("extracts the HTTP status from stderr and caps the reason at 180 chars", () => {
    const fail401 = ghFailureResult({ status: 1, stderr: "gh: HTTP 401 Unauthorized\nBad credentials" });
    assert.equal(fail401.ok, false);
    assert.equal(fail401.status, 401);
    assert.equal(fail401.reason, "gh: HTTP 401 Unauthorized");
    const long = "x".repeat(500);
    assert.equal(ghFailureResult({ status: 1, stderr: long }).reason.length, 180);
  });

  // Adapted on the #119 x #124 merge: the old guidance ("the alerts API needs
  // security-events: read") was MEASURED to be wrong — the workflow GITHUB_TOKEN
  // gets HTTP 403 "Resource not accessible by integration" even with
  // security-events: write granted (PR #119 CI, 2026-11-06). The gate now names
  // the credential that can actually read the endpoint, split by whether
  // WEB_AUDIT_ALERTS_TOKEN is present (injected second argument; default = this
  // run's env, pinned both ways below so host env cannot flip the assertion).
  it("401/403 name the alerts-capable credential for both token states", () => {
    for (const status of [401, 403]) {
      const run = ghFailureResult({ status: 1, stderr: `gh: HTTP ${status} Forbidden` });
      const noToken = alertFailureReason(run, false);
      assert.match(noToken, new RegExp(`^gh: HTTP ${status} Forbidden; `));
      assert.match(
        noToken,
        /no alerts-capable credential — GITHUB_TOKEN cannot read this endpoint \(measured 403 with SecurityEvents: write\)\. Set the repo secret WEB_AUDIT_ALERTS_TOKEN \(fine-grained PAT, "Dependabot alerts: read"\)$/,
      );
      const rejected = alertFailureReason(run, true);
      assert.match(
        rejected,
        new RegExp(`^gh: HTTP ${status} Forbidden; WEB_AUDIT_ALERTS_TOKEN was rejected — it needs "Dependabot alerts: read"`),
      );
      // The env default mirrors the explicit argument for this run's own state.
      const hasTokenEnv = (process.env.WEB_AUDIT_ALERTS_TOKEN ?? "").trim() !== "";
      assert.equal(alertFailureReason(run), hasTokenEnv ? rejected : noToken);
    }
  });

  it("the 401/403 branch wins over the auth-keyword branch", () => {
    const out = alertFailureReason({ status: 403, reason: "HTTP authentication failed" }, false);
    assert.match(out, /no alerts-capable credential/);
  });

  it("auth-mentioning reasons without 401/403 get the login hint", () => {
    assert.equal(
      alertFailureReason({ status: undefined, reason: "GH_TOKEN expired" }),
      "GH_TOKEN expired; gh needs a local login, WEB_AUDIT_ALERTS_TOKEN, or an exported GH_TOKEN/GITHUB_TOKEN",
    );
  });

  it("other statuses get the HTTP suffix; missing reasons never crash", () => {
    assert.equal(alertFailureReason({ status: 502, reason: "bad gateway" }), "bad gateway (HTTP 502)");
    assert.equal(alertFailureReason({ status: undefined, reason: "gh CLI not installed" }), "gh CLI not installed");
    assert.equal(alertFailureReason({ status: undefined }), "no output");
  });
});

describe("collectAdvisories + auditHeader — severity aggregation", () => {
  it("splits a pnpm audit document into findings and severity totals", () => {
    const { advisories, summary } = collectAdvisories({
      advisories: { "1": { module_name: "a" }, "2": { module_name: "b" } },
      metadata: { vulnerabilities: { high: 2 } },
    });
    assert.deepEqual(advisories, [{ module_name: "a" }, { module_name: "b" }]);
    assert.deepEqual(summary, { high: 2 });
  });

  it("tolerates an audit document with nothing in it", () => {
    assert.deepEqual(collectAdvisories({}), { advisories: [], summary: {} });
  });

  it("prints the exact header line, missing severity classes as zero", () => {
    const { advisories, summary } = collectAdvisories({
      advisories: { "1": {}, "2": {} },
      metadata: { vulnerabilities: { high: 1, moderate: 1 } },
    });
    assert.equal(
      auditHeader(advisories, summary),
      "\nweb audit — 2 advisory finding(s), severity totals: high=1 critical=0 moderate=1 low=0\n\n",
    );
    const empty = collectAdvisories({});
    assert.equal(
      auditHeader(empty.advisories, empty.summary),
      "\nweb audit — 0 advisory finding(s), severity totals: high=0 critical=0 moderate=0 low=0\n\n",
    );
  });

  it("keyOf binds package and range, never package alone", () => {
    assert.equal(keyOf("left-pad", ">=1.0.0"), "left-pad|>=1.0.0");
  });
});

describe("evaluateAdvisories — matching, blocking kinds, wording", () => {
  it("a matching, in-date exception with a live issue is KNOWN and not blocking", () => {
    const v = runEval([advisory], [exception]);
    assert.deepEqual(v.blocking, []);
    assert.deepEqual(v.warnings, []);
    assert.equal(v.known.length, 1);
    assert.equal(v.known[0].name, "left-pad");
    assert.equal(v.known[0].match, exception);
  });

  it("an advisory without an exception is NEW and blocking, worded exactly", () => {
    const other = {
      module_name: "other-pkg",
      vulnerable_versions: ">=9.0.0",
      severity: "low",
      title: "Prototype pollution",
    };
    const v = runEval([advisory, other], [exception]);
    assert.equal(v.blocking.length, 1);
    assert.equal(
      v.blocking[0],
      "NEW       other-pkg (low) vulnerable >=9.0.0 — Prototype pollution\n" +
        "          no acceptance exists. Upgrade, or add a dated entry to web/audit-exceptions.json.",
    );
    assert.equal(v.known.length, 1);
  });

  it("NEW prints '?' when the advisory carries no vulnerable range", () => {
    const v = runEval([{ module_name: "ghost", severity: "low" }], []);
    assert.equal(v.blocking.length, 1);
    assert.match(v.blocking[0], /^NEW       ghost \(low\) vulnerable \? — no title\n/);
  });

  it("STALE kills an acceptance once a patch exists — and does NOT double-report as DEAD", () => {
    const stale = { ...exception, patchedVersionsExpected: null };
    const v = runEval([advisory], [stale]);
    assert.deepEqual(v.blocking, [
      'STALE     left-pad: acceptance says "no patch exists", but >=1.0.5 is available now (issue https://github.com/o/r/issues/42).\n' +
        "          Upgrade and delete this exception — the reason for accepting it no longer holds.",
    ]);
    assert.equal(v.known.length, 0);
  });

  it("STALE needs a real patched_versions value (null/undefined/empty do not fire)", () => {
    const stale = { ...exception, patchedVersionsExpected: null };
    for (const patched of [undefined, null, ""]) {
      const v = runEval([{ ...advisory, patched_versions: patched }], [stale]);
      assert.deepEqual(v.blocking, [], `patched_versions=${JSON.stringify(patched)}`);
      assert.equal(v.known.length, 1);
    }
    const notStale = runEval([advisory], [{ ...exception, patchedVersionsExpected: ">=1.0.5" }]);
    assert.deepEqual(notStale.blocking, []);
  });

  it("a malformed entry (missing reviewBy) is BAD ENTRY, never an implicit pass", () => {
    for (const reviewBy of [undefined, "", "nonsense", "2026-13-45"]) {
      const v = runEval([advisory], [{ ...exception, reviewBy }]);
      assert.deepEqual(
        v.blocking,
        ["BAD ENTRY left-pad: exception has no valid `reviewBy` date — every acceptance must expire."],
        `reviewBy=${JSON.stringify(reviewBy)}`,
      );
    }
  });

  it("OVERDUE blocks at the exact expiry second + 1ms, prints '?' fallbacks", () => {
    const v = runEval([advisory], [{ ...exception, reviewBy: "2026-01-15" }]);
    assert.deepEqual(v.blocking, [
      "OVERDUE   left-pad: accepted on 2026-10-01 for review by 2026-01-15, which has passed (issue https://github.com/o/r/issues/42).\n" +
        "          Re-decide: fix it, or re-accept with a new date and a reason. Do not just extend blindly.",
    ]);
    const dayEnd = Date.parse("2026-11-06T23:59:59.000Z");
    assert.deepEqual(runEval([advisory], [{ ...exception, reviewBy: "2026-11-06" }], { nowMs: dayEnd }).blocking, []);
    const expired = runEval(
      [advisory],
      [{ ...exception, reviewBy: "2026-11-06", acceptedOn: undefined, issue: undefined }],
      { nowMs: dayEnd + 1 },
    );
    assert.match(expired.blocking[0], /^OVERDUE   left-pad: accepted on \? for review by 2026-11-06, which has passed \(issue \?\)\.\n/);
  });

  it("ORPHANED blocks on a closed or missing linked issue, worded exactly", () => {
    const closed = runEval([advisory], [exception], {
      resolveIssue: () => ({ kind: "closed", num: "42" }),
    });
    assert.deepEqual(closed.blocking, [
      `ORPHANED  left-pad: the exception links ${ISSUE_URL}, but issue #42 is CLOSED while this acceptance is still live.\n` +
        "          A closed issue is not a paper trail. Reopen the issue as long as the exception is\n" +
        "          carried, or upgrade and delete the exception.",
    ]);
    const missing = runEval([advisory], [exception], {
      resolveIssue: () => ({ kind: "missing", num: "42" }),
    });
    assert.match(missing.blocking[0], /issue #42 is MISSING \(HTTP 404\) while/);
  });

  it("an unverifiable issue state is a WARNING and still KNOWN — never a failure", () => {
    const v = runEval([advisory], [exception], {
      resolveIssue: () => ({ kind: "unverifiable", note: "gh CLI not installed" }),
    });
    assert.deepEqual(v.blocking, []);
    assert.equal(v.known.length, 1);
    assert.deepEqual(v.warnings, [
      `linked issue of exception left-pad (${ISSUE_URL}): GitHub state could not be checked (gh CLI not installed) — unverified, so not treated as a failure.`,
    ]);
    const noLink = runEval([advisory], [{ ...exception, issue: undefined }], {
      resolveIssue: () => ({ kind: "unverifiable", note: "no parseable issue link in the exception" }),
    });
    assert.match(noLink.warnings[0], /left-pad \(no link\): GitHub state could not be checked/);
  });

  it("GitHub is never consulted for NEW / STALE / OVERDUE / BAD ENTRY lines", () => {
    let calls = 0;
    const spy = () => { calls += 1; return { kind: "open", num: "1" }; };
    const cases = [
      [[advisory], []],
      [[advisory], [{ ...exception, patchedVersionsExpected: null }]],
      [[advisory], [{ ...exception, reviewBy: "2020-01-01" }]],
      [[advisory], [{ ...exception, reviewBy: "broken" }]],
    ];
    for (const [advs, excs] of cases) runEval(advs, excs, { resolveIssue: spy });
    assert.equal(calls, 0);
    const seen = [];
    runEval([advisory], [exception], { resolveIssue: (url) => { seen.push(url); return { kind: "open", num: "42" }; } });
    assert.deepEqual(seen, [ISSUE_URL]);
  });

  it("DEAD catches exceptions that match nothing — range drift included", () => {
    const dead = { ...exception, vulnerableVersions: ">=1.0.0 <1.0.4" };
    const v = runEval([advisory], [dead]);
    assert.deepEqual(v.blocking, [
      "NEW       left-pad (high) vulnerable >=1.0.0 <1.0.5 — Rendering injection\n" +
        "          no acceptance exists. Upgrade, or add a dated entry to web/audit-exceptions.json.",
      "DEAD      exception for left-pad (>=1.0.0 <1.0.4) matches nothing.\n" +
        "          The advisory is gone or the range changed — delete the entry instead of letting it rot.",
    ]);
    const alone = runEval([], [exception]);
    assert.equal(alone.blocking.length, 1);
    assert.match(alone.blocking[0], /^DEAD      exception for left-pad \(>=1\.0\.0 <1\.0\.5\) matches nothing\.\n/);
    const shapeless = runEval([], [{ reason: "typo in the field names" }]);
    assert.match(shapeless.blocking[0], /DEAD      exception for undefined \(undefined\) matches nothing\./);
  });

  it("two exceptions with the same key: the advisory is KNOWN once, neither reports DEAD", () => {
    const v = runEval([advisory], [exception, { ...exception, reviewBy: "2027-01-01" }]);
    assert.deepEqual(v.blocking, []);
    assert.equal(v.known.length, 1);
  });

  it("blocking order follows audit order, DEAD comes last — never re-sorted", () => {
    const v = runEval(
      [{ module_name: "zebra", severity: "low", vulnerable_versions: ">=1" }, advisory],
      [exception, { ...exception, package: "unused-pkg", vulnerableVersions: ">=0", reviewBy: "2030-01-01" }],
    );
    assert.equal(v.blocking.length, 2);
    assert.match(v.blocking[0], /^NEW       zebra/);
    assert.match(v.blocking[1], /^DEAD      exception for unused-pkg/);
  });
});

describe("GitHub-only classification against the lockfile", () => {
  const alerts = [
    { number: 1, ghsa: "GHSA-1", pkg: "vite", severity: "high", range: "<6.0.4" },
    { number: 2, ghsa: "GHSA-2", pkg: "left-pad", severity: "moderate", range: "<2" },
  ];

  it("selectGitHubOnlyAlerts is the set difference by GHSA id", () => {
    assert.deepEqual(selectGitHubOnlyAlerts(alerts, new Set(["GHSA-2"])), [alerts[0]]);
    assert.deepEqual(selectGitHubOnlyAlerts(alerts, new Set()), alerts);
    assert.deepEqual(selectGitHubOnlyAlerts([], new Set(["GHSA-1"])), []);
  });

  it("disposition: pnpm's own name set wins, else the lockfile decides tri-state", () => {
    const names = new Set(["vite"]);
    assert.equal(ghOnlyDisposition(alerts[0], names, () => { throw new Error("must not read lockfile"); }), true);
    assert.equal(ghOnlyDisposition(alerts[1], names, () => true), true);
    assert.equal(ghOnlyDisposition(alerts[1], names, () => false), false);
    assert.equal(ghOnlyDisposition(alerts[1], names, () => null), null);
    const asked = [];
    ghOnlyDisposition(alerts[1], names, (pkg) => { asked.push(pkg); return false; });
    assert.deepEqual(asked, ["left-pad"]);
  });

  it("each disposition gets its own honest warning — three exact texts", () => {
    assert.equal(
      ghOnlyWarning(alerts[0], true),
      "GitHub alert #1 GHSA-1 — vite (high, <6.0.4) is NOT yet visible to `pnpm audit`. Source lag, " +
        "not absence: the gate stays green until pnpm's source catches up, then this becomes NEW and " +
        "blocking. Fix it while it is still cheap.",
    );
    assert.equal(
      ghOnlyWarning(alerts[0], false),
      "GitHub alert #1 GHSA-1 — vite (high, <6.0.4) is NOT in web/pnpm-lock.yaml. The cross-check " +
        "cannot rule it out — a stale alert, a different manifest, or a tree mismatch. UNVERIFIED, " +
        "not accepted: close it in the Security tab with a reason or explain it in an issue.",
    );
    assert.equal(
      ghOnlyWarning(alerts[0], null),
      "GitHub alert #1 GHSA-1 — vite (high, <6.0.4) could not be matched against " +
        "web/pnpm-lock.yaml (unreadable from this run). UNVERIFIED, not accepted — check the " +
        "Security tab manually.",
    );
  });
});

describe("lockfile membership test (used by the classification above)", () => {
  const lock = [
    "lockfileVersion: '9.0'",
    "packages:",
    "  vite@6.0.3:",
    "    resolution: {integrity: sha256-x}",
    "  '@vue/compiler-sfc@3.5.13':",
    "    engines: {node: ^18.0.0}",
    "  minimatch@9.0.5:",
    "",
  ].join("\n");

  it("finds plain and quoted scoped package keys at line start", () => {
    assert.equal(lockfileTextHasPackage(lock, "vite"), true);
    assert.equal(lockfileTextHasPackage(lock, "@vue/compiler-sfc"), true);
    assert.equal(lockfileTextHasPackage(lock, "minimatch"), true);
  });

  it("does not match substrings, absent packages, or mid-line mentions", () => {
    assert.equal(lockfileTextHasPackage(lock, "ite"), false);
    assert.equal(lockfileTextHasPackage(lock, "react"), false);
    assert.equal(lockfileTextHasPackage(lock, "node"), false); // appears mid-line only (engines)
    assert.equal(lockfileTextHasPackage("", "vite"), false);
  });

  it("escapeRegExp neutralises regex metacharacters in package names", () => {
    assert.equal(escapeRegExp("a.b+c(d)"), "a\\.b\\+c\\(d\\)");
    assert.equal(lockfileTextHasPackage("  vite2x@1.0.0:", "vite"), false); // `.` must not be a wildcard
    assert.equal(lockfileTextHasPackage("  jquery.ui@1.0.0:", "jquery.ui"), true);
  });
});

// ---------------------------------------------------------------------------
// End-to-end: the gate as CI runs it, but with pnpm/git/gh shimmed onto an
// ISOLATED PATH (only the shim directory — the real binaries are unreachable,
// so "gh missing" is really missing) in a throwaway tree. The shims are Node
// scripts with an absolute-shebang to the running executable, so nothing on
// PATH is needed to boot them. This is what catches silent-no-op regressions
// in the wiring that unit tests of pure functions alone would still miss.
// ---------------------------------------------------------------------------

describe("gate end-to-end via PATH shims — exit codes and loud degradation", { concurrency: 1 }, () => {
  const gateSource = join(dirname(fileURLToPath(import.meta.url)), "check-web-audit.mjs");

  const pnpmAuditJson = (extraAdvisories) => {
    const advisories = {};
    extraAdvisories.forEach((a, i) => { advisories[String(i + 1)] = a; });
    return JSON.stringify({ advisories, metadata: { vulnerabilities: {} } });
  };

  const shebang = `#!${process.execPath}\n`;

  // Fake gh: logs every invocation to $GH_CALLS, then behaves per $GH_MODE:
  // "healthy" answers the alerts call with $GH_ALERTS_BODY and issue #42 with
  // $GH_ISSUE_STATE; "fail401" mimics an unauthorized call; "empty" succeeds
  // with no body at all.
  const ghShimScript = shebang + `
const fs = require("node:fs");
const args = process.argv.slice(2);
fs.appendFileSync(process.env.GH_CALLS, "gh " + args.join(" ") + "\\n");
const url = args[1] || "";
if (process.env.GH_MODE === "fail401") {
  process.stderr.write("gh: HTTP 401 Unauthorized - Bad credentials\\n");
  process.exit(1);
}
if (process.env.GH_MODE === "empty") process.exit(0);
if (url.includes("dependabot/alerts")) {
  process.stdout.write(fs.readFileSync(process.env.GH_ALERTS_BODY, "utf8"));
} else if (/\\/issues\\/42$/.test(url)) {
  process.stdout.write(process.env.GH_ISSUE_STATE + "\\n");
} else {
  process.exit(1);
}
`;

  const pnpmShimScript = shebang + `
process.stdout.write(require("node:fs").readFileSync(process.env.PNPM_AUDIT_JSON, "utf8"));
`;

  const gitShimScript = shebang + `
if (process.argv.slice(2).join(" ") === "config --get remote.origin.url") {
  process.stdout.write("https://github.com/o/r.git\\n");
} else {
  process.exit(128);
}
`;

  function runGate({
    exceptions = [],
    advisories = [],
    alertsBody = "[]\n",
    issueState = "open",
    ghMode = "healthy",
    noGh = false,
  } = {}) {
    const root = mkdtempSync(join(tmpdir(), "web-audit-harness-"));
    try {
      mkdirSync(join(root, "scripts"));
      mkdirSync(join(root, "web"));
      const bin = join(root, "bin");
      mkdirSync(bin);
      copyFileSync(gateSource, join(root, "scripts", "check-web-audit.mjs"));
      writeFileSync(join(root, "web", "audit-exceptions.json"), JSON.stringify({ advisories: exceptions }));
      writeFileSync(
        join(root, "web", "pnpm-lock.yaml"),
        "lockfileVersion: '9.0'\npackages:\n  vite@6.0.3:\n    resolution: {}\n  left-pad@1.0.0:\n    resolution: {}\n",
      );
      const callsPath = join(root, "gh-calls.log");
      writeFileSync(callsPath, "");
      const shims = { pnpm: pnpmShimScript, git: gitShimScript, gh: noGh ? null : ghShimScript };
      for (const [name, body] of Object.entries(shims)) {
        if (body === null) continue;
        const shimPath = join(bin, name);
        writeFileSync(shimPath, body);
        chmodSync(shimPath, 0o755);
      }
      const auditJsonPath = join(root, "pnpm-audit.json");
      writeFileSync(auditJsonPath, pnpmAuditJson(advisories));
      const alertsBodyPath = join(root, "gh-alerts.json");
      writeFileSync(alertsBodyPath, alertsBody);
      // PATH is ONLY the shim dir: the real pnpm/git/gh cannot leak in, and
      // "no gh" really means ENOENT.
      const env = {
        ...process.env,
        PATH: bin,
        GH_CALLS: callsPath,
        GH_MODE: ghMode,
        GH_ALERTS_BODY: alertsBodyPath,
        GH_ISSUE_STATE: issueState,
        PNPM_AUDIT_JSON: auditJsonPath,
      };
      delete env.WEB_AUDIT_OFFLINE;
      // The merged gate (#119) gained env-driven branches — a dedicated alerts
      // token and strict mode. Pin every e2e scenario to the non-strict,
      // no-dedicated-token path these tests describe, whatever the invoking
      // shell happens to export, exactly like WEB_AUDIT_OFFLINE above.
      delete env.WEB_AUDIT_ALERTS_TOKEN;
      delete env.WEB_AUDIT_REQUIRE_CROSSCHECK;
      const run = spawnSync(process.execPath, [join(root, "scripts", "check-web-audit.mjs")], {
        cwd: join(root, "web"),
        env,
        encoding: "utf8",
        timeout: 30_000,
      });
      if (run.error) throw run.error;
      return { rc: run.status, stdout: run.stdout ?? "", calls: readFileSync(callsPath, "utf8") };
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  }

  const liveAdvisory = { ...advisory, github_advisory_id: "GHSA-e2e0001" };
  const freshException = { ...exception, reviewBy: "2099-12-31" };

  it("unaccepted advisory with an empty exceptions file exits 1 — loudly", () => {
    const out = runGate({ advisories: [liveAdvisory] });
    assert.equal(out.rc, 1);
    assert.match(out.stdout, /no exceptions file entries/);
    assert.match(out.stdout, /web audit: FAILED/);
  });

  it("accepted, in-date advisory with a live issue exits 0 and re-prints KNOWN", () => {
    const out = runGate({ advisories: [liveAdvisory], exceptions: [freshException] });
    assert.equal(out.rc, 0);
    assert.match(out.stdout, /  KNOWN     left-pad \(high\) vulnerable >=1\.0\.0 <1\.0\.5\n/);
    assert.match(out.stdout, /web audit: OK — 1 accepted finding\(s\)/);
  });

  it("DEAD exception exits 1 even when pnpm reports nothing", () => {
    const out = runGate({ exceptions: [freshException] });
    assert.equal(out.rc, 1);
    assert.match(out.stdout, /^  DEAD      exception for left-pad \(>=1\.0\.0 <1\.0\.5\) matches nothing\./m);
    assert.match(out.stdout, /web audit: FAILED/);
  });

  it("closed linked issue exits 1 as ORPHANED", () => {
    const out = runGate({
      advisories: [liveAdvisory],
      exceptions: [freshException],
      issueState: "closed",
    });
    assert.equal(out.rc, 1);
    assert.match(out.stdout, /ORPHANED  left-pad/);
  });

  it("the alerts cross-check calls `gh api --paginate` — and never --slurp (#104 regression)", () => {
    const out = runGate();
    assert.equal(out.rc, 0);
    const alertCall = out.calls.split("\n").find((line) => line.includes("dependabot/alerts"));
    assert.ok(alertCall, "no dependabot/alerts call was made at all — the cross-check went dark");
    assert.match(alertCall, /--paginate/);
    assert.doesNotMatch(alertCall, /--slurp/);
    assert.match(out.stdout, /GitHub dependabot open:  none \(0\)/);
  });

  it("a missing gh CLI degrades LOUDLY but never changes the exit code", () => {
    const out = runGate({ noGh: true });
    assert.equal(out.rc, 0);
    assert.match(out.stdout, /COULD NOT CHECK — UNVERIFIED \(gh CLI not installed/);
    assert.match(out.stdout, /WARNING   Dependabot cross-check did not run/);
    assert.match(out.stdout, /web audit: OK/);
  });

  it("HTTP 401 names the missing permission in the degradation, exit stays per the old rules", () => {
    const out = runGate({ ghMode: "fail401" });
    assert.equal(out.rc, 0);
    assert.match(
      out.stdout,
      /COULD NOT CHECK — UNVERIFIED \(gh: HTTP 401 Unauthorized - Bad credentials; no alerts-capable credential — GITHUB_TOKEN cannot read this endpoint \(measured 403 with SecurityEvents: write\)\. Set the repo secret WEB_AUDIT_ALERTS_TOKEN \(fine-grained PAT, "Dependabot alerts: read"\)\)/,
    );
  });

  it("an empty successful response is 'no output at all', NOT zero alerts", () => {
    const out = runGate({ ghMode: "empty" });
    assert.equal(out.rc, 0);
    assert.match(out.stdout, /COULD NOT CHECK — UNVERIFIED \(dependabot alerts call returned no output at all\)/);
  });

  it("a GitHub-only alert in the lockfile is called source lag, not absence", () => {
    const out = runGate({
      alertsBody: JSON.stringify([
        {
          number: 5,
          security_advisory: { ghsa_id: "GHSA-lag01" },
          security_vulnerability: {
            package: { name: "vite", ecosystem: "npm" },
            severity: "high",
            vulnerable_version_range: "<6.0.4",
          },
        },
      ]) + "\n",
    });
    assert.equal(out.rc, 0);
    assert.match(out.stdout, /difference \(GitHub-only, pnpm source behind\): GHSA-lag01/);
    assert.match(out.stdout, /is NOT yet visible to `pnpm audit`\. Source lag, not absence/);
  });

  it("importing the gate module never runs it (harness precondition)", async () => {
    // If this import ever executed main(), the real pnpm/gh would run inside
    // the test process; the pure-function blocks above would still pass, so
    // pin it: importing yields the API and nothing else has happened.
    const mod = await import("./check-web-audit.mjs");
    assert.equal(typeof mod.evaluateAdvisories, "function");
    assert.equal(typeof mod.parseJsonDocuments, "function");
  });
});
