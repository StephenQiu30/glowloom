// Package config loads and validates process configuration from GLOWLOOM_* environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ErrInvalid reports a configuration value that fails validation.
var ErrInvalid = errors.New("invalid configuration")

// Config is the validated configuration shared by every backend role.
type Config struct {
	CodexBinary                   string
	CodexHome                     string
	CodexWorkRoot                 string
	Env                           string
	HTTPAddr                      string
	PublicOrigin                  string
	WorkerHealthAddr              string
	RelayHealthAddr               string
	LogLevel                      string
	DBDSN                         string
	PartitionMaintenanceDBDSN     string
	RedisURL                      string
	KafkaBrokers                  string
	TemporalAddr                  string
	TemporalNamespace             string
	ObjectStorageEndpoint         string
	ObjectStorageBucket           string
	ObjectStorageAccessKey        string
	ObjectStorageSecretKey        string
	ObjectStorageRegion           string
	ObjectStorage                 *ObjectStorageConfig
	CredentialPublicKeyFile       string
	CredentialKeyID               string
	CredentialRequestMACKey       string
	CredentialPrivateKeyFile      string
	MediaResultAllowedOrigins     string
	MediaAllowTestLoopbackTLS     bool
	WhisperEndpoint               string
	VideoDepthPythonPath          string
	VideoDepthSourceDir           string
	VideoDepthModelPath           string
	VideoDepthDevice              string
	OTelEndpoint                  string
	SMTPEnabled                   bool
	SMTPHost                      string
	SMTPPort                      int
	SMTPTLSMode                   string
	SMTPUsername                  string
	SMTPPassword                  string
	SMTPFromEmail                 string
	SMTPFromName                  string
	IdentitySessionV2Enabled      bool
	IdentityLegacyUpgradeUntil    time.Time
	IdentityJWTPrivateKey         string
	IdentityJWTVerifyKeys         string
	IdentityJWTActiveKID          string
	IdentityRefreshReceiptKey     string
	IdentityKey                   string
	IdentityTermsVersion          string
	IdentityMailSourceHourlyLimit int
	IdentityMailGlobalHourlyLimit int
}

var (
	validEnvs      = map[string]bool{"local": true, "staging": true, "prod": true}
	validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
)

