#!/usr/bin/env node
/**
 * Web dependency audit with teeth.
 *
 * Replaces a bare `pnpm audit`, which is red for every finding forever and therefore
 * trains everyone to ignore red. That is not "tracked", that is noise that gets muted.
 *
 * Instead this check distinguishes four things and fails loudly on three of them:
 *
 *   NEW     an advisory with no matching exception              -> blocking
 *   STALE   an exception expected no patch, but one exists now  -> blocking
 *   DEAD    an exception that matches nothing anymore           -> blocking
 *   OVERDUE an exception past its `reviewBy` date               -> blocking
 *   KNOWN   a still-unpatchable, accepted, in-date advisory      -> printed, not blocking
 *
 * Every KNOWN finding is re-printed on every run, so nothing is ever silently
 * swallowed: the acceptance is visible in the build log, dated, attributed, and
 * linked to an issue. An unusable audit result is an error, never a pass.
 *
 * Run from the `web/` directory:  cd web && node ../scripts/check-web-audit.mjs
 */
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const exceptionsPath = join(repoRoot, "web", "audit-exceptions.json");

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

if (blocking.length > 0) {
  process.stdout.write("\n");
  fail(blocking);
}

process.stdout.write(
  `\nweb audit: OK — ${known.length} accepted finding(s) shown above, 0 unaccepted.\n` +
  `           Accepted findings are printed on every run on purpose.\n\n`,
);
