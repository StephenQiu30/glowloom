package providerhttp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/stretchr/testify/require"
)

func TestArkImageReferencesKeepCOSURLAndRejectUnverifiedInputs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ref       application.ProviderReference
		wantCalls int
	}{
		{"COS HTTPS", application.ProviderReference{MediaKind: "image", Role: "reference_image", URL: "https://fixture-123.cos.ap-shanghai.myqcloud.com/input.png?q-signature=fixture"}, 1},
		{"local URL without verified bytes", application.ProviderReference{MediaKind: "image", URL: "http://127.0.0.1:9000/input.png"}, 0},
		{"non-image bytes", application.ProviderReference{MediaKind: "image", ImageBytes: []byte("private text is not an image")}, 0},
		{"over inline limit", application.ProviderReference{MediaKind: "image", ImageBytes: make([]byte, (30<<20)+1)}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := NewClient(&http.Client{Transport: creationTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				var body struct {
					Images []string `json:"image"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, []string{tc.ref.URL}, body.Images)
				return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"fixture"}}`))}, nil
			})})
			result, err := c.Submit(t.Context(), application.GenerationRequest{Config: creationConfig("volcengine_ark"), ModelID: "doubao-seedream-5-0-flash-260915", Capability: "image.generate", Mode: "image2image", Prompt: "fixture", Params: map[string]any{"size": "2K"}, References: []application.ProviderReference{tc.ref}, OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, tc.wantCalls, calls)
			if tc.wantCalls == 0 {
				require.Equal(t, "not_submitted", result.Outcome)
			}
		})
	}
}
