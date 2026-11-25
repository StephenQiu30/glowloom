package identity_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"

	token "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/sessiontoken"
)

func TestSessionKeysLoadFromEnvironmentValues(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	public := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)}))
	keys, err := json.Marshal(map[string]string{"active": public})
	if err != nil {
		t.Fatal(err)
	}
	codec, err := token.LoadValues(private, string(keys), "active")
	if err != nil || !codec.CanSign() {
		t.Fatal("environment key material was not loaded")
	}
	verifier, err := token.LoadValues("", string(keys), "")
	if err != nil || verifier.CanSign() {
		t.Fatal("verification-only configuration not retained")
	}
	for _, invalid := range []string{"not-base64-secret", "/missing/file"} {
		_, err := token.LoadValues(invalid, string(keys), "active")
		if err == nil || strings.Contains(err.Error(), invalid) {
			t.Fatal("invalid key accepted or echoed")
		}
	}
	if _, err := token.LoadReceiptValue(base64.StdEncoding.EncodeToString(make([]byte, 32))); err != nil {
		t.Fatal(err)
	}
	if _, err := token.LoadReceiptValue("invalid-secret"); err == nil || strings.Contains(err.Error(), "invalid-secret") {
		t.Fatal("invalid receipt key accepted or echoed")
	}
}
