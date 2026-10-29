package application

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// MaxProjectBytes bounds one portable archive including its original media.
const MaxProjectBytes int64 = 500 << 20

// ProjectMediaExtension maps inspected media MIME types to portable file extensions.
func ProjectMediaExtension(mime string) (string, bool) {
	exts := map[string]string{"video/mp4": ".mp4", "video/quicktime": ".mov", "audio/mp4": ".m4a", "audio/mpeg": ".mp3", "audio/wave": ".wav", "audio/x-wav": ".wav", "audio/ogg": ".ogg", "image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}
	ext, ok := exts[mime]
	return ext, ok
}

// ValidateProjectExport rejects lossy unsupported edits before a durable export is queued.
func ValidateProjectExport(frozen domain.FrozenExport) error {
	if !frozen.OutputKind.IsArtifact() || frozen.Timeline.Validate() != nil || frozen.Source == nil || frozen.Source.CanvasID == uuid.Nil || frozen.Source.NodeID == uuid.Nil || frozen.Source.Revision < 1 {
		return ErrInvalidExport
	}
	if frozen.OutputKind == domain.OutputJianying && (frozen.ExportID == nil || *frozen.ExportID == uuid.Nil) {
		return ErrInvalidExport
	}
	ids, err := SourceIDs(frozen.Timeline)
	if err != nil || len(ids) == 0 || len(ids) != len(frozen.Inputs) {
		return ErrInvalidExport
	}
	var total int64
	inputs := make(map[uuid.UUID]domain.FrozenSource, len(ids))
	for i, source := range frozen.Inputs {
		_, ok := ProjectMediaExtension(source.MIMEType)
		if !ok || source.AssetID != ids[i] || source.Revision < 1 || source.ByteSize < 1 || source.ByteSize > MaxProjectBytes || len(source.SHA256) != 64 || strings.Trim(source.SHA256, "0123456789abcdef") != "" {
			return ErrInvalidExport
		}
		if source.AudioChannels != nil && (*source.AudioChannels < 1 || *source.AudioChannels > 32) {
			return ErrInvalidExport
		}
		if frozen.OutputKind == domain.OutputXMEML && source.AudioChannels != nil && *source.AudioChannels > 2 {
			return fmt.Errorf("%w: xmeml currently supports mono/stereo audio", ErrInvalidExport)
		}
		if source.Kind != "audio" && (source.Width == nil || source.Height == nil || *source.Width < 1 || *source.Height < 1) {
			return ErrInvalidExport
		}
		if (source.Kind == "audio" || source.Kind == "video") && (source.DurationMS == nil || *source.DurationMS < 1) {
			return ErrInvalidExport
		}
		total += source.ByteSize
		if total > MaxProjectBytes-(8<<20) {
			return fmt.Errorf("%w: package byte budget", ErrInvalidExport)
		}
		inputs[source.AssetID] = source
	}
	if frozen.OutputKind == domain.OutputMediaBundle {
		return nil
	}
	if _, _, err := frozen.VideoDimensions(); err != nil {
		return fmt.Errorf("%w: explicit project aspect required", ErrInvalidExport)
	}
	for _, clip := range frozen.Timeline.Clips {
		if clip.Kind == "text" || clip.Kind == "subtitle" || clip.Crop != nil || clip.FadeInMS != 0 || clip.FadeOutMS != 0 {
			return fmt.Errorf("%w: interchange does not yet support text, crop, or fades", ErrInvalidExport)
		}
		if clip.AssetID == nil {
			return ErrInvalidExport
		}
		source, ok := inputs[*clip.AssetID]
		if !ok || source.Kind != clip.Kind {
			return ErrInvalidExport
		}
		if source.Kind == "video" {
			if source.FPS == nil {
				return fmt.Errorf("%w: video frame-rate metadata required", ErrInvalidExport)
			}
			if _, _, ok := ProjectSourceFrameRate(*source.FPS); !ok {
				return fmt.Errorf("%w: unsupported source frame rate", ErrInvalidExport)
			}
		}
	}
	return nil
}

// ProjectSourceFrameRate recognizes supported integer and NTSC source rates.
func ProjectSourceFrameRate(fps float64) (int64, int64, bool) {
	if math.IsNaN(fps) || math.IsInf(fps, 0) {
		return 0, 0, false
	}
	for _, rate := range []int64{24, 25, 30, 50, 60} {
		if math.Abs(fps-float64(rate)) < .001 {
			return rate, 1, true
		}
	}
	for _, rate := range []int64{24000, 30000, 60000} {
		if math.Abs(fps-float64(rate)/1001) < .01 {
			return rate, 1001, true
		}
	}
	return 0, 0, false
}
