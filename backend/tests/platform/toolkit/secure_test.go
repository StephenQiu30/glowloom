package toolkit_test

import (
	"strings"
	"testing"

	. "github.com/stephenqiu30/lanverse/backend/src/platform/toolkit"
)

func TestRandomHexTokenAndSHA256Helpers(t *testing.T) {
	first, err := RandomHexToken(32)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RandomHexToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 || first == second {
		t.Fatalf("random tokens = %q and %q", first, second)
	}
	if got := SHA256String("lanverse"); len(got) != 64 || strings.Trim(got, "0123456789abcdef") != "" {
		t.Fatalf("SHA256String() = %q", got)
	}
}
