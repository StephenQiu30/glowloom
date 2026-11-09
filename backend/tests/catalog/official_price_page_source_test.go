package catalog_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/pricesync"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestOfficialPricePageSourceStoresExactModelRowsForManualReview(t *testing.T) {
	bodies := map[string]string{
		"docs.volcengine.com":   `<html><table><tr><th>模型名称</th><th>输入图单价</th><th>输出图单价</th></tr><tr><td>doubao-seedream-5-0-flash</td><td>免费</td><td>0.12 元</td></tr></table></html>`,
		"api-docs.deepseek.com": `<html><table><tr><th>功能</th><th>计费项</th><th>时段</th><th>deepseek-flash(1)</th><th>deepseek-v4-pro</th></tr><tr><td>价格(2)</td><td>百万 tokens 输入（缓存命中）</td><td>空闲时段</td><td>0.02 元</td><td>0.15 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输入（缓存命中）</td><td>高峰时段</td><td>0.04 元</td><td>0.30 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输入（缓存未命中）</td><td>空闲时段</td><td>1 元</td><td>4.5 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输入（缓存未命中）</td><td>高峰时段</td><td>2 元</td><td>9 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输出</td><td>空闲时段</td><td>4 元</td><td>13.5 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输出</td><td>高峰时段</td><td>8 元</td><td>27 元</td></tr></table></html>`,
		"developers.openai.com": `<html><table><tr><th>Model</th><th>Input</th><th>Output</th></tr><tr><td>gpt-6.1-sol</td><td>$2.00</td><td>$10.00</td></tr></table></html>`,
	}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, ok := bodies[request.URL.Hostname()]
		if !ok || request.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected official page request %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	source := pricesync.NewOfficialPricePageSource(&http.Client{Transport: transport})
	unit := domain.PricePer1KTokens
	imageUnit := domain.PricePerImage
	fetchedAt := time.Date(2026, 11, 9, 4, 0, 0, 0, time.UTC)
	targets := []catalogapp.OfficialPricePageTarget{
		{AdapterKey: "volcengine_ark", ProviderModelID: "doubao-seedream-5-0-flash-260915", SourceURL: "https://docs.volcengine.com/docs/ark/model-pricing", Currency: "CNY", Unit: &imageUnit, ReferenceLines: []string{"¥0.12 / 张"}},
		{AdapterKey: "deepseek", ProviderModelID: "deepseek-v4-pro", SourceURL: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/", Currency: "CNY", Unit: &unit, ReferenceLines: []string{"cache/input/output"}},
		{AdapterKey: "openai", ProviderModelID: "gpt-6.1-sol", SourceURL: "https://developers.openai.com/api/docs/pricing", Currency: "USD", ReferenceLines: []string{"short context"}},
	}
	candidates, err := source.FetchOfficialPageCandidates(t.Context(), targets, fetchedAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != len(targets) {
		t.Fatalf("candidate count=%d, want %d", len(candidates), len(targets))
	}
	for _, candidate := range candidates {
		if !candidate.SourceFetchedAt.Equal(fetchedAt) {
			t.Fatalf("candidate fetch time changed: %+v", candidate)
		}
		switch candidate.AdapterKey {
		case "volcengine_ark":
			var rule struct {
				Strategy string      `json:"pricing_strategy"`
				Base     json.Number `json:"base_micros"`
			}
			if candidate.Status != domain.PriceCandidatePendingReview || json.Unmarshal(candidate.Rule, &rule) != nil ||
				rule.Strategy != domain.PricingImageFlatV1 || rule.Base != "120000" {
				t.Fatalf("flat Seedream row did not normalize as a reviewable image price: %+v rule=%+v", candidate, rule)
			}
		case "deepseek":
			var rule struct {
				Input  json.Number `json:"input_micros_per_1k"`
				Cached json.Number `json:"cached_input_micros_per_1k"`
				Output json.Number `json:"output_micros_per_1k"`
				Time   string      `json:"time_pricing"`
			}
			if candidate.Status != domain.PriceCandidatePendingReview || json.Unmarshal(candidate.Rule, &rule) != nil ||
				rule.Input != "9000" || rule.Cached != "300" || rule.Output != "27000" || rule.Time != domain.DeepSeekCN2026 {
				t.Fatalf("DeepSeek timed rows did not normalize as a reviewable token price: %+v rule=%+v", candidate, rule)
			}
		default:
			if candidate.Status != domain.PriceCandidateNeedsReview || len(candidate.Rule) != 0 {
				t.Fatalf("unsupported source became executable without manual review: %+v", candidate)
			}
		}
		var snapshot struct {
			MatchStatus string   `json:"match_status"`
			MatchedRows []string `json:"matched_rows"`
		}
		if err := json.Unmarshal(candidate.SourcePricing, &snapshot); err != nil || snapshot.MatchStatus != "matched" || len(snapshot.MatchedRows) == 0 {
			t.Fatalf("missing model-specific official row for %s: %+v err=%v", candidate.ProviderModelID, snapshot, err)
		}
	}
	for _, candidate := range candidates {
		if candidate.AdapterKey == "volcengine_ark" && (candidate.Unit == nil || *candidate.Unit != domain.PricePerImage) {
			t.Fatalf("Seedream unit hint not preserved: %+v", candidate)
		}
	}
}

func TestOfficialPricePageSourceParsesDeepSeekTimedTokenRulesForReview(t *testing.T) {
	const body = `<html><table><tr><th>功能</th><th>计费项</th><th>时段</th><th>deepseek-flash(1)</th><th>deepseek-v4-pro</th></tr><tr><td>价格(2)</td><td>百万 tokens 输入（缓存命中）</td><td>空闲时段</td><td>0.02 元</td><td>0.15 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输入（缓存命中）</td><td>高峰时段</td><td>0.04 元</td><td>0.30 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输入（缓存未命中）</td><td>空闲时段</td><td>1 元</td><td>4.5 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输入（缓存未命中）</td><td>高峰时段</td><td>2 元</td><td>9 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输出</td><td>空闲时段</td><td>4 元</td><td>13.5 元</td></tr><tr><td>价格(2)</td><td>百万 tokens 输出</td><td>高峰时段</td><td>8 元</td><td>27 元</td></tr></table></html>`
	fetch := func(t *testing.T, page string, fetchedAt time.Time) []domain.PriceCandidate {
		t.Helper()
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(page)), Request: request}, nil
		})
		source := pricesync.NewOfficialPricePageSource(&http.Client{Transport: transport})
		unit := domain.PricePer1KTokens
		candidates, err := source.FetchOfficialPageCandidates(t.Context(), []catalogapp.OfficialPricePageTarget{
			{AdapterKey: "deepseek", ProviderModelID: "deepseek-flash", SourceURL: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/", Currency: "CNY", Unit: &unit},
			{AdapterKey: "deepseek", ProviderModelID: "deepseek-v4-pro", SourceURL: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/", Currency: "CNY", Unit: &unit},
		}, fetchedAt)
		if err != nil {
			t.Fatal(err)
		}
		return candidates
	}
	fetchedAt := time.Date(2026, 11, 9, 4, 0, 0, 0, time.UTC)
	candidates := fetch(t, body, fetchedAt)
	want := map[string][]string{
		"deepseek-flash":  {"2000", "40", "8000"},
		"deepseek-v4-pro": {"9000", "300", "27000"},
	}
	for _, candidate := range candidates {
		var rule struct {
			Input  json.Number `json:"input_micros_per_1k"`
			Cached json.Number `json:"cached_input_micros_per_1k"`
			Output json.Number `json:"output_micros_per_1k"`
			Time   string      `json:"time_pricing"`
		}
		if err := json.Unmarshal(candidate.Rule, &rule); err != nil {
			t.Fatalf("decode DeepSeek rule: %v", err)
		}
		got := []string{rule.Input.String(), rule.Cached.String(), rule.Output.String()}
		if candidate.Status != domain.PriceCandidatePendingReview || candidate.Unit == nil || *candidate.Unit != domain.PricePer1KTokens ||
			candidate.ParserVersion != "deepseek_cn_2026_timed_token_v1" || rule.Time != domain.DeepSeekCN2026 ||
			strings.Join(got, ",") != strings.Join(want[candidate.ProviderModelID], ",") {
			t.Errorf("incorrect DeepSeek timed-price candidate: model=%s status=%s rule=%+v", candidate.ProviderModelID, candidate.Status, rule)
		}
	}
	if len(candidates) != len(want) {
		t.Fatalf("candidate count=%d, want %d", len(candidates), len(want))
	}

	mismatchedPeakOffPeak := strings.Replace(body, "0.02 元</td>", "0.03 元</td>", 1)
	unsupportedTokenBasis := strings.Replace(body, "百万 tokens", "千 tokens", 1)
	cachedInputOffPeak := `<tr><td>价格(2)</td><td>百万 tokens 输入（缓存命中）</td><td>空闲时段</td><td>0.02 元</td><td>0.15 元</td></tr>`
	duplicatePriceRow := strings.Replace(body, cachedInputOffPeak, cachedInputOffPeak+cachedInputOffPeak, 1)
	for _, test := range []struct {
		name            string
		page            string
		fetchedAt       time.Time
		expectAllReview bool
	}{
		{name: "source rates do not follow half-price contract", page: mismatchedPeakOffPeak, fetchedAt: fetchedAt},
		{name: "unreviewed calendar year", page: body, fetchedAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "calendar year rolls in Beijing timezone", page: body, fetchedAt: time.Date(2026, 12, 31, 16, 0, 0, 0, time.UTC)},
		{name: "unsupported token basis", page: unsupportedTokenBasis, fetchedAt: fetchedAt, expectAllReview: true},
		{name: "duplicated rate row", page: duplicatePriceRow, fetchedAt: fetchedAt, expectAllReview: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, candidate := range fetch(t, test.page, test.fetchedAt) {
				needsReview := test.expectAllReview || test.fetchedAt.UTC().Add(8*time.Hour).Year() != 2026 || candidate.ProviderModelID == "deepseek-flash"
				if needsReview && (candidate.Status != domain.PriceCandidateNeedsReview || len(candidate.Rule) != 0) {
					t.Errorf("unsafe timed-price table was normalized: model=%s status=%s rule=%s", candidate.ProviderModelID, candidate.Status, candidate.Rule)
				}
				if !needsReview && candidate.Status != domain.PriceCandidatePendingReview {
					t.Errorf("unaffected model was not normalized: model=%s status=%s rule=%s", candidate.ProviderModelID, candidate.Status, candidate.Rule)
				}
			}
		})
	}
}

