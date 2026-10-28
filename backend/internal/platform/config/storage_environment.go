package config

import (
	"fmt"
	"strings"
)

// StorageEnvironment is a connection loaded exclusively from the unified env.
// Its secret values are never included in formatted diagnostics.
type StorageEnvironment struct{ Provider, Endpoint, SigningEndpoint, Bucket, Region, ProfileID, AccessKey, SecretKey string }

// Format redacts connection credentials from diagnostic formatting.
func (StorageEnvironment) Format(s fmt.State, _ rune) {
	_, _ = s.Write([]byte("[redacted storage connection]"))
}

func storageEnvironment(get func(string) string, prefix string) StorageEnvironment {
	return StorageEnvironment{Provider: strings.TrimSpace(get(prefix + "PROVIDER")), Endpoint: strings.TrimSpace(get(prefix + "ENDPOINT")), SigningEndpoint: strings.TrimSpace(get(prefix + "SIGNING_ENDPOINT")), Bucket: strings.TrimSpace(get(prefix + "BUCKET")), Region: strings.TrimSpace(get(prefix + "REGION")), ProfileID: strings.TrimSpace(get(prefix + "PROFILE_ID")), AccessKey: get(prefix + "ACCESS_KEY"), SecretKey: get(prefix + "SECRET_KEY")}
}
func (e StorageEnvironment) configured() bool {
	return e.Endpoint != "" || e.Bucket != "" || e.Region != "" || e.AccessKey != "" || e.SecretKey != ""
}
func (e StorageEnvironment) profile(prefix string) StorageProfile {
	return StorageProfile{Provider: e.Provider, Mode: "read_write", Endpoint: e.Endpoint, SigningEndpoint: e.SigningEndpoint, Bucket: e.Bucket, Region: e.Region, Credentials: StorageCredentialsRef{Source: "env", AccessKeyEnv: prefix + "ACCESS_KEY", SecretKeyEnv: prefix + "SECRET_KEY"}}
}

func (c *Config) configureStorage(get func(string) string) error {
	const prefix = "GLOWLOOM_OBJECT_STORAGE_"
	primary := storageEnvironment(get, prefix)
	c.ObjectStorageProvider = primary.Provider
	c.ObjectStorageLegacy = storageEnvironment(get, prefix+"LEGACY_")
	if c.ObjectStorageLegacy.configured() {
		if c.ObjectStorageLegacy.Provider == "" {
			c.ObjectStorageLegacy.Provider = "minio"
		}
		if c.ObjectStorageLegacy.Region == "" && c.ObjectStorageLegacy.Provider == "minio" {
			c.ObjectStorageLegacy.Region = "us-east-1"
		}
		if c.ObjectStorageLegacy.ProfileID == "" {
			c.ObjectStorageLegacy.ProfileID = "legacy-main"
		}
		if c.ObjectStorageLegacy.Endpoint == "" || c.ObjectStorageLegacy.Bucket == "" || c.ObjectStorageLegacy.AccessKey == "" || c.ObjectStorageLegacy.SecretKey == "" {
			return fmt.Errorf("%w: incomplete legacy object storage environment", ErrInvalid)
		}
	}
	if primary.Provider == "" {
		return nil
	} // The existing single MinIO connection keeps its original compatibility contract.
	if primary.Provider != "minio" && primary.Provider != "cos" && primary.Provider != "tos" {
		return fmt.Errorf("%w: unsupported object storage provider", ErrInvalid)
	}
	if !primary.configured() {
		return nil
	}
	if primary.Endpoint == "" || primary.Bucket == "" || primary.Region == "" || primary.AccessKey == "" || primary.SecretKey == "" {
		return fmt.Errorf("%w: incomplete object storage environment", ErrInvalid)
	}
	if primary.ProfileID == "" {
		primary.ProfileID = primary.Provider + "-main"
	}
	revision := strings.TrimSpace(get(prefix + "REVISION"))
	if revision == "" {
		revision = "env-v1"
	}
	cfg := defaultStorageConfig()
	cfg.environment = c.Env
	cfg.SchemaVersion = 1
	cfg.Revision = revision
	cfg.DefaultWriteProfile = primary.ProfileID
	cfg.Profiles = map[string]StorageProfile{primary.ProfileID: primary.profile(prefix)}
	cfg.credentialLookup = get
	if c.ObjectStorageLegacy.configured() {
		p := c.ObjectStorageLegacy.profile(prefix + "LEGACY_")
		p.Mode = "read_only"
		if previous, ok := cfg.Profiles[c.ObjectStorageLegacy.ProfileID]; ok {
			if previous.ResourceIdentity() != p.ResourceIdentity() {
				return fmt.Errorf("%w: legacy profile rebinds a different resource", ErrInvalid)
			}
		} else {
			cfg.Profiles[c.ObjectStorageLegacy.ProfileID] = p
		}
	}
	validated, err := cfg.Validated()
	if err != nil {
		return err
	}
	c.ObjectStorage = &validated
	return nil
}

// ResolveCredentials retains the configured unified env loader, including values
// loaded from .env, without exporting them to global process environment.
func (c ObjectStorageConfig) ResolveCredentials(ref StorageCredentialsRef) (StorageCredentials, error) {
	if c.credentialLookup != nil {
		return resolveStorageCredentials(ref, c.credentialLookup)
	}
	return ResolveStorageCredentials(ref)
}

// LegacyStorage returns only an explicitly named original read connection. The
// original provider-less MinIO contract is handled separately by its old client.
func (c Config) LegacyStorage() (*ObjectStorageConfig, error) {
	e := c.ObjectStorageLegacy
	if !e.configured() {
		return nil, nil
	}
	if e.Provider == "" {
		e.Provider = "minio"
	}
	if e.ProfileID == "" {
		e.ProfileID = "legacy-main"
	}
	if e.Region == "" && e.Provider == "minio" {
		e.Region = "us-east-1"
	}
	cfg := defaultStorageConfig()
	cfg.environment = c.Env
	cfg.SchemaVersion = 1
	cfg.Revision = "legacy-env-v1"
	cfg.DefaultWriteProfile = e.ProfileID
	cfg.Profiles = map[string]StorageProfile{e.ProfileID: e.profile("GLOWLOOM_OBJECT_STORAGE_LEGACY_")}
	cfg.credentialLookup = func(key string) string {
		switch key {
		case "GLOWLOOM_OBJECT_STORAGE_LEGACY_ACCESS_KEY":
			return e.AccessKey
		case "GLOWLOOM_OBJECT_STORAGE_LEGACY_SECRET_KEY":
			return e.SecretKey
		default:
			return ""
		}
	}
	validated, err := cfg.Validated()
	if err != nil {
		return nil, err
	}
	return &validated, nil
}
