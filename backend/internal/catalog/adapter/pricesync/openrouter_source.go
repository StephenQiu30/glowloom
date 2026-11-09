package pricesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

const (
	maxOpenRouterCatalogBytes = 32 << 20
	maxOpenRouterModels       = 1000
)

// OpenRouterSource reads the public catalog and emits candidates only for the allowlist.
type OpenRouterSource struct {
	client *http.Client
}

// NewOpenRouterSource constructs a client that only follows HTTPS redirects to OpenRouter.
func NewOpenRouterSource(client *http.Client) *OpenRouterSource {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	} else {
		clone := *client
		client = &clone
	}
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" || !strings.EqualFold(request.URL.Host, "openrouter.ai") {
			return errors.New("OpenRouter catalog redirected outside its official host")
		}
		if len(via) >= 3 {
			return errors.New("too many OpenRouter catalog redirects")
		}
		if previousRedirect != nil {
			return previousRedirect(request, via)
		}
		return nil
	}
	return &OpenRouterSource{client: client}
}

type openRouterCatalog struct {
	Data []struct {
		ID      string          `json:"id"`
		Pricing json.RawMessage `json:"pricing"`
	} `json:"data"`
}

// FetchOpenRouterCandidates fetches one public catalog snapshot without credentials.
func (s *OpenRouterSource) FetchOpenRouterCandidates(ctx context.Context, modelIDs []string, fetchedAt time.Time) ([]domain.PriceCandidate, error) {
	if s == nil || s.client == nil || fetchedAt.IsZero() || len(modelIDs) == 0 || len(modelIDs) > maxOpenRouterModels {
		return nil, ErrInvalidOpenRouterPricing
	}
	allowed := make(map[string]struct{}, len(modelIDs))
	for _, id := range modelIDs {
		if strings.TrimSpace(id) == "" || len(id) > 256 {
			return nil, ErrInvalidOpenRouterPricing
		}
		allowed[id] = struct{}{}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, OpenRouterCatalogURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build OpenRouter catalog request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request OpenRouter catalog: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if err := response.Body.Close(); err != nil {
			return nil, fmt.Errorf("close unsuccessful OpenRouter response: %w", err)
		}
		return nil, fmt.Errorf("OpenRouter catalog returned HTTP %d", response.StatusCode)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxOpenRouterCatalogBytes+1))
	closeErr := response.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read OpenRouter catalog: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close OpenRouter response: %w", closeErr)
	}
	if len(body) > maxOpenRouterCatalogBytes {
		return nil, ErrInvalidOpenRouterPricing
	}
	var catalog openRouterCatalog
	if err := json.Unmarshal(body, &catalog); err != nil || catalog.Data == nil {
		return nil, ErrInvalidOpenRouterPricing
	}
	candidates := make([]domain.PriceCandidate, 0, len(modelIDs))
	seen := make(map[string]struct{}, len(modelIDs))
	for _, model := range catalog.Data {
		if _, ok := allowed[model.ID]; !ok {
			continue
		}
		if _, duplicate := seen[model.ID]; duplicate {
			return nil, ErrInvalidOpenRouterPricing
		}
		seen[model.ID] = struct{}{}
		pricing := model.Pricing
		if len(pricing) == 0 || string(pricing) == "null" {
			// Missing price metadata is a review fact, never a free-price candidate.
			pricing = json.RawMessage(`{"pricing_missing":true}`)
		}
		candidate, err := ParseOpenRouterPriceCandidate(model.ID, pricing, fetchedAt)
		if err != nil {
			return nil, fmt.Errorf("parse OpenRouter price for %q: %w", model.ID, err)
		}
		candidates = append(candidates, candidate)
	}
	slices.SortFunc(candidates, func(a, b domain.PriceCandidate) int {
		return strings.Compare(a.ProviderModelID, b.ProviderModelID)
	})
	return candidates, nil
}
