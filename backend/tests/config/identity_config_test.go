package config_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestIdentityReusesSMTPConfiguration(t *testing.T) {
	t.Setenv("SMTP_ENABLED", "true")
	t.Setenv("SMTP_HOST", "smtp.example.invalid")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_TLS_MODE", "tls")
	t.Setenv("SMTP_USERNAME", "fixture")
	t.Setenv("SMTP_PASSWORD", "fixture-secret")
	t.Setenv("SMTP_FROM_EMAIL", "noreply@example.invalid")
	t.Setenv("SMTP_FROM_NAME", "织光")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SMTPEnabled || cfg.SMTPPort != 465 || cfg.SMTPTLSMode != "tls" || cfg.SMTPFromName != "织光" {
		t.Fatal("existing SMTP variable names not reused")
	}
	t.Setenv("SMTP_TLS_MODE", "none")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatal("plaintext SMTP accepted")
	}
}

func TestIdentityConfigurationIsExplicit(t *testing.T) {
	t.Setenv("GLOWLOOM_IDENTITY_KEY", "configured-json-value")
	t.Setenv("GLOWLOOM_IDENTITY_TERMS_VERSION", "v1")
	t.Setenv("GLOWLOOM_IDENTITY_MAIL_SOURCE_HOURLY_LIMIT", "100")
	t.Setenv("GLOWLOOM_IDENTITY_MAIL_GLOBAL_HOURLY_LIMIT", "1000")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IdentityKey != "configured-json-value" || cfg.IdentityTermsVersion != "v1" || cfg.IdentityMailSourceHourlyLimit != 100 || cfg.IdentityMailGlobalHourlyLimit != 1000 {
		t.Fatal("identity configuration missing")
	}
	t.Setenv("GLOWLOOM_IDENTITY_KEY", strings.Repeat("x", 4097))
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatal("oversized identity key value accepted")
	}
}

func TestLegacyUpgradeDeadline(t *testing.T) {
	for _, tc := range []struct {
		name  string
		until time.Time
		bad   bool
	}{
		{"baseline", time.Time{}, false}, {"past", time.Now().UTC().Add(-time.Hour), false},
		{"bounded", time.Now().UTC().Add(24 * time.Hour), false}, {"too far", time.Now().UTC().Add(8 * 24 * time.Hour), true},
		{"non UTC", time.Now().In(time.FixedZone("offset", 3600)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := (config.Config{IdentityLegacyUpgradeUntil: tc.until}).ValidateIdentitySessionV2()
			if (err != nil) != tc.bad {
				t.Fatal("deadline validation", err)
			}
		})
	}
}

func TestSessionV2RequiresFixedLegacyDeadline(t *testing.T) {
	cfg := config.Config{IdentitySessionV2Enabled: true, IdentityJWTPrivateKey: "/controlled/private", IdentityJWTVerifyKeys: "/controlled/verify", IdentityJWTActiveKID: "current", IdentityRefreshReceiptKey: "/controlled/receipt"}
	if err := cfg.ValidateIdentitySessionV2(); err == nil {
		t.Fatal("v2 issuance enabled without fixed deadline")
	}
	cfg.IdentityLegacyUpgradeUntil = time.Now().UTC().Add(-time.Hour)
	if err := cfg.ValidateIdentitySessionV2(); err != nil {
		t.Fatal("past cutoff prevented restart", err)
	}
}
