package microservices_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func TestWriteFenceReceiptNormalizesNumbersWithoutPostgresAmplification(t *testing.T) {
	got, err := writefence.CanonicalReceipt([]byte(`{"value":1e3,"fraction":1.2300,"negativeZero":-0.0,"large":9007199254740993}`))
	if err != nil || string(got) != `{"fraction":1.23,"large":9007199254740993,"negativeZero":0,"value":1000}` {
		t.Fatalf("numeric receipt not stable across JSONB: %s, %v", got, err)
	}
	for _, raw := range []string{
		`{"nul":"\u0000"}`,
		`{"\u0000":"invalid key"}`,
		`{"value":1e131072}`,
		`{"value":1e-16384}`,
		`{"value":1e999999999999999999999999999999999999}`,
		`{"values":[` + strings.Repeat(`1e100000,`, 11) + `1]}`,
		`{"nested":` + strings.Repeat("[", 300) + `1` + strings.Repeat("]", 300) + `}`,
	} {
		if _, err := writefence.CanonicalReceipt([]byte(raw)); !errors.Is(err, writefence.ErrInvalid) {
			t.Fatalf("unrecoverable/expanded receipt accepted: len=%d, %v", len(raw), err)
		}
	}
}
