package domain

import (
	"math/big"
	"slices"
)

const (
	// SpeechCharacters meters the count reported by a successful TTS terminal frame.
	SpeechCharacters = "speech_characters_v1"
	// OriginalAudioSeconds meters the provider's original duration before playback speed changes.
	OriginalAudioSeconds = "original_audio_seconds_v1"
	// ArkImagePixels meters one delivered image and its additional supplied images.
	ArkImagePixels = "ark_image_pixels_v1"
	// ImageTokens bills the Images API text input and image output token buckets.
	ImageTokens = "image_tokens_v1"
	// ReportedImageAmount uses the official gateway's actual USD charge.
	ReportedImageAmount = "reported_image_amount_v1"
	// OutputVideoSeconds bills the duration of the verified output video track.
	OutputVideoSeconds = "output_video_seconds_v1"
)

// MediaTariff stores a price per official billing unit and an explicit quantity ceiling.
type MediaTariff struct {
	Meter               string `json:"meter"`
	UnitPrice           string `json:"unit_price"`
	UnitSize            int64  `json:"unit_size"`
	MaxQuantity         int64  `json:"max_quantity"`
	CutoffPixels        int64  `json:"cutoff_pixels,omitempty"`
	HighPrice           string `json:"high_price,omitempty"`
	ExtraReferencePrice string `json:"extra_reference_price,omitempty"`
	SourceURL           string `json:"source_url,omitempty"`
	Observed            string `json:"observed_on,omitempty"`
	Adapter             string `json:"adapter,omitempty"`
	OutputTokenPrice    string `json:"output_token_price,omitempty"`
}

func (m MediaTariff) valid() bool {
	if m.Meter == ReportedImageAmount {
		return m.MatchesAdapter(m.Adapter) && m.UnitPrice == "" && m.UnitSize == 0 &&
			(m.Adapter == "openrouter" && m.MaxQuantity >= 0 && m.MaxQuantity <= 16 ||
				m.Adapter == "vercel" && m.MaxQuantity == 0) &&
			m.CutoffPixels == 0 && m.HighPrice == "" && m.ExtraReferencePrice == "" && m.OutputTokenPrice == ""
	}
	if m.Meter == ImageTokens {
		return m.UnitSize == 1000000 && m.MaxQuantity == 0 && positiveRate(m.UnitPrice) && positiveRate(m.OutputTokenPrice) &&
			m.Adapter == "" && m.CutoffPixels == 0 && m.HighPrice == "" && m.ExtraReferencePrice == ""
	}
	if !decimalRate.MatchString(m.UnitPrice) || m.MaxQuantity <= 0 || m.MaxQuantity > 1<<20 ||
		m.UnitSize <= 0 || m.UnitSize > 1<<20 || m.rate().Sign() <= 0 {
		return false
	}
	if m.OutputTokenPrice != "" {
		return false
	}
	switch m.Meter {
	case OutputVideoSeconds:
		return m.MatchesAdapter(m.Adapter) && m.UnitSize == 1 && m.MaxQuantity <= 600 &&
			m.CutoffPixels == 0 && m.HighPrice == "" && m.ExtraReferencePrice == ""
	case SpeechCharacters, OriginalAudioSeconds:
		return m.CutoffPixels == 0 && m.HighPrice == "" && m.ExtraReferencePrice == ""
	case ArkImagePixels:
		return m.UnitSize == 1 && m.MaxQuantity <= 16 && m.CutoffPixels > 0 && m.CutoffPixels <= 1<<30 &&
			decimalRate.MatchString(m.HighPrice) && decimalRate.MatchString(m.ExtraReferencePrice)
	default:
		return false
	}
}

func positiveRate(raw string) bool {
	if !decimalRate.MatchString(raw) {
		return false
	}
	r, _ := new(big.Rat).SetString(raw)
	return r.Sign() > 0
}

// UsesAuthorizedCeiling identifies media with no supplier token ceiling.
func (m MediaTariff) UsesAuthorizedCeiling() bool {
	return m.Meter == ReportedImageAmount || m.Meter == ImageTokens
}

func (m MediaTariff) rate() *big.Rat {
	rate, _ := new(big.Rat).SetString(m.UnitPrice)
	return rate.Mul(rate, big.NewRat(1000000, m.UnitSize))
}

