package server_test

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openlocalcrm/openlocalcrm/internal/db/demo"
	"github.com/openlocalcrm/openlocalcrm/internal/server"
	"github.com/openlocalcrm/openlocalcrm/internal/sse"
)

// routeEntry is one row in the route inventory.
type routeEntry struct {
	Method    string
	Path      string
	AuthGroup string
}

// key returns a stable sort/compare key for the entry.
func (e routeEntry) key() string { return e.Method + " " + e.Path }

// ---------------------------------------------------------------------------
// AC2-inventory-one-row-per-registered-route + AC3-guard-green (happy path)
// AC4-guard-fails-on-undocumented-registered-route (bidirectional check)
// AC5-guard-fails-on-phantom-documented-route (bidirectional check)
// AC6-guard-fails-closed-never-vacuous (self-tests below)
// AC7-red-without-the-fix (verified by the verifier's without-the-fix run)
// ---------------------------------------------------------------------------

// TestRouteInventoryMatchesRouter is the main drift guard.  It constructs the
// router the package itself builds, walks every leaf route, parses the route
// inventory from docs/API_REFERENCE.md, and compares them bidirectionally.
//
// Criterion: AC2 — one row per registered route, no duplicates, row count
// equals walked-route count; AC3 — runs inside `go test ./...`, zero new
// dependencies; AC4/AC5 — the comparison fails when a route is missing in
// either direction.
func TestRouteInventoryMatchesRouter(t *testing.T) {
	// 1. Construct the router with the demo querier so every handler is
	//    registered.  This is the same pattern used by every existing router
	//    test (router_test.go).
	querier := demo.NewInMemoryQuerier()
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	r := server.NewRouter(server.Config{
		DB:       querier,
		SSEHub:   sse.NewHub(),
		PubKey:   pubKey,
		PrivKey:  privKey,
		DemoMode: true,
	})

	// 2. Walk every leaf route registered under /api/v1.
	mux, ok := r.(*chi.Mux)
	if !ok {
		t.Fatal("router is not a *chi.Mux — cannot walk routes")
	}
	registered := walkRoutes(mux, "/api/v1")
	n := len(registered)
	t.Logf("walked %d leaf routes from router", n)

	// AC6 — a zero-route walk must fail, printed in verbose output.
	if n == 0 {
		t.Fatal("router enumeration yielded zero leaf routes — cannot guard against drift on an empty route set")
	}

	// 3. Parse the inventory from the reference document.
	docPath := filepath.Join("..", "..", "docs", "API_REFERENCE.md")
	documented, err := parseInventory(docPath)
	if err != nil {
		// AC6 — unreadable file or absent heading/section.
		t.Fatalf("cannot read route inventory from %s: %v", docPath, err)
	}
	t.Logf("parsed %d routes from inventory in %s", len(documented), docPath)

	// AC6 — zero-row parse must fail.
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

	// 5. Validate every inventory row: method, path, auth group.
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
		if e.AuthGroup == "" || e.AuthGroup == "TBD" {
			t.Errorf("inventory row %s has empty or placeholder auth group %q", e.key(), e.AuthGroup)
		}
	}

	// 6. Bidirectional comparison.
	missing, phantom := compareRoutes(registered, documented)

	// AC4 — a registered route missing from the inventory must fail the test.
	for _, m := range missing {
		t.Errorf("registered route not in inventory: %s", m.key())
	}

	// AC5 — a documented route the router does not register must fail the test.
	for _, p := range phantom {
		t.Errorf("inventory route not registered by router: %s", p.key())
	}
}

// ---------------------------------------------------------------------------
// Self-tests: the guard must prove it catches drift.
// ---------------------------------------------------------------------------

// TestGuardFailsOnMissingInventory — AC6: the guard must fail when the
// reference file cannot be read (unreadable path) or the inventory
// heading/section is absent.
func TestGuardFailsOnMissingInventory(t *testing.T) {
	// Unreadable file
	_, err := parseInventory(filepath.Join("..", "..", "docs", "NONEXISTENT.md"))
	if err == nil {
		t.Error("guard did not fail when the reference file is unreadable — expected error, got nil")
	} else {
		t.Logf("correctly failed on unreadable file: %v", err)
	}

	// File exists but no inventory heading — create a temp copy without the
	// inventory heading and verify parseInventory fails.
	docPath := filepath.Join("..", "..", "docs", "API_REFERENCE.md")
	content, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("cannot read reference doc for self-test: %v", err)
	}
	// Strip any inventory section that may or may not exist.
	stripped := stripInventorySection(string(content))
	tmpFile, err := os.CreateTemp("", "api-reference-guard-test-*.md")
	if err != nil {
		t.Fatalf("cannot create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(stripped); err != nil {
		t.Fatalf("cannot write temp file: %v", err)
	}
	tmpFile.Close()

	_, err = parseInventory(tmpFile.Name())
	if err == nil {
		t.Error("guard did not fail when the inventory heading/section is absent — expected error, got nil")
	} else {
		t.Logf("correctly failed on absent inventory heading: %v", err)
	}
}

