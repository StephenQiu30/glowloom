package openai_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
)

type objects struct{ calls int }

func (o *objects) EnsurePrivateObject(context.Context, string, []byte, string, string) error {
	o.calls++
	return nil
}
func TestImageResponseMustBeStagedAndQueryNeverResubmits(t *testing.T) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 1536, 1024))); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		prompt, _ := request["prompt"].(string)
		if !strings.Contains(prompt, "different identities, missing view, text, watermark") {
			t.Error("formal exclusions were dropped")
		}
		if r.URL.Path != "/v1/images/generations" || r.Header.Get("Authorization") != "Bearer synthetic-test" {
			t.Error("wrong protocol")
		}
		_, _ = fmt.Fprintf(w, `{"created":123,"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString(buffer.Bytes()))
	}))
	defer server.Close()
	// Inject only transport; the production request keeps its fixed official URL.
	client := server.Client()
	transport := client.Transport
	client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme = "https"
		r.URL.Host = server.Listener.Addr().String()
		return transport.RoundTrip(r)
	})
	store := &objects{}
	factory := adapter.NewFactory(client, store, time.Now)
	runtime, err := factory.NewRuntime(app.ProviderRuntimeConfig{Connection: domain.ProviderConnectionVersion{ProviderKey: "openai", AdapterContractVersion: "openai-image-api"}, Profile: domain.ProviderModelProfileVersion{ProviderKey: "openai", ExternalModelID: "gpt-image-2", Modality: "image", AdapterTransportContract: "openai-image-api-nonstreaming"}, Credentials: []byte(`{"api_key":"synthetic-test"}`)})
	if err != nil {
		t.Fatal(err)
	}
	s := app.ProviderSubmission{WorkspaceID: uuid.NewString(), ProviderJobID: uuid.NewString(), ProviderCallID: uuid.NewString(), ProviderKey: "openai", ExternalModelID: "gpt-image-2", RequestedOutputCount: 1, Target: formalTarget(t)}
	s.WorkspaceID = s.Target.WorkspaceID
	s.ProjectID = s.Target.ProjectID
	out, err := runtime.Submit(context.Background(), s)
	if err != nil || out.Status != "succeeded" || store.calls != 1 || out.Output == nil {
		t.Fatalf("outcome=%+v err=%v", out, err)
	}
	_, err = runtime.Query(context.Background(), s)
	if err == nil || calls != 1 {
		t.Fatal("query submitted another image")
	}
	s.Target.ReferenceAsset.NegativePrompt = "changed after target was frozen"
	if err = runtime.Preflight(context.Background(), s); err == nil {
		t.Fatal("drifted target accepted")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestImageProtocolFailuresNeverProduceStagedSuccess(t *testing.T) {
	for _, mode := range []string{"server-error", "redirect", "duplicate-json", "extra-output", "invalid-image", "wrong-size", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			store := &objects{}
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				status := 200
				body := `{"data":[{"b64_json":"invalid"}]}`
				switch mode {
				case "server-error":
					status = 500
				case "redirect":
					status = 307
				case "duplicate-json":
					body = `{"data":[],"data":[{"b64_json":"AAAA"}]}`
				case "extra-output":
					body = `{"data":[{},{}]}`
				case "wrong-size":
					var buffer bytes.Buffer
					if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
						return nil, err
					}
					body = fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString(buffer.Bytes()))
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			runtime, err := adapter.NewFactory(client, store, time.Now).NewRuntime(app.ProviderRuntimeConfig{Connection: domain.ProviderConnectionVersion{ProviderKey: "openai", AdapterContractVersion: "openai-image-api"}, Profile: domain.ProviderModelProfileVersion{ProviderKey: "openai", ExternalModelID: "gpt-image-2", Modality: "image", AdapterTransportContract: "openai-image-api-nonstreaming"}, Credentials: []byte(`{"api_key":"synthetic-test"}`)})
			if err != nil {
				t.Fatal(err)
			}
			s := app.ProviderSubmission{WorkspaceID: uuid.NewString(), ProviderJobID: uuid.NewString(), ProviderCallID: uuid.NewString(), ProviderKey: "openai", ExternalModelID: "gpt-image-2", RequestedOutputCount: 1, Target: formalTarget(t)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			s.WorkspaceID = s.Target.WorkspaceID
			s.ProjectID = s.Target.ProjectID
			out, err := runtime.Submit(ctx, s)
			if err == nil || out.Status == "succeeded" || store.calls != 0 {
				t.Fatalf("invalid result accepted: %+v %v", out, err)
			}
			if calls > 1 || (mode == "cancelled" && calls != 0) {
				t.Fatal("unexpected retry or cancelled dispatch")
			}
		})
	}
}

func formalTarget(t *testing.T) domain.GenerationTarget {
	t.Helper()
	createdAt := time.Date(2026, 8, 29, 9, 30, 0, 0, time.UTC)
	input := domain.GenerationTargetInput{
		ID:          "10000000-0000-4000-8000-000000000001",
		WorkspaceID: "20000000-0000-4000-8000-000000000002",
		ProjectID:   "30000000-0000-4000-8000-000000000003",
		Kind:        domain.GenerationTargetReferenceAsset,
		SourceOwnerRef: domain.FrozenOwnerReference{
			Owner: "storyboard", Resource: "approved_storyboard_intents",
			ID: "40000000-0000-4000-8000-000000000004", Revision: 1,
			ContentHash: strings.Repeat("1", 64),
		},
		PolicySnapshotRef: domain.FrozenOwnerReference{
			Owner: "preset", Resource: "effective_style_snapshot",
			ID: "30000000-0000-4000-8000-000000000003", Revision: 3,
			ContentHash: strings.Repeat("2", 64),
		},
		ReferenceAsset: &domain.ReferenceAssetTarget{
			AssetID: "50000000-0000-4000-8000-000000000005", AssetKind: "character",
			SpecificationVersionRef: domain.FrozenOwnerReference{
				Owner: "production", Resource: "production_bible_specification_version",
				ID: "60000000-0000-4000-8000-000000000006", Revision: 1,
				ContentHash: strings.Repeat("3", 64),
			},
			AssetStateRef: domain.FrozenOwnerReference{
				Owner: "asset", Resource: "asset_state",
				ID: "70000000-0000-4000-8000-000000000007", Revision: 1,
				ContentHash: strings.Repeat("4", 64),
			},
			OutputKind: "reference_sheet", RequiredViewRoles: []string{"back", "front", "profile"},
			PromptVersion:  "character-reference-sheet",
			PositivePrompt: "same character, front profile and back views",
			NegativePrompt: "different identities, missing view, text, watermark",
			Width:          1536, Height: 1024, NumberResults: 4, OutputFormat: "PNG",
		},
		Revision: 1, CreatedBy: "80000000-0000-4000-8000-000000000008", CreatedAt: createdAt,
	}
	value, err := domain.NewGenerationTarget(input)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
