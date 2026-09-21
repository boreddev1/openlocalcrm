package server

import (
	"net/http"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
)

const (
	CSRFCookieName = auth.CSRFCookieName
	CSRFHeaderName = auth.CSRFHeaderName
)

func SetCSRFCookie(w http.ResponseWriter, secure bool) string {
	return auth.SetCSRFCookie(w, secure)
}

func CSRFProtectionMiddleware(next http.Handler) http.Handler {
	return auth.CSRFProtectionMiddleware(next)
}
