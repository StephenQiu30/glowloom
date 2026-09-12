package openai_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	adapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

type referenceObjects struct {
	calls           int
	key, mime, hash string
	contents        []byte
	err             error
}

func (o *referenceObjects) EnsurePrivateObject(ctx context.Context, key string, contents []byte, mime, hash string) error {
	o.calls++
	o.key, o.mime, o.hash, o.contents = key, mime, hash, bytes.Clone(contents)
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing staging deadline")
	}
	return o.err
}

func referenceTransportInput(t *testing.T) (app.ReferenceImageSubmission, domain.ReferenceCallDispatch, app.ProviderRuntimeConfig) {
	t.Helper()
	body, err := canonical.JSON([]byte(`{"model":"gpt-image-2","prompt":"frozen-view-prompt","size":"1024x1024","n":1,"quality":"high","output_format":"png","stream":false}`))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := canonical.Hash(body)
	if err != nil {
		t.Fatal(err)
	}
	ref := domain.GenerationRevisionRef{ID: uuid.NewString(), Revision: 1, ContentHash: strings.Repeat("a", 64)}
	_, calls, err := domain.BuildReferenceProviderJob(ref, []domain.ReferenceProviderCallInput{{BundleIndex: 0, SlotKey: "front", CompiledRequestHash: hash}})
	if err != nil {
		t.Fatal(err)
	}
	input := app.ReferenceImageSubmission{
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), Call: calls[0],
		Request: app.ReferenceImageRequest{SlotKey: "front", ContentHash: hash, Body: body},
		Slot:    domain.ReferenceOutputSlot{SlotKey: "front", ViewRole: "front", Required: true, AllowedMediaTypes: []string{"image/png"}, AspectRatio: "1:1", MinWidth: 1024, MinHeight: 1024, MaxBytes: 10 << 20},
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	dispatch := domain.ReferenceCallDispatch{SubmissionToken: uuid.NewString(), DispatchedBy: uuid.NewString(), MembershipTokenVersion: 1, DispatchedAt: now, DeadlineAt: now.Add(180 * time.Second)}
	config := app.ProviderRuntimeConfig{Connection: domain.ProviderConnectionVersion{ProviderKey: "openai", AdapterContractVersion: "openai-image-api"}, Profile: domain.ProviderModelProfileVersion{ProviderKey: "openai", ExternalModelID: "gpt-image-2", Modality: "image", AdapterTransportContract: "openai-image-api-nonstreaming"}, Credentials: []byte(`{"api_key":"synthetic-test"}`)}
	return input, dispatch, config
}

