package server_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

// ---------------------------------------------------------------------------
// Shared constants — defined once here, generator must match.
// ---------------------------------------------------------------------------

// inventoryHeading is the markdown heading that marks the route inventory
// section. The parser looks for a heading containing this exact substring.
const inventoryHeading = "Routen-Inventur"

// inventoryStartMarker and inventoryEndMarker delimit the generated table
// region. The parser looks for the start marker after the heading.
const inventoryStartMarker = "<!-- route-inventory:start -->"
const inventoryEndMarker = "<!-- route-inventory:end -->"

// routeEntry is one (method, path, auth class) triple.
//
// The Auth field's history is the history of issue #115. The salvaged branch
// documented an "Auth" column as "public" or "csrf" and claimed chi records
// the middleware chain per mount so the walk could derive it. That claim is
// false for chi v5.5.0: Mux.Routes() reports only the mount pattern and the
// HTTP methods, never the middlewares stacked on it. PR #113 therefore
// shipped the inventory without an auth column at all.
//
// Now the class is declared data instead of derived magic: server.RouteAuthClasses
// (internal/server/route_auth.go) maps path prefixes to classes, is authored
// next to the mounts it describes (router.go mounts the same constants), and is
// checked three ways:
//
//	a. every walked route resolves to exactly one table row (walkAndClassify
//	   fails otherwise — a missing or empty table is a failure, never a skip);
//	b. every table row covers at least one walked route (no stale rows);
//	c. an unauthenticated request against the built router answers the way the
//	   class promises (route_auth_class_test.go) — so deleting AuthMiddleware
//	   from the protected group fails the probe even if the table is untouched.
//
// The probe replaces the blanket "fire a request per row and read the status"
// idea rejected in #113, and it survives its traps only because it is narrow:
// it never authenticates, so the CSRF Bearer exemption cannot flip an answer;
// 401-class routes are rejected by the JWT middleware before any limiter is
// consulted, and the few limiter-guarded routes it does hit get one request
// per freshly built router, far below their budgets; the connector webhook and
// the SSE stream answer 401 for their own reasons, which the probe can prove
// only as "rejected unauthenticated", not as "JWT middleware mounted" — that
// distinction rests on the table row, and docs/API_REFERENCE.md says so.
type routeEntry struct {
	Method string
	Path   string
	Auth   string
}

// key returns a stable sort/compare key.
func (e routeEntry) key() string { return e.Method + " " + e.Path }

// ---------------------------------------------------------------------------
// MAIN GUARD — bidirectional route inventory check
//
// docs/API_REFERENCE.md <-> internal/server/router.go. Same shape as the repo's
// existing documentation-drift guard, model_label_drift_test.go: read the
// artifact, compare it against the thing it describes, fail on any mismatch.
// ---------------------------------------------------------------------------

// TestRouteInventoryMatchesRouter is the fail-closed drift guard. It constructs
// the router, walks every leaf route, parses the Routen-Inventur table from
// docs/API_REFERENCE.md, and compares bidirectionally. When the inventory is
// missing or empty, the test fails — there is no t.Logf-only bypass path.
//
// The self-tests below (TestGuardSensitivity*) exercise the comparison engine
// and example sweep independently on synthetic inputs and must all pass.
func TestRouteInventoryMatchesRouter(t *testing.T) {
	// 1+2. Build the router with the demo querier (so every handler is
	// registered) and walk every leaf route under /api/v1. walkedRoutes fails
	// closed on a zero-route walk, so the guard can never pass vacuously
	// because the enumeration silently found nothing.
	registered := walkedRoutes(t)
	n := len(registered)
	t.Logf("walked %d leaf routes from router", n)

	// 3. Parse the inventory table and the curl examples from the doc.
	docPath := filepath.Join("..", "..", "docs", "API_REFERENCE.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("cannot read API_REFERENCE.md: %v", err)
	}
	docContent := string(docBytes)

	// Fail-closed. The salvaged branch carried a `OPENCRM_RED_INVENTORY`
	// sentinel here that skipped the guard whenever the table was missing, and
	// the api-reference-drift branch's own second commit (259a509, the tip) had
	// degraded the same case to t.Logf + return. Both turned the guard into a
	// no-op, so the escape hatch is deleted, exactly as the sentinel's own
	// comment instructed: a missing, empty or malformed inventory is a
	// failure, never a skip.
	documented, err := parseInventoryFromContent(docContent, docPath)
	if err != nil {
		t.Fatalf("inventory parse failed (%v) — the Routen-Inventur table in %s is missing, empty or malformed; run `UPDATE_ROUTE_INVENTORY=1 go test ./internal/server/ -run TestAPIReferenceInventoryUpdate -count=1` and commit the regenerated table", err, docPath)
	}
	t.Logf("parsed %d routes from inventory", len(documented))

	// A well-formed table with zero data rows must fail too
	// (parseInventoryFromContent already errors on it; the double-assert keeps
	// the intent visible at the call site).
	if len(documented) == 0 {
		t.Fatal("route inventory parsed zero rows — guard cannot verify anything")
	}

	// 4. Check for duplicate (method, path) pairs in the inventory.
	seen := map[string]bool{}
	for _, e := range documented {
		k := e.key()
		if seen[k] {
			t.Errorf("duplicate route in inventory: %s", k)
		}
		seen[k] = true
	}

	// 5. Validate every inventory row.
	for _, e := range documented {
		if e.Method == "" {
			t.Errorf("inventory row with empty method: path=%q", e.Path)
		}
		if e.Method != strings.ToUpper(e.Method) {
			t.Errorf("inventory row method not uppercase: %q (path=%s)", e.Method, e.Path)
		}
		if e.Path == "" || e.Path[0] != '/' {
			t.Errorf("inventory row with invalid path: method=%s path=%q", e.Method, e.Path)
		}
		// The inventory spells path parameters the way chi registers them. This
		// is not cosmetics: compareRoutes keys on the literal path, so a
		// hand-written "<contact_id>" row would read as a phantom route, and the
		// angle-bracket form is what the curl prose below uses for concrete
		// example IDs. Rejecting it here names the real mistake instead.
		for _, seg := range strings.Split(strings.Trim(e.Path, "/"), "/") {
			if strings.HasPrefix(seg, "<") || strings.HasSuffix(seg, ">") {
				t.Errorf("inventory path %q uses angle-bracket placeholder %q — the inventory uses chi's {param} spelling", e.Path, seg)
			}
		}
	}

	// 6. Bidirectional comparison.
	missing, phantom := compareRoutes(registered, documented)

	// Direction (a): every route the router registers must be in the table.
	// Without this, an endpoint can be shipped with no documentation at all.
	for _, m := range missing {
		t.Errorf("registered route not in inventory: %s", m.key())
	}

	// Direction (b): no table row may name a route the router no longer
	// registers. This is the one an endpoint removal or rename produces, and the
	// one that leaves integrators calling a URL that only ever 404s.
	for _, p := range phantom {
		t.Errorf("inventory route not registered by router: %s", p.key())
	}

	// 6b. Auth class column. The registered rows carry their class from
	// server.RouteAuthClasses (resolved in walkAndClassify, which fails closed
	// when a route resolves to no row); the documented rows carry what the
	// file says. Any difference is documentation drift on a security property
	// — exactly what issue #115 was opened for.
	for _, e := range documented {
		if e.Auth == "" {
			t.Errorf("inventory row %s has an empty auth class — the class is mandatory (fail-closed)", e.key())
		}
	}
	for _, issue := range compareAuthClasses(registered, documented) {
		t.Errorf("auth class drift: %s", issue)
	}

	// 6c. Stale table rows: a prefix in the auth-class table that covers no
	// registered route means the mount it describes was removed or renamed,
	// and the table is lying about a route that no longer exists.
	if len(server.RouteAuthClasses) == 0 {
		t.Fatal("server.RouteAuthClasses is empty — the auth ground truth is missing; that is a failure, not a skip (issue #115)")
	}
	coveredPrefixes := map[string]bool{}
	for _, e := range registered {
		if rc, ok := server.ResolveRouteAuthClass(e.Path); ok {
			coveredPrefixes[rc.Prefix] = true
		}
	}
	for _, row := range server.RouteAuthClasses {
		if !coveredPrefixes[row.Prefix] {
			t.Errorf("auth class row %q (%s) covers no registered route — the mount was removed or renamed; fix internal/server/route_auth.go", row.Prefix, row.Class)
		}
	}

	// 7. Sweep the curl examples below the table for calls that do not exist.
	exampleIssues := sweepCurlExamples(docContent, registered)
	for _, issue := range exampleIssues {
		t.Errorf("curl example issue: %s", issue)
	}
}

