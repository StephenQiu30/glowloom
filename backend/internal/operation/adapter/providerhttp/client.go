// Package providerhttp implements the reviewed Volcengine and OpenRouter wire
// protocols. It never retries a paid submission or follows authenticated redirects.
package providerhttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// Client uses one bounded HTTP client without an implicit retry middleware.
type Client struct{ client http.Client }

// NewClient copies transport settings and disables redirects that could leak Keys.
func NewClient(client *http.Client) *Client {
	var copied http.Client
	if client != nil {
		copied = *client
	}
	copied.Timeout = 300 * time.Second
	copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{client: copied}
}

func (c *Client) request(ctx context.Context, config catalogdomain.ProviderConfig, key, method, path string, payload any, limit int64) ([]byte, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(body)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(timeoutCtx, method, config.BaseURL+path, reader)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, response.StatusCode, err
	}
	if int64(len(data)) > limit {
		return nil, response.StatusCode, io.ErrShortBuffer
	}
	return data, response.StatusCode, nil
}

// Submit translates a validated request and makes exactly one charged POST.
func (c *Client) Submit(ctx context.Context, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	path, payload, ok := submission(input)
	if !ok || !validKey(key) {
		return application.GenerationResult{Outcome: "not_submitted", ErrorCode: "invalid_provider_request"}, nil
	}
	data, status, err := c.request(ctx, input.Config, key, http.MethodPost, path, payload, 64<<20)
	if err != nil {
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "transport_unknown"}, nil
	}
	if status < 200 || status >= 300 {
		outcome := "unknown"
		if slices.Contains([]int{400, 401, 402, 403, 404, 422, 429}, status) {
			outcome = "rejected"
		}
		return application.GenerationResult{Outcome: outcome, ErrorCode: safeHTTPError(status)}, nil
	}
	var response struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Data []struct {
			Base64    string `json:"b64_json"`
			MediaType string `json:"media_type"`
		} `json:"data"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &response) != nil {
		return unknownResult(), nil
	}
	result := application.GenerationResult{Outcome: "completed", State: "succeeded", Usage: safeUsage(response.Usage)}
	switch input.Capability {
	case "video.generate":
		if !validTaskID(response.ID) {
			return unknownResult(), nil
		}
		return application.GenerationResult{Outcome: "accepted", State: "pending", TaskID: response.ID}, nil
	case "text.generate":
		if len(response.Choices) != 1 || response.Choices[0].Message.Content == "" || response.Choices[0].FinishReason != "stop" {
			return unknownResult(), nil
		}
		result.Text = response.Choices[0].Message.Content
	case "image.generate":
		if len(response.Data) != input.OutputCount {
			return unknownResult(), nil
		}
		for _, image := range response.Data {
			data, err := base64.StdEncoding.DecodeString(image.Base64)
			if err != nil || len(data) == 0 || len(data) > 32<<20 {
				return unknownResult(), nil
			}
			kind := http.DetectContentType(data)
			if !slices.Contains([]string{"image/png", "image/jpeg", "image/webp"}, kind) || (image.MediaType != "" && image.MediaType != kind) {
				return unknownResult(), nil
			}
			result.Images = append(result.Images, application.GeneratedImage{Bytes: data, MediaType: kind})
		}
	default:
		return unknownResult(), nil
	}
	return result, nil
}
func unknownResult() application.GenerationResult {
	return application.GenerationResult{Outcome: "unknown", State: "unknown", ErrorCode: "provider_response_invalid"}
}
func validKey(key string) bool {
	return key != "" && len(key) <= 4096 && !strings.ContainsAny(key, "\r\n\x00")
}
func validTaskID(id string) bool {
	if len(id) == 0 || len(id) > 256 {
		return false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}
func safeHTTPError(status int) string {
	switch status {
	case 401, 403:
		return "auth_failed"
	case 402:
		return "insufficient_balance"
	case 429:
		return "rate_limited"
	case 400, 422:
		return "invalid_parameters"
	case 404:
		return "not_found"
	default:
		return "upstream_unavailable"
	}
}
func safeUsage(raw json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	allowed := map[string]json.RawMessage{}
	for _, key := range []string{"cost", "prompt_tokens", "completion_tokens", "total_tokens", "input_tokens", "output_tokens"} {
		if value, ok := fields[key]; ok {
			var number json.Number
			if json.Unmarshal(value, &number) == nil {
				if v, err := number.Float64(); err == nil && v >= 0 {
					allowed[key] = value
				}
			}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	data, _ := json.Marshal(allowed)
	return data
}

// Query uses only the original adapter/account and constructs its own task URL.
func (c *Client) Query(ctx context.Context, config catalogdomain.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	if config.Validate() != nil || !validKey(key) || !validTaskID(taskID) {
		return unknownResult(), nil
	}
	path := "/videos/" + taskID
	if config.AdapterKey == "volcengine_ark" {
		path = "/contents/generations/tasks/" + taskID
	}
	data, status, err := c.request(ctx, config, key, http.MethodGet, path, nil, 1<<20)
	if err != nil {
		return application.GenerationResult{State: "unknown", ErrorCode: "query_unreachable"}, nil
	}
	if status < 200 || status >= 300 {
		state := "unknown"
		if status == 404 {
			state = "not_found"
		}
		return application.GenerationResult{State: state, ErrorCode: safeHTTPError(status)}, nil
	}
	var response struct {
		ID           string   `json:"id"`
		Status       string   `json:"status"`
		UnsignedURLs []string `json:"unsigned_urls"`
		Content      struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &response) != nil || response.ID != taskID {
		return unknownResult(), nil
	}
	result := application.GenerationResult{TaskID: taskID, Usage: safeUsage(response.Usage)}
	switch response.Status {
	case "queued", "pending":
		result.State = "pending"
	case "running", "in_progress":
		result.State = "running"
	case "failed", "expired":
		result.State = "failed"
		result.ErrorCode = "generation_failed"
	case "cancelled":
		result.State = "cancelled"
	case "completed", "succeeded":
		result.State = "succeeded"
		if config.AdapterKey == "openrouter" {
			if len(response.UnsignedURLs) != 1 {
				return unknownResult(), nil
			}
			expected := config.BaseURL + "/videos/" + taskID + "/content?index=0"
			if response.UnsignedURLs[0] != expected {
				return unknownResult(), nil
			}
			result.URLs = response.UnsignedURLs
		} else {
			parsed, err := url.Parse(response.Content.VideoURL)
			if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || !strings.HasSuffix(parsed.Hostname(), ".tos-cn-beijing.volces.com") {
				return unknownResult(), nil
			}
			result.URLs = []string{response.Content.VideoURL}
		}
	default:
		return unknownResult(), nil
	}
	return result, nil
}

// Cancel deliberately does not map DELETE to cancellation: Ark DELETE may delete
// a completed task after a queued-to-completed race, and OpenRouter has no reviewed
// cancellation contract here. Model configuration must advertise supports_cancel=false.
func (c *Client) Cancel(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	return application.GenerationResult{Outcome: "not_cancelled", ErrorCode: "cancel_unsupported"}, nil
}

var _ application.GenerationProvider = (*Client)(nil)
