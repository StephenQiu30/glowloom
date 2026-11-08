// Package config loads and validates process configuration from GLOWLOOM_* environment variables.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ErrInvalid reports a configuration value that fails validation.
var ErrInvalid = errors.New("invalid configuration")

// Config is the validated configuration shared by every backend role.
type Config struct {
	AgentOrigin                    string
	AgentServiceToken              string
	AgentBusinessToken             string
	CanvasRuntimeURL               string
	CanvasRuntimeToken             string
	CodexBinary                    string
	CodexHome                      string
	CodexWorkRoot                  string
	Env                            string
	HTTPAddr                       string
	PublicOrigin                   string
	WorkerHealthAddr               string
	RelayHealthAddr                string
	LogLevel                       string
	DBDSN                          string
	PartitionMaintenanceDBDSN      string
	StorageMaintenanceDSN          string
	RedisURL                       string
	KafkaBrokers                   string
	ObjectStorage                  *ObjectStorageConfig
	CredentialPublicKeyPEM         string `json:"-" yaml:"-"`
	CredentialPrivateKeyPEM        string `json:"-" yaml:"-"`
	CredentialPublicKeyFile        string
	CredentialKeyID                string
	CredentialRequestMACKey        string
	CredentialPrivateKeyFile       string
	MediaResultAllowedOrigins      string
	MediaAllowTestLoopbackTLS      bool
	WhisperEndpoint                string
	VideoDepthPythonPath           string
	VideoDepthSourceDir            string
	VideoDepthModelPath            string
	VideoDepthDevice               string
	OTelEndpoint                   string
	SMTPEnabled                    bool
	SMTPHost                       string
	SMTPPort                       int
	SMTPTLSMode                    string
	SMTPUsername                   string
	SMTPPassword                   string
	SMTPFromEmail                  string
	SMTPFromName                   string
	IdentitySessionV2Enabled       bool
	IdentityLegacyUpgradeUntil     time.Time
	IdentityJWTPrivateKey          string
	IdentityJWTVerifyKeys          string
	IdentityJWTActiveKID           string
	IdentityRefreshReceiptKey      string
	IdentityKey                    string
	IdentityTermsVersion           string
	IdentityMailSourceHourlyLimit  int
	IdentityMailGlobalHourlyLimit  int
	CreditCommandRequestsPerMinute int
	CreditCommandMaxAmountMicros   int64
	CreditCommandMaxDailyMicros    int64
	CreditServiceCallers           []CreditServiceCaller
}

// CreditServiceCaller binds an internal service identity to explicit wallet
// scopes and command actions. Secrets are accepted only from runtime config.
type CreditServiceCaller struct {
	ID      string                   `json:"id"`
	Scopes  []string                 `json:"scopes"`
	Actions []string                 `json:"actions"`
	Keys    []CreditServiceCallerKey `json:"keys"`
}

// CreditServiceCallerKey supports overlap during HMAC key rotation.
type CreditServiceCallerKey struct {
	ID     string `json:"id"`
	Secret string `json:"secret" yaml:"-"`
}

var (
	validEnvs      = map[string]bool{"local": true, "staging": true, "prod": true}
	validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
)