// ---------------------------------------------------------------------------
// SENSITIVITY TESTS — proof that the guard can actually fail
//
// A fail-closed guard that cannot fail is worse than no guard, so each check the
// main guard performs has a test that performs the corresponding drift and
// asserts it is caught.
// ---------------------------------------------------------------------------

// TestGuardSensitivityMissingRow deletes one registered route's row from a
// temporary inventory copy and asserts compareRoutes reports it as missing.
func TestGuardSensitivityMissingRow(t *testing.T) {
	registered := []routeEntry{
		{Method: "GET", Path: "/api/v1/health"},
		{Method: "POST", Path: "/api/v1/auth/login"},
	}
	inventory := make([]routeEntry, len(registered))
	copy(inventory, registered)
	// Delete the second row from the inventory.
	inventory = append(inventory[:1], inventory[2:]...)

	missing, phantom := compareRoutes(registered, inventory)

	if len(missing) != 1 {
		t.Fatalf("expected 1 missing route after deleting a row, got %d", len(missing))
	}
	if missing[0].Method != "POST" || missing[0].Path != "/api/v1/auth/login" {
		t.Errorf("wrong missing route: %s (expected POST /api/v1/auth/login)", missing[0].key())
	}
	if len(phantom) != 0 {
		t.Errorf("expected 0 phantom routes, got %d", len(phantom))
	}
}

// TestGuardSensitivityPhantomRow appends the historical phantom
// POST /api/v1/admin/ownership/bulk-assign to a temporary inventory copy and
// asserts compareRoutes rejects it. That path is not a hypothetical: it is what
// this document said while the router served /api/v1/ownership/bulk-assign.
func TestGuardSensitivityPhantomRow(t *testing.T) {
	registered := []routeEntry{
		{Method: "GET", Path: "/api/v1/health"},
		{Method: "POST", Path: "/api/v1/ownership/bulk-assign"},
	}
	inventory := []routeEntry{
		{Method: "GET", Path: "/api/v1/health"},
		{Method: "POST", Path: "/api/v1/ownership/bulk-assign"},
		// The phantom route.
		{Method: "POST", Path: "/api/v1/admin/ownership/bulk-assign"},
	}

	missing, phantom := compareRoutes(registered, inventory)

	if len(phantom) != 1 {
		t.Fatalf("expected 1 phantom route, got %d", len(phantom))
	}
	if phantom[0].Method != "POST" || phantom[0].Path != "/api/v1/admin/ownership/bulk-assign" {
		t.Errorf("wrong phantom route: %s (expected POST /api/v1/admin/ownership/bulk-assign)", phantom[0].key())
	}
	if len(missing) != 0 {
		t.Errorf("expected 0 missing routes, got %d", len(missing))
	}
}

// TestGuardSensitivityPhantomExample tests the example sweep on synthetic
// content: one case with a phantom URL, one case with a real path but a method
// the router does not serve for it.
func TestGuardSensitivityPhantomExample(t *testing.T) {
	registered := []routeEntry{
		{Method: "GET", Path: "/api/v1/health"},
		{Method: "POST", Path: "/api/v1/ownership/bulk-assign"},
	}

	// Case 1: A curl example with an unregistered path.
	doc1 := "```bash\ncurl -X POST http://localhost:8080/api/v1/admin/ownership/bulk-assign \\\n  -H 'Content-Type: application/json'\n```"
	issues1 := sweepCurlExamples(doc1, registered)
	if len(issues1) == 0 {
		t.Error("expected phantom-example detection for /api/v1/admin/ownership/bulk-assign, got none")
	} else {
		t.Logf("correctly detected phantom example: %s", issues1[0])
	}

	// Case 2: A curl example with a registered path but wrong method.
	doc2 := "```bash\ncurl -X DELETE http://localhost:8080/api/v1/health\n```"
	issues2 := sweepCurlExamples(doc2, registered)
	if len(issues2) == 0 {
		t.Error("expected wrong-method detection for DELETE /api/v1/health (only GET registered), got none")
	} else {
		t.Logf("correctly detected wrong method: %s", issues2[0])
	}
}

