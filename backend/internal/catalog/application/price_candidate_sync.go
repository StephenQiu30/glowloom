package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ErrInvalidPriceCandidateSync reports an invalid synchronization dependency or source result.
var ErrInvalidPriceCandidateSync = errors.New("invalid price candidate synchronization")

const maxPriceSyncModels = 1000

// OfficialPricePageTarget binds a preset to its published, human-readable price source.
type OfficialPricePageTarget struct {
	AdapterKey      string
	ProviderModelID string
	SourceURL       string
	Currency        string
	Unit            *domain.PriceUnit
	ReferenceLines  []string
}

// PriceCandidateSyncStore reads preset identities and persists public candidates.
type PriceCandidateSyncStore interface {
	ListOpenRouterModelIDs(context.Context) ([]string, error)
	ListOfficialPricePageTargets(context.Context) ([]OfficialPricePageTarget, error)
	UpsertPriceCandidates(context.Context, []domain.PriceCandidate) (int, error)
}

// PriceCandidateSource fetches sources with distinct machine and page contracts.
type PriceCandidateSource interface {
	FetchOpenRouterCandidates(context.Context, []string, time.Time) ([]domain.PriceCandidate, error)
	FetchOfficialPageCandidates(context.Context, []OfficialPricePageTarget, time.Time) ([]domain.PriceCandidate, error)
}

// PriceCandidateSyncResult summarizes one bounded source sweep.
type PriceCandidateSyncResult struct {
	ModelsConsidered  int
	CandidatesFetched int
	CandidatesChanged int
}

// PriceCandidateSyncService coordinates a public price-source sweep.
type PriceCandidateSyncService struct {
	store  PriceCandidateSyncStore
	source PriceCandidateSource
	now    func() time.Time
}

// NewPriceCandidateSyncService injects the persistence, public source, and clock.
func NewPriceCandidateSyncService(store PriceCandidateSyncStore, source PriceCandidateSource, now func() time.Time) *PriceCandidateSyncService {
	return &PriceCandidateSyncService{store: store, source: source, now: now}
}

// Synchronize records review candidates without publishing a price or changing a release.
func (s *PriceCandidateSyncService) Synchronize(ctx context.Context) (PriceCandidateSyncResult, error) {
	if s == nil || s.store == nil || s.source == nil || s.now == nil {
		return PriceCandidateSyncResult{}, ErrInvalidPriceCandidateSync
	}
	fetchedAt := s.now().UTC()
	if fetchedAt.IsZero() {
		return PriceCandidateSyncResult{}, ErrInvalidPriceCandidateSync
	}
	var syncErrors []error
	modelIDs, err := s.store.ListOpenRouterModelIDs(ctx)
	if err != nil {
		syncErrors = append(syncErrors, fmt.Errorf("read OpenRouter model allowlist: %w", err))
	} else {
		if len(modelIDs) > maxPriceSyncModels {
			return PriceCandidateSyncResult{}, ErrInvalidPriceCandidateSync
		}
		modelIDs, err = normalizePriceSyncModelIDs(modelIDs)
		if err != nil {
			return PriceCandidateSyncResult{}, err
		}
	}
	pageTargets, targetErr := s.store.ListOfficialPricePageTargets(ctx)
	if targetErr != nil {
		syncErrors = append(syncErrors, fmt.Errorf("read official price page targets: %w", targetErr))
	}
	if len(pageTargets) > maxPriceSyncModels {
		return PriceCandidateSyncResult{}, ErrInvalidPriceCandidateSync
	}
	result := PriceCandidateSyncResult{ModelsConsidered: len(modelIDs) + len(pageTargets)}
	candidates := make([]domain.PriceCandidate, 0, len(modelIDs)+len(pageTargets))
	allowed := make(map[string]struct{}, len(modelIDs)+len(pageTargets))
	for _, modelID := range modelIDs {
		allowed[priceSyncIdentity("openrouter", modelID)] = struct{}{}
	}
	for _, target := range pageTargets {
		if strings.TrimSpace(target.AdapterKey) == "" || strings.TrimSpace(target.ProviderModelID) == "" ||
			target.SourceURL == "" || strings.TrimSpace(target.Currency) == "" {
			return PriceCandidateSyncResult{}, ErrInvalidPriceCandidateSync
		}
		allowed[priceSyncIdentity(target.AdapterKey, target.ProviderModelID)] = struct{}{}
	}
	if len(modelIDs) > 0 {
		fetched, fetchErr := s.source.FetchOpenRouterCandidates(ctx, modelIDs, fetchedAt)
		candidates = append(candidates, fetched...)
		if fetchErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("fetch OpenRouter prices: %w", fetchErr))
		}
	}
	if len(pageTargets) > 0 {
		fetched, fetchErr := s.source.FetchOfficialPageCandidates(ctx, pageTargets, fetchedAt)
		candidates = append(candidates, fetched...)
		if fetchErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("fetch official price pages: %w", fetchErr))
		}
	}
	if len(candidates) > len(allowed) {
		return PriceCandidateSyncResult{}, ErrInvalidPriceCandidateSync
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		identity := priceSyncIdentity(candidate.AdapterKey, candidate.ProviderModelID)
		if err := candidate.Validate(); err != nil || candidate.Status != domain.PriceCandidatePendingReview && candidate.Status != domain.PriceCandidateNeedsReview {
			return PriceCandidateSyncResult{}, ErrInvalidPriceCandidateSync
		}
		if _, ok := allowed[identity]; !ok {
			return PriceCandidateSyncResult{}, ErrInvalidPriceCandidateSync
		}
		if _, duplicated := seen[identity]; duplicated {
			return PriceCandidateSyncResult{}, ErrInvalidPriceCandidateSync
		}
		seen[identity] = struct{}{}
	}
	result.CandidatesFetched = len(candidates)
	if len(candidates) > 0 {
		result.CandidatesChanged, err = s.store.UpsertPriceCandidates(ctx, candidates)
		if err != nil {
			return PriceCandidateSyncResult{}, fmt.Errorf("persist price candidates: %w", err)
		}
	}
	if len(syncErrors) > 0 {
		return result, errors.Join(syncErrors...)
	}
	return result, nil
}

func priceSyncIdentity(adapterKey, providerModelID string) string {
	return adapterKey + "\\x00" + providerModelID
}

func normalizePriceSyncModelIDs(modelIDs []string) ([]string, error) {
	unique := make(map[string]struct{}, len(modelIDs))
	result := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" || len(modelID) > 256 || strings.ContainsAny(modelID, "\x00\r\n") {
			return nil, ErrInvalidPriceCandidateSync
		}
		if _, ok := unique[modelID]; ok {
			continue
		}
		unique[modelID] = struct{}{}
		result = append(result, modelID)
	}
	slices.Sort(result)
	return result, nil
}
