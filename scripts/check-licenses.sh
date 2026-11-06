#!/usr/bin/env bash
#
# Open-Source license compliance gate.
#
# Policy (AGENTS.md "License policy", docs/COMPLIANCE_AND_LICENSES.md §2):
# dependencies must be 100% permissive. No GPL/AGPL/SSPL or other copyleft.
#
# This script enforces that as a WHITELIST, not as a list of banned strings.
# The previous version grepped THIRD-PARTY-LICENSES.csv for
# "AGPL|SSPL|BUSL|Commons-Clause|GPL-2|GPL-3" and called anything it did not
# match compliant — which is how MPL-2.0 (file-level copyleft, and already in
# use by the queue worker) passed with nobody deciding it. Issue #126.
#
# Three tiers now:
#   PERMISSIVE       MIT / Apache-2.0 / BSD-2-Clause / BSD-3-Clause / 0BSD / ISC /
#                    CC0-1.0 / Unlicense  -> passes, no decision needed.
#   HARD-BANNED      anything carrying GPL/AGPL/LGPL/SSPL/BUSL/Commons-Clause/
#                    FSL/CC-BY-NC/NonCommercial/Proprietary -> fails, and no entry
#                    in license-exceptions.json can override it.
#   REQUIRES DECISION everything else (MPL-2.0, EPL, CDDL, Hippocratic-2.0/2.1,
#                    and any licence id the gate does not recognise) -> fails with
#                    UNACCEPTED unless license-exceptions.json carries a dated,
#                    attributed, issue-linked acceptance for that exact
#                    package + license pair.
#
# The gate also checks its own record, because a compliance list that rots is not
# evidence: rows must be well-formed and unique, every direct go.mod requirement
# and every runtime npm dependency must be listed, listed rows must still exist in
# the dependency graph, tagged LICENSE links must point at the version actually
# selected, and exception entries that match nothing (DEAD) or have expired
# (OVERDUE) fail the run.
#
# Exit 0 means: every listed row is permissive or carries a live acceptance, and
# the record is internally consistent. It does NOT mean "every transitive package
# was license-checked" — see the SCOPE block it prints and
# docs/COMPLIANCE_AND_LICENSES.md §2.6.
#
# Set LICENSE_GATE_ONLINE=1 to additionally probe every license URL in the CSV
# (dead link -> blocking, network failure -> unverified warning). Off by default:
# this runs inside git hooks, where the network may be unavailable.
set -euo pipefail

echo "========================================="
echo "Open-Source License Compliance Gate"
echo "========================================="

for tool in go node; do
    if ! command -v "$tool" > /dev/null 2>&1; then
        echo "❌ '$tool' is required to run this gate."
        echo "   A check that could not run must not report success — fix the toolchain, do not skip the step."
        exit 1
    fi
done

cd "$(dirname "$0")/.."

read -r -d '' LICENSE_GATE_PROGRAM <<'NODE_SCRIPT' || true
const fs = require("node:fs");
const { spawnSync } = require("node:child_process");

const CSV_PATH = "THIRD-PARTY-LICENSES.csv";
const EXC_PATH = "license-exceptions.json";
const PKG_PATH = "web/package.json";
const LOCK_PATH = "web/pnpm-lock.yaml";

// --- policy sets -----------------------------------------------------------
// Licences that need no decision. Extending this set is a policy change to
// docs/COMPLIANCE_AND_LICENSES.md §2.1 and needs the owner's approval — an
// entry in license-exceptions.json is the normal route for a single package.
const PERMISSIVE = new Set([
  "MIT", "MIT-0", "APACHE-2.0", "BSD-2-CLAUSE", "BSD-3-CLAUSE",
  "0BSD", "ISC", "CC0-1.0", "UNLICENSE",
]);

// Copyleft / source-available restrictions that no acceptance can override.
// Deliberately substring-based on a normalised id, so "GPLv3", "GPL-1.0",
// "GNU General Public License v3" and "AGPL-3.0" all land here. (The old
// regex missed "GPLv3" entirely.)
const HARD_BANNED = [
  { re: /AGPL|LGPL|GPL|SSPL|AFFERO|GENERAL-PUBLIC/, why: "GPL-family copyleft" },
  { re: /BUSL|BUSINESS-SOURCE/, why: "Business Source License" },
  { re: /COMMONS-CLAUSE/, why: "Commons Clause add-on" },
  { re: /FSL-|FUNCTIONAL-SOURCE/, why: "Functional Source License" },
  { re: /CC-BY-NC|NON-COMMERCIAL|NONCOMMERCIAL/, why: "non-commercial restriction" },
  { re: /PROPRIETARY|COMMERCIAL-LICENSE|RESTRICTED/, why: "proprietary / restricted" },
];

