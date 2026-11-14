package domain

// Pricing strategy IDs are immutable contracts within the existing price-rule
// JSON. Empty IDs preserve historical rules; an unknown version is never guessed.
const (
	PricingImageFlatV1           = "image_flat_v1"
	PricingTokenIOV1             = "token_io_v1"
	PricingSeedanceVideoTokensV1 = "seedance_video_tokens_v1"
	PricingCharacterCountV1      = "character_count_v1"
	PricingOriginalDurationV1    = "original_duration_v1"
)

// CompatiblePricingStrategy checks installed algorithms without an expression DSL.
func CompatiblePricingStrategy(id string, unit PriceUnit) bool {
	switch id {
	case "":
		return true
	case PricingImageFlatV1:
		return unit == PricePerImage
	case PricingTokenIOV1, PricingSeedanceVideoTokensV1:
		return unit == PricePer1KTokens
	case PricingCharacterCountV1:
		return unit == PricePer1KChars
	case PricingOriginalDurationV1:
		return unit == PricePerSecond
	default:
		return false
	}
}
