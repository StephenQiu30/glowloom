package objectstorage_test

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestPresignGetRejectsUnsafeKeysAndUnboundedLifetime(t *testing.T) {
	client, err := objectstorage.Open("https://canvas-fixture-1250000000.cos.ap-shanghai.myqcloud.com", "canvas-fixture-1250000000", "synthetic-access", "synthetic-secret", "ap-shanghai")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "/private", "projects/../private", "projects//private", "private\\object"} {
		if _, err := client.PresignGet(t.Context(), key, time.Minute); !errors.Is(err, objectstorage.ErrInvalidObject) {
			t.Fatalf("unsafe key allowed %q", key)
		}
	}
	for _, ttl := range []time.Duration{0, -time.Second, 16 * time.Minute} {
		if _, err := client.PresignGet(t.Context(), "projects/fixture/image.png", ttl); !errors.Is(err, objectstorage.ErrInvalidObject) {
			t.Fatal("unbounded lifetime allowed")
		}
	}
	signed, err := client.PresignGet(t.Context(), "projects/fixture/image.png", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(signed)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "canvas-fixture-1250000000.cos.ap-shanghai.myqcloud.com" || parsed.Query().Get("q-signature") == "" {
		t.Fatal("invalid bounded signature")
	}
}
