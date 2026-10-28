package config_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestCredentialPEMEnvironmentEscapedNewlinesAndNoKeyFile(t *testing.T) {
	t.Setenv("GLOWLOOM_ENV_FILE", "")
	t.Setenv("GLOWLOOM_CREDENTIAL_PUBLIC_KEY_FILE", "")
	t.Setenv("GLOWLOOM_CREDENTIAL_PRIVATE_KEY_FILE", "")
	t.Setenv("GLOWLOOM_CREDENTIAL_KEY_ID", "fixture-key")
	t.Setenv("GLOWLOOM_CREDENTIAL_PUBLIC_KEY_PEM", `-----BEGIN PUBLIC KEY-----\nfixture\n-----END PUBLIC KEY-----`)
	t.Setenv("GLOWLOOM_CREDENTIAL_PRIVATE_KEY_PEM", `-----BEGIN PRIVATE KEY-----\nfixture\n-----END PRIVATE KEY-----`)
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	public, err := c.CredentialPublicBytes()
	if err != nil || string(public) != "-----BEGIN PUBLIC KEY-----\nfixture\n-----END PUBLIC KEY-----" {
		t.Fatal("public PEM env not resolved", err)
	}
	private, err := c.CredentialPrivateBytes()
	if err != nil || string(private) != "-----BEGIN PRIVATE KEY-----\nfixture\n-----END PRIVATE KEY-----" {
		t.Fatal("private PEM env not resolved", err)
	}
	clear(private)
	if !c.HasCredentialPrivateKey() || !(config.Config{CredentialPrivateKeyFile: "/explicit/legacy.pem"}).HasCredentialPrivateKey() || (config.Config{}).HasCredentialPrivateKey() {
		t.Fatal("provider worker credential source detection differs from key loading")
	}
	t.Setenv("GLOWLOOM_CREDENTIAL_PRIVATE_KEY_FILE", "/must-not-read/legacy.pem")
	if _, err := config.Load(); !errors.Is(err, config.ErrInvalid) {
		t.Fatal("mixed private key sources accepted", err)
	}
}
