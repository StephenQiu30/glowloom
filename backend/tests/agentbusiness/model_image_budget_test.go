package agentbusiness_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	agenthttp "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/modelgateway"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

type imageProxy func(*http.Request) (*http.Response, error)

func (f imageProxy) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestModelBoundedImageTranscriptCrossesGoHTTPStoreAndProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, _, g := fixture(t)
	im := image.NewNRGBA(image.Rect(0, 0, 1000, 500))
	if _, err := rand.Read(im.Pix); err != nil {
		t.Fatal(err)
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, im); err != nil {
		t.Fatal(err)
	}
	if pngBytes.Len() < 1900000 || pngBytes.Len() > 2<<20 {
		t.Fatal("fixture must be an actual nearly2MiB PNG", pngBytes.Len())
	}
	encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Messages []struct {
				Content []struct {
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				}
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 1 || len(req.Messages[0].Content) != 1 || req.Messages[0].Content[0].ImageURL.URL != encoded {
			t.Error("actual image bytes lost before supplier", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"fixture image read"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	chat := modelgateway.NewChatClient(&http.Client{Transport: imageProxy(func(r *http.Request) (*http.Response, error) {
		next := r.Clone(r.Context())
		next.URL.Scheme = target.Scheme
		next.URL.Host = target.Host
		return transport.RoundTrip(next)
	})})
	cfg := catalog.ProviderConfig{AdapterKey: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Region: "overseas", Capabilities: []string{"text"}, TimeoutSeconds: 30, ConcurrencyLimit: 1, RateLimitPerMin: 10}
	capabilities := app.Capabilities{Model: func(ctx context.Context, _ domain.Scope, _ uuid.UUID, raw json.RawMessage, _ int64) (domain.CallResult, error) {
		out, err := chat.Complete(ctx, cfg, "fixture", "upstream", raw)
		if err != nil {
			return domain.CallResult{}, err
		}
		data, err := json.Marshal(out)
		return domain.CallResult{Status: "completed", Result: data}, err
	}}
	router := gin.New()
	agenthttp.NewBusinessHandler(s, capabilities, businessToken).Register(router)
	raw, _ := json.Marshal(map[string]any{"model_key": "fixture", "max_tokens": 10, "params": map[string]any{}, "tools": []any{}, "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": encoded}}}}}})
	invoke := func(input json.RawMessage, callID string) int {
		t.Helper()
		in := domain.CallInput{Grant: g, Call: domain.CallIdentity{TaskID: "image-model", CallID: callID, CheckpointID: "model", Attempt: 1, ContractVersion: "v1", InputHash: domain.Hash(input)}, Input: input, MaxMicros: 100}
		body, _ := json.Marshal(in)
		request := httptest.NewRequestWithContext(t.Context(), "POST", "/internal/agent-business/models/stream", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+businessToken)
		request.Header.Set("X-Agent-Audience", "agent-business")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code == 200 {
			var receipt domain.CallReceipt
			if json.Unmarshal(response.Body.Bytes(), &receipt) != nil || receipt.Status != "completed" {
				t.Fatal("model input failed after admission", response.Body.String())
			}
		}
		return response.Code
	}
	if status := invoke(raw, "original-image"); status != 200 || calls != 1 {
		t.Fatal("bounded native image rejected", status, calls)
	}
	oversized := json.RawMessage(`{"model_key":"fixture","max_tokens":10,"messages":[{"role":"user","content":"` + strings.Repeat("x", 8<<20) + `"}],"tools":[],"params":{}}`)
	if status := invoke(oversized, "too-large"); status != 413 || calls != 1 {
		t.Fatal("oversized history reached supplier", status, calls)
	}
}
