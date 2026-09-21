package auth

import (
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
)

type AccessClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

// GenerateAccessToken signs an Ed25519 JWT access token with the given expiration
func GenerateAccessToken(userID uuid.UUID, email string, role string, privKey ed25519.PrivateKey, duration time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := AccessClaims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
			Issuer:    "openlocalcrm",
			Audience:  jwt.ClaimStrings{"openlocalcrm-api"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(privKey)
}

// ValidateAccessToken parses and verifies an Ed25519 JWT access token
func ValidateAccessToken(tokenStr string, pubKey ed25519.PublicKey) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, ErrInvalidToken
		}
		return pubKey, nil
	}, jwt.WithIssuer("openlocalcrm"), jwt.WithAudience("openlocalcrm-api"))

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
