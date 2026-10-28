package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// StorageCredentialsRef selects exactly one deployment-controlled secret source.
type StorageCredentialsRef struct {
	Source          string `yaml:"source"`
	AccessKeyEnv    string `yaml:"access_key_env"`
	SecretKeyEnv    string `yaml:"secret_key_env"`
	SessionTokenEnv string `yaml:"session_token_env"`
	ExpiresAtEnv    string `yaml:"expires_at_env"`
	File            string `yaml:"file"`
}

// StorageCredentials is a resolved secret; structured and formatted logs redact it.
type StorageCredentials struct {
	AccessKey    string    `json:"-" yaml:"-"`
	SecretKey    string    `json:"-" yaml:"-"`
	SessionToken string    `json:"-" yaml:"-"`
	ExpiresAt    time.Time `json:"-" yaml:"-"`
}

// Format prevents generic struct diagnostics from disclosing credentials.
func (StorageCredentials) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[redacted storage credentials]")
}

func (r StorageCredentialsRef) validate() error {
	if r.Source == "file" && filepath.IsAbs(r.File) && r.AccessKeyEnv == "" && r.SecretKeyEnv == "" && r.SessionTokenEnv == "" && r.ExpiresAtEnv == "" {
		return nil
	}
	if r.Source != "env" || r.File != "" || r.AccessKeyEnv == "" || r.SecretKeyEnv == "" {
		return fmt.Errorf("%w: select one storage credential source", ErrInvalid)
	}
	name := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	for _, value := range []string{r.AccessKeyEnv, r.SecretKeyEnv, r.SessionTokenEnv, r.ExpiresAtEnv} {
		if value != "" && !name.MatchString(value) {
			return fmt.Errorf("%w: invalid storage credential environment reference", ErrInvalid)
		}
	}
	return nil
}

// ResolveStorageCredentials reads only the explicitly selected secret source.
// Callers resolve again when refreshing temporary credentials or rotating a file.
func ResolveStorageCredentials(ref StorageCredentialsRef) (StorageCredentials, error) {
	return resolveStorageCredentials(ref, os.Getenv)
}

func resolveStorageCredentials(ref StorageCredentialsRef, get func(string) string) (StorageCredentials, error) {
	if err := ref.validate(); err != nil {
		return StorageCredentials{}, err
	}
	var data struct {
		AccessKey    string    `json:"access_key"`
		SecretKey    string    `json:"secret_key"`
		SessionToken string    `json:"session_token"`
		ExpiresAt    time.Time `json:"expires_at"`
	}
	if ref.Source == "env" {
		data.AccessKey, data.SecretKey = get(ref.AccessKeyEnv), get(ref.SecretKeyEnv)
		data.SessionToken = get(ref.SessionTokenEnv)
		if value := get(ref.ExpiresAtEnv); value != "" {
			var err error
			data.ExpiresAt, err = time.Parse(time.RFC3339, value)
			if err != nil {
				return StorageCredentials{}, fmt.Errorf("%w: invalid storage credential expiry", ErrInvalid)
			}
		}
	} else {
		file, err := os.Open(ref.File)
		if err != nil {
			return StorageCredentials{}, fmt.Errorf("%w: cannot read storage secret file", ErrInvalid)
		}
		defer func() { _ = file.Close() }()
		raw, err := io.ReadAll(io.LimitReader(file, (32<<10)+1))
		if err != nil || len(raw) > 32<<10 {
			return StorageCredentials{}, fmt.Errorf("%w: storage secret exceeds read budget", ErrInvalid)
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&data); err != nil {
			return StorageCredentials{}, fmt.Errorf("%w: invalid storage secret schema", ErrInvalid)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return StorageCredentials{}, fmt.Errorf("%w: expected one storage secret record", ErrInvalid)
		}
	}
	if data.AccessKey == "" || data.SecretKey == "" || (data.SessionToken != "" && data.ExpiresAt.IsZero()) ||
		(!data.ExpiresAt.IsZero() && !data.ExpiresAt.After(time.Now().Add(30*time.Second))) {
		return StorageCredentials{}, fmt.Errorf("%w: missing or expired storage credentials", ErrInvalid)
	}
	return StorageCredentials{AccessKey: data.AccessKey, SecretKey: data.SecretKey, SessionToken: data.SessionToken, ExpiresAt: data.ExpiresAt}, nil
}