// TestGuardFailsOnUndocumentedRoute — AC4: the guard's comparison must fail
// when a registered route is removed from the inventory, naming the missing
// route's method and path.
func TestGuardFailsOnUndocumentedRoute(t *testing.T) {
	// Build a minimal set: two "registered" routes, one "documented".
	registered := []routeEntry{
		{Method: "GET", Path: "/api/v1/health", AuthGroup: "public"},
		{Method: "POST", Path: "/api/v1/auth/login", AuthGroup: "public"},
	}
	documented := []routeEntry{
		{Method: "GET", Path: "/api/v1/health", AuthGroup: "public"},
	}

	missing, phantom := compareRoutes(registered, documented)

	if len(missing) != 1 {
		t.Fatalf("expected 1 missing route, got %d", len(missing))
	}
	if missing[0].Method != "POST" || missing[0].Path != "/api/v1/auth/login" {
		t.Errorf("wrong missing route reported: %s (expected POST /api/v1/auth/login)", missing[0].key())
	}
	if len(phantom) != 0 {
		t.Errorf("expected 0 phantom routes, got %d", len(phantom))
	}
}

// TestGuardFailsOnPhantomRoute — AC5: the guard's comparison must fail when
// the inventory contains a route the router does not register, naming the
// unregistered method and path.  The canonical phantom is the historical
// POST /api/v1/admin/ownership/bulk-assign.
func TestGuardFailsOnPhantomRoute(t *testing.T) {
	registered := []routeEntry{
		{Method: "GET", Path: "/api/v1/health", AuthGroup: "public"},
		{Method: "POST", Path: "/api/v1/ownership/bulk-assign", AuthGroup: "admin"},
	}
	documented := []routeEntry{
		{Method: "GET", Path: "/api/v1/health", AuthGroup: "public"},
		{Method: "POST", Path: "/api/v1/ownership/bulk-assign", AuthGroup: "admin"},
		// The phantom route that the router does not register.
		{Method: "POST", Path: "/api/v1/admin/ownership/bulk-assign", AuthGroup: "admin"},
	}

	missing, phantom := compareRoutes(registered, documented)

	if len(phantom) != 1 {
		t.Fatalf("expected 1 phantom route, got %d", len(phantom))
	}
	if phantom[0].Method != "POST" || phantom[0].Path != "/api/v1/admin/ownership/bulk-assign" {
		t.Errorf("wrong phantom route reported: %s (expected POST /api/v1/admin/ownership/bulk-assign)", phantom[0].key())
	}
	if len(missing) != 0 {
		t.Errorf("expected 0 missing routes, got %d", len(missing))
	}
}

// TestGuardFailsOnZeroInventory — AC6: the guard must fail when the inventory
// parse yields zero rows.
func TestGuardFailsOnZeroInventory(t *testing.T) {
	registered := []routeEntry{
		{Method: "GET", Path: "/api/v1/health", AuthGroup: "public"},
	}
	documented := []routeEntry{}

	missing, phantom := compareRoutes(registered, documented)

	// When there are zero documented rows, every registered route is missing.
	if len(missing) != len(registered) {
		t.Errorf("expected all %d registered routes reported missing, got %d", len(registered), len(missing))
	}
	if len(phantom) != 0 {
		t.Errorf("expected 0 phantom routes with empty inventory, got %d", len(phantom))
	}
}

// TestGuardFailsOnZeroRegistered — AC6: the comparison must fail when the
// router enumeration yields zero routes (checked by the main test at runtime,
// but the comparison logic itself must not pass vacuously).
func TestGuardFailsOnZeroRegistered(t *testing.T) {
	registered := []routeEntry{}
	documented := []routeEntry{
		{Method: "GET", Path: "/api/v1/health", AuthGroup: "public"},
	}

	missing, phantom := compareRoutes(registered, documented)

	if len(phantom) != len(documented) {
		t.Errorf("expected all %d documented routes reported as phantom, got %d", len(documented), len(phantom))
	}
	if len(missing) != 0 {
		t.Errorf("expected 0 missing routes with empty registered set, got %d", len(missing))
	}
}

// ---------------------------------------------------------------------------
// Route walker — enumerates leaf routes from the chi router.
// ---------------------------------------------------------------------------

// isHTTPMethod returns true for the standard HTTP methods that chi registers.
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