func referencePNG(t *testing.T, width int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, width, 1024))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestReferenceImageTransportSendsFrozenRequestAndStagesExactPNG(t *testing.T) {
	input, dispatch, config := referenceTransportInput(t)
	pngBytes := referencePNG(t, 1024)
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, input.Request.Body) {
			t.Error("canonical request was changed")
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/generations" || r.Header.Get("Authorization") != "Bearer synthetic-test" {
			t.Error("wrong request protocol")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"created":123,"additive":"allowed","data":[{"b64_json":%q}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`, base64.StdEncoding.EncodeToString(pngBytes))
	}))
	defer server.Close()
	client := server.Client()
	transport := client.Transport
	client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/images/generations" || r.GetBody != nil || r.Header.Get("Idempotency-Key") != "" {
			t.Error("request endpoint or replay boundary changed")
		}
		r.URL.Host = server.Listener.Addr().String()
		return transport.RoundTrip(r)
	})
	store := &referenceObjects{}
	runtime, err := adapter.NewFactory(client, store, time.Now).NewReferenceImageRuntime(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Preflight(context.Background(), input); err != nil || requests != 0 {
		t.Fatalf("preflight: %v", err)
	}
	observation, err := runtime.Submit(context.Background(), input, dispatch)
	if err != nil || observation.Status != app.ReferenceImageStaged || observation.Output == nil || requests != 1 || store.calls != 1 {
		t.Fatalf("observation=%+v err=%v requests=%d writes=%d", observation, err, requests, store.calls)
	}
	digest := sha256.Sum256(pngBytes)
	expectedKey := "staging/reference/" + input.WorkspaceID + "/" + input.ProjectID + "/" + input.Call.ExecutionRef.ID + "/" + input.Call.CallKey + "/" + dispatch.SubmissionToken + "/image.png"
	if store.key != expectedKey || store.mime != "image/png" || store.hash != hex.EncodeToString(digest[:]) || !bytes.Equal(store.contents, pngBytes) || observation.Output.StagingObjectKey != store.key || observation.Output.SHA256 != store.hash || observation.Output.Bytes != int64(len(pngBytes)) || observation.Output.Width != 1024 || observation.Output.Height != 1024 || observation.CallKey != input.Call.CallKey || observation.SubmissionToken != dispatch.SubmissionToken || observation.Usage.TotalTokens != 5 {
		t.Fatal("staging identity or metadata changed")
	}
	raw, err := json.Marshal(observation)
	if err != nil || strings.Contains(string(raw), "synthetic-test") || strings.Contains(string(raw), "frozen-view-prompt") || strings.Contains(string(raw), "b64_json") {
		t.Fatal("observation leaked transient material")
	}
}

func TestReferenceImageTransportRejectsBeforeSending(t *testing.T) {
	for _, mode := range []string{"scope", "call", "hash", "slot", "body", "extra-field", "noncanonical", "model", "size", "count", "quality", "stream", "budget", "token", "expired", "deadline", "credential", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			input, dispatch, config := referenceTransportInput(t)
			ctx := context.Background()
			switch mode {
			case "scope":
				input.WorkspaceID = "../outside"
			case "call":
				input.Call.CallKey = strings.Repeat("b", 64)
			case "hash":
				input.Request.ContentHash = strings.Repeat("b", 64)
			case "slot":
				input.Slot.SlotKey = "side"
			case "body":
				input.Request.Body = []byte(`{}`)
			case "noncanonical":
				input.Request.Body = append([]byte(" "), input.Request.Body...)
			case "budget":
				input.Slot.MaxBytes = 11 << 20
			case "token":
				dispatch.SubmissionToken = "../bad"
			case "expired":
				dispatch.DispatchedAt = dispatch.DispatchedAt.Add(-time.Hour)
				dispatch.DeadlineAt = dispatch.DeadlineAt.Add(-time.Hour)
			case "deadline":
				dispatch.DeadlineAt = dispatch.DispatchedAt.Add(time.Hour)
			case "credential":
				config.Credentials = []byte(`{"api_key":"bad\r\nheader"}`)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			default:
				var body map[string]any
				if err := json.Unmarshal(input.Request.Body, &body); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "extra-field":
					body["input_fidelity"] = "low"
				case "model":
					body["model"] = "other"
				case "size":
					body["size"] = "auto"
				case "count":
					body["n"] = 2
				case "quality":
					body["quality"] = "auto"
				case "stream":
					body["stream"] = true
				}
				raw, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				input.Request.Body, err = canonical.JSON(raw)
				if err != nil {
					t.Fatal(err)
				}
				input.Request.ContentHash, err = canonical.Hash(raw)
				if err != nil {
					t.Fatal(err)
				}
				_, calls, err := domain.BuildReferenceProviderJob(input.Call.ExecutionRef, []domain.ReferenceProviderCallInput{{SlotKey: "front", CompiledRequestHash: input.Request.ContentHash}})
				if err != nil {
					t.Fatal(err)
				}
				input.Call = calls[0]
			}
			requests := 0
			store := &referenceObjects{}
			runtime, err := adapter.NewFactory(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { requests++; return nil, errors.New("must not send") })}, store, time.Now).NewReferenceImageRuntime(config)
			if err != nil {
				t.Fatal(err)
			}
			observation, err := runtime.Submit(ctx, input, dispatch)
			if err == nil || !reflect.DeepEqual(observation, app.ReferenceImageObservation{}) || requests != 0 || store.calls != 0 {
				t.Fatalf("invalid input crossed boundary: %v %+v", err, observation)
			}
		})
	}
}

func TestReferenceImageTransportNeverRetriesOrPromotesInvalidMedia(t *testing.T) {
	good := referencePNG(t, 1024)
	wrong := referencePNG(t, 1536)
	for _, mode := range []string{"disconnect", "unauthorized", "server-error", "redirect", "read-error", "response-limit", "duplicate-json", "extra-output", "url", "invalid-base64", "invalid-image", "wrong-size", "truncated-png", "trailing-bytes", "image-limit", "negative-usage", "stage-error"} {
		t.Run(mode, func(t *testing.T) {
			input, dispatch, config := referenceTransportInput(t)
			requests := 0
			store := &referenceObjects{}
			status, bytesOut := 200, good
			if mode == "wrong-size" {
				bytesOut = wrong
			}
			if mode == "truncated-png" {
				bytesOut = good[:len(good)-8]
			}
			if mode == "trailing-bytes" {
				bytesOut = append(bytes.Clone(good), []byte("hidden payload")...)
			}
			if mode == "invalid-image" {
				bytesOut = []byte("<svg>not png</svg>")
			}
			if mode == "image-limit" {
				input.Slot.MaxBytes = int64(len(good) - 1)
			}
			body := fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString(bytesOut))
			switch mode {
			case "unauthorized":
				status = 401
			case "server-error":
				status = 500
			case "redirect":
				status = 307
			case "duplicate-json":
				body = `{"data":[],"data":[]}`
			case "extra-output":
				body = `{"data":[{"b64_json":"one"},{"b64_json":"two"}]}`
			case "url":
				body = `{"data":[{"url":"https://untrusted.invalid/image"}]}`
			case "invalid-base64":
				body = `{"data":[{"b64_json":"?"}]}`
			case "response-limit":
				body = strings.Repeat(" ", 15<<20)
			case "negative-usage":
				body = fmt.Sprintf(`{"data":[{"b64_json":%q}],"usage":{"input_tokens":-1}}`, base64.StdEncoding.EncodeToString(good))
			case "stage-error":
				store.err = errors.New("synthetic-test private storage failure")
			}
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				requests++
				if mode == "disconnect" {
					return nil, errors.New("synthetic-test transport failure")
				}
				var reader io.ReadCloser = io.NopCloser(strings.NewReader(body))
				if mode == "read-error" {
					reader = io.NopCloser(referenceFailingReader{})
				}
				return &http.Response{StatusCode: status, Body: reader, Header: http.Header{"Location": []string{"https://untrusted.invalid/redirect"}}, Request: r}, nil
			})}
			runtime, err := adapter.NewFactory(client, store, time.Now).NewReferenceImageRuntime(config)
			if err != nil {
				t.Fatal(err)
			}
			observation, err := runtime.Submit(context.Background(), input, dispatch)
			if err != nil || observation.Output != nil || observation.Status == app.ReferenceImageStaged || observation.ReasonCode == "" || requests != 1 {
				t.Fatalf("unsafe observation: %+v err=%v calls=%d", observation, err, requests)
			}
			if mode == "disconnect" || mode == "unauthorized" || mode == "server-error" || mode == "redirect" || mode == "read-error" || mode == "stage-error" {
				if observation.Status != app.ReferenceImageOutcomeUnknown {
					t.Fatal("uncertainty was converted to remote failure")
				}
			} else if observation.Status != app.ReferenceImageOutputRejected {
				t.Fatal("invalid media was not rejected")
			}
			expectedWrites := 0
			if mode == "stage-error" {
				expectedWrites = 1
			}
			if store.calls != expectedWrites {
				t.Fatalf("unexpected object writes: %d", store.calls)
			}
			raw, _ := json.Marshal(observation)
			if strings.Contains(string(raw), "synthetic-test") || strings.Contains(string(raw), "untrusted.invalid") {
				t.Fatal("raw failure leaked")
			}
		})
	}
}

type referenceFailingReader struct{}

func (referenceFailingReader) Read([]byte) (int, error) {
	return 0, errors.New("synthetic-test response read failure")
}

func TestReferenceImageTransportDeadlineCoversHTTPAndStaging(t *testing.T) {
	for _, mode := range []string{"http", "staging"} {
		t.Run(mode, func(t *testing.T) {
			input, dispatch, config := referenceTransportInput(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests, writes := 0, 0
			pngBytes := referencePNG(t, 1024)
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				requests++
				deadline, ok := r.Context().Deadline()
				if !ok || !deadline.Equal(dispatch.DeadlineAt) {
					t.Error("HTTP lost frozen deadline")
				}
				if mode == "http" {
					cancel()
					return nil, r.Context().Err()
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString(pngBytes))))}, nil
			})}
			store := referenceObjectFunc(func(ctx context.Context, _ string, _ []byte, _, _ string) error {
				writes++
				deadline, ok := ctx.Deadline()
				if !ok || !deadline.Equal(dispatch.DeadlineAt) {
					t.Error("staging lost frozen deadline")
				}
				cancel()
				return nil // A late acknowledgement must not become a ready output.
			})
			runtime, err := adapter.NewFactory(client, store, time.Now).NewReferenceImageRuntime(config)
			if err != nil {
				t.Fatal(err)
			}
			observation, err := runtime.Submit(ctx, input, dispatch)
			if err != nil || observation.Status != app.ReferenceImageOutcomeUnknown || observation.Output != nil || requests != 1 {
				t.Fatalf("cancelled request reported success: %+v %v", observation, err)
			}
			if (mode == "http" && writes != 0) || (mode == "staging" && writes != 1) {
				t.Fatal("unexpected staging attempt")
			}
		})
	}
}

type referenceObjectFunc func(context.Context, string, []byte, string, string) error

func (f referenceObjectFunc) EnsurePrivateObject(ctx context.Context, key string, contents []byte, mime, hash string) error {
	return f(ctx, key, contents, mime, hash)
}

func TestReferenceImageTransportRejectsUnsupportedRuntime(t *testing.T) {
	for _, mode := range []string{"provider", "model", "modality", "adapter", "defaults", "connection-config", "storage", "factory"} {
		t.Run(mode, func(t *testing.T) {
			_, _, config := referenceTransportInput(t)
			factory := adapter.NewFactory(nil, &referenceObjects{}, time.Now)
			switch mode {
			case "provider":
				config.Connection.ProviderKey = "other"
			case "model":
				config.Profile.ExternalModelID = "other"
			case "modality":
				config.Profile.Modality = "video"
			case "adapter":
				config.Profile.AdapterTransportContract = "unsupported"
			case "defaults":
				config.Profile.Defaults = map[string]any{"quality": "auto"}
			case "connection-config":
				config.Connection.ResolvedConfig = map[string]any{"base_url": "https://untrusted.invalid"}
			case "storage":
				factory = adapter.NewFactory(nil, nil, nil)
			case "factory":
				factory = nil
			}
			if runtime, err := factory.NewReferenceImageRuntime(config); err == nil || runtime != nil {
				t.Fatal("unsupported runtime accepted")
			}
		})
	}
}
