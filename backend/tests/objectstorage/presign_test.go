package objectstorage_test

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestPresignGetRejectsUnsafeKeysAndUnboundedLifetime(t *testing.T) {
	client, err := objectstorage.Open("https://objects.example.invalid", "canvas-fixture", "synthetic-access", "synthetic-secret", "us-east-1")
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
	if err != nil || parsed.Scheme != "https" || parsed.Host != "objects.example.invalid" || parsed.Query().Get("X-Amz-Expires") != "600" {
		t.Fatal("invalid bounded signature")
	}
}
