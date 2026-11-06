package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/core/invite"
)

// InviteHandler serves the two public accept-invite endpoints. It owns request
// parsing, status codes and the safe response projection only; every token and
// password decision lives in internal/core/invite.
//
// Neither endpoint establishes a session. Login stays the only operation that
// sets an auth cookie, so an accepted invite still requires a real sign-in.
type InviteHandler struct {
	service *invite.Service
}

func NewInviteHandler(service *invite.Service) *InviteHandler {
	return &InviteHandler{service: service}
}

// validateResponse is the explicit projection of invite metadata. db.UserInvite
// carries token_hash, so serializing the model would leak the persisted secret.
type validateResponse struct {
	Valid     bool   `json:"valid"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	ExpiresAt string `json:"expires_at"`
}

// Validate is GET /api/v1/auth/accept-invite?token=... It is read-only: opening
// the link twice must never consume the invite.
func (h *InviteHandler) Validate(w http.ResponseWriter, r *http.Request) {
	rawToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if rawToken == "" {
		writeInviteNotUsable(w, "Der Einladungslink ist unvollständig. Bitte fordern Sie eine neue Einladung an.")
		return
	}

	view, err := h.service.Validate(inviteRequestContext(r), rawToken)
	if err != nil {
		writeInviteFailure(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(validateResponse{
		Valid:     true,
		Email:     view.Email,
		FirstName: view.FirstName,
		LastName:  view.LastName,
		ExpiresAt: view.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

type acceptInviteRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type acceptInviteResponse struct {
	Status    string `json:"status"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	// LoginRequired is always true: accepting an invite deliberately does not
	// log the user in, so the client must send them to the login form.
	LoginRequired bool `json:"login_required"`
}

// Accept is POST /api/v1/auth/accept-invite. It is public, CSRF-exempt and
// rate-limited like login, because the invitee has no session yet.
func (h *InviteHandler) Accept(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAcceptInviteBytes)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Der Anfragetext konnte nicht gelesen werden.")
		return
	}
	if strings.TrimSpace(req.Token) == "" || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Token und Passwort sind erforderlich.")
		return
	}

	user, err := h.service.Accept(inviteRequestContext(r), strings.TrimSpace(req.Token), req.Password)
	if err != nil {
		writeInviteFailure(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(acceptInviteResponse{
		Status:        user.Status,
		Email:         user.Email,
		FirstName:     user.FirstName,
		LoginRequired: true,
	})
}

// maxAcceptInviteBytes keeps a public, unauthenticated endpoint from being used
// as a memory sink. The payload is a token and a password.
const maxAcceptInviteBytes = 4 << 10

// inviteRequestContext attaches the real request identity for the audit trail.
// The invite service already documents that the audit rows must carry the client
// IP and user agent, so every entry point that reaches the service has to supply
// them — including the two public endpoints, whose rejections are exactly the
// rows an administrator has to investigate afterwards.
func inviteRequestContext(r *http.Request) context.Context {
	return invite.WithRequestMeta(r.Context(), auth.ClientIP(r), r.UserAgent())
}

func writeInviteFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, invite.ErrWeakPassword):
		writeJSONError(w, http.StatusBadRequest, "weak_password",
			"Das Passwort muss mindestens 8 Zeichen lang sein. Die Einladung bleibt gültig.")
	case errors.Is(err, invite.ErrNoGateway):
		writeJSONError(w, http.StatusNotImplemented, "invite_unavailable",
			"Die Einladungsfunktion benötigt einen Datenbankzugriff, der hier nicht verfügbar ist.")
	default:
		writeInviteNotUsable(w,
			"Diese Einladung ist nicht mehr gültig. Sie wurde bereits verwendet, wurde widerrufen oder ist abgelaufen. Bitte fordern Sie eine neue Einladung an.")
	}
}

func writeInviteNotUsable(w http.ResponseWriter, message string) {
	writeJSONError(w, http.StatusBadRequest, "invite_not_usable", message)
}

// inviteBaseURL resolves the browser-facing origin for the copyable link.
//
// The configured DOMAIN wins so a link generated behind a reverse proxy still
// points at the public address. Without it the request decides, in this order:
//
//  1. X-Forwarded-Proto, because the hop that reached this process is often
//     plaintext (Caddy → localhost) while the browser used TLS. Only the first
//     hop is read, and the value is reduced to "http" or "https".
//  2. The connection itself: r.TLS non-nil means it was encrypted, which agrees
//     with the https default. That branch is a no-op for the scheme, and it has
//     to stay one: treating TLS as "http" would hand the invitee a plaintext
//     link for a connection that was never plaintext.
//  3. The https default, which is the safe direction to be wrong in: a wrong
//     origin produces an unusable link, a downgraded one produces a link that
//     leaks a one-time token over cleartext.
//
// r.Host is the client-supplied Host header, so it is untrusted input and is run
// through invite.SanitizeHost. An unusable host yields an empty origin, and the
// link degrades to the bare /accept-invite path rather than carrying a
// header-shaped string an administrator would hand to somebody.
func inviteBaseURL(r *http.Request) string {
	if domain := strings.TrimSpace(os.Getenv("DOMAIN")); domain != "" {
		return domain
	}
	host := invite.SanitizeHost(r.Host)
	if host == "" {
		return ""
	}
	return requestScheme(r) + "://" + host
}

// requestScheme reports the scheme the browser used to reach this instance.
func requestScheme(r *http.Request) string {
	// X-Forwarded-Proto wins, because the hop that reached this process is often
	// plaintext (Caddy → localhost) while the browser used TLS. Only the first
	// hop of a comma-separated list is read, and the value is reduced to "http"
	// or "https" so nothing else can ride in.
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if forwarded != "" {
		if first, _, _ := strings.Cut(forwarded, ","); strings.EqualFold(strings.TrimSpace(first), "http") {
			return "http"
		}
		return "https"
	}
	// Nothing was forwarded, so the connection itself decides. A direct TLS
	// connection (r.TLS != nil) is encrypted, and so is the default, so both land
	// on https: an encrypted connection is never a reason to emit http. That
	// inversion shipped once already and is pinned by
	// TestInviteLinkOriginFollowsTheRequest.
	return "https"
}
