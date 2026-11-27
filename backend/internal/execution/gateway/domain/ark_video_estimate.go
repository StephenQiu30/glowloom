package domain

import "math/big"

// ArkVideoEstimateProfile is a catalogue snapshot of the public calculator rules.
type ArkVideoEstimateProfile struct {
	FramesPerSecond           int64                           `json:"frames_per_second"`
	PixelsPerToken            int64                           `json:"pixels_per_token"`
	ReferenceFloorNumerator   int64                           `json:"reference_floor_numerator"`
	ReferenceFloorDenominator int64                           `json:"reference_floor_denominator"`
	MinReferenceSeconds       int64                           `json:"min_reference_seconds"`
	MaxReferenceSeconds       int64                           `json:"max_reference_seconds"`
	MinOutputSeconds          int64                           `json:"min_output_seconds"`
	MaxOutputSeconds          int64                           `json:"max_output_seconds"`
	Resolutions               map[string]ArkVideoEstimateRate `json:"resolutions"`
}

// ArkVideoEstimateRate ties the vendor tariffs to the precise sizes at one resolution.
type ArkVideoEstimateRate struct {
	CNYPerMillionWithoutVideo int64                    `json:"cny_per_million_without_video"`
	CNYPerMillionWithVideo    int64                    `json:"cny_per_million_with_video"`
	Geometries                map[string]VideoGeometry `json:"geometries"`
}

// ArkVideoEstimateInput selects a fixed output and the total duration of the input videos.
type ArkVideoEstimateInput struct {
	Model            string `json:"model" binding:"required"`
	Resolution       string `json:"resolution" binding:"required"`
	Ratio            string `json:"ratio" binding:"required"`
	ReferenceSeconds int64  `json:"reference_seconds"`
	OutputSeconds    int64  `json:"output_seconds" binding:"required"`
	Count            int64  `json:"count" binding:"required"`
}

// ArkVideoEstimate is a public CNY estimate formatted for display.
type ArkVideoEstimate struct {
	AmountCNY string `json:"amount_cny"`
}

// Estimate applies Ark's reference-video tariff and calculator floor before multiplying by the count.
func (p ArkVideoEstimateProfile) Estimate(in ArkVideoEstimateInput) (ArkVideoEstimate, error) {
	rate := p.Resolutions[in.Resolution]
	size, ok := rate.Geometries[in.Ratio]
	if !ok || size.Width <= 0 || size.Height <= 0 || p.FramesPerSecond <= 0 || p.PixelsPerToken <= 0 ||
		p.MinOutputSeconds <= 0 || in.OutputSeconds < p.MinOutputSeconds || in.OutputSeconds > p.MaxOutputSeconds ||
		in.Count <= 0 {
		return ArkVideoEstimate{}, ErrInvalid
	}

	seconds := in.OutputSeconds
	unitPrice := rate.CNYPerMillionWithoutVideo
	if in.ReferenceSeconds != 0 {
		if p.MinReferenceSeconds <= 0 || in.ReferenceSeconds < p.MinReferenceSeconds ||
			in.ReferenceSeconds > p.MaxReferenceSeconds || p.ReferenceFloorNumerator <= 0 || p.ReferenceFloorDenominator <= 0 {
			return ArkVideoEstimate{}, ErrInvalid
		}
		floorNumerator := new(big.Int).Mul(big.NewInt(in.OutputSeconds), big.NewInt(p.ReferenceFloorNumerator))
		floor, err := ceil(new(big.Rat).SetFrac(floorNumerator, big.NewInt(p.ReferenceFloorDenominator)))
		if err != nil {
			return ArkVideoEstimate{}, err
		}
		effectiveReference := max(floor, in.ReferenceSeconds)
		if effectiveReference > p.MaxReferenceSeconds {
			return ArkVideoEstimate{}, ErrInvalid
		}
		seconds += effectiveReference
		unitPrice = rate.CNYPerMillionWithVideo
	}
	if unitPrice <= 0 || seconds <= 0 {
		return ArkVideoEstimate{}, ErrInvalid
	}
	tokens := videoFrameTokens(seconds, size, p.FramesPerSecond, p.PixelsPerToken)
	amount := tokens.Mul(tokens, big.NewRat(unitPrice, 1_000_000))
	amount.Mul(amount, big.NewRat(in.Count, 1))
	return ArkVideoEstimate{AmountCNY: amount.FloatString(2)}, nil
}
