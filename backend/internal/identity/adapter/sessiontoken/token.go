// Package sessiontoken implements fixed-purpose access credentials and refresh receipts.
package sessiontoken

import (
	"crypto/rsa"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

const issuer = "glowloom.identity"
const audience = "glowloom.web"

// Codec signs and verifies fixed-purpose RS256 access credentials.
type Codec struct {
	kid     string
	private *rsa.PrivateKey
	keys    map[string]*rsa.PublicKey
}
type claims struct {
	jwt.RegisteredClaims
	SessionID       uuid.UUID `json:"sid"`
	CredentialEpoch int64     `json:"credential_epoch"`
}

// New requires a signing key. Verification-only rollback uses NewVerifier.
func New(kid string, private []byte, public map[string][]byte) (*Codec, error) {
	c, err := NewVerifier(public)
	if err != nil {
		return nil, err
	}
	k, err := jwt.ParseRSAPrivateKeyFromPEM(private)
	if err != nil || k.N.BitLen() < 2048 {
		return nil, errors.New("invalid access signing key")
	}
	p := c.keys[kid]
	if p == nil || p.N.Cmp(k.N) != 0 || p.E != k.E {
		return nil, errors.New("active access key mismatch")
	}
	c.kid = kid
	c.private = k
	return c, nil
}

// NewVerifier retains public-key validation when issuance is disabled.
func NewVerifier(public map[string][]byte) (*Codec, error) {
	c := &Codec{keys: make(map[string]*rsa.PublicKey)}
	if len(public) == 0 {
		return nil, errors.New("access verification keys required")
	}
	for id, b := range public {
		p, e := jwt.ParseRSAPublicKeyFromPEM(b)
		if e != nil || p.N.BitLen() < 2048 || id == "" || len(id) > 128 {
			return nil, errors.New("invalid access verification key")
		}
		c.keys[id] = p
	}
	return c, nil
}
func valid(c app.AccessClaims) bool {
	return c.UserID != uuid.Nil && c.SessionID != uuid.Nil && c.TokenID != uuid.Nil && c.CredentialEpoch > 0 && !c.IssuedAt.IsZero() && !c.NotBefore.IsZero() && c.ExpiresAt.After(c.IssuedAt) && c.ExpiresAt.Sub(c.IssuedAt) <= 15*time.Minute && !c.NotBefore.After(c.ExpiresAt)
}

// CanSign reports whether this codec has a configured private signing key.
func (c *Codec) CanSign() bool { return c != nil && c.private != nil }

// Sign issues an access credential bounded to fifteen minutes.
func (c *Codec) Sign(a app.AccessClaims) (string, error) {
	if c.private == nil {
		return "", app.ErrRefreshDependency
	}
	if !valid(a) {
		return "", app.ErrAccessInvalid
	}
	v := claims{jwt.RegisteredClaims{Issuer: issuer, Audience: jwt.ClaimStrings{audience}, Subject: a.UserID.String(), ID: a.TokenID.String(), IssuedAt: jwt.NewNumericDate(a.IssuedAt), NotBefore: jwt.NewNumericDate(a.NotBefore), ExpiresAt: jwt.NewNumericDate(a.ExpiresAt)}, a.SessionID, a.CredentialEpoch}
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, v)
	t.Header["typ"] = "at+jwt"
	t.Header["kid"] = c.kid
	return t.SignedString(c.private)
}

// Verify authenticates the token before classifying time expiry.
func (c *Codec) Verify(raw string, now time.Time) (app.AccessClaims, error) {
	var v claims
	// Authenticate all non-time claims before classifying expiry as recoverable.
	t, err := jwt.ParseWithClaims(raw, &v, func(t *jwt.Token) (any, error) {
		id, ok := t.Header["kid"].(string)
		if !ok || t.Header["typ"] != "at+jwt" || c.keys[id] == nil {
			return nil, app.ErrAccessInvalid
		}
		return c.keys[id], nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithoutClaimsValidation())
	if err != nil || !t.Valid || v.Issuer != issuer || len(v.Audience) != 1 || v.Audience[0] != audience || v.IssuedAt == nil || v.NotBefore == nil || v.ExpiresAt == nil {
		return app.AccessClaims{}, app.ErrAccessInvalid
	}
	u, e := uuid.Parse(v.Subject)
	if e != nil {
		return app.AccessClaims{}, app.ErrAccessInvalid
	}
	id, e := uuid.Parse(v.ID)
	if e != nil {
		return app.AccessClaims{}, app.ErrAccessInvalid
	}
	a := app.AccessClaims{UserID: u, SessionID: v.SessionID, TokenID: id, CredentialEpoch: v.CredentialEpoch, IssuedAt: v.IssuedAt.Time, NotBefore: v.NotBefore.Time, ExpiresAt: v.ExpiresAt.Time}
	if !valid(a) || a.IssuedAt.After(now.Add(30*time.Second)) || a.NotBefore.After(now.Add(30*time.Second)) {
		return app.AccessClaims{}, app.ErrAccessInvalid
	}
	if !now.Before(a.ExpiresAt.Add(30 * time.Second)) {
		return app.AccessClaims{}, app.ErrAccessExpired
	}
	return a, nil
}
