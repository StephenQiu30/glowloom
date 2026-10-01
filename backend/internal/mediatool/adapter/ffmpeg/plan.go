// Package ffmpeg renders owned local timelines with bounded command arguments.
package ffmpeg

import (
	"fmt"
	"math"
	"slices"
	"strings"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

type segment struct {
	start, end int64
	clips      []canvasdomain.TimelineClip
}

func renderSegments(config canvasdomain.TimelineConfig) ([]segment, error) {
	if config.Validate() != nil {
		return nil, application.ErrInvalidExport
	}
	boundaries := []int64{0}
	tracks := make(map[string]canvasdomain.TimelineTrack, len(config.Tracks))
	for _, track := range config.Tracks {
		tracks[track.ID.String()] = track
	}
	hasMedia := false
	for _, clip := range config.Clips {
		track := tracks[clip.TrackID.String()]
		if !track.Visible {
			continue
		}
		hasMedia = true
		boundaries = append(boundaries, frame(clip.StartMS, config.FPS), frame(clip.StartMS+clip.DurationMS, config.FPS))
	}
	if !hasMedia {
		return nil, application.ErrInvalidExport
	}
	slices.Sort(boundaries)
	boundaries = slices.Compact(boundaries)
	var segments []segment
	for i := 1; i < len(boundaries); i++ {
		if boundaries[i] <= boundaries[i-1] {
			continue
		}
		part := segment{start: boundaries[i-1], end: boundaries[i]}
		for _, track := range config.Tracks {
			if !track.Visible {
				continue
			}
			for _, clip := range config.Clips {
				if clip.TrackID == track.ID && part.start >= frame(clip.StartMS, config.FPS) && part.end <= frame(clip.StartMS+clip.DurationMS, config.FPS) && clip.Kind != "text" && clip.Kind != "subtitle" {
					part.clips = append(part.clips, clip)
				}
			}
		}
		segments = append(segments, part)
	}
	return segments, nil
}

func frame(ms int64, fps int) int64 { return int64(math.Round(float64(ms) * float64(fps) / 1000)) }
func seconds(ms float64) string     { return fmt.Sprintf("%.6f", ms/1000) }
func frameSeconds(frames int64, fps int) string {
	return fmt.Sprintf("%.6f", float64(frames)/float64(fps))
}

func audioEnvelope(clip canvasdomain.TimelineClip, elapsedMS float64) string {
	// afade evaluates per sample. Preserve the clip's absolute offset while each
	// rendered segment starts at zero, then reset timestamps for local mixing.
	parts := []string{fmt.Sprintf("asetpts=PTS-STARTPTS+%.6f/TB", elapsedMS/1000), fmt.Sprintf("volume=%.6f", clip.Volume)}
	if clip.FadeInMS > 0 {
		parts = append(parts, fmt.Sprintf("afade=t=in:st=0:d=%.6f", float64(clip.FadeInMS)/1000))
	}
	if clip.FadeOutMS > 0 {
		parts = append(parts, fmt.Sprintf("afade=t=out:st=%.6f:d=%.6f", float64(clip.DurationMS-clip.FadeOutMS)/1000, float64(clip.FadeOutMS)/1000))
	}
	parts = append(parts, "asetpts=PTS-STARTPTS")
	return strings.Join(parts, ",")
}

func dimensions(aspect string) (int, int) {
	switch aspect {
	case "9:16":
		return 1080, 1920
	case "1:1":
		return 1080, 1080
	default:
		return 1920, 1080
	}
}
