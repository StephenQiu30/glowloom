package platform_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

func TestCanonicalRejectsAmbiguousObjectKeys(t *testing.T) {
	for _, raw := range []string{`{"id":1,"id":2}`, `{"nested":[{"scope":"a","scope":"b"}]}`, `{"x":1,"\u0078":2}`} {
		if _, err := canonical.JSON([]byte(raw)); err == nil {
			t.Errorf("ambiguous payload accepted: %s", raw)
		}
	}
}

func TestCanonicalClosedDecodeRejectsTrailingUnknownAndDeepValues(t *testing.T) {
	for _, raw := range []string{`{"id":1} {"id":2}`, `{"id":1,"unknown":true}`, strings.Repeat("[", 130) + "0" + strings.Repeat("]", 130)} {
		var value struct {
			ID int `json:"id"`
		}
		if err := canonical.Decode([]byte(raw), &value); err == nil {
			t.Error("invalid closed contract accepted")
		}
	}
	raw, err := canonical.JSON([]byte(`{"b":[],"a":{"x":1}}`))
	if err != nil || string(raw) != `{"a":{"x":1},"b":[]}` {
		t.Fatalf("valid canonical bytes drifted: %s: %v", raw, err)
	}
}
