package identity_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	"github.com/google/uuid"
)

func TestIdentityKeyFileFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.key")
	if _, err := credentialproof.LoadFile(path); err == nil {
		t.Fatal("missing keys accepted")
	}
	if err := credentialproof.CreateFile(path); err != nil {
		t.Fatal(err)
	}
	first, err := credentialproof.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	sealed, err := first.Seal(id, []byte("private proof"))
	if err != nil {
		t.Fatal(err)
	}
	if err := credentialproof.CreateFile(path); err == nil {
		t.Fatal("existing identity key overwritten")
	}
	second, err := credentialproof.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := second.Open(id, first.KeyID(), sealed)
	if err != nil || string(plain) != "private proof" {
		t.Fatal("key changed between process starts")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := credentialproof.LoadFile(path); err == nil {
		t.Fatal("publicly readable keys accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link.key")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := credentialproof.LoadFile(link); err == nil {
		t.Fatal("symbolic identity key accepted")
	}
	if err := os.WriteFile(path, []byte(`{"key_id":"bad","proof_key":"secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := credentialproof.LoadFile(path); err == nil {
		t.Fatal("malformed identity keys accepted")
	}
}
