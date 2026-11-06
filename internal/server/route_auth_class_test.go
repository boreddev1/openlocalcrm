package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/server"
)

// ---------------------------------------------------------------------------
// The behavioral half of issue #115.
//
// api_reference_test.go binds docs/API_REFERENCE.md to server.RouteAuthClasses
// and binds that table to the router walk. That still leaves the table itself
// assertible-but-unproven: it could say "jwt+csrf" while the protected group
// has no AuthMiddleware in front of it any more. This file probes the built
// router with one *unauthenticated* request per route — in-process httptest,
// no server, no DB — and demands that the answer matches the declared class.
//
// Why this does not repeat the blanket-probe mistake rejected in #113:
//
//   - It never authenticates. The CSRF middleware exempts requests that
//     authenticated via a verified Bearer token, so "does this route require
//     CSRF" genuinely is not a property a probe can read — but "what answers
//     WITHOUT any credential" is deterministic, and that is all that is asked.
//   - 401-class routes are rejected by auth.AuthMiddleware at the head of the
//     protected chain, before CSRF, before RBAC, before every sub-group rate
//     limiter, and before any handler logic — so the limiter that would answer
//     429 under load is never consulted for them. The few routes the probe
//     does reach the limiters of (login, accept-invite) get one request each
//     against a freshly built router (limiters are per-router), far below
//     their budgets, so no 429 can masquerade as a classification.
//   - The connector webhook and the SSE stream answer 401 from their own
//     checks, not from AuthMiddleware. The probe therefore proves only
//     "unauthenticated callers are rejected" for those two rows; the name on
//     the row (connector-token vs jwt-inline vs jwt+csrf) rests on the table
//     and the anchor comments at the mounts. Status codes cannot tell those
//     apart, and this guard pretends otherwise nowhere.
//
// The exclusion list, so the boundary of the proof is explicit: no probe ever
// distinguishes *which* guard answered 401 on the connector/SSE rows, none
// tests RBAC permissions or token_version semantics (covered by
// rbac_routes_test.go and internal/auth), and none authenticates.
// ---------------------------------------------------------------------------

// unauthenticatedExpectation maps a declared class to what an unauthenticated
// request must see. "open" means: any answer except 401/403/429 — 401 or 403
// would prove a credential gate sits in front of a route declared credential-
// free, and 429 means a limiter fired, which the probe treats as unreadable
// rather than as a classification. Returning ok=false for an unknown class
// keeps this fail-closed: a new AuthClass constant must be given an
// expectation here *deliberately*, it cannot slip through unprobed.
func unauthenticatedExpectation(class server.AuthClass) (want string, ok bool) {
	switch class {
	case server.AuthNone:
		return "open", true
	case server.AuthCSRFOnly:
		// CSRFDoubleSubmitMiddleware is the first middleware on these mounts
		// and answers 403 on the missing CSRF cookie before any rate limiter
		// or handler runs — the answer is deterministic.
		return "403", true
	case server.AuthJWTCSRF, server.AuthConnectorToken, server.AuthJWTInline:
		// jwt+csrf: auth.AuthMiddleware answers 401 ("missing authentication
		// token"). connector-token and jwt-inline: their own check answers 401.
		// Same observable, different mechanism — see the note above.
		return "401", true
	default:
		return "", false
	}
}

// concreteProbePath replaces chi {param} segments with one concrete value.
// For every 401/403-class route the middleware chain short-circuits before
// pattern parameters matter, and the none-class routes have no parameters.
func concreteProbePath(pattern string) string {
	for strings.Contains(pattern, "{") {
		start := strings.Index(pattern, "{")
		end := strings.Index(pattern, "}")
		if end < start {
			break
		}
		pattern = pattern[:start] + "probe-value" + pattern[end+1:]
	}
	return pattern
}

