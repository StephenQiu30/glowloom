package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
)

func imageUsage(meter string, raw json.RawMessage) (domain.Usage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return domain.Usage{}, domain.ErrUnknown
	}
	if meter == domain.ReportedImageAmount {
		var cost json.Number
		if json.Unmarshal(fields["cost"], &cost) != nil || cost == "" {
			return domain.Usage{}, domain.ErrUnknown
		}
		return domain.Usage{ProviderAmount: cost}, nil
	}
	input, inputErr := count(fields, "input_tokens", true)
	output, outputErr := count(fields, "output_tokens", true)
	total, totalErr := count(fields, "total_tokens", false)
	if inputErr != nil || outputErr != nil || totalErr != nil || input <= 0 || output <= 0 ||
		fields["total_tokens"] != nil && total != input+output ||
		!imageTokenDetails(fields["input_tokens_details"], input, false) ||
		fields["output_tokens_details"] != nil && !imageTokenDetails(fields["output_tokens_details"], output, true) {
		return domain.Usage{}, domain.ErrUnknown
	}
	return domain.Usage{Input: input, Output: output}, nil
}

func imageTokenDetails(raw json.RawMessage, expected int64, imageOutput bool) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	text, textErr := count(fields, "text_tokens", true)
	image, imageErr := count(fields, "image_tokens", true)
	return textErr == nil && imageErr == nil && text+image == expected &&
		(!imageOutput && image == 0 || imageOutput && text == 0)
}

// The AI SDK image contract exposes the Vercel generation ID for accounting.
const vercelImageOrigin = "https://ai-gateway.vercel.sh"

var vercelGenerationID = regexp.MustCompile(`^gen_[A-Za-z0-9]{26}$`)

func submitVercelImage(ctx context.Context, transport *mediaTransport, input operation.GenerationRequest) (operation.GenerationResult, error) {
	if strings.TrimRight(input.Config.BaseURL, "/") != vercelImageOrigin+"/v1" || input.Capability != "image.generate" ||
		input.OutputCount != 1 || input.Mode != "text2image" || len(input.References) != 0 {
		return operation.GenerationResult{}, application.ErrNotSent
	}
	payload := map[string]any{"prompt": input.Prompt, "n": 1}
	for key, value := range input.Params {
		switch key {
		case "size":
			payload[key] = value
		case "quality":
			payload["providerOptions"] = map[string]any{"openai": map[string]any{"quality": value}}
		default:
			return operation.GenerationResult{}, application.ErrNotSent
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return operation.GenerationResult{}, application.ErrNotSent
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, vercelImageOrigin+"/v4/ai/image-model", bytes.NewReader(body))
	if err != nil {
		return operation.GenerationResult{}, application.ErrNotSent
	}
	response, err := transport.RoundTrip(req)
	if err != nil {
		return operation.GenerationResult{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return operation.GenerationResult{}, domain.ErrUnknown
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	var reply struct {
		Images           []string `json:"images"`
		ProviderMetadata struct {
			Gateway struct {
				Cost         json.RawMessage `json:"cost"`
				GenerationID string          `json:"generationId"`
			} `json:"gateway"`
		} `json:"providerMetadata"`
	}
	if err != nil || json.Unmarshal(data, &reply) != nil || len(reply.Images) != 1 {
		return operation.GenerationResult{}, domain.ErrUnknown
	}
	image, err := base64.StdEncoding.DecodeString(reply.Images[0])
	if err != nil || len(image) == 0 {
		return operation.GenerationResult{}, domain.ErrUnknown
	}
	kind := http.DetectContentType(image)
	if !slices.Contains([]string{"image/png", "image/jpeg", "image/webp"}, kind) {
		return operation.GenerationResult{}, domain.ErrUnknown
	}
	result := operation.GenerationResult{State: operation.ProviderTaskSucceeded, Outcome: operation.ProviderOutcomeCompleted,
		Images: []operation.GeneratedImage{{Bytes: image, MediaType: kind}}}
	evidence := reply.ProviderMetadata.Gateway
	var cost json.Number
	if json.Unmarshal(evidence.Cost, &cost) != nil {
		cost = ""
	}
	if vercelGenerationID.MatchString(evidence.GenerationID) {
		result.ProviderLogID = evidence.GenerationID
		if cost == "" {
			cost = vercelImageCost(ctx, transport.fence, evidence.GenerationID, input.ModelID)
		}
	}
	if cost != "" {
		result.Usage, _ = json.Marshal(map[string]json.Number{"cost": cost})
	}
	return result, nil
}

func vercelImageCost(ctx context.Context, fence fenced, id, model string) json.Number {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, vercelImageOrigin+"/v1/generation?id="+url.QueryEscape(id), nil)
	if err != nil {
		return ""
	}
	response, err := fence.Do(request)
	if err != nil {
		return ""
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	var info struct {
		Data struct {
			ID, Model string
			TotalCost json.Number `json:"total_cost"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&info) != nil ||
		info.Data.ID != id || info.Data.Model != model {
		return ""
	}
	return info.Data.TotalCost
}