// Known weak-copyleft / use-restricted ids, for clearer messages. Anything not
// listed here and not permissive is treated as UNRECOGNISED — also needs a
// decision, because an unrecognised licence id is never silently trusted.
const KNOWN_DECISION_NEEDED = [
  { re: /^MPL-/, why: "file-level copyleft (MPL)" },
  { re: /^EPL-|^CPL-|^CDDL-|^MS-RL-|^APACL-|^SLEEPYCAT/, why: "weak copyleft" },
  { re: /^HIPPOCRATIC/, why: "MIT plus a use restriction — not an OSI-permissive licence" },
  { re: /^WTFPL|^BEERWARE|^ZLIB|^LIBPNG|^BSD-4-CLAUSE|^BSD-3-CLAUSE-NO-(MODIFICATION|PATENT)/, why: "not in the machine-enforced permissive set" },
];

const norm = (s) => String(s ?? "").trim().toLowerCase();
const canonLic = (s) => String(s ?? "").trim().toUpperCase().replace(/[\s_]+/g, "-");
const isDate = (s) => /^\d{4}-\d{2}-\d{2}$/.test(String(s ?? "")) && !Number.isNaN(Date.parse(`${s}T23:59:59Z`));

/** Split "MIT OR Apache-2.0" style dual licence expressions. */
function alternatives(license) {
  return canonLic(license).split(/\s+OR\s+/).map((p) => p.trim()).filter(Boolean);
}

/** Classify one CSV row's licence column. */
function classify(license) {
  const alts = alternatives(license);
  if (alts.length === 0) return { tier: "decision", why: "empty or unreadable licence column" };
  // Dual-licensed: the licensee may choose, so one permissive alternative is enough.
  if (alts.some((a) => PERMISSIVE.has(a))) return { tier: "permissive", why: `permissive (${alts.filter((a) => PERMISSIVE.has(a)).join(" / ")})` };
  for (const a of alts) {
    for (const rule of HARD_BANNED) if (rule.re.test(a)) return { tier: "banned", why: `${rule.why} — hard policy limit, no acceptance can override` };
  }
  for (const a of alts) for (const known of KNOWN_DECISION_NEEDED) {
    if (known.re.test(a)) return { tier: "decision", why: known.why };
  }
  return { tier: "decision", why: "licence not in the machine-enforced permissive set" };
}

function run(cmd, args) {
  const r = spawnSync(cmd, args, { encoding: "utf8", maxBuffer: 32 * 1024 * 1024 });
  return r;
}

function close(msg) {
  process.stdout.write("\n");
  for (const line of msg) process.stdout.write(`  ${line}\n`);
  process.stdout.write("\nlicense gate: FAILED\n");
  process.exit(1);
}

/** Does this lockfile text carry the package as one of its keys? */
function lockHas(text, pkg) {
  const esc = pkg.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`(?:^|\\n)[ \\t]*'?(?:[^'\\n]*/)?${esc}@`).test(text);
}

