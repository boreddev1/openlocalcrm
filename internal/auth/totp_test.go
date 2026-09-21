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
