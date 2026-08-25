package authentication

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestVerifierAcceptsCompatiblePythonHS256Claims(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	verifier := NewVerifier("secret", "lanverse-api", "lanverse-web", func() time.Time { return now })
	token := signedToken(t, "secret", map[string]any{"sub": "019ff900-a000-7000-8000-000000000001", "ver": 2, "type": "access", "jti": "token-1", "iss": "lanverse-api", "aud": "lanverse-web", "iat": 900, "exp": 1100})
	claims, err := verifier.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "019ff900-a000-7000-8000-000000000001" || claims.TokenVersion != 2 {
		t.Fatalf("claims = %#v", claims)
	}
}
func TestVerifierRejectsExpiredOrTamperedToken(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	verifier := NewVerifier("secret", "lanverse-api", "lanverse-web", func() time.Time { return now })
	expired := signedToken(t, "secret", map[string]any{"sub": "019ff900-a000-7000-8000-000000000001", "ver": 1, "type": "access", "jti": "token-1", "iss": "lanverse-api", "aud": "lanverse-web", "iat": 800, "exp": 999})
	if _, err := verifier.Verify(expired); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired error = %v", err)
	}
	if _, err := verifier.Verify(expired + "x"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("tampered error = %v", err)
	}
}

func TestIssuerCreatesTokenAcceptedByVerifier(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	issuer := NewIssuer("secret", "lanverse-api", "lanverse-web", 30*time.Minute, func() time.Time { return now }, func() string { return "token-1" })
	verifier := NewVerifier("secret", "lanverse-api", "lanverse-web", func() time.Time { return now })

	token, err := issuer.Issue("019ff900-a000-7000-8000-000000000001", 3)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "019ff900-a000-7000-8000-000000000001" || claims.TokenVersion != 3 {
		t.Fatalf("claims = %#v", claims)
	}
}
func signedToken(t *testing.T, secret string, payload map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	body, _ := json.Marshal(payload)
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedBody := base64.RawURLEncoding.EncodeToString(body)
	signed := encodedHeader + "." + encodedBody
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signed))
	return signed + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