// TestGuardFailsOnMissingInventory: parseInventoryFromContent must return an
// error when the inventory heading is absent, when the heading has no table
// under it, and when the table has no data rows. Those three are the ways a
// "guard" quietly stops guarding, so the parser has to refuse all of them.
func TestGuardFailsOnMissingInventory(t *testing.T) {
	// (a) No inventory heading at all.
	content := "# Some doc\n\nNo inventory here.\n"
	_, err := parseInventoryFromContent(content, "<synthetic>")
	if err == nil {
		t.Error("guard did not fail when inventory heading is absent — expected error, got nil")
	} else {
		t.Logf("correctly failed on absent heading: %v", err)
	}

	// (b) Heading present, but no table follows.
	contentB := "## " + inventoryHeading + "\n\nSome text but no table.\n"
	_, err = parseInventoryFromContent(contentB, "<synthetic>")
	if err == nil {
		t.Error("guard did not fail when heading present but no table — expected error, got nil")
	} else {
		t.Logf("correctly failed on heading without table: %v", err)
	}

	// (c) Heading present, separator line present, but zero data rows.
	contentC := "## " + inventoryHeading + "\n\n" + inventoryStartMarker + "\n| Method | Path | AuthGroup |\n| --- | --- | --- |\n" + inventoryEndMarker + "\n"
	_, err = parseInventoryFromContent(contentC, "<synthetic>")
	if err == nil {
		t.Error("guard did not fail when table has zero data rows — expected error, got nil")
	} else {
		t.Logf("correctly failed on zero-row table: %v", err)
	}

	// (d) The old two-column shape must not be tolerated: deleting the auth
	// column cannot be allowed to turn the auth check into a silent no-op.
	contentD := "## " + inventoryHeading + "\n\n" + inventoryStartMarker + "\n| Methode | Pfad |\n| --- | --- |\n| GET | /api/v1/health |\n" + inventoryEndMarker + "\n"
	_, err = parseInventoryFromContent(contentD, "<synthetic>")
	if err == nil {
		t.Error("a two-column row without the auth class was accepted; the parser must fail closed on the pre-#115 shape")
	} else {
		t.Logf("correctly failed on old two-column shape: %v", err)
	}

	// (e) A row whose auth cell is empty: missing ground truth is a failure.
	contentE := "## " + inventoryHeading + "\n\n" + inventoryStartMarker + "\n| Methode | Pfad | Authentifizierung |\n| --- | --- | --- |\n| GET | /api/v1/health |  |\n" + inventoryEndMarker + "\n"
	_, err = parseInventoryFromContent(contentE, "<synthetic>")
	if err == nil {
		t.Error("a row with an empty auth class was accepted; fail-closed requires a parse error")
	} else {
		t.Logf("correctly failed on empty auth class cell: %v", err)
	}
}

// TestGuardSensitivityAuthClassDrift proves the auth column is compared, not
// just printed: a documented row whose class differs from the router table is
// reported, an agreeing row is not, and a phantom route is left to
// compareRoutes instead of double-reporting.
func TestGuardSensitivityAuthClassDrift(t *testing.T) {
	registered := []routeEntry{
		{Method: "GET", Path: "/api/v1/health", Auth: "none"},
		{Method: "POST", Path: "/api/v1/contacts", Auth: "jwt+csrf"},
	}

	if issues := compareAuthClasses(registered, registered); len(issues) != 0 {
		t.Errorf("agreeing rows reported %d auth issues, want 0: %v", len(issues), issues)
	}

	wrong := []routeEntry{
		{Method: "GET", Path: "/api/v1/health", Auth: "jwt+csrf"}, // understated protection: the dangerous direction
		{Method: "POST", Path: "/api/v1/contacts", Auth: "jwt+csrf"},
	}
	issues := compareAuthClasses(registered, wrong)
	if len(issues) != 1 || !strings.Contains(issues[0], "GET /api/v1/health") {
		t.Fatalf("flipping the documented class of GET /api/v1/health was not detected: %v", issues)
	}
	t.Logf("correctly detected auth drift: %s", issues[0])

	// A documented route the router does not register is compareRoutes'
	// problem; the auth comparison must stay quiet about it.
	phantomOnly := []routeEntry{
		{Method: "GET", Path: "/api/v1/health", Auth: "none"},
		{Method: "POST", Path: "/api/v1/contacts", Auth: "jwt+csrf"},
		{Method: "POST", Path: "/api/v1/ghost", Auth: "banana"},
	}
	if issues := compareAuthClasses(registered, phantomOnly); len(issues) != 0 {
		t.Errorf("phantom row produced %d auth issues, want 0: %v", len(issues), issues)
	}
}

// TestGuardFailsOnZeroWalk: an empty router walk must not make the comparison
// pass vacuously — every documented row then counts as phantom.
func TestGuardFailsOnZeroWalk(t *testing.T) {
	registered := []routeEntry{}
	documented := []routeEntry{
		{Method: "GET", Path: "/api/v1/health"},
	}
	missing, phantom := compareRoutes(registered, documented)
	if len(phantom) != len(documented) {
		t.Errorf("expected all %d documented routes reported as phantom with empty walk, got %d", len(documented), len(phantom))
	}
	if len(missing) != 0 {
		t.Errorf("expected 0 missing routes with empty walk, got %d", len(missing))
	}
}

