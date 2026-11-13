package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func TestSalePreviewShowsIndependentProcurementAndDiscount(t *testing.T) {
	in := SalePreviewRequest{Unit: domain.PricePerImage, Currency: "CNY", Rule: json.RawMessage(`{"base_micros":7560000}`), Count: 1, Markup: "1.5", Discount: "0.8"}
	got, err := sampleSalePrice(in)
	if err != nil || got != (SalePreview{7560000, 9072000, 114, 91}) {
		t.Fatalf("preview = %+v: %v", got, err)
	}
	for _, v := range []string{"0", "1.05", "-1", "NaN", "0.1234567"} {
		in.Discount = v
		if _, err := sampleSalePrice(in); err == nil {
			t.Errorf("accepted %q", v)
		}
	}
}

func TestSalePreviewRequiresPlatformAdmin(t *testing.T) {
	router := gin.New()
	NewHandler(Dependencies{}).Register(router.Group("/api"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "POST", "/api/admin/model-price-preview", nil))
	if response.Code != 403 {
		t.Fatalf("anonymous preview status = %d", response.Code)
	}
}
