package auth

import "github.com/openlocalcrm/openlocalcrm/internal/db"

// SetAccessTokenSignerForTest overrides access-token signing.
func (s *AuthService) SetAccessTokenSignerForTest(f func(db.User) (string, error)) {
	s.signAccessToken = f
}
