package providerhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// VolcengineArkDriver implements the communication strategy for Volcengine Ark (方舟)
// and BytePlus MaaS APIs (chat completions, image generations, and video tasks).
type VolcengineArkDriver struct{}

func init() {
	RegisterDriver(&VolcengineArkDriver{}, "volcengine_ark", "byteplus")
}

func (d *VolcengineArkDriver) Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	if !validKey(key) || !validateCommonInput(input) {
		return invalidSubmissionResult(), nil
	}
	payload := map[string]any{"model": input.ModelID, "prompt": input.Prompt}
	path := ""

	var (
		frames     []map[string]any
		references []map[string]any
		seenFrames = map[string]bool{}
	)

	switch input.Capability {
	case "text.generate":
		if input.OutputCount != 1 || len(input.References) != 0 || input.Mode != "text_to_text" {
			return invalidSubmissionResult(), nil
		}
		path = "/chat/completions"
		delete(payload, "prompt")
		payload["messages"] = []map[string]string{{"role": "user", "content": input.Prompt}}
		payload["stream"] = false

	case "image.generate":
		if input.OutputCount != 1 {
			return invalidSubmissionResult(), nil
		}
		path = "/images/generations"
		payload["response_format"] = "b64_json"
		payload["sequential_image_generation"] = "disabled"
		payload["stream"] = false
		if len(input.References) > 0 {
			images := make([]string, 0, len(input.References))
			for _, ref := range input.References {
				parsed, err := url.Parse(ref.URL)
				if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || len(ref.URL) > 8192 {
					return invalidSubmissionResult(), nil
				}
				images = append(images, ref.URL)
			}
			payload["image"] = images
		}

	case "video.generate":
		if input.OutputCount != 1 {
			return invalidSubmissionResult(), nil
		}
		path = "/contents/generations/tasks"
		arkContent := []map[string]any{{"type": "text", "text": input.Prompt}}
		for _, ref := range input.References {
			parsed, err := url.Parse(ref.URL)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || len(ref.URL) > 8192 {
				return invalidSubmissionResult(), nil
			}
			if ref.MediaKind != "" && ref.MediaKind != "image" {
				kind := ref.MediaKind
				if input.Mode != "omni_reference" || (kind != "video" && kind != "audio") ||
					(ref.Role != "subject" && ref.Role != "reference" && ref.Role != "reference_"+kind) {
					return invalidSubmissionResult(), nil
				}
				item := map[string]any{"type": kind + "_url", kind + "_url": map[string]string{"url": ref.URL}, "role": "reference_" + kind}
				references = append(references, item)
				arkContent = append(arkContent, item)
				continue
			}
			item := map[string]any{"type": "image_url", "image_url": map[string]string{"url": ref.URL}}
			switch ref.Role {
			case "first_frame", "last_frame":
				if seenFrames[ref.Role] {
					return invalidSubmissionResult(), nil
				}
				seenFrames[ref.Role] = true
				item["role"] = ref.Role
				frames = append(frames, item)
			case "reference_image", "reference", "subject":
				item["role"] = "reference_image"
				references = append(references, item)
			default:
				return invalidSubmissionResult(), nil
			}
			arkContent = append(arkContent, item)
		}
		if len(frames) > 0 && len(references) > 0 || seenFrames["last_frame"] && !seenFrames["first_frame"] {
			return invalidSubmissionResult(), nil
		}
		delete(payload, "prompt")
		payload["content"] = arkContent

	default:
		return invalidSubmissionResult(), nil
	}

	if !applyAllowedParameters(input, payload, input.Capability == "video.generate") {
		return invalidSubmissionResult(), nil
	}
	if strings.HasPrefix(input.ModelID, "doubao-seedance-2-5-") && input.Mode == "image2video" && input.Capability == "video.generate" {
		payload["ratio"] = "adaptive"
	}

	validMode := false
	switch input.Capability {
	case "text.generate":
		validMode = input.Mode == "text_to_text"
	case "image.generate":
		validMode = input.Mode == "text2image" && len(input.References) == 0 || input.Mode == "image2image" && len(input.References) > 0
	case "video.generate":
		validMode = input.Mode == "text2video" && len(input.References) == 0 || input.Mode == "image2video" && seenFrames["first_frame"] || input.Mode == "omni_reference" && len(references) > 0
	}
	if !validMode {
		return invalidSubmissionResult(), nil
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
	result, ok := parseStandardGenerationResponse(input, data)
	if !ok {
		return unknownResult(), nil
	}
	return result, nil
}

func (d *VolcengineArkDriver) Query(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	if config.Validate() != nil || !validKey(key) || !validTaskID(taskID) {
		return unknownResult(), nil
	}
	path := "/contents/generations/tasks/" + taskID
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
		ID      string `json:"id"`
		Status  string `json:"status"`
		Content struct {
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
		parsed, err := url.Parse(response.Content.VideoURL)
		if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || !strings.HasSuffix(parsed.Hostname(), ".tos-cn-beijing.volces.com") {
			return unknownResult(), nil
		}
		result.URLs = []string{response.Content.VideoURL}
	default:
		return unknownResult(), nil
	}
	return result, nil
}

func (d *VolcengineArkDriver) Probe(ctx context.Context, c *Client, config catalogdomain.ProviderConfig, key string) ProbeResult {
	result := ProbeResult{Auth: "unknown", Adapter: "ok", Discovery: "unsupported", Candidates: []catalogapp.ModelCandidate{}}
	if config.Validate() != nil || !validKey(key) {
		result.Adapter = "unsupported"
		return result
	}
	_, status, err := c.request(ctx, config, key, http.MethodGet, "/contents/generations/tasks?page_size=1", nil, 1<<20)
	if err != nil {
		result.Auth = "unreachable"
		return result
	}
	if status != 200 {
		result.Auth = safeHTTPError(status)
		return result
	}
	result.Auth = "ok"
	return result
}

func (d *VolcengineArkDriver) ProbeModelDefaultParams(capability string) map[string]any {
	if capability == "image.generate" {
		return map[string]any{"size": "2K"}
	}
	return nil
}
