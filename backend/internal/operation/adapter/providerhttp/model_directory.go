package providerhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"unicode"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ReadModelIDs uses only the installed directory paths on validated endpoints.
func (c *Client) ReadModelIDs(ctx context.Context, config catalogdomain.ProviderConfig, key string) ([]string, error) {
	if config.Validate() != nil {
		return nil, catalogapp.ErrModelDirectoryUnsupported
	}
	switch config.AdapterKey {
	case "openai", "deepseek", "openrouter", "vercel", "volcengine_ark":
	default:
		return nil, catalogapp.ErrModelDirectoryUnsupported
	}

	body, status, err := c.request(ctx, config, key, http.MethodGet, "/models", nil, 1<<20)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return nil, catalogapp.ErrModelDirectoryCredential
	case status == http.StatusTooManyRequests:
		return nil, catalogapp.ErrModelDirectoryRateLimited
	case err != nil || status != http.StatusOK:
		return nil, catalogapp.ErrModelDirectoryUnavailable
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Data == nil {
		return nil, catalogapp.ErrModelDirectoryUnavailable
	}

	seen := make(map[string]bool, len(payload.Data))
	ids := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := item.ID
		if id == "" || len(id) > 200 || strings.ContainsFunc(id, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}
