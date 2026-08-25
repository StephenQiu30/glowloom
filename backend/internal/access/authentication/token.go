package authentication

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrUnauthenticated = errors.New("invalid credentials")

type Claims struct {
	UserID       string
	TokenVersion int
}

type accessClaims struct {
	TokenVersion int    `json:"ver"`
	TokenType    string `json:"type"`
	jwt.RegisteredClaims
}

type Verifier struct {
	secret           []byte
	issuer, audience string
	now              func() time.Time
}

type Issuer struct {
	secret           []byte
	issuer, audience string
	ttl              time.Duration
	now              func() time.Time
	newID            func() string
}

func NewVerifier(secret, issuer, audience string, now func() time.Time) *Verifier {
	return &Verifier{secret: []byte(secret), issuer: issuer, audience: audience, now: now}
}

func NewIssuer(secret, issuer, audience string, ttl time.Duration, now func() time.Time, newID func() string) *Issuer {
	return &Issuer{secret: []byte(secret), issuer: issuer, audience: audience, ttl: ttl, now: now, newID: newID}
}

func (issuer *Issuer) Issue(userID string, tokenVersion int) (string, error) {
	issuedAt := issuer.now().UTC()
	claims := accessClaims{
		TokenVersion: tokenVersion,
		TokenType:    "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer.issuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{issuer.audience},
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(issuer.ttl)),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ID:        issuer.newID(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(issuer.secret)
}

func (verifier *Verifier) Authenticate(request *http.Request) (Claims, error) {
	header := request.Header.Get("Authorization")
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return Claims{}, ErrUnauthenticated
	}
	return verifier.Verify(token)
}

func (verifier *Verifier) Verify(value string) (Claims, error) {
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(
		value,
		claims,
		func(token *jwt.Token) (any, error) { return verifier.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(verifier.issuer),
		jwt.WithAudience(verifier.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(verifier.now),
	)
	if err != nil || !token.Valid || claims.TokenType != "access" || claims.TokenVersion < 1 || claims.Subject == "" || claims.ID == "" {
		return Claims{}, ErrUnauthenticated
	}
	if _, err = uuid.Parse(claims.Subject); err != nil {
		return Claims{}, ErrUnauthenticated
	}
	return Claims{UserID: claims.Subject, TokenVersion: claims.TokenVersion}, nil
}
