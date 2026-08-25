package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIHost            = "0.0.0.0"
	defaultAPIPort            = 8686
	defaultJWTSecret          = "development-only-jwt-secret-change-before-production"
	defaultJWTIssuer          = "lanverse-api"
	defaultJWTAudience        = "lanverse-web"
	defaultAccessTokenMinutes = 30
	defaultSessionTTLSeconds  = 30 * 24 * 60 * 60
	defaultAgentURL           = "http://127.0.0.1:8787"
	defaultAgentSecret        = "development-only-agent-execution-secret"
	defaultAgentPollMillis    = 500
	defaultAgentLeaseSeconds  = 30 * 60
	defaultTemporalAddress    = "127.0.0.1:7233"
	defaultTemporalNamespace  = "default"
	defaultTemporalTaskQueue  = "lanverse-production-v1"
)

var numericVerificationCode = regexp.MustCompile(`^\d{6}$`)

type Config struct {
	ListenAddress                string
	DatabaseURL                  string
	JWTSecret                    string
	JWTIssuer                    string
	JWTAudience                  string
	AccessTokenTTL               time.Duration
	SessionTTL                   time.Duration
	Environment                  string
	RegistrationVerificationCode string
	AllowedOrigins               []string
	ObjectStoreEndpoint          string
	ObjectStorePublicEndpoint    string
	ObjectStoreAccessKey         string
	ObjectStoreSecretKey         string
	ObjectStoreBucket            string
	ObjectStoreRegion            string
	ObjectStoreSecure            bool
	ObjectStorePublicSecure      bool
	AgentURL                     string
	AgentExecutionSecret         string
	AgentPollInterval            time.Duration
	AgentClaimLease              time.Duration
	TemporalAddress              string
	TemporalNamespace            string
	TemporalTaskQueue            string
}

func Load() (Config, error) {
	host := environmentValue("API_HOST", defaultAPIHost)
	port, err := positiveInteger("API_PORT", defaultAPIPort)
	if err != nil {
		return Config{}, err
	}
	if port > 65535 {
		return Config{}, fmt.Errorf("API_PORT must not exceed 65535")
	}
	databaseURL, err := postgresURL(os.Getenv("DATABASE_URL"))
	if err != nil {
		return Config{}, err
	}
	accessMinutes, err := positiveInteger("JWT_ACCESS_TOKEN_MINUTES", defaultAccessTokenMinutes)
	if err != nil {
		return Config{}, err
	}
	sessionSeconds, err := positiveInteger("AUTH_SESSION_TTL_SECONDS", defaultSessionTTLSeconds)
	if err != nil {
		return Config{}, err
	}
	verificationCode := os.Getenv("REGISTRATION_VERIFICATION_CODE")
	if verificationCode != "" && !numericVerificationCode.MatchString(verificationCode) {
		return Config{}, errors.New("REGISTRATION_VERIFICATION_CODE must contain exactly 6 digits")
	}
	allowedOrigins, err := stringList("CORS_ORIGINS", []string{"http://localhost:8123", "http://127.0.0.1:8123"})
	if err != nil {
		return Config{}, err
	}
	objectStoreSecure, err := boolean("MINIO_SECURE", false)
	if err != nil {
		return Config{}, err
	}
	objectStorePublicSecure, err := boolean("MINIO_PUBLIC_SECURE", false)
	if err != nil {
		return Config{}, err
	}
	agentPollMillis, err := positiveInteger("AGENT_POLL_INTERVAL_MS", defaultAgentPollMillis)
	if err != nil {
		return Config{}, err
	}
	agentLeaseSeconds, err := positiveInteger("AGENT_CLAIM_LEASE_SECONDS", defaultAgentLeaseSeconds)
	if err != nil {
		return Config{}, err
	}
	temporalAddress, err := hostPort("TEMPORAL_ADDRESS", defaultTemporalAddress)
	if err != nil {
		return Config{}, err
	}
	temporalNamespace, err := boundedName("TEMPORAL_NAMESPACE", defaultTemporalNamespace, 255)
	if err != nil {
		return Config{}, err
	}
	temporalTaskQueue, err := boundedName("TEMPORAL_TASK_QUEUE", defaultTemporalTaskQueue, 255)
	if err != nil {
		return Config{}, err
	}
	return Config{
		ListenAddress:                net.JoinHostPort(host, strconv.Itoa(port)),
		DatabaseURL:                  databaseURL,
		JWTSecret:                    environmentValue("JWT_SECRET_KEY", defaultJWTSecret),
		JWTIssuer:                    environmentValue("JWT_ISSUER", defaultJWTIssuer),
		JWTAudience:                  environmentValue("JWT_AUDIENCE", defaultJWTAudience),
		AccessTokenTTL:               time.Duration(accessMinutes) * time.Minute,
		SessionTTL:                   time.Duration(sessionSeconds) * time.Second,
		Environment:                  environmentValue("ENVIRONMENT", "development"),
		RegistrationVerificationCode: verificationCode,
		AllowedOrigins:               allowedOrigins,
		ObjectStoreEndpoint:          environmentValue("MINIO_ENDPOINT", "127.0.0.1:9000"),
		ObjectStorePublicEndpoint:    environmentValue("MINIO_PUBLIC_ENDPOINT", "127.0.0.1:9000"),
		ObjectStoreAccessKey:         environmentValue("MINIO_ACCESS_KEY", "lanverse"),
		ObjectStoreSecretKey:         environmentValue("MINIO_SECRET_KEY", "lanverse-development-only"),
		ObjectStoreBucket:            environmentValue("MINIO_BUCKET", "lanverse-media"),
		ObjectStoreRegion:            environmentValue("MINIO_REGION", "us-east-1"),
		ObjectStoreSecure:            objectStoreSecure,
		ObjectStorePublicSecure:      objectStorePublicSecure,
		AgentURL:                     environmentValue("AGENT_URL", defaultAgentURL),
		AgentExecutionSecret:         environmentValue("AGENT_EXECUTION_SECRET", defaultAgentSecret),
		AgentPollInterval:            time.Duration(agentPollMillis) * time.Millisecond,
		AgentClaimLease:              time.Duration(agentLeaseSeconds) * time.Second,
		TemporalAddress:              temporalAddress,
		TemporalNamespace:            temporalNamespace,
		TemporalTaskQueue:            temporalTaskQueue,
	}, nil
}