func TestOfficialPricePageSourceKeepsSuccessfulCandidatesWhenOneSiteFails(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Hostname() == "docs.volcengine.com" {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
		}
		body := `<html><table><tr><th>Model</th><th>Input</th><th>Output</th></tr><tr><td>gpt-6.1-sol</td><td>$2.00</td><td>$10.00</td></tr></table></html>`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	source := pricesync.NewOfficialPricePageSource(&http.Client{Transport: transport})
	targets := []catalogapp.OfficialPricePageTarget{
		{AdapterKey: "volcengine_ark", ProviderModelID: "doubao-seedream-5-0-flash-260915", SourceURL: "https://docs.volcengine.com/docs/ark/model-pricing", Currency: "CNY"},
		{AdapterKey: "openai", ProviderModelID: "gpt-6.1-sol", SourceURL: "https://developers.openai.com/api/docs/pricing", Currency: "USD"},
	}
	candidates, err := source.FetchOfficialPageCandidates(t.Context(), targets, time.Now().UTC())
	if err == nil || len(candidates) != 1 || candidates[0].AdapterKey != "openai" {
		t.Fatalf("partial source failure lost successful snapshots: candidates=%+v err=%v", candidates, err)
	}
}

func TestOfficialPricePageSourceDoesNotMatchRelatedModelNames(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `<html><table><tr><th>模型名称</th><th>输入</th><th>输出</th></tr><tr><td>deepseek-flash-preview</td><td>0.1元</td><td>0.2元</td></tr></table></html>`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	source := pricesync.NewOfficialPricePageSource(&http.Client{Transport: transport})
	candidates, err := source.FetchOfficialPageCandidates(t.Context(), []catalogapp.OfficialPricePageTarget{{
		AdapterKey: "deepseek", ProviderModelID: "deepseek-flash", SourceURL: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/", Currency: "CNY",
	}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		MatchStatus string   `json:"match_status"`
		MatchedRows []string `json:"matched_rows"`
	}
	if len(candidates) != 1 || json.Unmarshal(candidates[0].SourcePricing, &snapshot) != nil ||
		snapshot.MatchStatus != "no_model_row" || len(snapshot.MatchedRows) != 0 {
		t.Fatalf("related model row was incorrectly attributed: candidates=%+v snapshot=%+v", candidates, snapshot)
	}
}

func TestOfficialPricePageSourceRejectsRedirectOutsideAllowlist(t *testing.T) {
	requests := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"https://evil.example/pricing"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	source := pricesync.NewOfficialPricePageSource(&http.Client{Transport: transport})
	_, err := source.FetchOfficialPageCandidates(t.Context(), []catalogapp.OfficialPricePageTarget{{
		AdapterKey: "deepseek", ProviderModelID: "deepseek-flash", SourceURL: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/", Currency: "CNY",
	}}, time.Now().UTC())
	if err == nil || requests != 1 {
		t.Fatalf("official source followed an off-domain redirect: requests=%d err=%v", requests, err)
	}
}

func TestOfficialPricePageSourceRejectsUnregisteredURL(t *testing.T) {
	called := false
	transport := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	})
	source := pricesync.NewOfficialPricePageSource(&http.Client{Transport: transport})
	_, err := source.FetchOfficialPageCandidates(t.Context(), []catalogapp.OfficialPricePageTarget{{
		AdapterKey: "openai", ProviderModelID: "openai/model", SourceURL: "https://evil.example/pricing", Currency: "USD",
	}}, time.Now().UTC())
	if err == nil || called {
		t.Fatalf("unregistered source was fetched: called=%v err=%v", called, err)
	}
}
