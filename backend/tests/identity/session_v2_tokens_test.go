package identity_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/google/uuid"

	token "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/sessiontoken"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func TestSessionV2Tokens(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	public := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})
	codec, err := token.New("active", private, map[string][]byte{"active": public})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	claims := app.AccessClaims{UserID: uuid.New(), SessionID: uuid.New(), TokenID: uuid.New(), CredentialEpoch: 1, IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(15 * time.Minute)}
	raw, err := codec.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	got, err := codec.Verify(raw, now)
	if err != nil || got.UserID != claims.UserID {
		t.Fatalf("roundtrip %v", err)
	}
	parsed, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		t.Fatal(err)
	}
	body, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims type")
	}
	for _, name := range []string{"platform_role", "role", "team_role", "status", "email"} {
		if _, present := body[name]; present {
			t.Fatalf("access token carries %s", name)
		}
	}
	if _, err = codec.Verify(raw, claims.ExpiresAt.Add(31*time.Second)); !errors.Is(err, app.ErrAccessExpired) {
		t.Fatalf("expiry %v", err)
	}
	if _, err = codec.Verify(raw, claims.ExpiresAt.Add(29*time.Second)); err != nil {
		t.Fatalf("JWT leeway: %v", err)
	}
	claims.ExpiresAt = now.Add(16 * time.Minute)
	if _, err = codec.Sign(claims); err == nil {
		t.Fatal("overlong")
	}
	if _, err = token.New("active", nil, map[string][]byte{"active": public}); err == nil {
		t.Fatal("missing private")
	}
	p, err := token.NewReceiptProtector(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sid, op := uuid.New(), uuid.New()
	sealed, err := p.Seal(sid, 1, op, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Open(sid, 1, op, sealed); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		s uuid.UUID
		g int64
		o uuid.UUID
	}{{uuid.New(), 1, op}, {sid, 2, op}, {sid, 1, uuid.New()}} {
		if _, err = p.Open(v.s, v.g, v.o, sealed); err == nil {
			t.Fatal("AAD accepted")
		}
	}
}

func TestSessionV2Configuration(t *testing.T) {
	t.Setenv("LV_IDENTITY_SESSION_V2_ENABLED", "true")
	if _, err := config.Load(); err == nil {
		t.Fatal("enabled without keys")
	}
}

func TestSessionV2TokensRejectHeaders(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	public := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})
	c, err := token.NewVerifier(map[string][]byte{"old": public})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	claims := jwt.MapClaims{"iss": "glowloom.identity", "aud": "glowloom.web", "sub": uuid.NewString(), "sid": uuid.NewString(), "jti": uuid.NewString(), "credential_epoch": 1, "iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(time.Minute).Unix()}
	sign := func(method jwt.SigningMethod, kid, typ string, k any) string {
		v := jwt.NewWithClaims(method, claims)
		v.Header["kid"] = kid
		v.Header["typ"] = typ
		raw, e := v.SignedString(k)
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	raw := sign(jwt.SigningMethodRS256, "old", "at+jwt", key)
	if _, err = c.Verify(raw, now); err != nil {
		t.Fatal("old public key rejected", err)
	}
	for _, raw := range []string{sign(jwt.SigningMethodRS256, "unknown", "at+jwt", key), sign(jwt.SigningMethodRS256, "old", "JWT", key), sign(jwt.SigningMethodHS256, "old", "at+jwt", []byte("secret")), sign(jwt.SigningMethodNone, "old", "at+jwt", jwt.UnsafeAllowNoneSignatureType)} {
		if _, err = c.Verify(raw, now); !errors.Is(err, app.ErrAccessInvalid) {
			t.Fatalf("bad header %v", err)
		}
	}
	claims["exp"] = now.Add(-time.Minute).Unix()
	claims["iat"] = now.Add(-2 * time.Minute).Unix()
	claims["nbf"] = now.Add(-2 * time.Minute).Unix()
	badKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	if _, err = c.Verify(sign(jwt.SigningMethodRS256, "old", "at+jwt", badKey), now); !errors.Is(err, app.ErrAccessInvalid) {
		t.Fatalf("tampered expiry %v", err)
	}
}

func TestSessionV2ConfigurationInvalidDeadline(t *testing.T) {
	t.Setenv("LV_IDENTITY_LEGACY_UPGRADE_UNTIL", "invalid")
	if _, err := config.Load(); err == nil {
		t.Fatal("invalid deadline accepted")
	}
}

func TestSessionV2ConfigurationRollback(t *testing.T) {
	c := config.Config{IdentityJWTVerifyKeysFile: "/controlled/verify.json"}
	if err := c.ValidateIdentitySessionV2(); err != nil {
		t.Fatal(err)
	}
	c.IdentityJWTActiveKID = "retained"
	c.IdentityJWTVerifyKeysFile = ""
	if err := c.ValidateIdentitySessionV2(); err == nil {
		t.Fatal("rollback lost verifier")
	}
	if _, err := token.LoadFiles("/missing/private", "/missing/verify", "active", true); err == nil {
		t.Fatal("missing keys accepted")
	}
}

func TestSessionV2TokenClaimBoundaries(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})
	c, err := token.NewVerifier(map[string][]byte{"fixture": pub})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		field string
		value any
	}{{"iss", "wrong"}, {"aud", "wrong"}, {"sub", uuid.Nil.String()}, {"sid", uuid.Nil.String()}, {"jti", "invalid"}, {"credential_epoch", 0}, {"iat", now.Add(31 * time.Second).Unix()}, {"nbf", now.Add(31 * time.Second).Unix()}, {"exp", now.Add(16 * time.Minute).Unix()}} {
		t.Run(tc.field, func(t *testing.T) {
			claims := jwt.MapClaims{"iss": "glowloom.identity", "aud": "glowloom.web", "sub": uuid.NewString(), "sid": uuid.NewString(), "jti": uuid.NewString(), "credential_epoch": 1, "iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(time.Minute).Unix()}
			claims[tc.field] = tc.value
			v := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			v.Header["kid"] = "fixture"
			v.Header["typ"] = "at+jwt"
			raw, e := v.SignedString(key)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = c.Verify(raw, now); !errors.Is(e, app.ErrAccessInvalid) {
				t.Fatal("invalid claim accepted", e)
			}
		})
	}
}

