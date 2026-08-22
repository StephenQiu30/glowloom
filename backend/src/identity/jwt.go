package identity

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
)

type AccessClaims struct {
	WorkspaceID string          `json:"workspace_id"`
	SessionID   string          `json:"session_id"`
	TokenType   AccessTokenType `json:"token_type"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret    []byte
	issuer    string
	audience  string
	accessTTL time.Duration
}

func NewJWTManager(secret, issuer, audience string, accessTTL time.Duration) (*JWTManager, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" || secret == insecureJWTSecretPlaceholder || len(secret) < 32 {
		return nil, errors.New("JWT secret must contain at least 32 non-placeholder characters")
	}
	if strings.TrimSpace(issuer) == "" || strings.TrimSpace(audience) == "" {
		return nil, errors.New("JWT issuer and audience are required")
	}
	if accessTTL <= 0 || accessTTL > time.Hour {
		return nil, errors.New("JWT access TTL must be between 1 second and 1 hour")
	}
	return &JWTManager{secret: []byte(secret), issuer: issuer, audience: audience, accessTTL: accessTTL}, nil
}

func NewJWTManagerFromEnv() (*JWTManager, error) {
	accessTTL, err := toolkit.DurationEnv("AUTH_ACCESS_TTL", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	return NewJWTManager(os.Getenv("AUTH_JWT_SECRET"), toolkit.EnvOr("AUTH_JWT_ISSUER", "lanverse"), toolkit.EnvOr("AUTH_JWT_AUDIENCE", "lanverse-admin"), accessTTL)
}

type AuthConfig struct {
	AccessTTL           time.Duration
	RefreshTTL          time.Duration
	RefreshCookieName   string
	RefreshCookiePath   string
	RefreshCookieDomain string
	RefreshCookieSecure bool
}

func AuthConfigFromEnv() (AuthConfig, error) {
	accessTTL, err := toolkit.DurationEnv("AUTH_ACCESS_TTL", 15*time.Minute)
	if err != nil {
		return AuthConfig{}, err
	}
	refreshTTL, err := toolkit.DurationEnv("AUTH_REFRESH_TTL", 30*24*time.Hour)
	if err != nil {
		return AuthConfig{}, err
	}
	secure, err := toolkit.BoolEnv("AUTH_COOKIE_SECURE", false)
	if err != nil {
		return AuthConfig{}, err
	}
	if accessTTL <= 0 || refreshTTL <= accessTTL {
		return AuthConfig{}, errors.New("AUTH_REFRESH_TTL must be greater than AUTH_ACCESS_TTL")
	}
	if strings.EqualFold(toolkit.EnvOr("ENVIRONMENT", "development"), "production") && !secure {
		return AuthConfig{}, errors.New("AUTH_COOKIE_SECURE must be true in production")
	}
	return AuthConfig{
		AccessTTL:           accessTTL,
		RefreshTTL:          refreshTTL,
		RefreshCookieName:   toolkit.EnvOr("AUTH_REFRESH_COOKIE", "lanverse_refresh"),
		RefreshCookiePath:   "/api/auth",
		RefreshCookieDomain: strings.TrimSpace(os.Getenv("AUTH_COOKIE_DOMAIN")),
		RefreshCookieSecure: secure,
	}, nil
}

func (m *JWTManager) Issue(session SessionIssue, now time.Time) (string, time.Time, error) {
	if m == nil {
		return "", time.Time{}, errors.New("JWT manager is not configured")
	}
	if session.SessionID == uuid.Nil || session.Identity.Account.ID == uuid.Nil || session.Identity.Workspace.ID == uuid.Nil {
		return "", time.Time{}, errors.New("session identity is incomplete")
	}
	now = now.UTC()
	expiresAt := now.Add(m.accessTTL)
	claims := AccessClaims{
		WorkspaceID: session.Identity.Workspace.ID.String(),
		SessionID:   session.SessionID.String(),
		TokenType:   AccessTokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   session.Identity.Account.ID.String(),
			Audience:  jwt.ClaimStrings{m.audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return raw, expiresAt, nil
}

func (m *JWTManager) Parse(raw string) (AccessClaims, error) {
	if m == nil {
		return AccessClaims{}, errors.New("JWT manager is not configured")
	}
	var claims AccessClaims
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if token.Method == nil {
			return nil, errors.New("missing JWT signing method")
		}
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected JWT signing method %s", token.Method.Alg())
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(m.issuer), jwt.WithAudience(m.audience))
	if err != nil || token == nil || !token.Valid {
		return AccessClaims{}, errors.New("invalid access token")
	}
	if claims.TokenType != AccessTokenTypeAccess || claims.ExpiresAt == nil || claims.IssuedAt == nil || claims.NotBefore == nil {
		return AccessClaims{}, errors.New("invalid access token claims")
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return AccessClaims{}, errors.New("invalid access token subject")
	}
	if _, err := uuid.Parse(claims.WorkspaceID); err != nil {
		return AccessClaims{}, errors.New("invalid access token workspace")
	}
	if _, err := uuid.Parse(claims.SessionID); err != nil {
		return AccessClaims{}, errors.New("invalid access token session")
	}
	return claims, nil
}

const insecureJWTSecretPlaceholder = "replace-with-at-least-32-random-characters"