// Load reads the optional environment file and process variables, then validates the result.
func Load() (Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetDefault("GLOWLOOM_ENV", "local")
	v.SetDefault("GLOWLOOM_HTTP_ADDR", "127.0.0.1:8080")
	v.SetDefault("GLOWLOOM_PUBLIC_ORIGIN", "http://localhost:3000")
	v.SetDefault("GLOWLOOM_WORKER_HEALTH_ADDR", ":8081")
	v.SetDefault("GLOWLOOM_RELAY_HEALTH_ADDR", ":8082")
	v.SetDefault("GLOWLOOM_LOG_LEVEL", "info")
	if path := environmentFile(); path != "" {
		v.SetConfigFile(path)
		v.SetConfigType("env")
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("%w: read GLOWLOOM_ENV_FILE: %w", ErrInvalid, err)
		}
	}

	applyLegacyEnvironment(v)

	cfg := Config{
		CodexBinary:                   strings.TrimSpace(v.GetString("GLOWLOOM_CODEX_BINARY")),
		CodexHome:                     strings.TrimSpace(v.GetString("GLOWLOOM_CODEX_HOME")),
		CodexWorkRoot:                 strings.TrimSpace(v.GetString("GLOWLOOM_CODEX_WORK_ROOT")),
		Env:                           strings.TrimSpace(v.GetString("GLOWLOOM_ENV")),
		HTTPAddr:                      strings.TrimSpace(v.GetString("GLOWLOOM_HTTP_ADDR")),
		PublicOrigin:                  strings.TrimSpace(v.GetString("GLOWLOOM_PUBLIC_ORIGIN")),
		WorkerHealthAddr:              strings.TrimSpace(v.GetString("GLOWLOOM_WORKER_HEALTH_ADDR")),
		RelayHealthAddr:               strings.TrimSpace(v.GetString("GLOWLOOM_RELAY_HEALTH_ADDR")),
		LogLevel:                      strings.TrimSpace(v.GetString("GLOWLOOM_LOG_LEVEL")),
		DBDSN:                         strings.TrimSpace(v.GetString("GLOWLOOM_DB_DSN")),
		PartitionMaintenanceDBDSN:     strings.TrimSpace(v.GetString("GLOWLOOM_PARTITION_MAINTENANCE_DB_DSN")),
		RedisURL:                      strings.TrimSpace(v.GetString("GLOWLOOM_REDIS_URL")),
		KafkaBrokers:                  strings.TrimSpace(v.GetString("GLOWLOOM_KAFKA_BROKERS")),
		TemporalAddr:                  strings.TrimSpace(v.GetString("GLOWLOOM_TEMPORAL_ADDR")),
		TemporalNamespace:             strings.TrimSpace(v.GetString("GLOWLOOM_TEMPORAL_NAMESPACE")),
		ObjectStorageEndpoint:         strings.TrimSpace(v.GetString("GLOWLOOM_OBJECT_STORAGE_ENDPOINT")),
		ObjectStorageBucket:           strings.TrimSpace(v.GetString("GLOWLOOM_OBJECT_STORAGE_BUCKET")),
		ObjectStorageAccessKey:        v.GetString("GLOWLOOM_OBJECT_STORAGE_ACCESS_KEY"),
		ObjectStorageSecretKey:        v.GetString("GLOWLOOM_OBJECT_STORAGE_SECRET_KEY"),
		ObjectStorageRegion:           strings.TrimSpace(v.GetString("GLOWLOOM_OBJECT_STORAGE_REGION")),
		CredentialPublicKeyFile:       strings.TrimSpace(v.GetString("GLOWLOOM_CREDENTIAL_PUBLIC_KEY_FILE")),
		CredentialKeyID:               strings.TrimSpace(v.GetString("GLOWLOOM_CREDENTIAL_KEY_ID")),
		CredentialRequestMACKey:       strings.TrimSpace(v.GetString("GLOWLOOM_CREDENTIAL_REQUEST_MAC_KEY")),
		CredentialPrivateKeyFile:      strings.TrimSpace(v.GetString("GLOWLOOM_CREDENTIAL_PRIVATE_KEY_FILE")),
		MediaResultAllowedOrigins:     strings.TrimSpace(v.GetString("GLOWLOOM_MEDIA_RESULT_ALLOWED_ORIGINS")),
		MediaAllowTestLoopbackTLS:     v.GetBool("GLOWLOOM_MEDIA_ALLOW_TEST_LOOPBACK_TLS"),
		WhisperEndpoint:               strings.TrimSpace(v.GetString("GLOWLOOM_WHISPER_ENDPOINT")),
		VideoDepthPythonPath:          strings.TrimSpace(v.GetString("GLOWLOOM_VIDEO_DEPTH_PYTHON_PATH")),
		VideoDepthSourceDir:           strings.TrimSpace(v.GetString("GLOWLOOM_VIDEO_DEPTH_SOURCE_DIR")),
		VideoDepthModelPath:           strings.TrimSpace(v.GetString("GLOWLOOM_VIDEO_DEPTH_MODEL_PATH")),
		VideoDepthDevice:              strings.TrimSpace(v.GetString("GLOWLOOM_VIDEO_DEPTH_DEVICE")),
		OTelEndpoint:                  strings.TrimSpace(v.GetString("GLOWLOOM_OTEL_ENDPOINT")),
		SMTPEnabled:                   v.GetBool("SMTP_ENABLED"),
		SMTPHost:                      strings.TrimSpace(v.GetString("SMTP_HOST")),
		SMTPPort:                      v.GetInt("SMTP_PORT"),
		SMTPTLSMode:                   strings.TrimSpace(v.GetString("SMTP_TLS_MODE")),
		SMTPUsername:                  v.GetString("SMTP_USERNAME"),
		SMTPPassword:                  v.GetString("SMTP_PASSWORD"),
		SMTPFromEmail:                 strings.TrimSpace(v.GetString("SMTP_FROM_EMAIL")),
		SMTPFromName:                  strings.TrimSpace(v.GetString("SMTP_FROM_NAME")),
		IdentitySessionV2Enabled:      v.GetBool("GLOWLOOM_IDENTITY_SESSION_V2_ENABLED"),
		IdentityLegacyUpgradeUntil:    v.GetTime("GLOWLOOM_IDENTITY_LEGACY_UPGRADE_UNTIL"),
		IdentityJWTPrivateKey:         strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_JWT_PRIVATE_KEY")),
		IdentityJWTVerifyKeys:         strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_JWT_VERIFY_KEYS")),
		IdentityJWTActiveKID:          strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_JWT_ACTIVE_KID")),
		IdentityRefreshReceiptKey:     strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_REFRESH_RECEIPT_KEY")),
		IdentityKey:                   strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_KEY")),
		IdentityTermsVersion:          strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_TERMS_VERSION")),
		IdentityMailSourceHourlyLimit: v.GetInt("GLOWLOOM_IDENTITY_MAIL_SOURCE_HOURLY_LIMIT"),
		IdentityMailGlobalHourlyLimit: v.GetInt("GLOWLOOM_IDENTITY_MAIL_GLOBAL_HOURLY_LIMIT"),
	}
	if raw := strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_LEGACY_UPGRADE_UNTIL")); raw != "" {
		deadline, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return Config{}, fmt.Errorf("%w: legacy upgrade deadline must be RFC3339", ErrInvalid)
		}
		cfg.IdentityLegacyUpgradeUntil = deadline
	}
	if path := strings.TrimSpace(v.GetString("GLOWLOOM_OBJECT_STORAGE_CONFIG")); path != "" {
		if cfg.ObjectStorageEndpoint != "" || cfg.ObjectStorageBucket != "" || cfg.ObjectStorageAccessKey != "" || cfg.ObjectStorageSecretKey != "" || cfg.ObjectStorageRegion != "" {
			return Config{}, fmt.Errorf("%w: YAML and legacy object storage settings conflict", ErrInvalid)
		}
		storage, err := LoadObjectStorage(path, cfg.Env)
		if err != nil {
			return Config{}, err
		}
		cfg.ObjectStorage = &storage
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if err := c.ValidateIdentitySessionV2(); err != nil {
		return err
	}

	if len(c.IdentityKey) > 4096 {
		return fmt.Errorf("%w: identity key configuration too large", ErrInvalid)
	}
	for _, limit := range []int{c.IdentityMailSourceHourlyLimit, c.IdentityMailGlobalHourlyLimit} {
		if limit < 0 || limit > 1000000 {
			return fmt.Errorf("%w: identity mail limit outside 0..1000000", ErrInvalid)
		}
	}
	if c.SMTPEnabled && (c.SMTPHost == "" || c.SMTPPort < 1 || c.SMTPPort > 65535 || (c.SMTPTLSMode != "tls" && c.SMTPTLSMode != "starttls") || c.SMTPFromEmail == "" || (c.SMTPUsername == "") != (c.SMTPPassword == "")) {
		return fmt.Errorf("%w: SMTP requires host, port, TLS, sender and paired credentials", ErrInvalid)
	}
	depthValues := []string{c.VideoDepthPythonPath, c.VideoDepthSourceDir, c.VideoDepthModelPath, c.VideoDepthDevice}
	depthConfigured := 0
	for _, value := range depthValues {
		if value != "" {
			depthConfigured++
		}
	}
	if depthConfigured != 0 {
		if depthConfigured != len(depthValues) {
			return fmt.Errorf("%w: video depth requires Python, source, model and device together", ErrInvalid)
		}
		for _, path := range depthValues[:3] {
			if !filepath.IsAbs(path) {
				return fmt.Errorf("%w: video depth runtime paths must be absolute", ErrInvalid)
			}
		}
		if c.VideoDepthDevice != "mps" {
			return fmt.Errorf("%w: video depth requires the verified mps device profile", ErrInvalid)
		}
	}
	if (c.CredentialPublicKeyFile == "") != (c.CredentialKeyID == "") || len(c.CredentialKeyID) > 128 {
		return fmt.Errorf("%w: credential public key file and key ID must be configured together", ErrInvalid)
	}
	if c.MediaAllowTestLoopbackTLS && c.Env != "local" {
		return fmt.Errorf("%w: GLOWLOOM_MEDIA_ALLOW_TEST_LOOPBACK_TLS is local-only", ErrInvalid)
	}
	if !validEnvs[c.Env] {
		return fmt.Errorf("%w: GLOWLOOM_ENV=%q, want local|staging|prod", ErrInvalid, c.Env)
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("%w: GLOWLOOM_LOG_LEVEL=%q, want debug|info|warn|error", ErrInvalid, c.LogLevel)
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("%w: GLOWLOOM_HTTP_ADDR is empty", ErrInvalid)
	}
	if c.WorkerHealthAddr == "" {
		return fmt.Errorf("%w: GLOWLOOM_WORKER_HEALTH_ADDR is empty", ErrInvalid)
	}
	if c.RelayHealthAddr == "" {
		return fmt.Errorf("%w: GLOWLOOM_RELAY_HEALTH_ADDR is empty", ErrInvalid)
	}
	return nil
}

// ValidatePublicOrigin requires a configured browser origin, HTTPS outside local development.
func ValidatePublicOrigin(origin, env string) error {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: GLOWLOOM_PUBLIC_ORIGIN must be an exact HTTP origin", ErrInvalid)
	}
	if u.Scheme == "http" && (env != "local" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1")) {
		return fmt.Errorf("%w: HTTP public origin is local-loopback only", ErrInvalid)
	}
	return nil
}
