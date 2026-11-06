package server

import "strings"

// ---------------------------------------------------------------------------
// Route auth classes (Issue #115)
//
// chi v5.5 records only the mount pattern and the verbs of a route —
// Mux.Routes() never exposes the middleware chain that guards it. That is why
// docs/API_REFERENCE.md could list every route but could not say, verifiably,
// how a caller authenticates against it. This file makes the auth class
// declared data instead of prose: one table row per mounted path prefix,
// authored next to the mount it describes (router.go mounts these very
// constants), and checked three ways by internal/server/api_reference_test.go
// and internal/server/route_auth_class_test.go:
//
//  1. Every route the router walk finds must resolve to exactly one row here.
//     A new endpoint with no declared class fails the build — the table cannot
//     silently fall behind the router.
//  2. Every row must cover at least one walked route — a row for a removed
//     mount fails instead of rotting.
//  3. An unauthenticated request against the *built* router must answer the
//     way the class promises (401 for jwt+csrf, 403 for csrf-only, never
//     401/403 for none). Deleting auth.AuthMiddleware from a group therefore
//     fails even if this table is left untouched.
//
// Deliberately coarse: the class names the mount's guard chain, not the RBAC
// permission inside it (auth.RequirePermission runs after the class fires),
// and the probe for connector-token / jwt-inline can only prove "un-authenticated
// callers are rejected" — those two answer 401 for their own reasons, not via
// AuthMiddleware. See the section "Authentifizierung" in
// docs/API_REFERENCE.md for the honest limits of what is checked.
// ---------------------------------------------------------------------------

// AuthClass names how a caller authenticates against the routes under a path
// prefix. The string values are what the generated inventory table in
// docs/API_REFERENCE.md prints; keep them stable, readers grep for them.
type AuthClass string

const (
	// AuthNone: reachable without any credential (health probe, login, invite
	// acceptance). Rate limiting still applies; several of these endpoints
	// accept a guessable secret and are guarded by it alone.
	AuthNone AuthClass = "none"

	// AuthCSRFOnly: no access token is required or checked. The caller proves
	// it is first-party via the CSRF double-submit cookie/header pair
	// (auth.CSRFDoubleSubmitMiddleware). /auth/refresh and /auth/logout
	// authenticate via the HttpOnly refresh cookie, which no browser sends
	// cross-site — that is why they sit outside the JWT group.
	AuthCSRFOnly AuthClass = "csrf-only"

	// AuthJWTCSRF: the protected group — auth.AuthMiddleware validates the
	// Ed25519 access token (cookie or Bearer) first, then
	// auth.CSRFProtectionMiddleware additionally requires the CSRF header on
	// mutating requests made with the cookie session (a request that
	// authenticated via a verified Bearer token is exempted — it carries no
	// ambient browser credential). RBAC permissions apply on top, per mount.
	AuthJWTCSRF AuthClass = "jwt+csrf"

	// AuthConnectorToken: the lead-intake webhook checks its own static
	// Bearer token (CONNECTOR_API_TOKEN) inside the handler. Unauthenticated
	// callers are rejected with 401 — by the handler, not by AuthMiddleware.
	AuthConnectorToken AuthClass = "connector-token"

	// AuthJWTInline: the SSE stream validates the same access token inline in
	// its handler (GET-only, EventSource cannot send a CSRF header, so the
	// CSRF middleware is not mounted; the token must still be presented via
	// cookie or Bearer header and is signature- and token_version-checked).
	AuthJWTInline AuthClass = "jwt-inline"
)

// Path fragments shared by the mounts in router.go and the table below, so a
// rename in one place cannot silently orphan the other.
const (
	apiV1MountPrefix        = "/api/v1"
	routeHealthFragment     = "/health"
	routeLoginFragment      = "/auth/login"
	routeRefreshFragment    = "/auth/refresh"
	routeLogoutFragment     = "/auth/logout"
	routeInviteFragment     = "/auth/accept-invite"
	routeLeadIntakeFragment = "/connectors/lead-intake"
	sseStreamRoutePath      = "/events/stream"
)

// RouteAuthClass ties one path prefix to its auth class. Prefixes match on
// segment boundaries only: the prefix "/api/v1/auth/logout" covers
// "/api/v1/auth/logout" but not "/api/v1/auth/logout-all".
type RouteAuthClass struct {
	Prefix string
	Class  AuthClass
}

// RouteAuthClasses is the declared ground truth. Order does not matter —
// matching is longest-prefix-wins — but the broadest row ("/api/v1") must stay
// the strict session class: route_auth_class_test.go refuses a loosened
// default, so an unlabelled new endpoint can never become public by accident.
var RouteAuthClasses = []RouteAuthClass{
	// Mounted on the root mux and at the top of the /api/v1 group in router.go,
	// before the protected group opens.
	{Prefix: apiV1MountPrefix + routeHealthFragment, Class: AuthNone},
	{Prefix: apiV1MountPrefix + routeLoginFragment, Class: AuthNone},
	{Prefix: apiV1MountPrefix + routeInviteFragment, Class: AuthNone},
	{Prefix: apiV1MountPrefix + routeRefreshFragment, Class: AuthCSRFOnly},
	{Prefix: apiV1MountPrefix + routeLogoutFragment, Class: AuthCSRFOnly},
	{Prefix: apiV1MountPrefix + routeLeadIntakeFragment, Class: AuthConnectorToken},
	{Prefix: sseStreamRoutePath, Class: AuthJWTInline},
	// Default for everything the protected group registers.
	{Prefix: apiV1MountPrefix, Class: AuthJWTCSRF},
}

// ResolveRouteAuthClass returns the table row whose prefix covers path.
// The longest matching prefix wins. ok is false when no row covers the path —
// callers (the inventory guard) must treat that as a failure, never as
// "unclassified is fine".
func ResolveRouteAuthClass(path string) (RouteAuthClass, bool) {
	var best RouteAuthClass
	found := false
	for _, entry := range RouteAuthClasses {
		if !pathUnderPrefix(path, entry.Prefix) {
			continue
		}
		if !found || len(entry.Prefix) > len(best.Prefix) {
			best = entry
			found = true
		}
	}
	return best, found
}

// pathUnderPrefix matches on segment boundaries: a prefix may cover the exact
// path or a path that continues it after a "/", never a merely string-prefixed
// sibling segment ("logout" must not swallow "logout-all").
func pathUnderPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}
