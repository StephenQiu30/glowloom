package domain

import (
	"math/big"
	"slices"

	"github.com/google/uuid"
)

// VideoMetering is the reviewed estimate profile of a single public video specification.
// MaxOutput on the published model is its confirmation and procurement bound.
type VideoMetering struct {
	Resolution      string                   `json:"resolution"`
	FramesPerSecond int64                    `json:"frames_per_second"`
	PixelsPerToken  int64                    `json:"pixels_per_token"`
	MinimumTokens   int64                    `json:"minimum_tokens,omitempty"`
	Geometries      map[string]VideoGeometry `json:"geometries"`
}

// VideoGeometry is the reviewed pixel size for one output aspect ratio.
type VideoGeometry struct {
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}

func (m VideoMetering) valid(limit int64) bool {
	if m.Resolution == "" || m.FramesPerSecond <= 0 || m.PixelsPerToken <= 0 ||
		m.MinimumTokens < 0 || m.MinimumTokens > limit || len(m.Geometries) == 0 {
		return false
	}
	for ratio, size := range m.Geometries {
		if ratio == "" || ratio == "adaptive" || size.Width <= 0 || size.Height <= 0 {
			return false
		}
	}
	return true
}

// EstimateVideo prices only fixed-duration, no-video-input profiles.
func (m Model) EstimateVideo(in MediaInput) (int64, error) {
	v := m.VideoMetering
	if v == nil || !v.valid(m.MaxOutput) {
		return 0, ErrInvalid
	}
	resolution, _ := in.Params["resolution"].(string)
	ratio, _ := in.Params["ratio"].(string)
	size, ok := v.Geometries[ratio]
	seconds, integer := in.Params["duration"].(float64)
	if resolution != v.Resolution || !ok || !integer || seconds <= 0 ||
		seconds > 1<<40 || seconds != float64(int64(seconds)) {
		return 0, ErrInvalid
	}
	quantity, err := ceil(videoFrameTokens(int64(seconds), size, v.FramesPerSecond, v.PixelsPerToken))
	quantity = max(quantity, v.MinimumTokens)
	if err != nil || quantity <= 0 || quantity > m.MaxOutput {
		return 0, ErrInvalid
	}
	return quantity, nil
}

// videoFrameTokens keeps the frame-based estimate exact until the caller's rounding boundary.
func videoFrameTokens(seconds int64, size VideoGeometry, fps, divisor int64) *big.Rat {
	tokens := big.NewInt(seconds)
	for _, factor := range []int64{size.Width, size.Height, fps} {
		tokens.Mul(tokens, big.NewInt(factor))
	}
	return new(big.Rat).SetFrac(tokens, big.NewInt(divisor))
}

// ValidateVideoInput restricts this meter to project-scoped outputs and at most an image first frame.
func (m Model) ValidateVideoInput(project uuid.UUID, in MediaInput) error {
	textInput := in.Mode == "text2video" && len(in.References) == 0
	firstFrame := in.Mode == "image2video" && len(in.References) == 1 && in.References[0].Role == "first_frame"
	if project == uuid.Nil || (!textInput && !firstFrame) {
		return ErrInvalid
	}
	return nil
}

func (m Model) validVideoContract() bool {
	return m.Protocol == "media" && m.Capability == "video.generate" && m.OutputPolicy == "provider_success_v1" &&
		m.VideoMetering.valid(m.MaxOutput) && m.Parameters["resolution"].Type == "string" &&
		m.Parameters["ratio"].Type == "string" && m.Parameters["duration"].Type == "integer" &&
		!slices.ContainsFunc(m.Modes, func(mode string) bool { return mode != "text2video" && mode != "image2video" }) &&
		m.Parameters["aspect_ratio"].Type == "" && m.Parameters["duration_ms"].Type == ""
}

// VideoEstimatedCredits prices the validated estimate without changing the frozen upper limit.
func (c Call) VideoEstimatedCredits() (int64, error) {
	if c.Model.VideoMetering == nil || c.EstimatedVideoTokens <= 0 {
		return 0, ErrInvalid
	}
	micros, err := c.Model.Price.Cost(Usage{Output: c.EstimatedVideoTokens})
	if err != nil {
		return 0, err
	}
	return Credits(micros)
}

// AcceptsMediaPrice validates either the legacy fixed contract or a video Token rate.
func (m Model) AcceptsMediaPrice(p Price) bool {
	if p.Validate() != nil || p.ExplicitFree || p.Input != 0 || p.CacheRead != 0 || p.CacheWrite != 0 {
		return false
	}
	if m.VideoMetering != nil {
		return p.Request == 0 && p.Output > 0
	}
	return p.Request > 0 && p.Output == 0
}

// IsVideoUsage verifies that only the provider's billable video quantity is present.
func (u Usage) IsVideoUsage() bool {
	return u.Output > 0 && u.Input == 0 && u.CacheRead == 0 && u.CacheWrite == 0 && u.Validate() == nil
}
