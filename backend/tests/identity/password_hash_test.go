package identity_test

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func TestArgonPasswordAndLegacyUpgrade(t *testing.T) {
	password := strings.Repeat("界", 128)
	first, err := domain.HashPassword(password, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.HashPassword(password, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "$argon2id$v=19$m=19456,t=2,p=1$") || first == second {
		t.Fatal("new credentials must use the accepted Argon2id parameters and fresh salt")
	}
	if !domain.ValidPasswordHash(first) || !domain.VerifyPassword(first, password) || domain.VerifyPassword(first, password+"x") {
		t.Fatal("Argon2id credential verification failed")
	}
	legacy, err := bcrypt.GenerateFromPassword([]byte("legacyPassword123"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.VerifyPassword(string(legacy), "legacyPassword123") || !domain.PasswordNeedsUpgrade(string(legacy)) || domain.PasswordNeedsUpgrade(first) {
		t.Fatal("legacy verification and upgrade decision mismatch")
	}
	for _, malformed := range []string{
		strings.Replace(first, "m=19456", "m=4294967295", 1),
		strings.Replace(first, "t=2", "t=99999", 1),
		strings.Replace(first, "p=1", "p=0", 1),
		strings.Replace(first, "v=19", "v=16", 1),
		first + "$extra", "$argon2id$v=19$m=19456,t=2,p=1$bad$bad",
	} {
		if domain.ValidPasswordHash(malformed) || domain.VerifyPassword(malformed, password) {
			t.Fatal("malformed or unbounded hash accepted")
		}
	}
}