// TestGuardFailsOnPhantomInLaterBlock — regression for the defect that made the
// salvaged sweep useless. The fence pattern the branch shipped
// ("(?s)```(?:bash|sh|shell)?\s*\n(.*?)```") cannot skip a ```json or ```text
// block, so it paired that block's closing fence with the next opening fence and
// read the document out of phase: against this repository's own API_REFERENCE.md
// it inspected 2 of 41 curl examples. A phantom that sits in a later block —
// which is exactly where the real POST /api/v1/admin/ownership/bulk-assign sits,
// after the auth section — was therefore never looked at, and the guard stayed
// green on documented drift.
//
// The three phantoms below are placed after a json block, after an unlabelled
// block and after a text block. All three must be reported.
func TestGuardFailsOnPhantomInLaterBlock(t *testing.T) {
	registered := []routeEntry{
		{Method: "GET", Path: "/api/v1/health"},
		{Method: "POST", Path: "/api/v1/contacts"},
		{Method: "GET", Path: "/api/v1/notes/{id}"},
	}
	content := "## A\n\n```json\n{ \"status\": \"healthy\" }\n```\n\n" +
		"```bash\ncurl -X POST http://localhost:8080/api/v1/ghost/one\n```\n\n" +
		"```bash\ncurl -X GET http://localhost:8080/api/v1/ghost/two\n```\n\n" +
		"```text\nevent: connected\n```\n\n" +
		"```bash\ncurl -X GET http://localhost:8080/api/v1/ghost/three\n```\n"

	issues := sweepCurlExamples(content, registered)
	if len(issues) != 3 {
		t.Fatalf("expected all 3 phantom examples reported, got %d: %v", len(issues), issues)
	}
	// The reported line numbers must point at the offending curl line, because
	// that is the only thing that makes the failure actionable for whoever edits
	// the document.
	for _, want := range []string{
		"line 8:", "line 12:", "line 20:",
	} {
		found := false
		for _, issue := range issues {
			if strings.HasPrefix(issue, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no issue reported at %s: %v", want, issues)
		}
	}
	for _, want := range []string{"/api/v1/ghost/one", "/api/v1/ghost/two", "/api/v1/ghost/three"} {
		found := false
		for _, issue := range issues {
			if strings.Contains(issue, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("phantom example %s not reported: %v", want, issues)
		}
	}
}

// TestGuardSensitivityRealDocumentPhantomExample runs the sweep over the actual
// docs/API_REFERENCE.md and asserts it inspects the whole document, not a slice
// of it: every curl line the file contains must be resolved to a URL. A sweep
// that silently skips blocks reports zero unparsed lines, so this is the check
// that the sweep is looking at all of them.
func TestGuardSensitivityRealDocumentPhantomExample(t *testing.T) {
	docPath := filepath.Join("..", "..", "docs", "API_REFERENCE.md")
	contentBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v", docPath, err)
	}
	content := string(contentBytes)

	total := 0
	parsed := 0
	for _, block := range fencedBlockBodies(content) {
		for _, cmd := range curlCommands(block) {
			total++
			if url, _ := parseCurlLine(cmd.text); url != "" {
				parsed++
			}
		}
	}
	if total == 0 {
		t.Fatal("no curl examples found in the reference document — the sweep has nothing to check")
	}
	if parsed != total {
		t.Errorf("sweep inspects %d of %d curl examples; %d examples are not parsed and could hide drift", parsed, total, total-parsed)
	}
	t.Logf("sweep covers all %d curl examples in %s", total, docPath)
}

// TestGuardFailsOnRealDocumentDrift runs the guard's own comparison against the
// real reference document with one row taken away, and with one row that does
// not exist added. This is the permanent form of the drill: it mutates the
// document only in memory, so nothing in the repository is touched, but it
// exercises the parser and the comparison on the real file rather than on a
// three-row fixture. A guard whose sensitivity only shows up on synthetic
// input has not been shown to work on the thing it is supposed to watch.
func TestGuardFailsOnRealDocumentDrift(t *testing.T) {
	registered := walkedRoutes(t)

	docPath := filepath.Join("..", "..", "docs", "API_REFERENCE.md")
	contentBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v", docPath, err)
	}
	content := string(contentBytes)

	documented, err := parseInventoryFromContent(content, docPath)
	if err != nil {
		t.Fatalf("cannot parse the inventory of %s: %v", docPath, err)
	}

	// Drift direction (a): a route the router registers but the document does
	// not list must be reported, at three different positions so a parser that
	// only reads the head or the tail of the table cannot pass.
	for _, idx := range []int{0, len(documented) / 2, len(documented) - 1} {
		dropped := make([]routeEntry, 0, len(documented)-1)
		dropped = append(dropped, documented[:idx]...)
		dropped = append(dropped, documented[idx+1:]...)

		missing, phantom := compareRoutes(registered, dropped)
		if len(missing) != 1 || missing[0].key() != documented[idx].key() {
			t.Errorf("deleting inventory row %d (%s) was not detected: missing=%v", idx, documented[idx].key(), keysOf(missing))
		}
		if len(phantom) != 0 {
			t.Errorf("deleting a row also produced %d phantom routes: %v", len(phantom), keysOf(phantom))
		}
	}

	// The same at the text level: removing the physical table line must reduce
	// the parsed row count, which is what makes the row above disappear from the
	// document rather than merely from a fixture.
	target := documented[len(documented)/2]
	line := "| " + target.Method + " | " + target.Path + " | " + target.Auth + " |"
	stripped := strings.Replace(content, line+"\n", "", 1)
	if stripped == content {
		t.Fatalf("table line %q not found in %s — the generator and the parser disagree about the row format", line, docPath)
	}
	after, err := parseInventoryFromContent(stripped, docPath)
	if err != nil {
		t.Fatalf("parse after removing one line: %v", err)
	}
	if len(after) != len(documented)-1 {
		t.Errorf("removing one table line left %d rows, want %d", len(after), len(documented)-1)
	}

	// Auth-column drills at the text level, on the real file (issue #115):
	// mislabelling a row's class, and deleting the column, must both be caught.
	flipped := "none"
	if target.Auth == "none" {
		flipped = "jwt+csrf"
	}
	flippedLine := "| " + target.Method + " | " + target.Path + " | " + flipped + " |"
	flippedContent := strings.Replace(content, line+"\n", flippedLine+"\n", 1)
	if flippedContent == content {
		t.Fatalf("table line %q not found for the auth-flip drill", line)
	}
	flippedDoc, err := parseInventoryFromContent(flippedContent, docPath)
	if err != nil {
		t.Fatalf("parse after flipping one auth class: %v", err)
	}
	if issues := compareAuthClasses(registered, flippedDoc); len(issues) != 1 || !strings.Contains(issues[0], target.key()) {
		t.Errorf("flipping the documented auth class of %s (%s -> %s) was not detected: %v", target.key(), target.Auth, flipped, issues)
	} else {
		t.Logf("correctly detected auth flip: %s", issues[0])
	}

	twoColLine := "| " + target.Method + " | " + target.Path + " |"
	twoColContent := strings.Replace(content, line+"\n", twoColLine+"\n", 1)
	if twoColContent == content {
		t.Fatalf("table line %q not found for the column-removal drill", line)
	}
	if _, err := parseInventoryFromContent(twoColContent, docPath); err == nil {
		t.Error("removing the auth column from one row still parsed; the guard must refuse the two-column shape")
	} else {
		t.Logf("correctly refused a row without the auth column: %v", err)
	}

	// Drift direction (b): a documented route the router does not register.
	phantomRow := routeEntry{Method: "GET", Path: "/api/v1/endpoint-that-does-not-exist"}
	added := make([]routeEntry, 0, len(documented)+1)
	added = append(added, documented...)
	added = append(added, phantomRow)

	missing, phantom := compareRoutes(registered, added)
	if len(phantom) != 1 || phantom[0].key() != phantomRow.key() {
		t.Errorf("inventing a documented route was not detected as phantom: %v", keysOf(phantom))
	}
	if len(missing) != 0 {
		t.Errorf("inventing a route also produced %d missing routes: %v", len(missing), keysOf(missing))
	}
}

func keysOf(entries []routeEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.key())
	}
	return out
}

