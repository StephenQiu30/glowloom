package aigateway

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	workflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

// downloadResult uses only the original supplier's frozen allowlist. Authenticated
// video content reads retain the original task and credential and refuse redirects.
func (r *Runtime) downloadResult(ctx context.Context, s deliverySource, result operation.GenerationResult, raw string) (*media.Downloaded, error) {
	var config catalog.ProviderConfig
	if json.Unmarshal(s.Route.DriverConfig, &config) != nil || config.Validate() != nil || config.BaseURL != s.Route.BaseURL {
		return nil, domain.ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Port() != "" {
		return nil, workflow.ErrUnsafeResultURL
	}
	authenticated := config.AdapterKey == "openrouter" && s.Call.Model.Capability == "video.generate"
	if authenticated {
		if result.TaskID == "" || strings.ContainsAny(result.TaskID, "/?#\\") || raw != config.BaseURL+"/videos/"+result.TaskID+"/content?index=0" {
			return nil, workflow.ErrUnsafeResultURL
		}
	} else if !catalog.ValidCreationResultURL(config.AdapterKey, raw) && (config.AdapterKey != "volcengine_ark" || !strings.HasSuffix(u.Hostname(), ".tos-cn-beijing.volces.com")) {
		return nil, workflow.ErrUnsafeResultURL
	}
	downloader, err := workflow.NewSecureDownloader(workflow.DownloadPolicy{AllowedOrigins: []string{"https://" + u.Host}, MaxBytes: 512 << 20})
	if err != nil {
		return nil, err
	}
	if !authenticated {
		return downloader.Download(ctx, raw)
	}
	if r.Keys == nil {
		return nil, domain.ErrUnavailable
	}
	secret, err := r.Keys.Open(s.Route.KeyID, s.Route.Supplier, s.Route.Credential, s.Route.Ciphertext)
	if err != nil {
		return nil, err
	}
	defer clear(secret)
	var envelope struct {
		APIKey string `json:"api_key"`
	}
	if json.Unmarshal(secret, &envelope) != nil || envelope.APIKey == "" || strings.ContainsAny(envelope.APIKey, "\r\n\x00") {
		return nil, domain.ErrInvalid
	}
	return downloader.DownloadAuthenticated(ctx, raw, "Bearer "+envelope.APIKey)
}
