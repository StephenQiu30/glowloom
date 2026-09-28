package application

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/domain"
)

// ErrInvalidQuoteCurrentFacts means a confirmation check has no valid clock.
var ErrInvalidQuoteCurrentFacts = errors.New("invalid current quote facts")

// QuoteCurrentFacts contains mutable facts reloaded for one confirmation.
// A nil current model or price ID means it is no longer available to quote.
// Callers must read these facts again while holding transaction B's locks.
type QuoteCurrentFacts struct {
	Now                    time.Time
	TargetVersionNo        *int32
	ModelProfileVersionID  *uuid.UUID
	PriceRuleVersionID     *uuid.UUID
	ConsentRevoked         bool
	ReuseSourceUnavailable bool
}

// QuoteFreshnessReasons returns stable, ordered reasons to expire a quote.
// It does not check actor rights, budget, or the source of the current facts.
func QuoteFreshnessReasons(quoted domain.Operation, current QuoteCurrentFacts) ([]string, error) {
	if err := quoted.Validate(); err != nil {
		return nil, err
	}
	if quoted.Status != domain.StatusQuoted || quoted.Origin == "upload" {
		return nil, domain.ErrQuoteNotConfirmable
	}
	if current.Now.IsZero() {
		return nil, ErrInvalidQuoteCurrentFacts
	}
	var reasons []string
	if !current.Now.Before(*quoted.QuoteExpiresAt) {
		reasons = append(reasons, "quote_expired")
	}
	if quoted.TargetVersionNo != nil &&
		(current.TargetVersionNo == nil || *current.TargetVersionNo != *quoted.TargetVersionNo) {
		reasons = append(reasons, "target_version_changed")
	}
	if quoted.ModelProfileVersionID != nil {
		switch {
		case current.ModelProfileVersionID == nil:
			reasons = append(reasons, "model_unavailable")
		case *current.ModelProfileVersionID != *quoted.ModelProfileVersionID:
			reasons = append(reasons, "model_version_changed")
		}
	}
	if quoted.PriceRuleVersionID != nil {
		switch {
		case current.PriceRuleVersionID == nil:
			reasons = append(reasons, "price_unavailable")
		case *current.PriceRuleVersionID != *quoted.PriceRuleVersionID:
			reasons = append(reasons, "price_version_changed")
		}
	}
	if current.ConsentRevoked {
		reasons = append(reasons, "consent_revoked")
	}
	if quoted.ReusedFromID != nil && current.ReuseSourceUnavailable {
		reasons = append(reasons, "reuse_source_unavailable")
	}
	return reasons, nil
}