// TestWalkRoutesNormalizesChiPatterns pins the two properties the inventory
// depends on: a walked path is an addressable path (no chi mount wildcards, no
// lost leaf segment). The refusal of a genuine catch-all is covered separately
// by TestNormalizeChiPatternRejectsCatchAll, because chi itself rejects a
// mid-path "*" at registration time and such a route never reaches the walk.
func TestWalkRoutesNormalizesChiPatterns(t *testing.T) {
	noop := func(w http.ResponseWriter, r *http.Request) {}

	r := chi.NewRouter()
	r.Get("/api/v1/health", noop)
	r.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/login", noop)
		api.Group(func(protected chi.Router) {
			protected.Route("/contacts", func(cr chi.Router) {
				cr.Get("/", noop)
				cr.Get("/{id}", noop)
			})
		})
	})

	// walkRoutes sorts by path, then method: /api/v1/auth/login <
	// /api/v1/contacts < /api/v1/contacts/{id} < /api/v1/health.
	got := walkRoutes(r, "/api/v1")
	want := []string{
		"POST /api/v1/auth/login",
		"GET /api/v1/contacts",
		"GET /api/v1/contacts/{id}",
		"GET /api/v1/health",
	}
	if len(got) != len(want) {
		t.Fatalf("walked %d routes, want %d: %+v", len(got), len(want), got)
	}
	for i, e := range got {
		if e.key() != want[i] {
			t.Errorf("route %d = %q, want %q", i, e.key(), want[i])
		}
		if strings.Contains(e.Path, "*") {
			t.Errorf("route %d kept a chi mount wildcard: %q", i, e.Path)
		}
	}
}

// TestNormalizeChiPatternRendersMounts covers the mount shapes the real router
// uses, so the normalisation is pinned by example rather than by the walk alone.
func TestNormalizeChiPatternRendersMounts(t *testing.T) {
	cases := []struct {
		parent, pattern, want string
	}{
		{"", "/api/v1/health", "/api/v1/health"},               // leaf on the root mux
		{"", "/api/v1/*", "/api/v1"},                           // r.Route("/api/v1", ...)
		{"/api/v1", "/", "/api/v1"},                            // api.Group(...)
		{"/api/v1", "/auth/login", "/api/v1/auth/login"},       // api.Post("/auth/login")
		{"/api/v1", "/contacts/*", "/api/v1/contacts"},         // protected.Route("/contacts")
		{"/api/v1/contacts", "/", "/api/v1/contacts"},          // cr.Get("/")
		{"/api/v1/contacts", "/{id}", "/api/v1/contacts/{id}"}, // cr.Get("/{id}")
		{"/api/v1/deals/{id}", "/revoke", "/api/v1/deals/{id}/revoke"},
	}
	for _, c := range cases {
		if got := normalizeChiPattern(c.parent, c.pattern); got != c.want {
			t.Errorf("normalizeChiPattern(%q, %q) = %q, want %q", c.parent, c.pattern, got, c.want)
		}
	}
}

// TestNormalizeChiPatternRejectsCatchAll — the normaliser may only drop the `/*`
// chi puts on a mount. A real wildcard route answers an unbounded set of paths
// and cannot be inventoried as one row, so it must abort rather than be silently
// rendered as its prefix. (chi itself refuses a mid-path `*` at registration
// time, which is why this exercises the function directly.)
func TestNormalizeChiPatternRejectsCatchAll(t *testing.T) {
	defer func() {
		p := recover()
		if p == nil {
			t.Error("normalizeChiPattern accepted a catch-all wildcard; expected it to refuse to invent an inventory row")
			return
		}
		t.Logf("correctly refused catch-all: %v", p)
	}()
	normalizeChiPattern("/api/v1", "/files/*/nested")
}

// ---------------------------------------------------------------------------
// GENERATOR — rewrites the inventory table from the router walk
// ---------------------------------------------------------------------------

// TestAPIReferenceInventoryUpdate regenerates the Routen-Inventur table in
// docs/API_REFERENCE.md from the walked router, so the generator's source of
// truth stays the router rather than a hand-kept list: whoever adds an endpoint
// re-runs this instead of transcribing a row. The Authentifizierung column
// comes from the same walk — walkAndClassify resolves each route against
// server.RouteAuthClasses and aborts (no silent blank cell) when a route has
// no declared class.
//
// The salvaged branch instead called out to a `scripts/gen_route_inventory.mjs`
// that was never landed; that indirection is dropped and the regeneration lives
// next to the guard consuming it.
//
// It is a maintenance command, not a gate. Without the environment variable it
// skips, so an ordinary `go test ./...` can never silently rewrite a tracked
// file:
//
//	UPDATE_ROUTE_INVENTORY=1 go test ./internal/server/ -run TestAPIReferenceInventoryUpdate -count=1
func TestAPIReferenceInventoryUpdate(t *testing.T) {
	if os.Getenv("UPDATE_ROUTE_INVENTORY") != "1" {
		t.Skip("re-run with UPDATE_ROUTE_INVENTORY=1 to rewrite the Routen-Inventur table in docs/API_REFERENCE.md")
	}

	registered := walkedRoutes(t)

	docPath := filepath.Join("..", "..", "docs", "API_REFERENCE.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v", docPath, err)
	}
	updated, err := replaceInventorySection(string(docBytes), renderInventory(registered))
	if err != nil {
		t.Fatalf("cannot regenerate inventory in %s: %v", docPath, err)
	}
	if err := os.WriteFile(docPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("cannot write %s: %v", docPath, err)
	}
	t.Logf("wrote %d routes to %s", len(registered), docPath)
}

// replaceInventorySection swaps the table between the start and end markers.
// It refuses to invent the section: if either marker is missing the caller must
// add the skeleton (heading + markers) by hand, which keeps a regeneration from
// silently creating a second inventory somewhere in the middle of the document.
func replaceInventorySection(content, table string) (string, error) {
	start := strings.Index(content, inventoryStartMarker)
	if start < 0 {
		return "", fmt.Errorf("start marker %q not found — add the %s section skeleton to the document first", inventoryStartMarker, inventoryHeading)
	}
	after := content[start+len(inventoryStartMarker):]
	end := strings.Index(after, inventoryEndMarker)
	if end < 0 {
		return "", fmt.Errorf("end marker %q not found after the start marker", inventoryEndMarker)
	}
	return content[:start] + inventoryStartMarker + "\n" + table + after[end:], nil
}

// renderInventory produces the markdown table for the walked routes. It is the
// only writer of the table's body, which is what makes the guard maintainable:
// the rows are the walk's output, so "documentation equals code" is the default
// state rather than something a human has to remember.
func renderInventory(entries []routeEntry) string {
	var b strings.Builder
	b.WriteString("| Methode | Pfad | Authentifizierung |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", e.Method, e.Path, e.Auth)
	}
	return b.String()
}

