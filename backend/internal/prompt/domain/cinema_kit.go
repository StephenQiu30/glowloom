package domain

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// CinemaCamera records four explicitly selected camera characteristics.
type CinemaCamera struct {
	Body        string `json:"body"`
	Lens        string `json:"lens"`
	FocalLength string `json:"focal_length"`
	Aperture    string `json:"aperture"`
}

// CinemaPalette freezes exact RGB values rather than a preview image URL.
type CinemaPalette struct {
	Preset string   `json:"preset,omitempty"`
	Name   string   `json:"name"`
	Colors []string `json:"colors"`
}

// CinemaSource identifies an authorized canvas input and its immutable setting content.
type CinemaSource struct {
	NodeID uuid.UUID `json:"node_id"`
	Kind   string    `json:"kind"`
	Hash   string    `json:"hash"`
}

// CinemaKit is a closed visual intent, distinct from provider-native parameters.
type CinemaKit struct {
	Version int            `json:"version"`
	Camera  *CinemaCamera  `json:"camera,omitempty"`
	Palette *CinemaPalette `json:"palette,omitempty"`
	Style   string         `json:"style,omitempty"`
	Motion  string         `json:"motion,omitempty"`
	Sources []CinemaSource `json:"sources,omitempty"`
}

//go:embed cinema_catalog.json
var cinemaCatalogJSON []byte
var cinemaCatalog = func() struct {
	Camera, Lens              []string
	FocalLength               []string `json:"focalLength"`
	Aperture, Styles, Motions []string
	Palettes                  []CinemaPalette
} {
	var out struct {
		Camera, Lens              []string
		FocalLength               []string `json:"focalLength"`
		Aperture, Styles, Motions []string
		Palettes                  []CinemaPalette
	}
	if err := json.Unmarshal(cinemaCatalogJSON, &out); err != nil {
		panic(err)
	}
	return out
}()
var cinemaColor = regexp.MustCompile(`^#[0-9A-F]{6}$`)
var cinemaHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Kinds returns a deterministic list of configured settings.
func (c CinemaKit) Kinds() []string {
	out := []string{}
	if c.Camera != nil {
		out = append(out, "camera")
	}
	if c.Palette != nil {
		out = append(out, "palette")
	}
	if c.Style != "" {
		out = append(out, "style")
	}
	if c.Motion != "" {
		out = append(out, "motion")
	}
	return out
}

// Validate rejects unknown choices, forged presets, unbounded colors and duplicate sources.
func (c CinemaKit) Validate() error {
	if c.Version != 1 || len(c.Kinds()) == 0 {
		return ErrInvalidDraft
	}
	if v := c.Camera; v != nil && (!slices.Contains(cinemaCatalog.Camera, v.Body) || !slices.Contains(cinemaCatalog.Lens, v.Lens) || !slices.Contains(cinemaCatalog.FocalLength, v.FocalLength) || !slices.Contains(cinemaCatalog.Aperture, v.Aperture)) {
		return ErrInvalidDraft
	}
	if c.Style != "" && !slices.Contains(cinemaCatalog.Styles, c.Style) || c.Motion != "" && !slices.Contains(cinemaCatalog.Motions, c.Motion) {
		return ErrInvalidDraft
	}
	if p := c.Palette; p != nil {
		if strings.TrimSpace(p.Name) == "" || !utf8.ValidString(p.Name) || utf8.RuneCountInString(p.Name) > 128 || len(p.Colors) < 1 || len(p.Colors) > 16 {
			return ErrInvalidDraft
		}
		for _, color := range p.Colors {
			if !cinemaColor.MatchString(color) {
				return ErrInvalidDraft
			}
		}
		if p.Preset != "" {
			found := false
			for _, preset := range cinemaCatalog.Palettes {
				if preset.Name == p.Preset && p.Name == p.Preset && slices.Equal(p.Colors, preset.Colors) {
					found = true
				}
			}
			if !found {
				return ErrInvalidDraft
			}
		}
	}
	seen := map[string]bool{}
	for _, source := range c.Sources {
		if source.NodeID == uuid.Nil || !slices.Contains(c.Kinds(), source.Kind) || seen[source.Kind] || !cinemaHash.MatchString(source.Hash) {
			return ErrInvalidDraft
		}
		seen[source.Kind] = true
	}
	return nil
}

// Supports bounds semantic compilation to the relevant actual model capability.
func (c CinemaKit) Supports(capability string) bool {
	return capability == "image.generate" && c.Motion == "" || capability == "video.generate" && c.Camera == nil
}

// Fingerprint identifies setting content without mutable node titles or positions.
func (c CinemaKit) Fingerprint() string {
	c.Sources = nil
	body, _ := json.Marshal(c)
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

// Constraints is a deterministic compiler projection; it does not claim native camera support.
func (c CinemaKit) Constraints() string {
	lines := []string{"视觉创作设置："}
	if p := c.Camera; p != nil {
		lines = append(lines, fmt.Sprintf("相机：%s；镜头：%s；焦距：%s；光圈：%s。", p.Body, p.Lens, p.FocalLength, p.Aperture))
	}
	if p := c.Palette; p != nil {
		lines = append(lines, "配色："+p.Name+"；颜色："+strings.Join(p.Colors, "、")+"。")
	}
	if c.Style != "" {
		lines = append(lines, "视觉风格："+c.Style+"。")
	}
	if c.Motion != "" {
		lines = append(lines, "镜头运动："+c.Motion+"。")
	}
	return strings.Join(lines, "\n")
}
