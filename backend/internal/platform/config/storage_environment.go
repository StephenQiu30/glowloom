package config

import (
	"fmt"
	"strings"
)

const objectStorageConfigEnvironment = "GLOWLOOM_OBJECT_STORAGE_CONFIG"

var obsoleteStorageEnvironmentKeys = []string{
	"GLOWLOOM_OBJECT_STORAGE_PROVIDER",
	"GLOWLOOM_OBJECT_STORAGE_ENDPOINT",
	"GLOWLOOM_OBJECT_STORAGE_SIGNING_ENDPOINT",
	"GLOWLOOM_OBJECT_STORAGE_BUCKET",
	"GLOWLOOM_OBJECT_STORAGE_REGION",
	"GLOWLOOM_OBJECT_STORAGE_PROFILE_ID",
	"GLOWLOOM_OBJECT_STORAGE_ACCESS_KEY",
	"GLOWLOOM_OBJECT_STORAGE_SECRET_KEY",
	"GLOWLOOM_OBJECT_STORAGE_REVISION",
	"GLOWLOOM_OBJECT_STORAGE_LEGACY_PROVIDER",
	"GLOWLOOM_OBJECT_STORAGE_LEGACY_ENDPOINT",
	"GLOWLOOM_OBJECT_STORAGE_LEGACY_SIGNING_ENDPOINT",
	"GLOWLOOM_OBJECT_STORAGE_LEGACY_BUCKET",
	"GLOWLOOM_OBJECT_STORAGE_LEGACY_REGION",
	"GLOWLOOM_OBJECT_STORAGE_LEGACY_PROFILE_ID",
	"GLOWLOOM_OBJECT_STORAGE_LEGACY_ACCESS_KEY",
	"GLOWLOOM_OBJECT_STORAGE_LEGACY_SECRET_KEY",
}

func (c *Config) configureStorage(get func(string) string) error {
	for _, key := range obsoleteStorageEnvironmentKeys {
		if strings.TrimSpace(get(key)) != "" {
			return fmt.Errorf("%w: configure object storage only with %s", ErrInvalid, objectStorageConfigEnvironment)
		}
	}
	raw := strings.TrimSpace(get(objectStorageConfigEnvironment))
	if raw == "" {
		return nil
	}
	configured, err := LoadObjectStorageEnvironment(raw, c.Env, get)
	if err != nil {
		return err
	}
	c.ObjectStorage = &configured
	return nil
}

// ResolveCredentials retains the unified environment loader, including values
// loaded from GLOWLOOM_ENV_FILE, without exporting secrets to global process state.
func (c ObjectStorageConfig) ResolveCredentials(ref StorageCredentialsRef) (StorageCredentials, error) {
	if c.credentialLookup != nil {
		return resolveStorageCredentials(ref, c.credentialLookup)
	}
	return ResolveStorageCredentials(ref)
}
