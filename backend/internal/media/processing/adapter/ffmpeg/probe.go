package ffmpeg

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
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/application"
)

type mediaProbe struct {
	Format struct {
		Name     string `json:"format_name"`
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		Type     string `json:"codec_type"`
		Codec    string `json:"codec_name"`
		Width    int32  `json:"width"`
		Height   int32  `json:"height"`
		FPS      string `json:"avg_frame_rate"`
		Duration string `json:"duration"`
	} `json:"streams"`
}

func probeMedia(ctx context.Context, name string) (mediaProbe, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries",
		"format=duration,format_name:stream=codec_type,codec_name,width,height,avg_frame_rate,duration", "-of", "json", name)
	var output probeOutput
	cmd.Stdout = &output
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return mediaProbe{}, fmt.Errorf("probe recording: %w", mediaProcessError(ctx, err))
	}
	var parsed mediaProbe
	if output.overflow || json.Unmarshal(output.Bytes(), &parsed) != nil {
		return mediaProbe{}, application.ErrUnsupported
	}
	return parsed, nil
}

func probeSource(ctx context.Context, name string) (string, error) {
	parsed, err := probeMedia(ctx, name)
	if err != nil {
		return "", err
	}
	if !strings.Contains(","+parsed.Format.Name+",", ",webm,") || len(parsed.Streams) != 1 {
		return "", application.ErrUnsupported
	}
	stream := parsed.Streams[0]
	if stream.Type != "video" || (stream.Codec != "vp8" && stream.Codec != "vp9") || !validDimensions(stream.Width, stream.Height) {
		return "", application.ErrUnsupported
	}
	if !validRecordingFrameRate(stream.FPS) {
		return "", application.ErrUnsupported
	}
	if _, ok := recordingDuration(parsed); !ok {
		return "", application.ErrUnsupported
	}
	return stream.Codec, nil
}

func probeCanonical(ctx context.Context, name string) error {
	parsed, err := probeMedia(ctx, name)
	if err != nil {
		return err
	}
	// The encoder request is not evidence: verify the actual MP4 container,
	// its single silent H264 stream, dimensions, FPS and longest duration.
	if !strings.Contains(","+parsed.Format.Name+",", ",mov,") || len(parsed.Streams) != 1 {
		return application.ErrUnsupported
	}
	stream := parsed.Streams[0]
	if stream.Type != "video" || stream.Codec != "h264" || !validDimensions(stream.Width, stream.Height) {
		return application.ErrUnsupported
	}
	if !validRecordingFrameRate(stream.FPS) {
		return application.ErrUnsupported
	}
	if duration, ok := recordingDuration(parsed); !ok || duration < 1 {
		return application.ErrUnsupported
	}
	return nil
}

func validDimensions(width, height int32) bool {
	return width >= 2 && height >= 2 && width <= 8192 && height <= 8192 && int64(width)*int64(height) <= 40_000_000
}

// recordingDuration accepts unknown source durations, but rejects every known
// excessive duration, including a stream duration longer than the container.
func recordingDuration(parsed mediaProbe) (int32, bool) {
	durations := []string{parsed.Format.Duration}
	for _, stream := range parsed.Streams {
		durations = append(durations, stream.Duration)
	}
	var longest int32
	for _, raw := range durations {
		seconds, err := strconv.ParseFloat(raw, 64)
		if err == nil && (math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds >= float64(math.MaxInt32)/1000) {
			return 0, false
		}
		if duration, ok := uploadMilliseconds(raw); ok {
			if duration > 60_000 {
				return 0, false
			}
			longest = max(longest, duration)
		}
	}
	return longest, true
}

func uploadMilliseconds(raw string) (int32, bool) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value >= float64(math.MaxInt32)/1000 {
		return 0, false
	}
	return int32(math.Ceil(value * 1000)), true
}

func validRecordingFrameRate(raw string) bool {
	// Live recordings may not publish a frame rate, but malformed or
	// nonfinite published values must not bypass the 60 FPS limit.
	if raw == "" || raw == "N/A" || raw == "0/0" {
		return true
	}
	fps, ok := frameRate(raw)
	return ok && fps <= 60
}

func frameRate(raw string) (float64, bool) {
	numerator, denominator, found := strings.Cut(raw, "/")
	if !found {
		return 0, false
	}
	n, nerr := strconv.ParseFloat(numerator, 64)
	d, derr := strconv.ParseFloat(denominator, 64)
	if nerr != nil || derr != nil || n <= 0 || d <= 0 || math.IsInf(n, 0) || math.IsInf(d, 0) {
		return 0, false
	}
	fps := n / d
	// Do not turn an extremely high, but finite, FPS into an unknown FPS:
	// the caller must still reject it against the recording's 60 FPS limit.
	if math.IsInf(fps, 0) || math.IsNaN(fps) {
		return 0, false
	}
	return fps, true
}

// probeOutput bounds diagnostic JSON while letting the child finish without
// a blocked stdout pipe. Overflow is rejected before JSON decoding.
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

// mediaProcessError preserves cancellation, startup failures and process
// signals. Only an ordinary decoder exit is classified as unsupported input.
func mediaProcessError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() >= 0 {
		return application.ErrUnsupported
	}
	return err
}
