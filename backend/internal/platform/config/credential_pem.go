package config

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func credentialBytes(value, path string) ([]byte, error) {
	if value != "" && path != "" {
		return nil, fmt.Errorf("%w: choose PEM environment or explicit legacy key file", ErrInvalid)
	}
	if value != "" {
		value = strings.TrimSpace(strings.ReplaceAll(value, `\n`, "\n"))
		if len(value) > 16384 || value == "" {
			return nil, fmt.Errorf("%w: credential PEM exceeds input budget", ErrInvalid)
		}
		return []byte(value), nil
	}
	if path == "" {
		return nil, fmt.Errorf("%w: credential PEM environment is required", ErrInvalid)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot open explicitly configured legacy credential key", ErrInvalid)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(raw) > 16384 || len(raw) == 0 {
		return nil, fmt.Errorf("%w: credential key unreadable or oversized", ErrInvalid)
	}
	return raw, nil
}

// CredentialPublicBytes resolves unified env PEM or an explicit old file.
func (c Config) CredentialPublicBytes() ([]byte, error) {
	return credentialBytes(c.CredentialPublicKeyPEM, c.CredentialPublicKeyFile)
}

// CredentialPrivateBytes returns worker-only sealed-credential key material.
// Callers must clear the returned bytes; it never enters workflow payloads.
func (c Config) CredentialPrivateBytes() ([]byte, error) {
	return credentialBytes(c.CredentialPrivateKeyPEM, c.CredentialPrivateKeyFile)
}

// HasCredentialPrivateKey reports whether the provider worker has an explicit key source.
func (c Config) HasCredentialPrivateKey() bool {
	return strings.TrimSpace(c.CredentialPrivateKeyPEM) != "" || strings.TrimSpace(c.CredentialPrivateKeyFile) != ""
}

// Format prevents process configuration from exposing any env credentials.
func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[redacted application configuration]")
}
