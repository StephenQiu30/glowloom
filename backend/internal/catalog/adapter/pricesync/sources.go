package pricesync

import (
	"context"
	"net/http"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// Sources selects the fixed provider price source contract for each sync lane.
type Sources struct {
	openRouter *OpenRouterSource
	official   *OfficialPricePageSource
}

// NewSources constructs the built-in OpenRouter and first-party page readers.
func NewSources(client *http.Client) *Sources {
	return &Sources{
		openRouter: NewOpenRouterSource(client),
		official:   NewOfficialPricePageSource(client),
	}
}

// FetchOpenRouterCandidates fetches structured rates from OpenRouter's public API.
func (s *Sources) FetchOpenRouterCandidates(ctx context.Context, modelIDs []string, fetchedAt time.Time) ([]domain.PriceCandidate, error) {
	if s == nil || s.openRouter == nil {
		return nil, ErrInvalidOpenRouterPricing
	}
	return s.openRouter.FetchOpenRouterCandidates(ctx, modelIDs, fetchedAt)
}

// FetchOfficialPageCandidates snapshots configured first-party price pages for manual review.
func (s *Sources) FetchOfficialPageCandidates(ctx context.Context, targets []application.OfficialPricePageTarget, fetchedAt time.Time) ([]domain.PriceCandidate, error) {
	if s == nil || s.official == nil {
		return nil, ErrInvalidOpenRouterPricing
	}
	return s.official.FetchOfficialPageCandidates(ctx, targets, fetchedAt)
}
