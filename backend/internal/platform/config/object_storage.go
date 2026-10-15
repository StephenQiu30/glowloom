package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const maxStorageConfigBytes = 256 << 10

var storageProfileID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ObjectStorageConfig describes deployment-owned connections, never inline secrets.
type ObjectStorageConfig struct {
	environment         string
	SchemaVersion       int                       `yaml:"schema_version"`
	Revision            string                    `yaml:"revision"`
	DefaultWriteProfile string                    `yaml:"default_write_profile"`
	Profiles            map[string]StorageProfile `yaml:"profiles"`
	Upload              StorageUploadPolicy       `yaml:"upload"`
	Access              StorageAccessPolicy       `yaml:"access"`
	Limits              StorageLimits             `yaml:"limits"`
}

// StorageProfile binds a stable name to one provider resource and bucket.
type StorageProfile struct {
	Provider        string                `yaml:"provider"`
	Mode            string                `yaml:"mode"`
	Endpoint        string                `yaml:"endpoint"`
	SigningEndpoint string                `yaml:"signing_endpoint"`
	Bucket          string                `yaml:"bucket"`
	Region          string                `yaml:"region"`
	AddressingStyle string                `yaml:"addressing_style"`
	Credentials     StorageCredentialsRef `yaml:"credentials"`
}

// StorageUploadPolicy contains the bounded upload policy frozen by an admission.
type StorageUploadPolicy struct {
	Mode                    string        `yaml:"mode"`
	SignedURLTTL            time.Duration `yaml:"signed_url_ttl"`
	SessionTTL              time.Duration `yaml:"session_ttl"`
	MultipartThresholdBytes int64         `yaml:"multipart_threshold_bytes"`
	PartSizeBytes           int64         `yaml:"part_size_bytes"`
	PartConcurrency         int           `yaml:"part_concurrency"`
	StagingRetention        time.Duration `yaml:"staging_retention"`
}

// StorageAccessPolicy bounds every private access grant.
type StorageAccessPolicy struct {
	PreviewTTL      time.Duration `yaml:"preview_ttl"`
	MaxSignedURLTTL time.Duration `yaml:"max_signed_url_ttl"`
}

// StorageLimits can narrow existing input limits; a hard quota needs its ledger.
type StorageLimits struct {
	MaxImageBytes         int64  `yaml:"max_image_bytes"`
	MaxVideoBytes         int64  `yaml:"max_video_bytes"`
	MaxAudioBytes         int64  `yaml:"max_audio_bytes"`
	MaxModelBytes         int64  `yaml:"max_model_bytes"`
	MaxDocumentBytes      int64  `yaml:"max_document_bytes"`
	PerScopeCapacityBytes *int64 `yaml:"per_scope_capacity_bytes"`
}

// LoadObjectStorage strictly decodes one bounded YAML document without network I/O.
func LoadObjectStorage(path, env string) (ObjectStorageConfig, error) {
	var document struct {
		ObjectStorage ObjectStorageConfig `yaml:"object_storage"`
	}
	document.ObjectStorage = defaultStorageConfig()
	file, err := os.Open(path)
	if err != nil {
		return ObjectStorageConfig{}, fmt.Errorf("%w: cannot read object storage configuration", ErrInvalid)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxStorageConfigBytes+1))
	if err != nil || len(raw) > maxStorageConfigBytes {
		return ObjectStorageConfig{}, fmt.Errorf("%w: object storage configuration exceeds read budget", ErrInvalid)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	// Decoder diagnostics can contain an invalid value, including an inline secret.
	if err := decoder.Decode(&document); err != nil {
		return ObjectStorageConfig{}, fmt.Errorf("%w: invalid object storage YAML schema", ErrInvalid)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ObjectStorageConfig{}, fmt.Errorf("%w: expected one object storage YAML document", ErrInvalid)
	}
	cfg := document.ObjectStorage
	cfg.environment = env
	if err := cfg.validate(env); err != nil {
		return ObjectStorageConfig{}, err
	}
	return cfg, nil
}

// Validated returns a normalized copy, retaining the deployment's HTTP policy.
// Factories call this even when configuration did not come from the YAML loader.
func (c ObjectStorageConfig) Validated() (ObjectStorageConfig, error) {
	profiles := make(map[string]StorageProfile, len(c.Profiles))
	for id, profile := range c.Profiles {
		profiles[id] = profile
	}
	c.Profiles = profiles
	if err := c.validate(c.environment); err != nil {
		return ObjectStorageConfig{}, err
	}
	return c, nil
}

func defaultStorageConfig() ObjectStorageConfig {
	return ObjectStorageConfig{
		DefaultWriteProfile: "minio-main",
		Upload: StorageUploadPolicy{Mode: "relay", SignedURLTTL: 10 * time.Minute,
			SessionTTL: 24 * time.Hour, MultipartThresholdBytes: 32 << 20,
			PartSizeBytes: 8 << 20, PartConcurrency: 3, StagingRetention: 48 * time.Hour},
		Access: StorageAccessPolicy{PreviewTTL: 5 * time.Minute, MaxSignedURLTTL: 15 * time.Minute},
		Limits: StorageLimits{MaxImageBytes: 20 << 20, MaxVideoBytes: 500 << 20,
			MaxAudioBytes: 100 << 20, MaxModelBytes: 64 << 20, MaxDocumentBytes: 20 << 20},
	}
}