// probeUnauthenticated fires one cookie-less, header-less request per route
// against the given handler and returns every answer that contradicts the
// route's declared class. The caller decides how to fail; this stays pure so
// the sensitivity drill below can feed it deliberately wrong tables.
func probeUnauthenticated(entries []routeEntry, handler http.Handler) []string {
	var violations []string
	for _, e := range entries {
		want, ok := unauthenticatedExpectation(server.AuthClass(e.Auth))
		if !ok {
			violations = append(violations, fmt.Sprintf("%s: auth class %q has no probe expectation — refusing to guess; extend unauthenticatedExpectation deliberately (fail-closed)", e.key(), e.Auth))
			continue
		}
		req := httptest.NewRequest(e.Method, concreteProbePath(e.Path), nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		code := rec.Code

		switch want {
		case "open":
			if code == http.StatusUnauthorized || code == http.StatusForbidden || code == http.StatusTooManyRequests {
				violations = append(violations, fmt.Sprintf("%s: class %q promises no credential gate, but an unauthenticated request got %d", e.key(), e.Auth, code))
			}
		case "401":
			if code != http.StatusUnauthorized {
				violations = append(violations, fmt.Sprintf("%s: class %q promises rejection of unauthenticated callers with 401, but got %d", e.key(), e.Auth, code))
			}
		case "403":
			if code != http.StatusForbidden {
				violations = append(violations, fmt.Sprintf("%s: class %q promises the CSRF double-submit gate (403 before limiter and handler), but got %d", e.key(), e.Auth, code))
			}
		}
	}
	return violations
}

// TestUnauthenticatedBehaviorMatchesDeclaredAuthClass is the live probe
// against the real router. It complements the doc guard: there, the document
// must agree with the table; here, the router's behavior must agree with the
// table. Both directions failing shut is what makes the auth column of
// docs/API_REFERENCE.md something other than prose.
func TestUnauthenticatedBehaviorMatchesDeclaredAuthClass(t *testing.T) {
	handler := newDemoRouter(t)
	entries := walkAndClassify(t, handler)

	violations := probeUnauthenticated(entries, handler)
	for _, v := range violations {
		t.Error(v)
	}
	t.Logf("probed %d routes unauthenticated; all answers matched the declared class", len(entries))
}

// TestProbeDetectsFlippedAuthClasses is the non-vacuity drill for the probe.
// Three flips against the REAL walked routes, each on a freshly built router
// so limiter budgets stay untouched:
//
//	a. a genuinely protected route labeled "none" — the probe expects an open
//	   answer and gets 401, exactly the mislabel that would tell an integrator
//	   (or an auditor) that an endpoint is public when it is not;
//	b. the health route labeled "jwt+csrf" — the probe expects 401 and gets
//	   200, catching a route claimed to be protected that is not;
//	c. /auth/refresh labeled "none" — it really answers 403 (CSRF gate), so
//	   this flip is caught by the gate's status, proving the probe reads the
//	   class split and not just "401 means protected".
//
// It also asserts an unclassifiable label is reported instead of skipped.
func TestProbeDetectsFlippedAuthClasses(t *testing.T) {
	flip := func(entries []routeEntry, method, path, to string) []routeEntry {
		out := make([]routeEntry, len(entries))
		copy(out, entries)
		for i := range out {
			if out[i].Method == method && out[i].Path == path {
				out[i].Auth = to
				return out
			}
		}
		t.Fatalf("fixture route %s %s not found in the walked inventory", method, path)
		return nil
	}

	cases := []struct {
		name             string
		method, path, to string
	}{
		{"protected-as-public", "GET", "/api/v1/contacts", "none"},
		{"public-as-protected", "GET", "/api/v1/health", "jwt+csrf"},
		{"csrf-gate-as-public", "POST", "/api/v1/auth/refresh", "none"},
	}
	for _, c := range cases {
		entries := flip(walkedRoutes(t), c.method, c.path, c.to)
		violations := probeUnauthenticated(entries, newDemoRouter(t))
		if len(violations) != 1 || !strings.Contains(violations[0], c.method+" "+c.path) {
			t.Errorf("%s: flipping %s %s to %q produced %d violations, want exactly 1: %v", c.name, c.method, c.path, c.to, len(violations), violations)
		} else {
			t.Logf("%s correctly detected: %s", c.name, violations[0])
		}
	}

	unknown := flip(walkedRoutes(t), "GET", "/api/v1/health", "banana")
	violations := probeUnauthenticated(unknown, newDemoRouter(t))
	if len(violations) != 1 || !strings.Contains(violations[0], "banana") {
		t.Errorf("an unclassifiable label was not reported as a violation: %v", violations)
	} else {
		t.Logf("unknown class refused: %s", violations[0])
	}
}

// TestRouteAuthClassTableIntegrity checks the table before anything consults
// it: no duplicate prefixes (longest-match would hide one), well-formed
// prefixes, every declared class actually used (a dead constant is a claim
// about a mount nobody has), and — the security property — the broadest row
// for the API tree is the strict session class. The default of the API must
// stay "requires a session", so an endpoint nobody classified falls into the
// protected class and the probe above fails the moment someone tries to call
// that default "none".
func TestRouteAuthClassTableIntegrity(t *testing.T) {
	if len(server.RouteAuthClasses) == 0 {
		t.Fatal("server.RouteAuthClasses is empty — the auth ground truth is missing")
	}

	seen := map[string]bool{}
	usedClasses := map[server.AuthClass]bool{}
	for _, row := range server.RouteAuthClasses {
		if seen[row.Prefix] {
			t.Errorf("duplicate prefix %q in RouteAuthClasses — longest-match would silently pick one", row.Prefix)
		}
		seen[row.Prefix] = true
		usedClasses[row.Class] = true

		if !strings.HasPrefix(row.Prefix, "/") {
			t.Errorf("prefix %q does not start with /", row.Prefix)
		}
		if strings.HasSuffix(row.Prefix, "/") || strings.Contains(row.Prefix, "*") {
			t.Errorf("prefix %q must not end in / or contain a wildcard — it would match unpredictably", row.Prefix)
		}
		if row.Class == "" {
			t.Errorf("row %q has an empty auth class", row.Prefix)
		}
	}

	for _, class := range []server.AuthClass{
		server.AuthNone, server.AuthCSRFOnly, server.AuthJWTCSRF,
		server.AuthConnectorToken, server.AuthJWTInline,
	} {
		if !usedClasses[class] {
			t.Errorf("declared auth class %q is not used by any RouteAuthClasses row", class)
		}
	}

	defaultRow, ok := server.ResolveRouteAuthClass("/api/v1/anything-unclassified")
	if !ok {
		t.Fatal("no RouteAuthClasses row covers an unclassified /api/v1 route")
	}
	if defaultRow.Class != server.AuthJWTCSRF {
		t.Errorf("the catch-all row for /api/v1 (%q) declares %q; the API default must stay %q so unclassified endpoints cannot become public by accident",
			defaultRow.Prefix, defaultRow.Class, server.AuthJWTCSRF)
	}
}

// TestResolveRouteAuthClassSegmentBoundaries pins the matching semantics the
// table relies on: "/api/v1/auth/logout" must NOT swallow "/api/v1/auth/logout-all"
// (which really sits inside the protected group), and the longest prefix wins.
func TestResolveRouteAuthClassSegmentBoundaries(t *testing.T) {
	cases := []struct {
		path      string
		wantClass server.AuthClass
		wantPref  string
	}{
		{"/api/v1/health", server.AuthNone, "/api/v1/health"},
		{"/api/v1/auth/logout", server.AuthCSRFOnly, "/api/v1/auth/logout"},
		{"/api/v1/auth/logout-all", server.AuthJWTCSRF, "/api/v1"},
		{"/api/v1/auth/login", server.AuthNone, "/api/v1/auth/login"},
		{"/api/v1/auth/refresh", server.AuthCSRFOnly, "/api/v1/auth/refresh"},
		{"/api/v1/auth/change-password", server.AuthJWTCSRF, "/api/v1"},
		{"/api/v1/contacts/123", server.AuthJWTCSRF, "/api/v1"},
		{"/api/v1/connectors/lead-intake", server.AuthConnectorToken, "/api/v1/connectors/lead-intake"},
		{"/events/stream", server.AuthJWTInline, "/events/stream"},
	}
	for _, c := range cases {
		row, ok := server.ResolveRouteAuthClass(c.path)
		if !ok {
			t.Errorf("ResolveRouteAuthClass(%q) found no row", c.path)
			continue
		}
		if row.Class != c.wantClass || row.Prefix != c.wantPref {
			t.Errorf("ResolveRouteAuthClass(%q) = (%q, %q), want (%q, %q)", c.path, row.Prefix, row.Class, c.wantPref, c.wantClass)
		}
	}

	if _, ok := server.ResolveRouteAuthClass("/other/root"); ok {
		t.Error("ResolveRouteAuthClass matched a path outside every declared prefix; it must report ok=false")
	}
}
