package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// FFProber validates media bytes with ffprobe and extracts basic media facts.
type FFProber struct{}

// Probe validates the downloaded bytes with ffprobe before storage.
func (FFProber) Probe(ctx context.Context, file *application.Downloaded) (application.ProbeResult, error) {
	if file == nil || file.File == nil {
		return application.ProbeResult{}, ErrUnsupportedMedia
	}
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries",
		"format=duration,format_name:stream=codec_type,codec_name,width,height,avg_frame_rate,channels,duration",
		"-of", "json", file.File.Name())
	output, err := cmd.Output()
	if err != nil {
		return application.ProbeResult{}, fmt.Errorf("probe media content: %w", ErrUnsupportedMedia)
	}
	if len(output) > 1_000_000 {
		return application.ProbeResult{}, ErrUnsupportedMedia
	}
	var parsed struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType    string `json:"codec_type"`
			CodecName    string `json:"codec_name"`
			Width        int32  `json:"width"`
			Height       int32  `json:"height"`
			AvgFrameRate string `json:"avg_frame_rate"`
			Channels     int32  `json:"channels"`
			Duration     string `json:"duration"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return application.ProbeResult{}, ErrUnsupportedMedia
	}
	probe := application.ProbeResult{}
	streamType := "video"
	switch file.MIMEType {
	case "image/jpeg":
		probe.Kind, probe.Extension = domain.KindImage, "jpg"
	case "image/png":
		probe.Kind, probe.Extension = domain.KindImage, "png"
	case "image/gif":
		probe.Kind, probe.Extension = domain.KindImage, "gif"
	case "image/webp":
		probe.Kind, probe.Extension = domain.KindImage, "webp"
	case "video/mp4":
		probe.Kind, probe.Extension = domain.KindVideo, "mp4"
	case "audio/mpeg":
		probe.Kind, probe.Extension, streamType = domain.KindAudio, "mp3", "audio"
	case "audio/wave", "audio/x-wav":
		probe.Kind, probe.Extension, streamType = domain.KindAudio, "wav", "audio"
	case "audio/ogg":
		probe.Kind, probe.Extension, streamType = domain.KindAudio, "ogg", "audio"
	default:
		return application.ProbeResult{}, ErrUnsupportedMedia
	}
	matched := false
	for _, stream := range parsed.Streams {
		if stream.CodecType != streamType || stream.CodecName == "" {
			continue
		}
		matched = true
		codec := stream.CodecName
		probe.Codec = &codec
		if stream.Width > 0 && stream.Height > 0 {
			probe.Width, probe.Height = &stream.Width, &stream.Height
		}
		if stream.Channels > 0 {
			probe.AudioChannels = &stream.Channels
		}
		if probe.Kind == domain.KindVideo {
			if fps, ok := frameRate(stream.AvgFrameRate); ok {
				probe.FPS = &fps
			}
		}
		duration := stream.Duration
		if duration == "" {
			duration = parsed.Format.Duration
		}
		if probe.Kind != domain.KindImage {
			if durationMS, ok := milliseconds(duration); ok {
				probe.DurationMS = &durationMS
			}
		}
		break
	}
	if !matched || (probe.Kind != domain.KindAudio && (probe.Width == nil || probe.Height == nil)) {
		return application.ProbeResult{}, ErrUnsupportedMedia
	}
	return probe, nil
}

func milliseconds(raw string) (int32, bool) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value >= float64(math.MaxInt32)/1000 {
		return 0, false
	}
	return int32(math.Round(value * 1000)), true
}

func frameRate(raw string) (float64, bool) {
	numerator, denominator, found := strings.Cut(raw, "/")
	if !found {
		return 0, false
	}
	n, nerr := strconv.ParseFloat(numerator, 64)
	d, derr := strconv.ParseFloat(denominator, 64)
	if nerr != nil || derr != nil || n <= 0 || d <= 0 {
		return 0, false
	}
	fps := n / d
	if math.IsInf(fps, 0) || math.IsNaN(fps) || fps > 1000 {
		return 0, false
	}
	return fps, true
}
