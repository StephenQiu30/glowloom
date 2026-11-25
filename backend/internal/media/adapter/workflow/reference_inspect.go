package workflow

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg" // Decode the bounded JPEG emitted by ffmpeg.
	"os"
	"os/exec"
	"strconv"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// ReferenceFrame is an actual decoded image. TimestampMS is the requested
// video sampling time, not a claim that the whole clip or its audio was read.
type ReferenceFrame struct {
	TimestampMS   int64
	Width, Height int
	Data          []byte
}

// InspectReferenceVisuals validates the source with ffprobe and decodes one
// still or three bounded video samples. Only local files are passed to ffmpeg.
func InspectReferenceVisuals(ctx context.Context, source *application.Downloaded) ([]ReferenceFrame, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	probe, err := (FFProber{}).Probe(ctx, source)
	if err != nil {
		return nil, err
	}
	if probe.Width == nil || probe.Height == nil || int64(*probe.Width)*int64(*probe.Height) > 40_000_000 {
		return nil, ErrUnsupportedMedia
	}
	times := []int64{0}
	if probe.Kind == domain.KindVideo {
		if probe.DurationMS == nil || *probe.DurationMS < 3 {
			return nil, ErrUnsupportedMedia
		}
		duration := int64(*probe.DurationMS)
		times = []int64{duration / 10, duration / 2, duration * 9 / 10}
	} else if probe.Kind != domain.KindImage {
		return nil, ErrUnsupportedMedia
	}
	frames := make([]ReferenceFrame, 0, len(times))
	for _, timestamp := range times {
		frame, err := inspectFrame(ctx, source.File.Name(), timestamp, probe.Kind == domain.KindVideo)
		if err != nil {
			return nil, err
		}
		frames = append(frames, frame)
	}
	return frames, ctx.Err()
}

func inspectFrame(ctx context.Context, path string, timestamp int64, video bool) (ReferenceFrame, error) {
	output, err := os.CreateTemp("", "glowloom-reference-*.jpg")
	if err != nil {
		return ReferenceFrame{}, fmt.Errorf("create reference frame: %w", err)
	}
	name := output.Name()
	defer func() { _ = os.Remove(name) }()
	if err := output.Close(); err != nil {
		return ReferenceFrame{}, err
	}
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-protocol_whitelist", "file,pipe", "-threads", "1", "-filter_threads", "1", "-y"}
	if video {
		args = append(args, "-ss", strconv.FormatFloat(float64(timestamp)/1000, 'f', 3, 64))
	}
	args = append(args, "-i", path, "-map", "0:v:0", "-an", "-frames:v", "1", "-vf", "scale=640:640:force_original_aspect_ratio=decrease", "-q:v", "6", name)
	command := exec.CommandContext(ctx, "ffmpeg", args...)
	var diagnostics limitedWriter
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		return ReferenceFrame{}, fmt.Errorf("decode reference frame: %w", mediaProcessError(ctx, err))
	}
	info, err := os.Stat(name)
	if err != nil || info.Size() < 1 || info.Size() > 2<<20 {
		return ReferenceFrame{}, ErrResultTooLarge
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return ReferenceFrame{}, err
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || format != "jpeg" || decoded.Bounds().Dx() < 1 || decoded.Bounds().Dy() < 1 || decoded.Bounds().Dx() > 640 || decoded.Bounds().Dy() > 640 {
		return ReferenceFrame{}, ErrUnsupportedMedia
	}
	return ReferenceFrame{TimestampMS: timestamp, Width: decoded.Bounds().Dx(), Height: decoded.Bounds().Dy(), Data: data}, ctx.Err()
}