// newDemoRouter builds the router exactly like the other server tests do (demo
// querier, so every handler is registered). Every call returns a fresh router
// with fresh rate limiters — the unauthenticated probe relies on that to keep
// its answers readable.
func newDemoRouter(t *testing.T) http.Handler {
	t.Helper()

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return server.NewRouter(server.Config{
		DB:       demo.NewInMemoryQuerier(),
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})
}

// walkAndClassify walks the given router and attaches the declared auth class
// from server.RouteAuthClasses to every route. Fail-closed in both directions:
// an empty walk, an empty table, or a route that resolves to no row aborts the
// test — there is no "unclassified" state the guard tolerates.
func walkAndClassify(t *testing.T, handler http.Handler) []routeEntry {
	t.Helper()

	mux, ok := handler.(*chi.Mux)
	if !ok {
		t.Fatal("router is not a *chi.Mux — cannot walk routes")
	}
	entries := walkAPIRoutes(mux)
	if len(entries) == 0 {
		t.Fatal("router enumeration yielded zero routes — cannot generate or verify an inventory")
	}
	if len(server.RouteAuthClasses) == 0 {
		t.Fatal("server.RouteAuthClasses is empty — no auth ground truth to check against")
	}
	for i := range entries {
		rc, ok := server.ResolveRouteAuthClass(entries[i].Path)
		if !ok {
			t.Fatalf("route %s resolves to no row in server.RouteAuthClasses — every registered route must declare its auth class next to its mount (issue #115)", entries[i].key())
		}
		entries[i].Auth = string(rc.Class)
	}
	return entries
}

// walkedRoutes returns the classified leaf routes of a freshly built router.
func walkedRoutes(t *testing.T) []routeEntry {
	t.Helper()
	return walkAndClassify(t, newDemoRouter(t))
}

// apiRoutePrefixes are the prefixes the inventory covers. They are listed
// explicitly instead of walking the whole tree because the router's last
// registration is `r.Mount("/", web.DistHandler())` — the embedded SPA, a
// catch-all that serves files rather than API endpoints and has no bounded list
// of paths to inventory. Anything added to the HTTP API surface outside these
// prefixes must be added here, which is a deliberate decision point rather than
// a silent omission.
var apiRoutePrefixes = []string{"/api/v1", "/events"}

// walkAPIRoutes returns every documented API route the router registers, across
// apiRoutePrefixes, sorted and de-duplicated.
func walkAPIRoutes(mux *chi.Mux) []routeEntry {
	seen := map[string]bool{}
	var out []routeEntry
	for _, prefix := range apiRoutePrefixes {
		for _, e := range walkRoutes(mux, prefix) {
			if seen[e.key()] {
				continue
			}
			seen[e.key()] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// ---------------------------------------------------------------------------
// ROUTE WALKER — walks the chi router tree.
// ---------------------------------------------------------------------------

var httpMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodPatch:   true,
	http.MethodDelete:  true,
	http.MethodConnect: true,
	http.MethodOptions: true,
	http.MethodTrace:   true,
}

// walkRoutes walks a chi router and returns every leaf route whose path starts
// with prefix, keeping only recognized HTTP methods (chi does not auto-register
// a HEAD counterpart for a GET). Patterns are normalised with
// normalizeChiPattern and sorted by path, then method.
func walkRoutes(mux *chi.Mux, prefix string) []routeEntry {
	var entries []routeEntry
	walkRouteList(mux.Routes(), "", "", prefix, &entries)

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Method < entries[j].Method
	})
	return entries
}

// walkRouteList recurses through the chi route tree. parentPattern is the
// concatenated raw mount patterns of the ancestors (used for the prefix check),
// docPattern is those same mounts with the wildcard segments removed (see
// normalizeChiPattern). The two are tracked separately so the prefix check
// still runs against the real router pattern while the emitted path stays
// readable.
//
// The middleware chain is NOT read here — a chi mount never reports it (see
// the note on routeEntry). The auth class is attached afterwards, from
// server.RouteAuthClasses, in walkAndClassify. The leftover comment fragment
// claiming a `groupAuth` value was carried down the walk was itself the drift
// issue #115 is about; it is deleted rather than reworded.
func walkRouteList(routes []chi.Route, parentPattern, docPattern, prefix string, entries *[]routeEntry) {
	for _, route := range routes {
		raw := parentPattern + route.Pattern
		doc := normalizeChiPattern(docPattern, route.Pattern)

		for method, handler := range route.Handlers {
			if handler == nil {
				continue
			}
			if !httpMethods[method] {
				continue
			}
			if !strings.HasPrefix(raw, prefix) {
				continue
			}
			*entries = append(*entries, routeEntry{
				Method: method,
				Path:   doc,
			})
		}
		if route.SubRoutes != nil {
			walkRouteList(route.SubRoutes.Routes(), raw, doc, prefix, entries)
		}
	}
}

// normalizeChiPattern rebuilds an addressable URL path from chi's mount
// patterns. chi reports `Route("/contacts", ...)` and `Group(...)` mounts with a
// trailing `/*` wildcard segment, so the raw concatenation for a contact lookup
// is `/api/v1/*/contacts/*/{id}`. Those `/*` are mount boundaries with no
// routing meaning, and how many appear depends on nesting depth rather than on
// the addressable path — a route moved from a Group into a Route would change
// its inventory row without changing the API. Dropping them yields
// `/api/v1/contacts/{id}`, which is what the router actually answers and what a
// reader can match against the curl examples below.
//
// A mounted sub-route's own `"/"` (the list endpoint of `Route("/contacts", ...)`)
// renders as `/api/v1/contacts` without a trailing slash. That is not a
// simplification: against the real router both `/api/v1/contacts` and
// `/api/v1/contacts/` dispatch to the same handler, and `/api/v1/contacts` is
// the form the reference document uses. Direct leaf routes keep their exact
// pattern, where a trailing slash really is a different (unregistered) path.
//
// Fail-closed: a `*` in any position other than the end of a mount pattern is a
// real catch-all wildcard (the router would match an unbounded number of paths)
// and is rejected with a panic rather than silently stripped, so this
// normalisation can never mask a wildcard registration.
func normalizeChiPattern(parent, pattern string) string {
	segs := strings.Split(strings.Trim(pattern, "/"), "/")
	for i, s := range segs {
		if s == "*" && i != len(segs)-1 {
			panic(fmt.Sprintf("server: unexpected chi catch-all wildcard at segment %d of %q — the route inventory guard cannot render a bounded path", i, pattern))
		}
	}
	// chi ends every mount pattern with "/*"; that wildcard is the mount itself.
	for len(segs) > 0 && segs[len(segs)-1] == "*" {
		segs = segs[:len(segs)-1]
	}
	out := parent
	if child := strings.Join(segs, "/"); child != "" && child != "/" {
		out += "/" + child
	}
	if out == "" {
		return "/"
	}
	return out
}

