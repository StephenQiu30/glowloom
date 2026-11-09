package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricesync"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

type priceCandidateSyncStore struct {
	modelIDs    []string
	pageTargets []catalogapp.OfficialPricePageTarget
	candidates  []domain.PriceCandidate
	upserts     int
	err         error
}

func (s *priceCandidateSyncStore) ListOpenRouterModelIDs(context.Context) ([]string, error) {
	return s.modelIDs, s.err
}

func (s *priceCandidateSyncStore) ListOfficialPricePageTargets(context.Context) ([]catalogapp.OfficialPricePageTarget, error) {
	return s.pageTargets, s.err
}

func (s *priceCandidateSyncStore) UpsertPriceCandidates(_ context.Context, candidates []domain.PriceCandidate) (int, error) {
	s.candidates = append([]domain.PriceCandidate(nil), candidates...)
	return s.upserts, s.err
}

type priceCandidateSource struct {
	modelIDs    []string
	at          time.Time
	items       []domain.PriceCandidate
	pageItems   []domain.PriceCandidate
	pageTargets []catalogapp.OfficialPricePageTarget
	called      bool
	pageCalled  bool
	err         error
	pageErr     error
}

func (s *priceCandidateSource) FetchOpenRouterCandidates(_ context.Context, modelIDs []string, at time.Time) ([]domain.PriceCandidate, error) {
	s.called = true
	s.modelIDs, s.at = append([]string(nil), modelIDs...), at
	return s.items, s.err
}

func (s *priceCandidateSource) FetchOfficialPageCandidates(_ context.Context, targets []catalogapp.OfficialPricePageTarget, at time.Time) ([]domain.PriceCandidate, error) {
	s.pageCalled = true
	s.pageTargets, s.at = append([]catalogapp.OfficialPricePageTarget(nil), targets...), at
	return s.pageItems, s.pageErr
}

func TestPriceCandidateSyncDeduplicatesIDsAndPersistsReviewedCandidates(t *testing.T) {
	fetchedAt := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	candidate, err := pricesync.ParseOpenRouterPriceCandidate("provider/a", json.RawMessage(`{"prompt":"0.0000025","completion":"0.00001"}`), fetchedAt)
	if err != nil {
		t.Fatal(err)
	}
	store := &priceCandidateSyncStore{
		modelIDs: []string{"provider/b", "provider/a", "provider/a"},
		upserts:  1,
	}
	source := &priceCandidateSource{items: []domain.PriceCandidate{candidate}}
	syncer := catalogapp.NewPriceCandidateSyncService(store, source, func() time.Time { return fetchedAt })
	result, err := syncer.Synchronize(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !source.called || !reflect.DeepEqual(source.modelIDs, []string{"provider/a", "provider/b"}) || !source.at.Equal(fetchedAt) {
		t.Fatalf("source call: called=%v ids=%v time=%s", source.called, source.modelIDs, source.at)
	}
	if result.ModelsConsidered != 2 || result.CandidatesFetched != 1 || result.CandidatesChanged != 1 ||
		len(store.candidates) != 1 || store.candidates[0].Status != domain.PriceCandidatePendingReview {
		t.Fatalf("sync result=%+v candidates=%+v", result, store.candidates)
	}
}

func TestPriceCandidateSyncSkipsNetworkWhenNoConfiguredOpenRouterModels(t *testing.T) {
	store := &priceCandidateSyncStore{}
	source := &priceCandidateSource{}
	syncer := catalogapp.NewPriceCandidateSyncService(store, source, time.Now)
	result, err := syncer.Synchronize(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if source.called || result.ModelsConsidered != 0 || result.CandidatesFetched != 0 || result.CandidatesChanged != 0 {
		t.Fatalf("empty model set made a source request: result=%+v source=%+v", result, source)
	}
}

func TestPriceCandidateSyncPersistsDirectPageCandidatesAsReviewOnly(t *testing.T) {
	fetchedAt := time.Date(2026, 11, 9, 4, 0, 0, 0, time.UTC)
	unit := domain.PricePer1KTokens
	candidate := domain.PriceCandidate{
		ID: uuid.New(), AdapterKey: "deepseek", ProviderModelID: "deepseek-flash",
		SourceURL: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/", SourceHash: "c" + strings.Repeat("a", 63),
		ParserVersion: "official_page_review_v1", SourceFetchedAt: fetchedAt, Currency: "CNY", Unit: &unit,
		SourcePricing: json.RawMessage(`{"evidence_kind":"official_page_excerpt","matched_rows":["高峰输出 | 8元"]}`),
		Status:        domain.PriceCandidateNeedsReview,
	}
	target := catalogapp.OfficialPricePageTarget{
		AdapterKey: "deepseek", ProviderModelID: candidate.ProviderModelID, SourceURL: candidate.SourceURL,
		Currency: candidate.Currency, Unit: &unit, ReferenceLines: []string{"上次审核的每百万Token价格"},
	}
	store := &priceCandidateSyncStore{pageTargets: []catalogapp.OfficialPricePageTarget{target}, upserts: 1}
	source := &priceCandidateSource{pageItems: []domain.PriceCandidate{candidate}}
	result, err := catalogapp.NewPriceCandidateSyncService(store, source, func() time.Time { return fetchedAt }).Synchronize(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelsConsidered != 1 || result.CandidatesFetched != 1 || result.CandidatesChanged != 1 ||
		!source.pageCalled || len(store.candidates) != 1 || store.candidates[0].Status != domain.PriceCandidateNeedsReview ||
		store.candidates[0].Unit == nil || len(store.candidates[0].Rule) != 0 {
		t.Fatalf("direct page candidate was not preserved for manual review: result=%+v candidate=%+v", result, store.candidates)
	}
}

func TestPriceCandidateSyncDoesNotPersistInvalidCandidate(t *testing.T) {
	fetchedAt := time.Now().UTC()
	candidate, err := pricesync.ParseOpenRouterPriceCandidate("provider/a", json.RawMessage(`{"prompt":"0.0000025","completion":"0.00001"}`), fetchedAt)
	if err != nil {
		t.Fatal(err)
	}
	candidate.SourceURL = "http://untrusted.invalid/models"
	store := &priceCandidateSyncStore{modelIDs: []string{"provider/a"}}
	source := &priceCandidateSource{items: []domain.PriceCandidate{candidate}}
	syncer := catalogapp.NewPriceCandidateSyncService(store, source, time.Now)
	if _, err := syncer.Synchronize(t.Context()); !errors.Is(err, catalogapp.ErrInvalidPriceCandidateSync) || len(store.candidates) != 0 {
		t.Fatalf("invalid source candidate reached storage: err=%v saved=%d", err, len(store.candidates))
	}
}
