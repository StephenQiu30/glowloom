package postgres

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingdomain "github.com/StephenQiu30/glowloom-server/backend/internal/billing/domain"
	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// EstimateGeneration uses the same model, media and price resolution as a
// formal quote. It leaves the draft, submission, operation and wallet untouched.
func (s *Store) EstimateGeneration(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, draft promptdomain.Draft, regenerate bool) (application.GenerationEstimate, error) {
	var result application.GenerationEstimate
	if s == nil || s.db == nil {
		return result, ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireProject(tx, actor, projectID, true); err != nil {
			return err
		}
		project, err := lockFreeQuoteProject(tx, actor, projectID)
		if err != nil {
			return err
		}
		input, err := compileEditorInput(tx, application.CreateFreeQuoteInput{
			ProjectID:          projectID,
			RequestID:          uuid.NewString(),
			FreeQuoteItemInput: application.FreeQuoteItemInput{ForceRegenerate: regenerate},
		}, draft)
		if err != nil {
			return err
		}
		emptyImagePrompt := input.Capability == "image.generate" && strings.TrimSpace(input.Prompt) == ""
		if emptyImagePrompt {
			// Fixed image tariffs do not depend on prompt length. Use a local
			// stand-in to pass generation validation and skip result reuse.
			input.Prompt = "."
			input.ForceRegenerate = true
		}
		if input.Validate() != nil {
			return application.ErrFreeQuoteInputNotReady
		}
		item, _, err := s.prepareFreeQuoteItem(ctx, tx, actor, input, project, time.Now().UTC())
		if err != nil {
			return err
		}
		op := item.Operation
		if op.ModelProfileVersionID == nil || op.ModelProfileVersionID.String() != *draft.ModelVersion {
			return application.ErrFreeQuoteModelMissing
		}
		if op.ReusedFromID == nil {
			ready, err := commercialPriceReady(tx, op.PriceRuleVersionID)
			if err != nil {
				return err
			}
			if !ready {
				return billingdomain.ErrWalletPriceUnavailable
			}
		}
		var detail application.QuoteCostDetail
		if err := json.Unmarshal(op.QuoteDetail, &detail); err != nil {
			return err
		}
		if emptyImagePrompt && detail.Unit != catalogdomain.PricePerImage && detail.Unit != catalogdomain.PricePerRequest {
			return application.ErrFreeQuoteInputNotReady
		}
		if detail.ExactCredits == "" {
			return application.ErrFreeQuoteModelMissing
		}
		kind := "estimate"
		if op.ReusedFromID == nil && (detail.Unit == catalogdomain.PricePer1KTokens || input.Mode == "omni_audio") {
			kind = "maximum"
		}
		quantity := detail.Quantity.String()
		if detail.Unit == catalogdomain.PricePerImage {
			quantity = strconv.FormatInt(detail.Outputs, 10)
		} else if detail.Unit == catalogdomain.PricePerSecond && detail.Outputs > 1 {
			quantity = ""
		}
		result = application.GenerationEstimate{
			Credits: detail.ExactCredits, Kind: kind, Basis: string(detail.Unit),
			Quantity: quantity, Reused: op.ReusedFromID != nil,
		}
		return nil
	})
	return result, err
}
