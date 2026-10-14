package config_test

import (
	"errors"
	"testing"

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
	t.Setenv("LV_IDENTITY_KEY_FILE", "/private/identity.key")
	t.Setenv("LV_IDENTITY_TERMS_VERSION", "v1")
	t.Setenv("LV_IDENTITY_MAIL_SOURCE_HOURLY_LIMIT", "100")
	t.Setenv("LV_IDENTITY_MAIL_GLOBAL_HOURLY_LIMIT", "1000")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IdentityKeyFile != "/private/identity.key" || cfg.IdentityTermsVersion != "v1" || cfg.IdentityMailSourceHourlyLimit != 100 || cfg.IdentityMailGlobalHourlyLimit != 1000 {
		t.Fatal("identity configuration missing")
	}
	t.Setenv("LV_IDENTITY_KEY_FILE", "relative.key")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatal("relative identity key path accepted")
	}
}
