package providerhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// VolcengineVisualDriver implements the communication strategy for Volcengine Visual OpenAPI
// (JiMeng Avatar and Motion Imitation).
type VolcengineVisualDriver struct{}

func init() {
	RegisterDriver(&VolcengineVisualDriver{}, "volcengine_avatar", "volcengine_motion")
}

func (d *VolcengineVisualDriver) Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	return c.submitVisual(ctx, input, key)
}

func (d *VolcengineVisualDriver) Query(ctx context.Context, c *Client, config catalog.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	return c.queryVisual(ctx, config, taskID, key)
}

func (d *VolcengineVisualDriver) Probe(ctx context.Context, c *Client, config catalog.ProviderConfig, key string) ProbeResult {
	if config.Validate() != nil {
		return ProbeResult{Auth: "unknown", Adapter: "unsupported", Discovery: "unsupported", Candidates: []catalogapp.ModelCandidate{}}
	}
	return ProbeResult{Auth: "unknown", Adapter: "ok", Discovery: "unsupported", Candidates: []catalogapp.ModelCandidate{}}
}

func (d *VolcengineVisualDriver) ProbeModelDefaultParams(capability string) map[string]any {
	return nil
}

const avatarModel = "jimeng_realman_avatar_picture_omni_v15"
const motionModel = "jimeng_dreamactor_m20_gen_video"

func visualProtocol(adapter string) (model, submit, query string) {
	if adapter == "volcengine_avatar" {
		return avatarModel, "/api/common/v3/submit_task", "/api/common/v3/get_result"
	}
	if adapter == "volcengine_motion" {
		return motionModel, "/api/common/v3/submit_task_v2", "/api/common/v3/get_result_v2"
	}
	return "", "", ""
}

func (c *Client) submitVisual(ctx context.Context, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	model, path, _ := visualProtocol(input.Config.AdapterKey)
	invalid := application.GenerationResult{Outcome: "not_submitted", ErrorCode: "invalid_provider_request"}
	if input.Config.Validate() != nil || !validKey(key) || input.ModelID != model || input.Capability != "video.generate" || input.OutputCount != 1 || input.EndpointTag != "" || len(input.References) != 2 {
		return invalid, nil
	}
	payload := map[string]any{"req_key": model}
	avatar := input.Config.AdapterKey == "volcengine_avatar"
	if avatar {
		if input.Mode != "avatar" || utf8.RuneCountInString(input.Prompt) > 300 {
			return invalid, nil
		}
		if strings.TrimSpace(input.Prompt) != "" {
			payload["prompt"] = input.Prompt
		}
	} else if input.Mode != "motion" || strings.TrimSpace(input.Prompt) != "" {
		return invalid, nil
	}
	seen := map[string]bool{}
	for _, ref := range input.References {
		u, err := url.Parse(ref.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(ref.URL) > 8192 || seen[ref.Role] {
			return invalid, nil
		}
		seen[ref.Role] = true
		switch {
		case ref.Role == "character_image" && ref.MediaKind == "image":
			if avatar {
				payload["image_url"] = ref.URL
			} else {
				payload["image_urls"] = []string{ref.URL}
			}
		case avatar && ref.Role == "driving_audio" && ref.MediaKind == "audio":
			payload["audio_url"] = ref.URL
		case !avatar && ref.Role == "driving_video" && ref.MediaKind == "video":
			payload["video_url"] = ref.URL
		default:
			return invalid, nil
		}
	}
	if !seen["character_image"] || (avatar && !seen["driving_audio"]) || (!avatar && !seen["driving_video"]) {
		return invalid, nil
	}
	for field, value := range input.Params {
		switch {
		case avatar && field == "output_resolution":
			n, ok := integerParam(value)
			if !ok || (n != 720 && n != 1080) {
				return invalid, nil
			}
			payload[field] = n
		case avatar && field == "pe_fast_mode" || !avatar && field == "cut_result_first_second_switch":
			b, ok := value.(bool)
			if !ok {
				return invalid, nil
			}
			payload[field] = b
		default:
			return invalid, nil
		}
	}
	data, status, err := c.request(ctx, input.Config, key, http.MethodPost, path, payload, 1<<20)
	if err != nil {
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "transport_unknown"}, nil
	}
	if status < 200 || status >= 300 {
		return submitHTTPFailure(status), nil
	}
	var response visualResponse
	if json.Unmarshal(data, &response) != nil || response.Code == nil || hasProviderError(response.Metadata.Error) {
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "invalid_response"}, nil
	}
	if *response.Code != 10000 {
		return visualError(*response.Code, true), nil
	}
	if response.Data == nil || !validTaskID(response.Data.TaskID) {
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "invalid_response"}, nil
	}
	return application.GenerationResult{Outcome: "accepted", State: "pending", TaskID: response.Data.TaskID}, nil
}

