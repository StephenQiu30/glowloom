package operation_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// This exercises the official Ark request shape with a controlled transport,
// without authenticating or submitting a paid request to a provider.
func TestSubjectMultimodalReferencesKeepKindsInArkVideoWire(t *testing.T) {
	var config catalog.ProviderConfig
	for _, preset := range catalog.BuiltinPresets() {
		if preset.Config.AdapterKey == "volcengine_ark" {
			config = preset.Config
			break
		}
	}
	calls := 0
	client := providerhttp.NewClient(&http.Client{Transport: editorWireTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var payload struct {
			Content []map[string]json.RawMessage `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Content) != 4 {
			t.Fatalf("multimodal content: %s", payload.Content)
		}
		for index, kind := range []string{"image", "video", "audio"} {
			var typ, role string
			var locator struct {
				URL string `json:"url"`
			}
			_ = json.Unmarshal(payload.Content[index+1]["type"], &typ)
			_ = json.Unmarshal(payload.Content[index+1]["role"], &role)
			_ = json.Unmarshal(payload.Content[index+1][kind+"_url"], &locator)
			if typ != kind+"_url" || role != "reference_"+kind || locator.URL != "https://subject-fixture.invalid/"+kind {
				t.Fatalf("%s mislabeled: %#v", kind, payload.Content[index+1])
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"subject-video-fixture"}`))}, nil
	})})
	input := operation.GenerationRequest{Config: config, ModelID: "doubao-seedance-2-0-260128", Capability: "video.generate", Mode: "omni_reference", Prompt: "保持主体与音色", OutputCount: 1, References: []operation.ProviderReference{}}
	for _, kind := range []string{"image", "video", "audio"} {
		input.References = append(input.References, operation.ProviderReference{Role: "subject", URL: "https://subject-fixture.invalid/" + kind, MediaKind: kind})
	}
	result, err := client.Submit(t.Context(), input, "synthetic-test-key")
	if err != nil || result.Outcome != "accepted" || calls != 1 {
		t.Fatalf("multimodal serialization: %+v %v calls=%d", result, err, calls)
	}
	for _, mode := range []string{"image2video", "text2video"} {
		input.Mode = mode
		result, err = client.Submit(t.Context(), input, "synthetic-test-key")
		if err != nil || result.Outcome != "not_submitted" || calls != 1 {
			t.Fatalf("non-omni mode dispatched mixed inputs: %+v %v", result, err)
		}
	}
}
