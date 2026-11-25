package identity_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

func TestEmailPolicyBoundaries(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{" A+tag@EXAMPLE.COM ", "a+tag@example.com"},
		{"first.last@example.com", "first.last@example.com"},
		{strings.Repeat("a", 64) + "@" + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61), strings.Repeat("a", 64) + "@" + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)},
	} {
		got, err := domain.NormalizeEmail(tt.input)
		if err != nil || got != tt.want {
			t.Errorf("NormalizeEmail = %q, %v", got, err)
		}
	}
	for _, input := range []string{"", "用户@example.com", "a@例子.com", "a..b@example.com", ".a@example.com", "a.@example.com", "a@-example.com", "a@example-.com", "a@example..com", "a@", "Display <a@example.com>", "a@example.com\nBcc:x@example.com", strings.Repeat("a", 65) + "@example.com", "a@" + strings.Repeat("b", 64) + ".com", strings.Repeat("a", 64) + "@" + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 62)} {
		if _, err := domain.NormalizeEmail(input); err == nil {
			t.Errorf("malformed mailbox accepted: length %d", len(input))
		}
	}
}

func TestPasswordUnicodeBoundaries(t *testing.T) {
	for _, password := range []string{strings.Repeat("界", 8), "a7bc8def", strings.Repeat("a", 14), strings.Repeat("a", 128), "  my pass phrase  "} {
		if err := domain.ValidateNewPassword(password); err != nil {
			t.Errorf("valid password rejected: %v", err)
		}
	}
	for _, password := range []string{strings.Repeat("a", 7), strings.Repeat("界", 7), strings.Repeat("界", 129), string([]byte{0xff}) + strings.Repeat("a", 20)} {
		if err := domain.ValidateNewPassword(password); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
}