async function main() {
  const blocking = [];
  const accepted = [];
  const warnings = [];

  // --- inputs -------------------------------------------------------------
  let csvText;
  try { csvText = fs.readFileSync(CSV_PATH, "utf8"); }
  catch (e) { close([`cannot read ${CSV_PATH}: ${e.message}`, "A check that cannot read its input must not report success."]); }

  let excRaw;
  try { excRaw = fs.readFileSync(EXC_PATH, "utf8"); }
  catch (e) { close([`cannot read ${EXC_PATH}: ${e.message}`,
    "Copyleft-tier licences are only allowed through that file. If no acceptance is needed, keep the file with an empty list.",
    "A check that cannot read its exceptions must not report success."]); }
  let exceptions;
  try { exceptions = JSON.parse(excRaw).dependencies ?? []; }
  catch (e) { close([`${EXC_PATH} is not valid JSON: ${e.message}`]); }
  if (!Array.isArray(exceptions)) close([`${EXC_PATH}: "dependencies" must be an array.`]);

  let pkgJson;
  try { pkgJson = JSON.parse(fs.readFileSync(PKG_PATH, "utf8")); }
  catch (e) { close([`cannot read ${PKG_PATH}: ${e.message}`]); }
  const npmRuntime = Object.keys(pkgJson.dependencies ?? {});
  const npmDev = Object.keys(pkgJson.devDependencies ?? {});

  let lockText = null;
  try { lockText = fs.readFileSync(LOCK_PATH, "utf8"); } catch { /* unverifiable below */ }

  // Go module graph: name -> selected version. `go list -m all` needs the
  // module cache/`go mod download`; if it cannot answer, the gate stops.
  const goList = run("go", ["list", "-m", "all"]);
  if (goList.error || goList.status !== 0) {
    close([`cannot resolve the Go module graph (\`go list -m all\`): ${(goList.error?.message ?? goList.stderr ?? "no output").toString().split("\n")[0]}`,
      "Coverage and version-drift checks would be blind, so the gate refuses to guess."]);
  }
  const goVersion = new Map();
  const replaced = new Set();
  for (const line of (goList.stdout ?? "").split("\n")) {
    const t = line.trim();
    if (!t) continue;
    const left = t.split(" => ")[0];
    if (t.includes(" => ")) replaced.add(norm(t.split(" => ")[0].split(/\s+/)[0]));
    const parts = left.split(/\s+/);
    // The main module is printed without a version — it is in the graph, but
    // there is no selected release to compare a CSV link against.
    if (parts.length === 1) goVersion.set(norm(parts[0]), "");
    else goVersion.set(norm(parts[0]), parts.slice(1).join(" "));
  }

  const goMod = run("go", ["mod", "edit", "-json"]);
  if (goMod.error || goMod.status !== 0) {
    close([`cannot read go.mod (\`go mod edit -json\`): ${(goMod.error?.message ?? goMod.stderr ?? "no output").toString().split("\n")[0]}`]);
  }
  let requires = [];
  try { requires = JSON.parse(goMod.stdout).Require ?? []; }
  catch (e) { close([`go mod edit -json output was not parsable: ${e.message}`]); }
  const goDirect = requires.filter((r) => r.Indirect !== true).map((r) => r.Path);

  // --- CSV rows ------------------------------------------------------------
  const rows = [];
  csvText.split(/\r?\n/).forEach((line, i) => {
    const no = i + 1;
    const t = line.trim();
    if (t === "" || t.startsWith("#")) return;
    const parts = t.split(",");
    if (parts.length !== 3 || parts.some((p) => p.trim() === "")) {
      blocking.push(`MALFORMED   ${CSV_PATH}:${no}: expected "package,url,licence", got "${t}"`);
      return;
    }
    rows.push({ no, name: parts[0].trim(), url: parts[1].trim(), license: parts[2].trim() });
  });

  const byName = new Map();
  for (const row of rows) {
    const key = norm(row.name);
    if (byName.has(key)) {
      blocking.push(`DUP ROW     ${row.name} appears twice (${CSV_PATH}:${byName.get(key).no} and :${row.no}) — one row per dependency.`);
      continue;
    }
    byName.set(key, row);
  }

  // --- coverage: the record has to describe what is actually used -----------
  for (const mod of goDirect) {
    if (!byName.has(norm(mod))) {
      blocking.push(
        `UNLISTED    ${mod} is a direct requirement in go.mod but has no row in ${CSV_PATH}.\n` +
        `              Add "<module>,<LICENSE url>,<SPDX id>" — the gate cannot classify what the record does not mention.`,
      );
    }
  }
  for (const dep of npmRuntime) {
    if (!byName.has(norm(dep))) {
      blocking.push(
        `UNLISTED    ${dep} is a runtime dependency in ${PKG_PATH} but has no row in ${CSV_PATH}.\n` +
        `              Add "<package>,<LICENSE url>,<SPDX id>" — this ships inside web/dist and the embedded binary.`,
      );
    }
  }

  // --- rows that no longer describe a dependency ----------------------------
  let listedGo = 0, listedNpm = 0, unverifiable = [];
  const goPathLike = (n) => /^[a-z0-9.-]+\.[a-z]{2,}\//i.test(n);
  for (const row of rows) {
    const key = norm(row.name);
    if (goVersion.has(key)) {
      listedGo += 1;
      const version = goVersion.get(key);
      const tag = (row.url.match(/\/(?:blob|tree)\/([^/]+)\//) ?? [])[1];
      const branchLink = /^(master|main|trunk|HEAD|develop|stable)$/i.test(tag);
      const pseudo = /-\d{14}-[0-9a-f]{12}$/.test(version);
      if (tag && !branchLink && !pseudo && version !== "" && !replaced.has(key)) {
        const a = tag.replace(/^v/, "").replace(/\+incompatible$/, "");
        const b = String(version).replace(/^v/, "").replace(/\+incompatible$/, "");
        if (a !== b) {
          blocking.push(
            `DRIFT       ${row.name}: ${CSV_PATH}:${row.no} links the licence at tag ${tag}, but the build selects ${version}.\n` +
            `              Point the row at the version you actually ship — a compliance record that describes another\n` +
            `              release is not evidence. Fix the link, or drop the tag if upstream has no tagged licence.`,
          );
        }
      } else {
        unverifiable.push(row.name);
      }
      row.goVersion = version;
    } else if (goPathLike(row.name)) {
      blocking.push(
        `GHOST ROW   ${row.name} (${CSV_PATH}:${row.no}) is not in the Go module graph — either it is gone (delete the row)\n` +
        `              or its name is misspelled (fix it so the coverage checks can see it).`,
      );
    } else {
      listedNpm += 1;
      const known = npmRuntime.includes(row.name) || npmDev.includes(row.name) ||
        (lockText !== null && lockHas(lockText, row.name));
      if (!known) {
        blocking.push(
          `GHOST ROW   ${row.name} (${CSV_PATH}:${row.no}) is neither in ${PKG_PATH} nor in ${LOCK_PATH} —` +
          (lockText === null ? ` and ${LOCK_PATH} could not be read to check.\n` : " delete the row or fix the package name.\n") +
          `              A stale row makes the whole list less credible, which is the point of checking it.`,
        );
      }
    }
  }

  // --- licence classification + exceptions ----------------------------------
  const key = (pkg, lic) => `${norm(pkg)}|${canonLic(lic)}`;
  const used = new Set();
  const now = Date.now();

  for (const row of rows) {
    const cls = classify(row.license);
    if (cls.tier === "permissive") continue;
    if (cls.tier === "banned") {
      blocking.push(
        `BANNED      ${row.name} (${CSV_PATH}:${row.no}) is ${row.license} — ${cls.why}.\n` +
        `              This is the hard limit from AGENTS.md ("No GPL/AGPL/SSPL or other copyleft"): removing the\n` +
        `              dependency is the only fix. license-exceptions.json cannot accept it, on purpose.`,
      );
      continue;
    }
    const match = exceptions.find((e) => key(e.package, e.license) === key(row.name, row.license));
    if (!match) {
      blocking.push(
        `UNACCEPTED  ${row.name} is ${row.license} (${cls.why}) — ${CSV_PATH}:${row.no}\n` +
        `              No copyleft-tier licence passes this gate without an explicit, dated, attributed, issue-linked\n` +
        `              acceptance in ${EXC_PATH}. Either replace the dependency with a permissive one, or record the\n` +
        `              decision the way docs/COMPLIANCE_AND_LICENSES.md §2.4 describes. Do not add the licence id to\n` +
        `              the permissive set as a side effect of a dependency bump — that is a policy change, not a fix.`,
      );
      continue;
    }
    used.add(key(match.package, match.license));
    accepted.push({ row, match, why: cls.why });
  }

  // --- exception entries themselves ----------------------------------------
  const seenEntry = new Set();
  for (const e of exceptions) {
    const label = `${e.package ?? "?"} (${e.license ?? "?"})`;
    const missing = ["package", "license", "decision", "reason", "issue", "recordedOn", "reviewBy"].filter((f) => !e[f]);
    if (missing.length) {
      blocking.push(`BAD ENTRY   ${label}: missing ${missing.join(", ")} — every acceptance must say what, why, who owes it, when, and until when.`);
      continue;
    }
    const k = key(e.package, e.license);
    if (seenEntry.has(k)) {
      blocking.push(`BAD ENTRY   ${label}: duplicated acceptance — keep one entry per dependency + licence.`);
      continue;
    }
    seenEntry.add(k);

    if (!isDate(e.recordedOn) || !isDate(e.reviewBy)) {
      blocking.push(`BAD ENTRY   ${label}: recordedOn/reviewBy must be YYYY-MM-DD dates — every acceptance has to expire.`);
      continue;
    }
    if (Date.parse(`${e.reviewBy}T23:59:59Z`) <= Date.parse(`${e.recordedOn}T23:59:59Z`)) {
      blocking.push(`BAD ENTRY   ${label}: reviewBy (${e.reviewBy}) is not after recordedOn (${e.recordedOn}).`);
      continue;
    }
    if (!/^https:\/\/github\.com\/[^/]+\/[^/]+\/(issues|pull)\/\d+/.test(e.issue)) {
      blocking.push(`BAD ENTRY   ${label}: issue must be a link to a GitHub issue or pull request, got "${e.issue}".`);
      continue;
    }
    if (e.decision !== "accepted" && e.decision !== "pending") {
      blocking.push(`BAD ENTRY   ${label}: decision must be "accepted" or "pending", got "${e.decision}".`);
      continue;
    }
    if (e.decision === "accepted" && !e.acceptedBy) {
      blocking.push(`BAD ENTRY   ${label}: decision "accepted" requires acceptedBy — name the human who decided.`);
      continue;
    }
    if (e.decision === "pending" && !e.decidedBy) {
      blocking.push(`BAD ENTRY   ${label}: decision "pending" requires decidedBy — name who owes the decision.`);
      continue;
    }
    if (e.decision === "pending") {
      const days = (Date.parse(`${e.reviewBy}T23:59:59Z`) - Date.parse(`${e.recordedOn}T23:59:59Z`)) / 86400000;
      if (days > 90) {
        blocking.push(
          `BAD ENTRY   ${label}: an unapproved ("pending") acceptance may not be parked for ${Math.round(days)} days.\n` +
          `              Decide it, or record it as accepted with a normal review interval — "pending" is not a long-term state.`,
        );
        continue;
      }
    }
    if (now > Date.parse(`${e.reviewBy}T23:59:59Z`)) {
      blocking.push(
        `OVERDUE     ${label}: recorded ${e.recordedOn}, review was due ${e.reviewBy} (issue ${e.issue}).\n` +
        `              Silence is not a decision: re-decide it (fix the reason, re-accept with a new date) or remove it.\n` +
        `              Do not just push the date out without a reason.`,
      );
      continue;
    }
    if (!used.has(k)) {
      const row = byName.get(norm(e.package));
      const cls = row ? classify(row.license).tier : null;
      const hint = !row
        ? `the dependency is not in ${CSV_PATH} at all — it was removed, renamed, or the entry is a typo.`
        : cls === "permissive"
          ? `the CSV lists it as ${row.license} (${CSV_PATH}:${row.no}), which is permissive — no acceptance is required, so this entry is noise, not a decision. Delete it.`
          : cls === "banned"
            ? `the CSV lists it as ${row.license} (${CSV_PATH}:${row.no}) — that licence is hard-banned, so no acceptance can ever cover it. Drop the dependency and this entry.`
            : `the CSV lists it as ${row.license} (${CSV_PATH}:${row.no}) — the licence changed or the entry is mistyped, so this acceptance does not cover what is in use now.`;
      blocking.push(
        `DEAD        acceptance for ${label} matches nothing.\n` +
        `              ${hint}\n` +
        `              Delete the entry instead of letting it rot — an unfounded acceptance is how exceptions outlive their decision.`,
      );
    }
  }

  // An acceptance for a permissive licence is noise, not a decision: it can
  // never be "used" above, so it surfaces as DEAD with that wording.

  // --- optional live-link check --------------------------------------------
  let linksLine;
  if (process.env.LICENSE_GATE_ONLINE === "1") {
    const results = await Promise.all(rows.map(async (row) => {
      try {
        const res = await fetch(row.url, { redirect: "follow", signal: AbortSignal.timeout(20000) });
        return { row, status: res.status };
      } catch (e) {
        return { row, error: e.name === "TimeoutError" ? "timed out" : e.message };
      }
    }));
    let ok = 0;
    for (const r of results) {
      if (r.error) warnings.push(`LINK UNVERIFIED ${r.row.name} (${r.row.url}): ${r.error} — network, not a licence verdict; check it manually.`);
      else if (r.status >= 400) blocking.push(`DEAD LINK   ${r.row.name} (${CSV_PATH}:${r.row.no}): ${r.row.url} -> HTTP ${r.status}\n              a compliance record must point at the licence the dependency actually ships.`);
      else ok += 1;
    }
    linksLine = `  LINKS       ${ok}/${rows.length} licence URLs fetched (live check requested via LICENSE_GATE_ONLINE=1)\n`;
  } else {
    linksLine = "  LINKS       not fetched (offline gate; run with LICENSE_GATE_ONLINE=1 to verify the URLs resolve)\n";
  }

  // --- report ---------------------------------------------------------------
  process.stdout.write(`\nlicense gate — ${rows.length} row(s) in ${CSV_PATH}, ` +
    `${PERMISSIVE.size} permissive licence id(s) accepted without a decision, ` +
    `${exceptions.length} acceptance(s) on file\n\n`);

  for (const a of accepted) {
    const head = a.match.decision === "pending"
      ? `PENDING     ${a.row.name} (${a.row.license}) — ${a.why}`
      : `ACCEPTED    ${a.row.name} (${a.row.license}) — ${a.why}`;
    const who = a.match.decision === "pending"
      ? `no decision yet — owed by ${a.match.decidedBy}`
      : `accepted by ${a.match.acceptedBy}`;
    process.stdout.write(
      `  ${head}\n` +
      `              ${who}; recorded ${a.match.recordedOn}, re-review by ${a.match.reviewBy}\n` +
      `              issue: ${a.match.issue}\n` +
      `              reason: ${a.match.reason}\n`,
    );
  }

  const unlistedGraphModules = [...goVersion.keys()].filter((m) => !byName.has(m));
  process.stdout.write(
    `\n  COVERAGE    go.mod direct requirements: ${goDirect.length} (${goDirect.filter((m) => byName.has(norm(m))).length} listed)\n` +
    `              npm runtime dependencies:   ${npmRuntime.length} (${npmRuntime.filter((d) => byName.has(d)).length} listed)\n` +
    `              npm build-time dev deps:    ${npmDev.length} (not required to be listed — see scope)\n` +
    `              Go graph modules:           ${goVersion.size} (${listedGo} listed, ${unlistedGraphModules.length} unlisted — mostly test-only)\n` +
    `              licence links not version-verifiable: ${unverifiable.length} (branch link, pseudo-version, main module, replace, or unrelated host)\n` +
    linksLine,
  );

  if (warnings.length) {
    process.stdout.write("\n");
    for (const w of warnings) process.stdout.write(`  WARNING   ${w}\n`);
  }

  if (blocking.length) {
    process.stdout.write("\n");
    for (const line of blocking) process.stdout.write(`  ${line}\n`);
    process.stdout.write(`\nlicense gate: FAILED (${blocking.length} blocking finding(s))\n`);
    process.exit(1);
  }

  const pending = accepted.filter((a) => a.match.decision === "pending").length;
  process.stdout.write(
    `\nlicense gate: OK — ${accepted.length} copyleft-tier acceptance(s) on file (${pending} still pending a decision), ` +
    `everything else permissive.\n` +
    `           Accepted and pending rows are re-printed on every run on purpose: the decision stays visible.\n` +
    `           "OK" means: every row of ${CSV_PATH} is permissive or carries a live, dated, issue-linked\n` +
    `           acceptance, every direct Go/npm runtime dependency is listed, and the record still matches the\n` +
    `           graph. It does NOT mean every transitive package was license-checked — see\n` +
    `           docs/COMPLIANCE_AND_LICENSES.md §2.6 for what is not covered.\n\n`,
  );
}

main().catch((e) => {
  process.stdout.write(`\nlicense gate crashed: ${e?.stack ?? e}\nA check that did not finish must not report success.\n`);
  process.exit(1);
});
NODE_SCRIPT

exec node -e "$LICENSE_GATE_PROGRAM"
