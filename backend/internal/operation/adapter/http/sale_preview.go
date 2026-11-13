package http

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	billing "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

// SalePreviewRequest describes an unpublished supplier tariff and a sample of
// already normalized billable quantities. Previewing never dispatches a task.
type SalePreviewRequest struct {
	Unit         catalog.PriceUnit `json:"unit"`
	Rule         json.RawMessage   `json:"rule" swaggertype:"object"`
	Currency     string            `json:"currency"`
	FXRateToCNY  string            `json:"fx_rate_to_cny,omitempty"`
	Markup       string            `json:"markup_multiplier"`
	Discount     string            `json:"discount_factor"`
	Count        int64             `json:"output_count"`
	DurationMS   int64             `json:"duration_ms"`
	InputTokens  int64             `json:"input_tokens"`
	OutputTokens int64             `json:"output_tokens"`
	Characters   int64             `json:"characters"`
	References   int64             `json:"reference_images"`
	Mode         string            `json:"mode"`
	Resolution   string            `json:"resolution"`
}

// SalePreview reports one sample in CNY micros and whole credits.
type SalePreview struct {
	ProcurementMicros int64 `json:"procurement_micros"`
	SaleMicros        int64 `json:"sale_micros"`
	RegularCredits    int64 `json:"regular_credits"`
	SaleCredits       int64 `json:"sale_credits"`
}

// PreviewSalePrice previews a draft without publishing or reserving funds.
// @Summary 预览模型销售价格
// @ID previewAdminSalePrice
// @Tags settings
// @Accept json
// @Produce json
// @Param body body SalePreviewRequest true "采购价格与示例用量"
// @Success 200 {object} SalePreview
// @Failure 403 {object} httpapi.Problem
// @Failure 422 {object} httpapi.Problem
// @Router /api/admin/model-price-preview [post]
func (h *Handler) PreviewSalePrice(c *gin.Context) {
	actor := identityhttp.Principal(c)
	if !catalogapp.IsPlatformAdmin(actor) || actor.OrgID == uuid.Nil {
		httpapi.WriteProblem(c, 403, "forbidden", nil)
		return
	}
	var b SalePreviewRequest
	if !httpapi.Decode(c, &b) {
		return
	}
	v, err := sampleSalePrice(b)
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, v)
}

func sampleSalePrice(b SalePreviewRequest) (SalePreview, error) {
	now := time.Now().UTC()
	price := catalog.PriceRuleVersion{ID: uuid.New(), ModelID: uuid.New(), VersionNo: 1, Unit: b.Unit, Rule: b.Rule, Currency: b.Currency, FXRateToCNY: b.FXRateToCNY, EffectiveFrom: now}
	in := application.PricingInput{QuotedAt: now, OutputCount: b.Count, TargetDurationMS: b.DurationMS, AllowedDurationsMS: []int64{b.DurationMS}, EstimatedInputTokens: b.InputTokens, MaxOutputTokens: b.OutputTokens, CharacterCount: b.Characters, ReferenceImageCount: b.References, Mode: b.Mode, Resolution: b.Resolution}
	sale, err := catalog.SalePriceFromProcurement(price, b.Markup, b.Discount)
	if err != nil {
		return SalePreview{}, err
	}
	base, err := application.CalculateQuote(price, in)
	if err != nil {
		return SalePreview{}, err
	}
	amount, details, err := application.CalculateQuoteDetailed(sale, in)
	if err != nil {
		return SalePreview{}, err
	}
	whole, err := billing.WholeCreditMicros(amount)
	if err != nil {
		return SalePreview{}, err
	}
	credits := whole / billing.MicrosPerCredit
	regular := credits
	if details.BeforeDiscountCredits != nil {
		regular = *details.BeforeDiscountCredits
	}
	return SalePreview{ProcurementMicros: base, SaleMicros: amount, RegularCredits: regular, SaleCredits: credits}, nil
}