func (c *ObjectStorageConfig) validate(env string) error {
	if c.SchemaVersion != 1 || !storageProfileID.MatchString(c.Revision) || len(c.Profiles) == 0 || len(c.Profiles) > 32 {
		return fmt.Errorf("%w: object storage version, revision or profiles", ErrInvalid)
	}
	seen := make(map[string]bool)
	for id, profile := range c.Profiles {
		if !storageProfileID.MatchString(id) {
			return fmt.Errorf("%w: invalid object storage profile ID", ErrInvalid)
		}
		if err := profile.validate(env); err != nil {
			return err
		}
		identity := profile.ResourceIdentity()
		if seen[identity] {
			return fmt.Errorf("%w: duplicate object storage resource alias", ErrInvalid)
		}
		seen[identity] = true
		c.Profiles[id] = profile
	}
	if target, ok := c.Profiles[c.DefaultWriteProfile]; !ok || target.Mode != "read_write" {
		return fmt.Errorf("%w: default object storage profile is not writable", ErrInvalid)
	}
	if c.Upload.Mode != "relay" || c.Limits.PerScopeCapacityBytes != nil {
		return fmt.Errorf("%w: direct upload and hard quotas require independently accepted capabilities", ErrInvalid)
	}
	if c.Access.MaxSignedURLTTL < time.Second || c.Access.MaxSignedURLTTL > 15*time.Minute ||
		c.Access.PreviewTTL < time.Second || c.Access.PreviewTTL > c.Access.MaxSignedURLTTL ||
		c.Upload.SignedURLTTL < time.Second || c.Upload.SignedURLTTL > c.Access.MaxSignedURLTTL ||
		c.Upload.SessionTTL < c.Upload.SignedURLTTL || c.Upload.StagingRetention < c.Upload.SessionTTL ||
		c.Upload.PartSizeBytes < 5<<20 || c.Upload.PartSizeBytes > 64<<20 ||
		c.Upload.MultipartThresholdBytes < c.Upload.PartSizeBytes ||
		c.Upload.PartConcurrency < 1 || c.Upload.PartConcurrency > 8 {
		return fmt.Errorf("%w: invalid object storage transfer or expiry budget", ErrInvalid)
	}
	for _, limit := range [][2]int64{{c.Limits.MaxImageBytes, 20 << 20}, {c.Limits.MaxVideoBytes, 500 << 20},
		{c.Limits.MaxAudioBytes, 100 << 20}, {c.Limits.MaxModelBytes, 64 << 20}, {c.Limits.MaxDocumentBytes, 20 << 20}} {
		if limit[0] < 1 || limit[0] > limit[1] {
			return fmt.Errorf("%w: object storage input limits exceed accepted bounds", ErrInvalid)
		}
	}
	return nil
}

func (p *StorageProfile) validate(env string) error {
	if p.Provider != "minio" && p.Provider != "cos" && p.Provider != "tos" {
		return fmt.Errorf("%w: unsupported object storage provider", ErrInvalid)
	}
	if p.Mode == "" {
		p.Mode = "read_write"
	}
	if p.Mode != "read_write" && p.Mode != "read_only" && p.Mode != "disabled" {
		return fmt.Errorf("%w: invalid object storage access mode", ErrInvalid)
	}
	if len(p.Bucket) < 3 || len(p.Bucket) > 63 || strings.Contains(p.Bucket, "..") ||
		!regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`).MatchString(p.Bucket) || net.ParseIP(p.Bucket) != nil ||
		!regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`).MatchString(p.Region) {
		return fmt.Errorf("%w: invalid object storage bucket or region", ErrInvalid)
	}
	var err error
	p.Endpoint, err = storageOrigin(p.Endpoint, env)
	if err != nil {
		return err
	}
	if p.SigningEndpoint == "" {
		p.SigningEndpoint = p.Endpoint
	}
	p.SigningEndpoint, err = storageOrigin(p.SigningEndpoint, env)
	if err != nil {
		return err
	}
	if p.Provider == "minio" {
		if p.AddressingStyle == "" {
			p.AddressingStyle = "path"
		}
		if p.AddressingStyle != "path" && p.AddressingStyle != "virtual" {
			return fmt.Errorf("%w: invalid MinIO addressing style", ErrInvalid)
		}
	} else if p.AddressingStyle != "" || p.SigningEndpoint != p.Endpoint {
		return fmt.Errorf("%w: native cloud addressing requires the verified origin", ErrInvalid)
	}
	if p.Provider == "cos" {
		if err := ValidateCOSOrigin(p.Endpoint, p.Bucket, p.Region, env == "local"); err != nil {
			return err
		}
	}
	return p.Credentials.validate()
}

func storageOrigin(raw, env string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.ForceQuery ||
		(u.Scheme != "https" && (env != "local" || u.Scheme != "http")) {
		return "", fmt.Errorf("%w: object storage requires a plain HTTPS origin (HTTP only in local)", ErrInvalid)
	}
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	return u.String(), nil
}