func hostPort(name, fallback string) (string, error) {
	rawValue := strings.TrimSpace(environmentValue(name, fallback))
	host, rawPort, err := net.SplitHostPort(rawValue)
	if err != nil || strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("%s must be a host:port address", name)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("%s must use a port between 1 and 65535", name)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func boundedName(name, fallback string, maximum int) (string, error) {
	value := strings.TrimSpace(environmentValue(name, fallback))
	if value == "" || len(value) > maximum {
		return "", fmt.Errorf("%s must contain between 1 and %d characters", name, maximum)
	}
	return value, nil
}

func boolean(name string, fallback bool) (bool, error) {
	rawValue := environmentValue(name, strconv.FormatBool(fallback))
	value, err := strconv.ParseBool(rawValue)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return value, nil
}

func stringList(name string, fallback []string) ([]string, error) {
	rawValue := os.Getenv(name)
	if rawValue == "" {
		return fallback, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(rawValue), &values); err != nil || len(values) == 0 {
		return nil, fmt.Errorf("%s must be a non-empty JSON string array", name)
	}
	for _, value := range values {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" {
			return nil, fmt.Errorf("%s contains an invalid origin", name)
		}
	}
	return values, nil
}

func postgresURL(rawValue string) (string, error) {
	if rawValue == "" {
		return "", errors.New("DATABASE_URL is required")
	}
	parsed, err := url.Parse(rawValue)
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", errors.New("DATABASE_URL must use the postgres or postgresql scheme")
	}
	if parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" {
		return "", errors.New("DATABASE_URL must include a host and database name")
	}
	return parsed.String(), nil
}

func environmentValue(name string, fallback string) string {
	value, exists := os.LookupEnv(name)
	if !exists || value == "" {
		return fallback
	}
	return value
}

func positiveInteger(name string, fallback int) (int, error) {
	rawValue := environmentValue(name, strconv.Itoa(fallback))
	value, err := strconv.Atoi(rawValue)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}
