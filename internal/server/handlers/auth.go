package handlers

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/openlocalcrm/openlocalcrm/internal/db"
)

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
	Token        string       `json:"token,omitempty"`
	RefreshToken string       `json:"refresh_token,omitempty"`
	TOTPRequired bool         `json:"totp_required,omitempty"`
	Message      string       `json:"message,omitempty"`
	User         *UserPayload `json:"user,omitempty"`
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

	ip := r.RemoteAddr
	userAgent := r.UserAgent()

	res, err := h.service.Login(r.Context(), req.Email, req.Password, req.TotpCode, ip, userAgent)
	if err != nil {
		status := http.StatusUnauthorized
		if err == auth.ErrUserNotActive {
			status = http.StatusForbidden
		} else if strings.Contains(err.Error(), "gesperrt") {
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
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	if res.RefreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    res.RefreshToken,
			Path:     "/api/v1/auth",
			Expires:  time.Now().Add(30 * 24 * time.Hour),
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteStrictMode,
		})
	}

	auth.SetCSRFCookie(w, r.TLS != nil)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(LoginResponse{
		Token:        res.Token,
		RefreshToken: res.RefreshToken,
		User: &UserPayload{
			UserID:    uuid.UUID(res.User.ID.Bytes).String(),
			Email:     res.User.Email,
			Role:      res.User.Role,
			FirstName: res.User.FirstName,
			LastName:  res.User.LastName,
		},
	})
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var token string

	// Check body first
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.RefreshToken != "" {
		token = req.RefreshToken
	} else if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
		token = cookie.Value
	}

	if token == "" {
		http.Error(w, `{"error":"missing_token","message":"refresh token required"}`, http.StatusBadRequest)
		return
	}

	newToken, newRefreshToken, err := h.service.RefreshToken(r.Context(), token, r.RemoteAddr, r.UserAgent())
	if err != nil {
		http.Error(w, `{"error":"invalid_token","message":"`+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    newToken,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	if newRefreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    newRefreshToken,
			Path:     "/api/v1/auth",
			Expires:  time.Now().Add(30 * 24 * time.Hour),
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteStrictMode,
		})
	}

	auth.SetCSRFCookie(w, r.TLS != nil)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"token":         newToken,
		"refresh_token": newRefreshToken,
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

	var accessToken string
	if cookie, err := r.Cookie("access_token"); err == nil && cookie.Value != "" {
		accessToken = cookie.Value
	} else if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		accessToken = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if accessToken != "" {
		h.service.RevokeAccessToken(accessToken)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	auth.ClearCSRFCookie(w)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
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
		http.Error(w, `{"error":"password_change_failed","message":"`+err.Error()+`"}`, status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *AuthHandler) SetupTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	res, err := h.service.SetupTOTP(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, `{"error":"totp_setup_failed","message":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

type VerifyTOTPRequest struct {
	Code string `json:"code"`
}

func (h *AuthHandler) VerifyTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.UserContextKey).(*auth.AccessClaims)
	if !ok || claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req VerifyTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		http.Error(w, `{"error":"invalid_request","message":"code is required"}`, http.StatusBadRequest)
		return
	}

	err := h.service.VerifyTOTP(r.Context(), claims.UserID, req.Code)
	if err != nil {
		http.Error(w, `{"error":"totp_verification_failed","message":"`+err.Error()+`"}`, http.StatusBadRequest)
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
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req DisableTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		http.Error(w, `{"error":"invalid_request","message":"password is required"}`, http.StatusBadRequest)
		return
	}

	err := h.service.DisableTOTP(r.Context(), claims.UserID, req.Password)
	if err != nil {
		http.Error(w, `{"error":"totp_disable_failed","message":"`+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "enabled": false})
}