type visualResponse struct {
	Code     *int `json:"code"`
	Metadata struct {
		Error json.RawMessage `json:"Error"`
	} `json:"ResponseMetadata"`
	Data *struct {
		TaskID   string `json:"task_id"`
		Status   string `json:"status"`
		VideoURL string `json:"video_url"`
	} `json:"data"`
}

func (c *Client) queryVisual(ctx context.Context, config catalog.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	model, _, path := visualProtocol(config.AdapterKey)
	if config.Validate() != nil || !validKey(key) || !validTaskID(taskID) {
		return unknownResult(), nil
	}
	data, status, err := c.request(ctx, config, key, http.MethodPost, path, map[string]string{"req_key": model, "task_id": taskID}, 1<<20)
	if err != nil || status != 200 {
		return unknownResult(), nil
	}
	var response visualResponse
	if json.Unmarshal(data, &response) != nil || response.Code == nil || hasProviderError(response.Metadata.Error) {
		return unknownResult(), nil
	}
	if *response.Code != 10000 {
		return visualError(*response.Code, false), nil
	}
	if response.Data == nil || response.Data.TaskID != "" && response.Data.TaskID != taskID {
		return unknownResult(), nil
	}
	result := application.GenerationResult{TaskID: taskID}
	switch response.Data.Status {
	case "in_queue":
		result.State = "pending"
	case "processing", "generating":
		result.State = "running"
	case "not_found":
		result.State = "not_found"
	case "expired":
		result.State = "unknown"
		result.ErrorCode = "task_expired"
	case "done":
		if !catalog.ValidCreationResultURL(config.AdapterKey, response.Data.VideoURL) {
			return unknownResult(), nil
		}
		result.State = "succeeded"
		result.URLs = []string{response.Data.VideoURL}
	default:
		return unknownResult(), nil
	}
	return result, nil
}

func visualError(code int, submitting bool) application.GenerationResult {
	result := application.GenerationResult{State: "unknown", ProviderTerminalCode: code, ErrorCode: "provider_error"}
	// Unknown server errors never authorize a duplicate charged submission.
	if submitting {
		result.Outcome = "unknown"
	}
	if submitting && slices.Contains([]int{50200, 50201, 50204, 50205, 50211, 50214, 50215, 50400, 50411, 50412, 50413, 50511, 50512, 50513, 50514, 50429, 50430}, code) {
		result.Outcome = "rejected"
	}
	if !submitting && slices.Contains([]int{50200, 50201, 50204, 50205, 50211, 50214, 50215, 50411, 50412, 50413, 50511, 50512, 50513, 50514}, code) {
		result.State = "failed"
		result.ErrorCode = "generation_failed"
	}
	return result
}

func submitHTTPFailure(status int) application.GenerationResult {
	result := application.GenerationResult{Outcome: "unknown", ErrorCode: safeHTTPError(status)}
	if slices.Contains([]int{400, 401, 402, 403, 404, 422, 429}, status) {
		result.Outcome = "rejected"
	}
	return result
}
