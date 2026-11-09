package catalog_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricesync"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOpenRouterSourceUsesPublicCatalogAndAllowlist(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != pricesync.OpenRouterCatalogURL ||
			request.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected OpenRouter request: method=%s url=%s authorization=%q", request.Method, request.URL, request.Header.Get("Authorization"))
		}
		body := `{"data":[{"id":"provider/selected","pricing":{"prompt":"0.0000025","completion":"0.00001"}},{"id":"provider/other","pricing":{"prompt":"0.000003","completion":"0.00002"}}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	source := pricesync.NewOpenRouterSource(&http.Client{Transport: transport})
	fetchedAt := time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC)
	candidates, err := source.FetchOpenRouterCandidates(t.Context(), []string{"provider/selected"}, fetchedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ProviderModelID != "provider/selected" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestOpenRouterSourceTreatsMissingPricingAsReviewNotZero(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"data":[{"id":"provider/unpriced","pricing":null}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	source := pricesync.NewOpenRouterSource(&http.Client{Transport: transport})
	candidates, err := source.FetchOpenRouterCandidates(t.Context(), []string{"provider/unpriced"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Status != domain.PriceCandidateNeedsReview || candidates[0].Unit != nil || len(candidates[0].Rule) != 0 {
		t.Fatalf("missing price became zero or disappeared: %+v", candidates)
	}
}

func TestOpenRouterSourceRejectsRedirectOutsideOfficialHost(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://evil.example/models"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	source := pricesync.NewOpenRouterSource(&http.Client{Transport: transport})
	if _, err := source.FetchOpenRouterCandidates(t.Context(), []string{"provider/model"}, time.Now().UTC()); err == nil {
		t.Fatal("source followed redirect outside OpenRouter")
	}
}
