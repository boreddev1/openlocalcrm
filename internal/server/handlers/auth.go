package handlers

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); strings.EqualFold(proto, "https") {
		return true
	}
	if strings.EqualFold(os.Getenv("FORCE_SECURE_COOKIES"), "true") {
		return true
	}
	return false
}

type AuthHandler struct {
	service *auth.AuthService
}

func NewAuthHandler(queries db.Querier, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey) *AuthHandler {
	svc := auth.NewAuthService(queries, privKey, pubKey, nil)
	return &AuthHandler{
		service: svc,
	}
}

func NewAuthHandlerWithService(service *auth.AuthService) *AuthHandler {
	return &AuthHandler{
		service: service,
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TotpCode string `json:"totp_code,omitempty"`
}

type UserPayload struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

type LoginResponse struct {
	TOTPRequired       bool         `json:"totp_required,omitempty"`
	MustChangePassword bool         `json:"must_change_password,omitempty"`
	Message            string       `json:"message,omitempty"`
	User               *UserPayload `json:"user,omitempty"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request","message":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"validation_error","message":"email and password are required"}`, http.StatusBadRequest)
		return
	}

	ip := auth.ClientIP(r)
	userAgent := r.UserAgent()

	res, err := h.service.Login(r.Context(), req.Email, req.Password, req.TotpCode, ip, userAgent)
	if err != nil {
		status := http.StatusUnauthorized
		if strings.Contains(err.Error(), "gesperrt") {
			status = http.StatusTooManyRequests
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "authentication_failed",
			"message": err.Error(),
		})
		return
	}

	if res.TOTPRequired {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(LoginResponse{
			TOTPRequired: true,
			Message:      "Zwei-Faktor-Authentifizierung (TOTP) erforderlich.",
		})
		return
	}

	// Set HttpOnly Cookies
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    res.Token,
		Path:     "/",
		Expires:  time.Now().Add(auth.AccessTokenDuration),
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})

	if res.RefreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    res.RefreshToken,
			Path:     "/api/v1/auth",
			Expires:  time.Now().Add(30 * 24 * time.Hour),
			HttpOnly: true,
			Secure:   isSecureRequest(r),
			SameSite: http.SameSiteStrictMode,
		})
	}

	if _, err := auth.SetCSRFCookie(w, isSecureRequest(r), uuid.UUID(res.User.ID.Bytes).String()); err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(LoginResponse{
		MustChangePassword: res.User.MustChangePassword,
		User: &UserPayload{
			UserID:    uuid.UUID(res.User.ID.Bytes).String(),
			Email:     res.User.Email,
			Role:      res.User.Role,
			FirstName: res.User.FirstName,
			LastName:  res.User.LastName,
		},
	})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"missing_token","message":"refresh token required"}`, http.StatusBadRequest)
		return
	}
	token := cookie.Value

	newToken, newRefreshToken, err := h.service.RefreshToken(r.Context(), token, auth.ClientIP(r), r.UserAgent())
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid_token", "Ungültiger oder abgelaufener Refresh-Token")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    newToken,
		Path:     "/",
		Expires:  time.Now().Add(auth.AccessTokenDuration),
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})

	if newRefreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    newRefreshToken,
			Path:     "/api/v1/auth",
			Expires:  time.Now().Add(30 * 24 * time.Hour),
			HttpOnly: true,
			Secure:   isSecureRequest(r),
			SameSite: http.SameSiteStrictMode,
		})
	}

	if _, err := auth.SetCSRFCookie(w, isSecureRequest(r)); err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var refreshToken string
	if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
		refreshToken = cookie.Value
	}
	if refreshToken != "" {
		_ = h.service.RevokeRefreshToken(r.Context(), refreshToken)
	}

	ClearAuthCookies(w, r)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ClearAuthCookies removes the session cookies (access_token, refresh_token, csrf_token).
func ClearAuthCookies(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})
	auth.ClearCSRFCookie(w, isSecureRequest(r))
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request","message":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	err := h.service.ChangePassword(r.Context(), claims.UserID, req.OldPassword, req.NewPassword)
	if err != nil {
		status := http.StatusBadRequest
		if err == auth.ErrInvalidOldPassword {
			status = http.StatusUnauthorized
		}
		writeJSONError(w, status, "password_change_failed", "Passwortänderung fehlgeschlagen")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *AuthHandler) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}

	res, err := h.service.SetupTOTP(r.Context(), claims.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "totp_setup_failed", "Zwei-Faktor-Authentifizierung konnte nicht initialisiert werden")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

type VerifyTOTPRequest struct {
	Secret string `json:"secret"`
	Code   string `json:"code"`
}

func (h *AuthHandler) VerifyTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}

	var req VerifyTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" || req.Secret == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "code and secret are required")
		return
	}

	err := h.service.VerifyTOTP(r.Context(), claims.UserID, req.Secret, req.Code)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "totp_verification_failed", "Ungültiger Bestätigungscode")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "enabled": true})
}

type DisableTOTPRequest struct {
	Password string `json:"password"`
}

func (h *AuthHandler) DisableTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Nicht angemeldet")
		return
	}

	var req DisableTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "password is required")
		return
	}

	err := h.service.DisableTOTP(r.Context(), claims.UserID, req.Password)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "totp_disable_failed", "Deaktivierung fehlgeschlagen: falsches Passwort")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "enabled": false})
}