// Load reads the optional environment file and process variables, then validates the result.
func Load() (Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.AllowEmptyEnv(true)
	v.SetDefault("GLOWLOOM_ENV", "local")
	v.SetDefault("GLOWLOOM_HTTP_ADDR", "127.0.0.1:8080")
	v.SetDefault("GLOWLOOM_PUBLIC_ORIGIN", "http://localhost:3000")
	v.SetDefault("GLOWLOOM_WORKER_HEALTH_ADDR", ":8081")
	v.SetDefault("GLOWLOOM_RELAY_HEALTH_ADDR", ":8082")
	v.SetDefault("GLOWLOOM_LOG_LEVEL", "info")
	v.SetDefault("GLOWLOOM_CREDIT_COMMANDS_PER_MINUTE", 120)
	if path := os.Getenv("GLOWLOOM_ENV_FILE"); path != "" {
		v.SetConfigFile(path)
		v.SetConfigType("env")
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("%w: read GLOWLOOM_ENV_FILE: %w", ErrInvalid, err)
		}
	}

	cfg := Config{
		AgentOrigin:                    strings.TrimSpace(v.GetString("GLOWLOOM_AGENT_ORIGIN")),
		AgentServiceToken:              strings.TrimSpace(v.GetString("GLOWLOOM_AGENT_SERVICE_TOKEN")),
		AgentBusinessToken:             strings.TrimSpace(v.GetString("GLOWLOOM_AGENT_BUSINESS_TOKEN")),
		CanvasRuntimeURL:               strings.TrimSpace(v.GetString("GLOWLOOM_CANVAS_RUNTIME_URL")),
		CanvasRuntimeToken:             v.GetString("GLOWLOOM_COLLAB_RUNTIME_TOKEN"),
		CodexBinary:                    strings.TrimSpace(v.GetString("GLOWLOOM_CODEX_BINARY")),
		CodexHome:                      strings.TrimSpace(v.GetString("GLOWLOOM_CODEX_HOME")),
		CodexWorkRoot:                  strings.TrimSpace(v.GetString("GLOWLOOM_CODEX_WORK_ROOT")),
		Env:                            strings.TrimSpace(v.GetString("GLOWLOOM_ENV")),
		HTTPAddr:                       strings.TrimSpace(v.GetString("GLOWLOOM_HTTP_ADDR")),
		PublicOrigin:                   strings.TrimSpace(v.GetString("GLOWLOOM_PUBLIC_ORIGIN")),
		WorkerHealthAddr:               strings.TrimSpace(v.GetString("GLOWLOOM_WORKER_HEALTH_ADDR")),
		RelayHealthAddr:                strings.TrimSpace(v.GetString("GLOWLOOM_RELAY_HEALTH_ADDR")),
		LogLevel:                       strings.TrimSpace(v.GetString("GLOWLOOM_LOG_LEVEL")),
		DBDSN:                          strings.TrimSpace(v.GetString("GLOWLOOM_DB_DSN")),
		PartitionMaintenanceDBDSN:      strings.TrimSpace(v.GetString("GLOWLOOM_PARTITION_MAINTENANCE_DB_DSN")),
		StorageMaintenanceDSN:          strings.TrimSpace(v.GetString("GLOWLOOM_STORAGE_MAINTENANCE_DSN")),
		RedisURL:                       strings.TrimSpace(v.GetString("GLOWLOOM_REDIS_URL")),
		KafkaBrokers:                   strings.TrimSpace(v.GetString("GLOWLOOM_KAFKA_BROKERS")),
		CredentialPublicKeyPEM:         v.GetString("GLOWLOOM_CREDENTIAL_PUBLIC_KEY_PEM"),
		CredentialPrivateKeyPEM:        v.GetString("GLOWLOOM_CREDENTIAL_PRIVATE_KEY_PEM"),
		CredentialPublicKeyFile:        strings.TrimSpace(v.GetString("GLOWLOOM_CREDENTIAL_PUBLIC_KEY_FILE")),
		CredentialKeyID:                strings.TrimSpace(v.GetString("GLOWLOOM_CREDENTIAL_KEY_ID")),
		CredentialRequestMACKey:        strings.TrimSpace(v.GetString("GLOWLOOM_CREDENTIAL_REQUEST_MAC_KEY")),
		CredentialPrivateKeyFile:       strings.TrimSpace(v.GetString("GLOWLOOM_CREDENTIAL_PRIVATE_KEY_FILE")),
		MediaResultAllowedOrigins:      strings.TrimSpace(v.GetString("GLOWLOOM_MEDIA_RESULT_ALLOWED_ORIGINS")),
		MediaAllowTestLoopbackTLS:      v.GetBool("GLOWLOOM_MEDIA_ALLOW_TEST_LOOPBACK_TLS"),
		WhisperEndpoint:                strings.TrimSpace(v.GetString("GLOWLOOM_WHISPER_ENDPOINT")),
		VideoDepthPythonPath:           strings.TrimSpace(v.GetString("GLOWLOOM_VIDEO_DEPTH_PYTHON_PATH")),
		VideoDepthSourceDir:            strings.TrimSpace(v.GetString("GLOWLOOM_VIDEO_DEPTH_SOURCE_DIR")),
		VideoDepthModelPath:            strings.TrimSpace(v.GetString("GLOWLOOM_VIDEO_DEPTH_MODEL_PATH")),
		VideoDepthDevice:               strings.TrimSpace(v.GetString("GLOWLOOM_VIDEO_DEPTH_DEVICE")),
		OTelEndpoint:                   strings.TrimSpace(v.GetString("GLOWLOOM_OTEL_ENDPOINT")),
		SMTPEnabled:                    v.GetBool("SMTP_ENABLED"),
		SMTPHost:                       strings.TrimSpace(v.GetString("SMTP_HOST")),
		SMTPPort:                       v.GetInt("SMTP_PORT"),
		SMTPTLSMode:                    strings.TrimSpace(v.GetString("SMTP_TLS_MODE")),
		SMTPUsername:                   v.GetString("SMTP_USERNAME"),
		SMTPPassword:                   v.GetString("SMTP_PASSWORD"),
		SMTPFromEmail:                  strings.TrimSpace(v.GetString("SMTP_FROM_EMAIL")),
		SMTPFromName:                   strings.TrimSpace(v.GetString("SMTP_FROM_NAME")),
		IdentitySessionV2Enabled:       v.GetBool("GLOWLOOM_IDENTITY_SESSION_V2_ENABLED"),
		IdentityLegacyUpgradeUntil:     v.GetTime("GLOWLOOM_IDENTITY_LEGACY_UPGRADE_UNTIL"),
		IdentityJWTPrivateKey:          strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_JWT_PRIVATE_KEY")),
		IdentityJWTVerifyKeys:          strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_JWT_VERIFY_KEYS")),
		IdentityJWTActiveKID:           strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_JWT_ACTIVE_KID")),
		IdentityRefreshReceiptKey:      strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_REFRESH_RECEIPT_KEY")),
		IdentityKey:                    strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_KEY")),
		IdentityTermsVersion:           strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_TERMS_VERSION")),
		IdentityMailSourceHourlyLimit:  v.GetInt("GLOWLOOM_IDENTITY_MAIL_SOURCE_HOURLY_LIMIT"),
		IdentityMailGlobalHourlyLimit:  v.GetInt("GLOWLOOM_IDENTITY_MAIL_GLOBAL_HOURLY_LIMIT"),
		CreditCommandRequestsPerMinute: v.GetInt("GLOWLOOM_CREDIT_COMMANDS_PER_MINUTE"),
		CreditCommandMaxAmountMicros:   v.GetInt64("GLOWLOOM_CREDIT_MAX_AMOUNT_MICROS"),
		CreditCommandMaxDailyMicros:    v.GetInt64("GLOWLOOM_CREDIT_MAX_DAILY_AMOUNT_MICROS"),
	}
	callers, err := parseCreditServiceCallers(v.GetString("GLOWLOOM_CREDIT_SERVICE_CALLERS_JSON"))
	if err != nil {
		return Config{}, err
	}
	cfg.CreditServiceCallers = callers
	if raw := strings.TrimSpace(v.GetString("GLOWLOOM_IDENTITY_LEGACY_UPGRADE_UNTIL")); raw != "" {
		deadline, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return Config{}, fmt.Errorf("%w: legacy upgrade deadline must be RFC3339", ErrInvalid)
		}
		cfg.IdentityLegacyUpgradeUntil = deadline
	}
	if err := cfg.configureStorage(v.GetString); err != nil {
		return Config{}, err
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.CredentialPublicKeyPEM != "" && c.CredentialPublicKeyFile != "" || c.CredentialPrivateKeyPEM != "" && c.CredentialPrivateKeyFile != "" || len(c.CredentialPublicKeyPEM) > 16384 || len(c.CredentialPrivateKeyPEM) > 16384 {
		return fmt.Errorf("%w: conflicting or oversized credential key source", ErrInvalid)
	}
	if c.CanvasRuntimeURL != "" || c.CanvasRuntimeToken != "" {
		endpoint, err := url.Parse(c.CanvasRuntimeURL)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || len(c.CanvasRuntimeToken) < 32 {
			return fmt.Errorf("%w: invalid internal canvas runtime configuration", ErrInvalid)
		}
	}

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
	if c.CreditCommandRequestsPerMinute < 0 || c.CreditCommandRequestsPerMinute > 1000000 || c.CreditCommandMaxAmountMicros < 0 || c.CreditCommandMaxDailyMicros < 0 {
		return fmt.Errorf("%w: credit command limits must be nonnegative and within supported range", ErrInvalid)
	}
	creditLimitsConfigured := c.CreditCommandMaxAmountMicros > 0 || c.CreditCommandMaxDailyMicros > 0
	if creditLimitsConfigured && (c.CreditCommandMaxAmountMicros == 0 || c.CreditCommandMaxDailyMicros == 0) {
		return fmt.Errorf("%w: credit command single and daily amount limits must be configured together", ErrInvalid)
	}
	if c.CreditCommandMaxAmountMicros > c.CreditCommandMaxDailyMicros {
		return fmt.Errorf("%w: credit command single amount limit cannot exceed the daily amount limit", ErrInvalid)
	}
	if len(c.CreditServiceCallers) > 0 && (c.CreditCommandRequestsPerMinute < 1 || !creditLimitsConfigured) {
		return fmt.Errorf("%w: credit command callers require request and amount limits", ErrInvalid)
	}
	if err := validateCreditServiceCallers(c.CreditServiceCallers); err != nil {
		return err
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
	if (c.CredentialPublicKeyFile == "" && c.CredentialPublicKeyPEM == "") != (c.CredentialKeyID == "") || len(c.CredentialKeyID) > 128 {
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

func parseCreditServiceCallers(raw string) ([]CreditServiceCaller, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 16384 {
		return nil, fmt.Errorf("%w: credit service caller configuration too large", ErrInvalid)
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var callers []CreditServiceCaller
	if err := decoder.Decode(&callers); err != nil {
		return nil, fmt.Errorf("%w: invalid credit service caller configuration", ErrInvalid)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing credit service caller configuration", ErrInvalid)
	}
	if err := validateCreditServiceCallers(callers); err != nil {
		return nil, err
	}
	return callers, nil
}

func validateCreditServiceCallers(callers []CreditServiceCaller) error {
	if len(callers) > 64 {
		return fmt.Errorf("%w: too many credit service callers", ErrInvalid)
	}
	callerIDs := make(map[string]struct{}, len(callers))
	for _, caller := range callers {
		if !validCreditCallerToken(caller.ID, 80) || len(caller.Scopes) == 0 || len(caller.Scopes) > 256 || len(caller.Actions) == 0 || len(caller.Actions) > 6 || len(caller.Keys) == 0 || len(caller.Keys) > 4 {
			return fmt.Errorf("%w: incomplete credit service caller", ErrInvalid)
		}
		if _, exists := callerIDs[caller.ID]; exists {
			return fmt.Errorf("%w: duplicate credit service caller", ErrInvalid)
		}
		callerIDs[caller.ID] = struct{}{}
		scopes := make(map[string]struct{}, len(caller.Scopes))
		for _, scope := range caller.Scopes {
			if !validUUID(scope) {
				return fmt.Errorf("%w: invalid credit service scope mapping", ErrInvalid)
			}
			if _, exists := scopes[scope]; exists {
				return fmt.Errorf("%w: duplicate credit service scope mapping", ErrInvalid)
			}
			scopes[scope] = struct{}{}
		}
		actions := make(map[string]struct{}, len(caller.Actions))
		for _, action := range caller.Actions {
			switch action {
			case "read", "grant", "debit", "freeze", "commit", "release":
			default:
				return fmt.Errorf("%w: invalid credit service action mapping", ErrInvalid)
			}
			if _, exists := actions[action]; exists {
				return fmt.Errorf("%w: duplicate credit service action mapping", ErrInvalid)
			}
			actions[action] = struct{}{}
		}
		keys := make(map[string]struct{}, len(caller.Keys))
		for _, key := range caller.Keys {
			if !validCreditCallerToken(key.ID, 80) || len(key.Secret) < 32 || len(key.Secret) > 256 || strings.TrimSpace(key.Secret) != key.Secret || strings.ContainsAny(key.Secret, "\r\n\t ") {
				return fmt.Errorf("%w: invalid credit service key", ErrInvalid)
			}
			if _, exists := keys[key.ID]; exists {
				return fmt.Errorf("%w: duplicate credit service key ID", ErrInvalid)
			}
			keys[key.ID] = struct{}{}
		}
	}
	return nil
}

func validCreditCallerToken(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, r := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
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