// ---------------------------------------------------------------------------
// INVENTORY PARSER — extracts the Routen-Inventur table.
// ---------------------------------------------------------------------------

// parseInventoryFromContent extracts route entries from the Routen-Inventur
// markdown table in content. Returns an error when the heading is absent, the
// table cannot be found, or the table has zero data rows.
func parseInventoryFromContent(content, sourceName string) ([]routeEntry, error) {
	lines := strings.Split(content, "\n")

	// Find the inventory heading.
	headingIdx := -1
	for i, line := range lines {
		if isMarkdownHeading(line) && strings.Contains(line, inventoryHeading) {
			headingIdx = i
			break
		}
	}
	if headingIdx < 0 {
		return nil, fmt.Errorf("inventory heading %q not found in %s", inventoryHeading, sourceName)
	}

	// Find the start marker after the heading.
	startIdx := -1
	for i := headingIdx; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == inventoryStartMarker {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return nil, fmt.Errorf("inventory start marker %q not found after heading in %s", inventoryStartMarker, sourceName)
	}

	// Find the table separator after the start marker.
	sepIdx := -1
	for i := startIdx + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if isTableSeparator(line) {
			sepIdx = i
			break
		}
		if line == inventoryEndMarker {
			return nil, fmt.Errorf("inventory end marker %q reached before table separator in %s", inventoryEndMarker, sourceName)
		}
	}
	if sepIdx < 0 {
		return nil, fmt.Errorf("no table separator found after start marker in %s", sourceName)
	}

	// Parse table rows until the end marker or blank line.
	var entries []routeEntry
	for i := sepIdx + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == inventoryEndMarker || line == "" {
			break
		}
		if !strings.HasPrefix(line, "|") {
			break
		}
		entry, err := parseTableRow(line)
		if err != nil {
			return nil, fmt.Errorf("parse table row %q: %w", line, err)
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("inventory table has zero data rows in %s", sourceName)
	}

	return entries, nil
}

func isMarkdownHeading(line string) bool {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	return i > 0 && i < len(line) && line[i] == ' '
}

func isTableSeparator(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "|") || !strings.HasSuffix(t, "|") {
		return false
	}
	inner := t[1 : len(t)-1]
	cols := strings.Split(inner, "|")
	if len(cols) < 2 {
		return false
	}
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c == "" {
			return false
		}
		for _, r := range c {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

// parseTableRow parses one inventory row. Exactly three columns are required:
// the auth class became mandatory with issue #115, and a row without one is a
// parse failure, not a tolerated old shape — a guard that accepts "no auth
// cell" could be neutered by simply deleting the column.
func parseTableRow(line string) (routeEntry, error) {
	cols := splitTableRow(line)
	if len(cols) != 3 {
		return routeEntry{}, fmt.Errorf("expected exactly 3 columns (method, path, auth class), got %d — regenerate the table with UPDATE_ROUTE_INVENTORY=1", len(cols))
	}
	auth := strings.TrimSpace(cols[2])
	if auth == "" {
		return routeEntry{}, fmt.Errorf("empty auth class cell in %q — the class is mandatory (fail-closed, issue #115)", line)
	}
	return routeEntry{
		Method: strings.TrimSpace(cols[0]),
		Path:   strings.TrimSpace(cols[1]),
		Auth:   auth,
	}, nil
}

func splitTableRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	var cells []string
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

// ---------------------------------------------------------------------------
// CURL EXAMPLE SWEEP — every curl call in the prose must be a real route
// ---------------------------------------------------------------------------

// curlMethodRe extracts the explicit method from a curl line.
var curlMethodRe = regexp.MustCompile(`(?:-X|--request)\s+(\S+)`)

// curlURLRe extracts the request URL. The host part is intentionally loose: the
// document spells the base as localhost:8080 today, and a future example that
// uses $DOMAIN or a placeholder host still has to name a real path.
var curlURLRe = regexp.MustCompile(`(https?://[^\s"']+|/(?:api|auth|admin|webhook)/[^\s"']+)`)

// fencedBlockBodies returns the text of every ``` fenced block, one entry per
// block, and additionally the 1-based line number each body starts on (for
// error messages).
//
// This replaces a regex that matched "```bash\n(.*?)```" across the document.
// That regex could not work: its opening alternative only accepted bash|sh|shell
// or nothing, so a ```json or ```text block was never consumed as a block, and
// the scanner then paired that block's *closing* fence with the *next* opening
// fence. The result was that roughly every other block — including most of the
// curl examples — was skipped: measured against this document it inspected only
// 2 of 41 curl lines, which is how the guard could sit on top of a phantom
// example for weeks and stay green. Fences are now matched line-wise, the way
// CommonMark defines them: an opening line starting with ``` (optionally with an
// info string) and a closing line that is a bare ```.
func fencedBlockBodies(content string) []blockBody {
	var out []blockBody
	lines := strings.Split(content, "\n")
	var body []string
	start := 0
	inFence := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !inFence {
			if strings.HasPrefix(t, "```") {
				inFence = true
				start = i + 2 // body begins on the line after the opening fence
				body = nil
			}
			continue
		}
		if t == "```" {
			out = append(out, blockBody{start: start, text: strings.Join(body, "\n")})
			inFence = false
			continue
		}
		body = append(body, line)
	}
	if inFence {
		out = append(out, blockBody{start: start, text: strings.Join(body, "\n")})
	}
	return out
}

type blockBody struct {
	start int // 1-based line number of the first body line
	text  string
}

// sweepCurlExamples extracts every curl invocation from the markdown document,
// parses the URL and method, and reports any example that names a path or
// method the router does not register. Matching is segment-wise: a {...} segment
// on either side matches any single concrete segment, so the document can write
// a realistic example ID (app-1) or a placeholder (<contact_id>) where the
// router registers {id}.
func sweepCurlExamples(docContent string, registered []routeEntry) []string {
	var issues []string
	for _, block := range fencedBlockBodies(docContent) {
		for _, cmd := range curlCommands(block) {
			url, method := parseCurlLine(cmd.text)
			if url == "" {
				continue
			}
			path := extractURLPath(url)
			if path == "" {
				continue
			}
			if method == "" {
				// curl's default is GET; the document spells -X GET explicitly
				// everywhere, so an unprefixed line is a bare "curl <url>".
				method = http.MethodGet
			}
			if !matchesAnyPath(method, path, registered) {
				issues = append(issues, fmt.Sprintf("line %d: curl example %s %s — no route the router registers matches (check the method, the path, or a stale segment count)", cmd.line, method, path))
			}
		}
	}
	return issues
}

