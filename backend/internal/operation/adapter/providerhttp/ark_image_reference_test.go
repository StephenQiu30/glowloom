package providerhttp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
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
				require.EqualValues(t, "not_submitted", result.Outcome)
			}
		})
	}
}

func TestArkImageErrorsDistinguishModerationFromInvalidParameters(t *testing.T) {
	for raw, want := range map[string]string{"InputTextSensitiveContentDetected": "input_content_rejected", "OutputImageSensitiveContentDetected.PolicyViolation": "output_content_rejected", "InvalidParameter": "invalid_parameters", "UnrecognizedPrivateData": "request_rejected"} {
		t.Run(raw, func(t *testing.T) {
			c := NewClient(&http.Client{Transport: creationTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 400, Header: http.Header{"X-Request-Id": {"20261113123456789ABC"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"` + raw + `","message":"private detail"}}`))}, nil
			})})
			result, err := c.Submit(t.Context(), application.GenerationRequest{Config: creationConfig("volcengine_ark"), ModelID: "test-model", Capability: "image.generate", Mode: "text2image", Prompt: "fixture", OutputCount: 1}, "fixture-key")
			require.NoError(t, err)
			require.Equal(t, want, result.ErrorCode)
			require.Equal(t, http.StatusBadRequest, result.HTTPStatus)
			require.Equal(t, "20261113123456789ABC", result.ProviderLogID)
			require.Equal(t, application.ProviderOutcomeUnknown, result.Outcome)
		})
	}
}

func TestArkTruncatedResponsePreservesReceivedHeaders(t *testing.T) {
	client := NewClient(&http.Client{Transport: creationTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": {"test-request-42"}}, Body: io.NopCloser(iotest.ErrReader(io.ErrUnexpectedEOF))}, nil
	})})
	cfg := creationConfig("volcengine_ark")
	submit, err := client.Submit(t.Context(), application.GenerationRequest{Config: cfg, ModelID: "fixture", Capability: "image.generate", Mode: "text2image", Prompt: "fixture", OutputCount: 1}, "fixture-key")
	require.NoError(t, err)
	require.Equal(t, application.ProviderOutcomeUnknown, submit.Outcome)
	query, err := client.Query(t.Context(), cfg, "fixture-task", "fixture-key")
	require.NoError(t, err)
	for _, r := range []application.GenerationResult{submit, query} {
		require.Equal(t, 200, r.HTTPStatus)
		require.Equal(t, "test-request-42", r.ProviderLogID)
	}
}