// walkRoutes walks a chi router and returns every leaf route whose path
// starts with prefix.  Only routes with a recognised HTTP method are kept;
// mounted sub-routers (method=="") and internal chi routes (MethodNotAllowed,
// HEAD auto-generation) are excluded.
func walkRoutes(mux *chi.Mux, prefix string) []routeEntry {
	var entries []routeEntry
	walkRouteList(mux.Routes(), "", prefix, &entries)

	// Sort for deterministic output and comparison.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Method < entries[j].Method
	})
	return entries
}

// walkRouteList recursively walks a []chi.Route tree, building full paths.
func walkRouteList(routes []chi.Route, parentPattern string, prefix string, entries *[]routeEntry) {
	for _, route := range routes {
		pattern := parentPattern + route.Pattern
		// Collect leaf route handlers: each (method, handler) at this node.
		for method, handler := range route.Handlers {
			if handler == nil {
				continue
			}
			if !httpMethods[method] {
				continue
			}
			if !strings.HasPrefix(pattern, prefix) {
				continue
			}
			*entries = append(*entries, routeEntry{
				Method: method,
				Path:   pattern,
			})
		}
		// Recurse into sub-routes.
		if route.SubRoutes != nil {
			walkRouteList(route.SubRoutes.Routes(), pattern, prefix, entries)
		}
	}
}

// ---------------------------------------------------------------------------
// Inventory parser — extracts the route table from docs/API_REFERENCE.md.
// ---------------------------------------------------------------------------

// inventoryHeading is the stable named heading that marks the route inventory
// table.  The parser looks for a markdown heading (##, ###, etc.) containing
// this exact substring.  The implementer must place the table immediately
// after this heading.
const inventoryHeading = "Route Inventory"

// parseInventory reads path and extracts route entries from a markdown table
// under the inventory heading.  Returns an error when the file cannot be
// read, the heading is absent, or the table cannot be parsed.
func parseInventory(path string) ([]routeEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// Find the inventory heading.
	inSection := false
	for scanner.Scan() {
		line := scanner.Text()
		if isMarkdownHeading(line) && strings.Contains(line, inventoryHeading) {
			inSection = true
			continue
		}
		if inSection {
			// Skip blank lines before the table.
			if strings.TrimSpace(line) == "" {
				continue
			}
			// Expect the separator line of a markdown table: |---|---|...
			if isTableSeparator(line) {
				break
			}
			// If we hit a non-blank, non-separator line after the heading that
			// is not a separator, the heading is present but no table follows.
			return nil, fmt.Errorf("inventory heading %q found but no table separator follows", inventoryHeading)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	if !inSection {
		return nil, fmt.Errorf("inventory heading %q not found in %s", inventoryHeading, path)
	}

	// Parse table rows.
	var entries []routeEntry
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break // end of table
		}
		if !strings.HasPrefix(line, "|") {
			break // end of table
		}
		entry, err := parseTableRow(line)
		if err != nil {
			return nil, fmt.Errorf("parse table row %q: %w", line, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("inventory heading %q found but table contains zero rows", inventoryHeading)
	}

	return entries, nil
}

// isMarkdownHeading returns true when line starts with one or more '#' followed by a space.
func isMarkdownHeading(line string) bool {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	return i > 0 && i < len(line) && line[i] == ' '
}

// isTableSeparator returns true when line looks like a markdown table
// separator: pipes separated by dashes (and optional colons).
func isTableSeparator(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "|") || !strings.HasSuffix(t, "|") {
		return false
	}
	inner := t[1 : len(t)-1]
	cols := strings.Split(inner, "|")
	if len(cols) < 3 {
		return false
	}
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c == "" {
			return false
		}
		for _, r := range c {
			if r != '-' && r != ':' {
				return false
			}
		}
	}
	return true
}

// parseTableRow parses a single markdown table row into a routeEntry.
// Expected format: | METHOD | /path | auth-group |
func parseTableRow(line string) (routeEntry, error) {
	cols := splitTableRow(line)
	if len(cols) < 3 {
		return routeEntry{}, fmt.Errorf("expected at least 3 columns, got %d", len(cols))
	}
	return routeEntry{
		Method:    strings.TrimSpace(cols[0]),
		Path:      strings.TrimSpace(cols[1]),
		AuthGroup: strings.TrimSpace(cols[2]),
	}, nil
}

// splitTableRow splits a markdown table row by '|' and returns non-empty cells.
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

// stripInventorySection removes the inventory heading and everything after it
// from the document content, used by the self-test that verifies the guard
// fails when the heading is absent.
func stripInventorySection(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	for _, line := range lines {
		if isMarkdownHeading(line) && strings.Contains(line, inventoryHeading) {
			break
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// Comparison engine — bidirectional set comparison.
// ---------------------------------------------------------------------------

// compareRoutes returns (missing, phantom):
//   - missing:  routes registered by the router but not in the inventory.
//   - phantom:  routes in the inventory that the router does not register.
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
