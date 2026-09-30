package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	return probeFile(ctx, file, false)
}

func probeFile(ctx context.Context, file *application.Downloaded, upload bool) (application.ProbeResult, error) {
	if file == nil || file.File == nil {
		return application.ProbeResult{}, ErrUnsupportedMedia
	}
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries",
		"format=duration,format_name:stream=codec_type,codec_name,width,height,avg_frame_rate,channels,duration",
		"-of", "json", file.File.Name())
	var output probeOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return application.ProbeResult{}, fmt.Errorf("probe media content: %w", mediaProcessError(ctx, err))
	}
	if output.overflow {
		return application.ProbeResult{}, ErrUnsupportedMedia
	}
	var parsed struct {
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
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
	if err := json.Unmarshal(output.Bytes(), &parsed); err != nil {
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
	case "video/quicktime":
		probe.Kind, probe.Extension = domain.KindVideo, "mov"
	case "audio/mp4":
		probe.Kind, probe.Extension, streamType = domain.KindAudio, "m4a", "audio"
	case "audio/mpeg":
		probe.Kind, probe.Extension, streamType = domain.KindAudio, "mp3", "audio"
	case "audio/wave", "audio/x-wav":
		probe.Kind, probe.Extension, streamType = domain.KindAudio, "wav", "audio"
	case "audio/ogg":
		probe.Kind, probe.Extension, streamType = domain.KindAudio, "ogg", "audio"
	default:
		return application.ProbeResult{}, ErrUnsupportedMedia
	}
	if (file.MIMEType == "video/mp4" || file.MIMEType == "video/quicktime" || file.MIMEType == "audio/mp4") &&
		!strings.Contains(","+parsed.Format.Name+",", ",mov,") {
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
	if upload && probe.Kind != domain.KindImage {
		if excessiveUploadDuration(parsed.Format.Duration) {
			return application.ProbeResult{}, ErrUnsupportedMedia
		}
		longest, _ := uploadMilliseconds(parsed.Format.Duration)
		for _, stream := range parsed.Streams {
			if stream.CodecType != "video" && stream.CodecType != "audio" {
				continue
			}
			if excessiveUploadDuration(stream.Duration) {
				return application.ProbeResult{}, ErrUnsupportedMedia
			}
			if duration, valid := uploadMilliseconds(stream.Duration); valid && duration > longest {
				longest = duration
			}
		}
		if longest <= 0 {
			return application.ProbeResult{}, ErrUnsupportedMedia
		}
		probe.DurationMS = &longest
	}
	return probe, nil
}

func excessiveUploadDuration(raw string) bool {
	seconds, err := strconv.ParseFloat(raw, 64)
	return err == nil && (math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds >= float64(math.MaxInt32)/1000)
}

// probeOutput bounds diagnostic JSON while letting the child finish without a
// blocked stdout pipe. Oversized output is rejected before JSON decoding.
type probeOutput struct {
	bytes.Buffer
	overflow bool
}

func (p *probeOutput) Write(value []byte) (int, error) {
	const limit = 1_000_000
	available := limit - p.Len()
	if len(value) > available {
		p.overflow = true
	}
	if available > 0 {
		_, _ = p.Buffer.Write(value[:min(len(value), available)])
	}
	return len(value), nil
}

func uploadMilliseconds(raw string) (int32, bool) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value >= float64(math.MaxInt32)/1000 {
		return 0, false
	}
	return int32(math.Ceil(value * 1000)), true
}

// mediaProcessError keeps worker cancellation and process startup failures retryable.
// Only a decoder's ordinary nonzero exit indicates an unsupported media input.
func mediaProcessError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() >= 0 {
		return ErrUnsupportedMedia
	}
	return err
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