func TestSessionV2RotatedKeyRefresh(t *testing.T) {
	_, m, now, a := sessionHTTPAuthFixture(t, false)
	private := map[string][]byte{}
	public := map[string][]byte{}
	for _, kid := range []string{"old", "new"} {
		k, e := rsa.GenerateKey(rand.Reader, 2048)
		if e != nil {
			t.Fatal(e)
		}
		private[kid] = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
		public[kid] = pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&k.PublicKey)})
	}
	old, e := token.New("old", private["old"], public)
	if e != nil {
		t.Fatal(e)
	}
	next, e := token.New("new", private["new"], public)
	if e != nil {
		t.Fatal(e)
	}
	a.WithSessionSecurity(old, &v2Receipt{}, &v2Coordinator{})
	login, e := a.Login(t.Context(), m.user.Email, "short", false)
	if e != nil {
		t.Fatal(e)
	}
	first, e := a.Refresh(t.Context(), login.RefreshToken, login.CSRFToken, "peer", uuid.New())
	if e != nil {
		t.Fatal(e)
	}
	*now = now.Add(11 * time.Second)
	a.WithSessionSecurity(next, &v2Receipt{}, &v2Coordinator{})
	second, e := a.Refresh(t.Context(), first.RefreshToken, first.CSRFToken, "peer", uuid.New())
	if e != nil {
		t.Fatal(e)
	}
	if second.Session.RefreshGeneration != 2 {
		t.Fatal("refresh did not rotate")
	}
	if _, e = a.CurrentAccess(t.Context(), first.AccessToken, first.CSRFToken, app.NoActivity); e != nil {
		t.Fatal("retained old public key rejected", e)
	}
	if _, e = next.Verify(first.AccessToken, first.AccessExpiresAt.Add(29*time.Second)); e != nil {
		t.Fatal("old public key recognition lost", e)
	}
	if _, e = next.Verify(first.AccessToken, first.AccessExpiresAt.Add(30*time.Second)); !errors.Is(e, app.ErrAccessExpired) {
		t.Fatal("old token exceeded recognition", e)
	}
	verifier, e := token.NewVerifier(map[string][]byte{"new": public["new"]})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = verifier.Verify(first.AccessToken, *now); !errors.Is(e, app.ErrAccessInvalid) {
		t.Fatal("unknown kid accepted", e)
	}
}

func TestSessionV2ControlledReceiptFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "receipt")
	if err := os.WriteFile(path, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := token.LoadReceiptFile(path); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{link, "relative-receipt", dir} {
		if _, err := token.LoadReceiptFile(bad); err == nil {
			t.Fatal("uncontrolled file accepted")
		}
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := token.LoadReceiptFile(path); err == nil {
		t.Fatal("publicly readable receipt key accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, 65537), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := token.LoadReceiptFile(path); err == nil {
		t.Fatal("oversize key accepted")
	}
}
