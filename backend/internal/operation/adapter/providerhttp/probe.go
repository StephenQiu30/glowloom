package providerhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ProbeResult contains independent safe result categories and catalog candidates.
type ProbeResult struct {
	Auth, Adapter, Discovery string
	Candidates               []catalogapp.ModelCandidate
}

// Probe performs read-only authentication and discovery; it cannot generate output.
func (c *Client) Probe(ctx context.Context, config catalogdomain.ProviderConfig, key string) ProbeResult {
	result := ProbeResult{Auth: "unknown", Adapter: "ok", Discovery: "unsupported", Candidates: []catalogapp.ModelCandidate{}}
	if config.Validate() != nil || (!validKey(key) && config.AdapterKey != "volcengine_music") {
		result.Adapter = "unsupported"
		return result
	}
	// Speech has no supported read-only Key probe. Generating audio is allowed
	// only through an explicit management model validation.
	if config.AdapterKey == "volcengine_speech" || config.AdapterKey == "volcengine_music" || config.AdapterKey == "volcengine_avatar" || config.AdapterKey == "volcengine_motion" {
		result.Adapter = "ok"
		return result
	}
	path := "/key"
	if config.AdapterKey == "volcengine_ark" || config.AdapterKey == "byteplus" {
		path = "/contents/generations/tasks?page_size=1"
	}
	if config.AdapterKey == "deepseek" {
		path = "/models"
	}
	authData, status, err := c.request(ctx, config, key, http.MethodGet, path, nil, 1<<20)
	if err != nil {
		result.Auth = "unreachable"
		return result
	}
	if status != 200 {
		result.Auth = safeHTTPError(status)
		return result
	}
	result.Auth = "ok"
	if config.AdapterKey == "volcengine_ark" || config.AdapterKey == "byteplus" {
		return result
	}
	if config.AdapterKey == "deepseek" {
		var response struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if json.Unmarshal(authData, &response) != nil || response.Data == nil || len(response.Data) > 200 {
			result.Discovery = "invalid_response"
			return result
		}
		for _, model := range response.Data {
			if model.ID == "" || len(model.ID) > 256 {
				result.Discovery = "invalid_response"
				return result
			}
			result.Candidates = append(result.Candidates, catalogapp.ModelCandidate{ProviderModelID: model.ID, DisplayName: model.ID, Capability: "text.generate", Metadata: json.RawMessage(`{"verified":false}`)})
		}
		result.Discovery = "ok"
		return result
	}
	for _, entry := range []struct{ Path, Capability string }{{"/models", "text.generate"}, {"/images/models", "image.generate"}, {"/videos/models", "video.generate"}} {
		data, status, err := c.request(ctx, config, key, http.MethodGet, entry.Path, nil, 8<<20)
		if err != nil || status != 200 {
			result.Discovery = "unreachable"
			return result
		}
		var response struct {
			Data []struct {
				ID                  string          `json:"id"`
				Name                string          `json:"name"`
				Architecture        json.RawMessage `json:"architecture"`
				SupportedParameters json.RawMessage `json:"supported_parameters"`
				Pricing             json.RawMessage `json:"pricing"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &response) != nil || len(response.Data) > 5000 {
			result.Discovery = "invalid_response"
			return result
		}
		if len(response.Data) > 200 {
			response.Data = response.Data[:200]
			result.Discovery = "partial"
		}
		for _, model := range response.Data {
			if entry.Capability == "text.generate" {
				var architecture struct {
					OutputModalities []string `json:"output_modalities"`
				}
				if json.Unmarshal(model.Architecture, &architecture) != nil || !slices.Contains(architecture.OutputModalities, "text") {
					continue
				}
			}
			if model.ID == "" || len(model.ID) > 256 || len(model.Name) > 300 {
				continue
			}
			metadata, _ := json.Marshal(map[string]any{"architecture": model.Architecture, "supported_parameters": model.SupportedParameters, "pricing": model.Pricing, "verified": false})
			if len(metadata) > 2048 {
				continue
			}
			result.Candidates = append(result.Candidates, catalogapp.ModelCandidate{ProviderModelID: model.ID, DisplayName: model.Name, Capability: entry.Capability, Metadata: metadata})
		}
	}
	if result.Discovery != "partial" {
		result.Discovery = "ok"
	}
	return result
}
