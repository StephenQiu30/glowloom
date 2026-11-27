package application

import (
	"encoding/json"
	"strings"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// KeyEndpoint is an official, authenticated read-only endpoint. Base URLs come
// from the provider catalogue; custom and compatible hosts are never probed.
type KeyEndpoint struct {
	Provider  string `json:"provider"`
	BaseURL   string `json:"base_url"`
	Path      string `json:"-"`
	MediaOnly bool   `json:"media_only,omitempty"`
}

var keyPaths = map[string]string{
	"volcengine_ark":    "/models",
	"openai":            "/models",
	"deepseek":          "/models",
	"openrouter":        "/key",
	"vercel":            "/credits",
	"anthropic":         "/models?limit=1",
	"byteplus":          "/contents/generations/tasks?page_num=1&page_size=1",
	"volcengine_speech": "/tts/bidirection",
	"volcengine_motion": "/api/common/v3/get_result_v2",
	"volcengine_avatar": "/api/common/v3/get_result",
}

// KeyEndpoints returns the endpoint choices supported for automatic verification.
func KeyEndpoints() []KeyEndpoint {
	out := []KeyEndpoint{}
	for _, spec := range catalog.AdapterSpecifications() {
		if path := keyPaths[spec.AdapterKey]; path != "" {
			endpoint := KeyEndpoint{Provider: spec.AdapterKey, BaseURL: spec.AllowedBaseURL, Path: path}
			switch spec.AdapterKey {
			case "volcengine_speech", "volcengine_motion", "volcengine_avatar":
				endpoint.MediaOnly = true
			}
			out = append(out, endpoint)
			if spec.AdapterKey == "deepseek" {
				endpoint.BaseURL += "/v1"
				out = append(out, endpoint)
			}
		}
	}
	return out
}

// KeyEndpointFor resolves the trusted read-only endpoint for a validated route.
func KeyEndpointFor(route domain.Route) (KeyEndpoint, bool) {
	var adapter string
	switch route.Protocol {
	case "chat", "responses", "messages":
	case domain.EmbeddingProtocol:
		adapter = "volcengine_ark"
	case "media":
		var driver catalog.ProviderConfig
		if json.Unmarshal(route.DriverConfig, &driver) != nil || driver.BaseURL != route.BaseURL || driver.Validate() != nil {
			return KeyEndpoint{}, false
		}
		adapter = driver.AdapterKey
	default:
		return KeyEndpoint{}, false
	}
	for _, endpoint := range KeyEndpoints() {
		if strings.TrimSuffix(route.BaseURL, "/") == endpoint.BaseURL &&
			(!endpoint.MediaOnly || route.Protocol == "media") &&
			(adapter == "" || adapter == endpoint.Provider) &&
			(route.Protocol == "messages") == (endpoint.Provider == "anthropic") {
			return endpoint, true
		}
	}
	return KeyEndpoint{}, false
}
