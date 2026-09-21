package auth

import (
	"github.com/pquerna/otp/totp"
)

// GenerateTOTPKey generates a new TOTP secret key for a user
func GenerateTOTPKey(accountName string) (secret string, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "OpenLocalCRM",
		AccountName: accountName,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateTOTPCode validates a 6-digit passcode against the stored secret
func ValidateTOTPCode(passcode, secret string) bool {
	if len(passcode) != 6 || secret == "" {
		return false
	}
	return totp.Validate(passcode, secret)
}