func (m MediaTariff) cost(u Usage) (*big.Rat, error) {
	switch m.Meter {
	case OutputVideoSeconds:
		// The quote's ceiling limits the user's debit. An overlong original
		// result still carries a measurable procurement cost.
		if u.VideoDurationMS <= 0 {
			return nil, ErrUnknown
		}
		return new(big.Rat).Mul(m.rate(), big.NewRat(u.VideoDurationMS, 1000)), nil
	case ReportedImageAmount:
		if !positiveRate(string(u.ProviderAmount)) {
			return nil, ErrUnknown
		}
		cost, _ := new(big.Rat).SetString(string(u.ProviderAmount))
		return cost.Mul(cost, big.NewRat(1000000, 1)), nil
	case ImageTokens:
		if u.Input <= 0 || u.Output <= 0 || u.CacheRead != 0 || u.CacheWrite != 0 {
			return nil, ErrUnknown
		}
		output, _ := new(big.Rat).SetString(m.OutputTokenPrice)
		input := new(big.Rat).Mul(m.rate(), big.NewRat(u.Input, 1))
		return input.Add(input, output.Mul(output, big.NewRat(u.Output, 1))), nil
	}
	if m.Meter == ArkImagePixels {
		if u.Pixels <= 0 || u.Pixels > 1<<30 || u.ReferenceImages < 0 || u.ReferenceImages > m.MaxQuantity {
			return nil, ErrUnknown
		}
		return m.imageCost(u.Pixels > m.CutoffPixels, u.ReferenceImages), nil
	}
	var q *big.Rat
	if m.Meter == SpeechCharacters {
		q = new(big.Rat).SetInt64(u.Characters)
	} else if decimalRate.MatchString(string(u.OriginalSeconds)) {
		q, _ = new(big.Rat).SetString(string(u.OriginalSeconds))
	}
	if q == nil || q.Sign() <= 0 || q.Cmp(big.NewRat(m.MaxQuantity, 1)) > 0 {
		return nil, ErrUnknown
	}
	return q.Mul(q, m.rate()), nil
}

func (m MediaTariff) imageCost(high bool, references int64) *big.Rat {
	base := m.UnitPrice
	if high {
		base = m.HighPrice
	}
	price, _ := new(big.Rat).SetString(base)
	ref, _ := new(big.Rat).SetString(m.ExtraReferencePrice)
	price.Add(price, ref.Mul(ref, big.NewRat(max(0, references-1), 1)))
	return price.Mul(price, big.NewRat(1000000, 1))
}

func (m MediaTariff) upperMicros() *big.Rat {
	if m.Meter == ArkImagePixels {
		return m.imageUpperMicros(m.MaxQuantity)
	}
	return new(big.Rat).Mul(m.rate(), big.NewRat(m.MaxQuantity, 1))
}

func (m MediaTariff) imageUpperMicros(refs int64) *big.Rat {
	low, high := m.imageCost(false, refs), m.imageCost(true, refs)
	if low.Cmp(high) > 0 {
		return low
	}
	return high
}

// ImageUpperCost bounds a single delivery for the already verified reference count.
func (p Price) ImageUpperCost(refs int64) (int64, error) {
	m := p.MediaTariff
	if m == nil || m.Meter != ArkImagePixels || p.Validate() != nil || refs < 0 || refs > m.MaxQuantity {
		return 0, ErrInvalid
	}
	return p.convert(m.imageUpperMicros(refs))
}

func (m MediaTariff) accepts(model Model) bool {
	if m.UsesAuthorizedCeiling() {
		if model.Capability != "image.generate" || model.PurchaseCap <= 0 || len(model.Modes) == 0 {
			return false
		}
		return !slices.ContainsFunc(model.Modes, func(mode string) bool {
			return mode != "text2image" && (m.Adapter != "openrouter" || mode != "image2image")
		})
	}
	if m.Meter == OutputVideoSeconds {
		return model.Capability == "video.generate" && model.OutputPolicy == "provider_success_v1" && len(model.Modes) == 1 &&
			(m.Adapter == "volcengine_avatar" && model.Modes[0] == "avatar" ||
				m.Adapter == "volcengine_motion" && model.Modes[0] == "motion")
	}
	if m.Meter == ArkImagePixels {
		return model.Capability == "image.generate" && len(model.Modes) > 0 &&
			!slices.ContainsFunc(model.Modes, func(mode string) bool { return mode != "text2image" && mode != "image2image" })
	}
	if model.Capability != "audio.generate" || len(model.Modes) != 1 {
		return false
	}
	return m.Meter == SpeechCharacters && model.Modes[0] == "text2audio" ||
		m.Meter == OriginalAudioSeconds && model.Modes[0] == "omni_audio"
}

// MatchesAdapter ties the meter to the adapter that supplies its evidence.
func (m MediaTariff) MatchesAdapter(adapter string) bool {
	switch m.Meter {
	case ImageTokens:
		return adapter == "openai"
	case ReportedImageAmount:
		return m.Adapter == adapter && (adapter == "openrouter" || adapter == "vercel")
	case OutputVideoSeconds:
		return m.Adapter == adapter && (adapter == "volcengine_avatar" || adapter == "volcengine_motion")
	case ArkImagePixels:
		return adapter == "volcengine_ark"
	case SpeechCharacters, OriginalAudioSeconds:
		return adapter == "volcengine_speech"
	default:
		return false
	}
}

// UsesVideoDeliveryMeter means settlement depends on the original archived video.
func (m Model) UsesVideoDeliveryMeter() bool {
	return m.VideoMetering != nil || m.UsesOutputVideoDuration()
}

// UsesOutputVideoDuration selects the media-verified duration instead of a provider token count.
func (m Model) UsesOutputVideoDuration() bool {
	return m.Price.MediaTariff != nil && m.Price.MediaTariff.Meter == OutputVideoSeconds
}

// CNYInvoiceMicros converts a reported original supplier amount without
// applying any nominal duration rate or sales multiplier.
func CNYInvoiceMicros(raw string) (int64, error) {
	if !decimalRate.MatchString(raw) {
		return 0, ErrInvalid
	}
	amount, _ := new(big.Rat).SetString(raw)
	return ceil(amount.Mul(amount, big.NewRat(1000000, 1)))
}
