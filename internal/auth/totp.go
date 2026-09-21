package auth

import (
	"crypto/subtle"
	"time"

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

// ValidateTOTPCodeWithStep validates a 6-digit passcode against the stored secret,
// checking time steps in [currentStep - 1, currentStep, currentStep + 1] and ensuring
// the step is strictly greater than lastUsedStep (mitigating replay attacks per F-02).
func ValidateTOTPCodeWithStep(passcode, secret string, lastUsedStep int64) (bool, int64) {
	if len(passcode) != 6 || secret == "" {
		return false, 0
	}

	now := time.Now().Unix()
	currentStep := now / 30

	// Check window: previous, current, next step
	for step := currentStep - 1; step <= currentStep+1; step++ {
		if step <= lastUsedStep {
			continue // Already consumed step, cannot be replayed
		}

		stepTime := time.Unix(step*30, 0)
		expectedCode, err := totp.GenerateCode(secret, stepTime)
		if err != nil {
			continue
		}

		if subtle.ConstantTimeCompare([]byte(expectedCode), []byte(passcode)) == 1 {
			return true, step
		}
	}

	return false, 0
}

// ValidateTOTPCode validates a 6-digit passcode against the stored secret without step tracking
func ValidateTOTPCode(passcode, secret string) bool {
	valid, _ := ValidateTOTPCodeWithStep(passcode, secret, 0)
	return valid
}
