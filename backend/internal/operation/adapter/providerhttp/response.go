package providerhttp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func parseStandardGenerationResponse(input application.GenerationRequest, data []byte) (application.GenerationResult, bool) {
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
		return unknownResult(), false
	}
	result := application.GenerationResult{Outcome: "completed", State: "succeeded", Usage: safeUsage(response.Usage)}
	switch input.Capability {
	case "video.generate":
		if !validTaskID(response.ID) {
			return unknownResult(), false
		}
		return application.GenerationResult{Outcome: "accepted", State: "pending", TaskID: response.ID}, true
	case "text.generate":
		if len(response.Choices) != 1 || response.Choices[0].Message.Content == "" || response.Choices[0].FinishReason != "stop" {
			return unknownResult(), false
		}
		result.Text = response.Choices[0].Message.Content
	case "image.generate":
		if len(response.Data) != input.OutputCount {
			return unknownResult(), false
		}
		for _, image := range response.Data {
			decoded, err := base64.StdEncoding.DecodeString(image.Base64)
			if err != nil || len(decoded) == 0 || len(decoded) > 32<<20 {
				return unknownResult(), false
			}
			kind := http.DetectContentType(decoded)
			if !slices.Contains([]string{"image/png", "image/jpeg", "image/webp"}, kind) || (image.MediaType != "" && image.MediaType != kind) {
				return unknownResult(), false
			}
			result.Images = append(result.Images, application.GeneratedImage{Bytes: decoded, MediaType: kind})
		}
	default:
		return unknownResult(), false
	}
	return result, true
}
