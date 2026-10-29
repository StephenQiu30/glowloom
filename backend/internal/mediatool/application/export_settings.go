package application

import (
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// NormalizeExportSettings validates user choices before hashing the command.
func NormalizeExportSettings(settings domain.ExportSettings) (domain.ExportSettings, error) {
	settings.FileName = strings.TrimSpace(settings.FileName)
	if settings.FileName == "" {
		settings.FileName = "时间线"
	}
	if utf8.RuneCountInString(settings.FileName) > 120 || len(settings.FileName) > 251 || !utf8.ValidString(settings.FileName) || strings.ContainsAny(settings.FileName, `/\:*?"<>|`) || strings.ContainsFunc(settings.FileName, unicode.IsControl) || settings.FileName == "." || settings.FileName == ".." {
		return settings, ErrInvalidExport
	}
	if settings.Destination == "" {
		settings.Destination = "canvas"
	}
	if settings.Destination != "canvas" && settings.Destination != "local" {
		return settings, ErrInvalidExport
	}
	if settings.Resolution == 0 {
		settings.Resolution = 720
	}
	if !slices.Contains([]int{480, 720, 1080, 1440, 2160}, settings.Resolution) {
		return settings, ErrInvalidExport
	}
	return settings, nil
}

// ApplyExportSettings resolves auto from the first visible visual clip's verified
// metadata, never from a browser URL. Audio-only timelines use 16:9.
func ApplyExportSettings(frozen *domain.FrozenExport, settings domain.ExportSettings) error {
	settings, err := NormalizeExportSettings(settings)
	if err != nil {
		return err
	}
	width, height := 16, 9
	if frozen.Timeline.AspectRatio != "auto" {
		width, height, err = frozen.Timeline.VideoDimensions()
		if err != nil {
			return err
		}
	} else {
		visible := make(map[string]bool)
		for _, track := range frozen.Timeline.Tracks {
			visible[track.ID.String()] = track.Visible
		}
		earliest := int64(math.MaxInt64)
		for _, clip := range frozen.Timeline.Clips {
			if !visible[clip.TrackID.String()] || (clip.Kind != "video" && clip.Kind != "image") || clip.AssetID == nil || clip.StartMS >= earliest {
				continue
			}
			for _, source := range frozen.Inputs {
				if source.AssetID == *clip.AssetID && source.Width != nil && source.Height != nil && *source.Width > 0 && *source.Height > 0 {
					width, height = int(*source.Width), int(*source.Height)
					earliest = clip.StartMS
					break
				}
			}
		}
	}
	scale := float64(settings.Resolution) / float64(min(width, height))
	even := func(n int) int { return max(2, int(math.Round(float64(n)*scale/2))*2) }
	frozen.Width, frozen.Height = even(width), even(height)
	if frozen.Width > 16384 || frozen.Height > 16384 {
		return ErrInvalidExport
	}
	frozen.Settings = &settings
	return nil
}
