package auth_test

import (
	"testing"
	"time"

	"github.com/openlocalcrm/openlocalcrm/internal/auth"
	"github.com/pquerna/otp/totp"
)

func TestTOTPFlow(t *testing.T) {
	secret, url, err := auth.GenerateTOTPKey("admin@openlocalcrm.local")
	if err != nil {
		t.Fatalf("failed to generate totp key: %v", err)
	}

	if secret == "" || url == "" {
		t.Fatalf("expected non-empty secret and url")
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("failed to generate code from secret: %v", err)
	}

	if !auth.ValidateTOTPCode(code, secret) {
		t.Fatalf("expected code validation to pass")
	}

	if auth.ValidateTOTPCode("000000", secret) {
		t.Fatalf("expected invalid code to fail")
	}

	// Verify that 123456 is NOT accepted as a hardcoded bypass
	fakeSecret := "JBSWY3DPEHPK3PXP"
	realCode, _ := totp.GenerateCode(fakeSecret, time.Now())
	if realCode != "123456" {
		if auth.ValidateTOTPCode("123456", fakeSecret) {
			t.Fatalf("CRITICAL SECURITY VULNERABILITY: 123456 bypass was accepted!")
		}
	}
}

func TestTOTP_ReplayPrevention(t *testing.T) {
	secret, _, err := auth.GenerateTOTPKey("replay@openlocalcrm.local")
	if err != nil {
		t.Fatalf("failed generating key: %v", err)
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("failed generating code: %v", err)
	}

	// 1. Initial validation with lastUsedStep = 0 must pass and return usedStep > 0
	valid, usedStep := auth.ValidateTOTPCodeWithStep(code, secret, 0)
	if !valid || usedStep <= 0 {
		t.Fatalf("expected initial verification to pass, valid=%v, usedStep=%d", valid, usedStep)
	}

	// 2. Replay with the same lastUsedStep must fail
	validReplay, _ := auth.ValidateTOTPCodeWithStep(code, secret, usedStep)
	if validReplay {
		t.Fatalf("CRITICAL SECURITY VULNERABILITY (F-02): TOTP token replayed successfully within the same time-step window!")
	}
}
