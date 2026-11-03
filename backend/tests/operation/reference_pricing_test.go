package operation_test

import (
	"encoding/json"
	"fmt"
	"testing"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// Mathematical fixtures, not published supplier prices or counting contracts.
func TestReferencePricingFrozenFactorsAndMissingUsage(t *testing.T) {
	for _, tc := range []struct {
		scope string
		want  int64
	}{{"per_task", 500000}, {"per_output", 600000}} {
		t.Run(tc.scope, func(t *testing.T) {
			price := pricingRule(catalogdomain.PricePerImage, fmt.Sprintf(`{"base_micros":200000,"free_reference_images":1,"reference_image_micros":100000,"reference_scope":%q}`, tc.scope), "CNY", "")
			var input application.PricingInput
			if err := json.Unmarshal([]byte(`{"OutputCount":2,"ReferenceImageCount":2}`), &input); err != nil {
				t.Fatal(err)
			}
			got, _, err := application.CalculateQuoteDetailed(price, input)
			if err != nil || got != tc.want {
				t.Fatalf("reference quote=%d,%v want %d", got, err, tc.want)
			}
			usage, err := application.ParseBillableUsage(json.RawMessage(`{"output_count":2,"reference_image_count":2}`))
			if err != nil {
				t.Fatal(err)
			}
			got, err = application.CalculateActualCost(price, usage, "", "")
			if err != nil || got != tc.want {
				t.Fatalf("actual=%d,%v want %d", got, err, tc.want)
			}
			count := int64(2)
			if _, err := application.CalculateActualCost(price, application.BillableUsage{OutputCount: &count}, "", ""); err == nil {
				t.Fatal("missing reference evidence priced as zero")
			}
			for _, refs := range []int64{0, 1} {
				raw := fmt.Sprintf(`{"OutputCount":2,"ReferenceImageCount":%d}`, refs)
				if err := json.Unmarshal([]byte(raw), &input); err != nil {
					t.Fatal(err)
				}
				got, err = application.CalculateQuote(price, input)
				if err != nil || got != 400000 {
					t.Fatalf("free references=%d,%v", got, err)
				}
			}
		})
	}
}

func TestPriceExplanationFreezesReviewedVersionAndSalesFactors(t *testing.T) {
	price := pricingRule(catalogdomain.PricePerRequest, `{"base_micros":1,"markup_multiplier":"1.5"}`, "USD", "0.8")
	reviewed := price.EffectiveFrom
	price.SourceURL = "https://example.invalid/prices"
	price.ReviewedAt = &reviewed
	amount, detail, err := application.CalculateQuoteDetailed(price, application.PricingInput{})
	if err != nil || amount != 2 || detail.PriceRuleVersionID != price.ID || detail.PriceVersionNo != price.VersionNo || detail.SourceURL != price.SourceURL || detail.ReviewedAt == nil || !detail.ReviewedAt.Equal(reviewed) || detail.AmountKind != "reservation" {
		t.Fatalf("frozen sales explanation=%d,%+v,%v", amount, detail, err)
	}
}