// curlCommand is one complete curl invocation as written in the document,
// possibly spread over several physical lines.
type curlCommand struct {
	line int    // 1-based line of the "curl" keyword, for the error message
	text string // the invocation with its backslash continuations joined
}

// curlCommands collects the curl invocations in a fenced block, following
// backslash continuations. This matters: the reference document writes nearly
// every example multi-line, and the URL is not always on the first line — the
// SSE example keeps it last. Reading only the line that starts with "curl"
// would leave such an example unchecked no matter how the block is delimited,
// so the whole invocation is flattened before it is parsed.
func curlCommands(block blockBody) []curlCommand {
	lines := strings.Split(block.text, "\n")
	var out []curlCommand
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "curl") {
			continue
		}
		start := block.start + i
		parts := []string{strings.TrimSuffix(trimmed, "\\")}
		for strings.HasSuffix(trimmed, "\\") && i+1 < len(lines) {
			i++
			trimmed = strings.TrimSpace(lines[i])
			parts = append(parts, strings.TrimSuffix(trimmed, "\\"))
		}
		out = append(out, curlCommand{line: start, text: strings.Join(parts, " ")})
	}
	return out
}

// parseCurlLine extracts the URL and explicit HTTP method from a curl command
// line. Supports -X METHOD and --request METHOD forms. Backslash continuations
// are handled by the caller passing in a joined line.
func parseCurlLine(line string) (url, method string) {
	// Remove backslash continuations (the caller passes the full block text
	// line by line; multi-line curls with backslashes are already flattened).
	// Extract explicit method: -X METHOD or --request METHOD.
	if m := curlMethodRe.FindStringSubmatch(line); m != nil {
		method = strings.ToUpper(m[1])
	}

	// Extract URL: the first token that looks like a URL or an API path.
	if m := curlURLRe.FindStringSubmatch(line); m != nil {
		url = m[1]
	}
	return url, method
}

// extractURLPath strips scheme, host, port, and any $VARIABLE-style prefix,
// returning the path portion (including leading /).
func extractURLPath(rawURL string) string {
	// Remove scheme
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		rest := rawURL[idx+3:]
		// Remove host[:port]
		if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
			rest = rest[slashIdx:]
			// Remove query string.
			if qIdx := strings.Index(rest, "?"); qIdx >= 0 {
				rest = rest[:qIdx]
			}
			return rest
		}
		return ""
	}
	// Raw path: remove query string.
	if qIdx := strings.Index(rawURL, "?"); qIdx >= 0 {
		return rawURL[:qIdx]
	}
	return rawURL
}

// matchesAnyPath checks whether (method, path) matches any registered route,
// using segment-wise matching where {...} on either side acts as a wildcard.
func matchesAnyPath(method, path string, registered []routeEntry) bool {
	for _, e := range registered {
		if e.Method != method {
			continue
		}
		if matchSegments(e.Path, path) {
			return true
		}
	}
	return false
}

// matchSegments compares two URL paths segment by segment. A segment of the
// form {name} on either side is treated as a wildcard matching any
// non-empty segment.
func matchSegments(registeredPath, docPath string) bool {
	regSegs := splitPath(registeredPath)
	docSegs := splitPath(docPath)
	if len(regSegs) != len(docSegs) {
		return false
	}
	for i := range regSegs {
		if isWildcard(regSegs[i]) || isWildcard(docSegs[i]) {
			continue
		}
		if regSegs[i] != docSegs[i] {
			return false
		}
	}
	return true
}

func splitPath(p string) []string {
	if p == "" || p == "/" {
		return nil
	}
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// isWildcard reports whether a path segment stands for "any single segment".
//
// Two spellings count, and the difference is deliberate rather than sloppy: the
// inventory table is generated from the router, so it always uses chi's own
// {id}; the prose curl examples below it are written for a human reading along
// and use the <contact_id> convention that predates this guard (11 places).
// Both denote one concrete path segment, so both match a registered {id}. A
// literal example ID such as app-1 matches on the same rule, which is why
// /api/v1/appointments/app-1/ics is a valid example for {id}/ics.
// The generated table itself stays strict about the spelling — see the row
// validation in step 5 of the guard.
func isWildcard(seg string) bool {
	if len(seg) < 2 {
		return false
	}
	switch {
	case seg[0] == '{' && seg[len(seg)-1] == '}':
		return true
	case seg[0] == '<' && seg[len(seg)-1] == '>':
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// COMPARISON ENGINE — bidirectional set comparison.
// ---------------------------------------------------------------------------

// compareRoutes returns (missing, phantom):
//
//	missing: routes registered by the router but not in the inventory.
//	phantom: routes in the inventory that the router does not register.
func compareRoutes(registered, documented []routeEntry) (missing, phantom []routeEntry) {
	regSet := make(map[string]routeEntry, len(registered))
	for _, e := range registered {
		regSet[e.key()] = e
	}
	docSet := make(map[string]routeEntry, len(documented))
	for _, e := range documented {
		docSet[e.key()] = e
	}

	for k, e := range regSet {
		if _, ok := docSet[k]; !ok {
			missing = append(missing, e)
		}
	}
	for k, e := range docSet {
		if _, ok := regSet[k]; !ok {
			phantom = append(phantom, e)
		}
	}

	sort.Slice(missing, func(i, j int) bool {
		if missing[i].Path != missing[j].Path {
			return missing[i].Path < missing[j].Path
		}
		return missing[i].Method < missing[j].Method
	})
	sort.Slice(phantom, func(i, j int) bool {
		if phantom[i].Path != phantom[j].Path {
			return phantom[i].Path < phantom[j].Path
		}
		return phantom[i].Method < phantom[j].Method
	})
	return missing, phantom
}

// compareAuthClasses reports rows both sides agree exist (same method+path)
// but disagree about how a caller authenticates against them. Routes that are
// missing on one side are deliberately skipped — compareRoutes owns that
// verdict, and reporting an "auth class drift" for a phantom row would add
// noise on top of the real error.
func compareAuthClasses(registered, documented []routeEntry) []string {
	reg := make(map[string]routeEntry, len(registered))
	for _, e := range registered {
		reg[e.key()] = e
	}
	var issues []string
	for _, d := range documented {
		r, ok := reg[d.key()]
		if !ok {
			continue
		}
		if d.Auth != r.Auth {
			issues = append(issues, fmt.Sprintf("%s: document says %q, router auth-class table (internal/server/route_auth.go) says %q", d.key(), d.Auth, r.Auth))
		}
	}
	sort.Strings(issues)
	return issues
}
