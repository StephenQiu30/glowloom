package authentication

import "testing"

func TestPasswordHasherRoundTrip(t *testing.T) {
	t.Parallel()
	hasher := NewPasswordHasher()
	hash, err := hasher.Hash("a-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "a-secure-password" {
		t.Fatal("password must not be stored in plaintext")
	}
	if !hasher.Verify(hash, "a-secure-password") {
		t.Fatal("valid password was rejected")
	}
	if hasher.Verify(hash, "wrong-password") {
		t.Fatal("invalid password was accepted")
	}
}
