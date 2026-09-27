package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// FFRenderer makes the preview formats representable by media.rendition.
// The original provider object remains the media asset's source file.
type FFRenderer struct{}

// Render creates image thumbnails or video poster/proxy using bounded local files.
func (FFRenderer) Render(ctx context.Context, source *application.Downloaded, probe application.ProbeResult, aspectRatio string) ([]application.RenditionFile, error) {
	if source == nil || source.File == nil {
		return nil, ErrUnsupportedMedia
	}
	var specs []renderSpec
	switch probe.Kind {
	case domain.KindImage:
		specs = []renderSpec{
			{kind: domain.RenditionThumb256, ext: "png", mimeType: "image/png", args: []string{
				"-frames:v", "1", "-vf", "scale=256:256:force_original_aspect_ratio=decrease", "-c:v", "png",
			}},
			{kind: domain.RenditionThumb640, ext: "png", mimeType: "image/png", args: []string{
				"-frames:v", "1", "-vf", "scale=640:640:force_original_aspect_ratio=decrease", "-c:v", "png",
			}},
		}
	case domain.KindVideo:
		width, height := "1280", "720"
		if aspectRatio == "9:16" {
			width, height = "720", "1280"
		} else if aspectRatio != "16:9" {
			return nil, ErrUnsupportedMedia
		}
		specs = []renderSpec{
			{kind: domain.RenditionPoster, ext: "png", mimeType: "image/png", args: []string{
				"-frames:v", "1", "-vf", "scale=640:640:force_original_aspect_ratio=decrease", "-c:v", "png",
			}},
			{kind: domain.RenditionProxy720p, ext: "mp4", mimeType: "video/mp4", args: []string{
				"-vf", "scale=" + width + ":" + height + ":force_original_aspect_ratio=decrease,pad=" +
					width + ":" + height + ":(ow-iw)/2:(oh-ih)/2,setsar=1",
				"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p",
				"-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart",
			}},
		}
	case domain.KindAudio:
		return nil, nil
	default:
		return nil, ErrUnsupportedMedia
	}
	results := make([]application.RenditionFile, 0, len(specs))
	keep := false
	defer func() {
		if !keep {
			for _, result := range results {
				_ = result.Result.Close()
			}
		}
	}()
	for _, spec := range specs {
		result, err := renderOne(ctx, source.File.Name(), spec)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	keep = true
	return results, nil
}

type renderSpec struct {
	kind     domain.RenditionKind
	ext      string
	mimeType string
	args     []string
}

func renderOne(ctx context.Context, sourcePath string, spec renderSpec) (application.RenditionFile, error) {
	output, err := os.CreateTemp("", "lanverse-render-*-."+spec.ext)
	if err != nil {
		return application.RenditionFile{}, fmt.Errorf("create media preview temp file: %w", err)
	}
	name := output.Name()
	if err := output.Close(); err != nil {
		_ = os.Remove(name)
		return application.RenditionFile{}, fmt.Errorf("close media preview temp file: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(name)
		}
	}()
	args := append([]string{"-nostdin", "-hide_banner", "-loglevel", "error", "-protocol_whitelist", "file,pipe", "-y", "-i", sourcePath}, spec.args...)
	args = append(args, name)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr limitedWriter
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return application.RenditionFile{}, fmt.Errorf("render %s media preview: %w", spec.kind, ErrUnsupportedMedia)
	}
	file, err := os.Open(name)
	if err != nil {
		return application.RenditionFile{}, fmt.Errorf("open rendered media preview: %w", err)
	}
	defer func() {
		if !keep {
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return application.RenditionFile{}, fmt.Errorf("stat rendered media preview: %w", err)
	}
	if info.Size() < 1 || info.Size() > maxResultBytes {
		return application.RenditionFile{}, ErrResultTooLarge
	}
	var magic [512]byte
	count, err := file.ReadAt(magic[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return application.RenditionFile{}, fmt.Errorf("inspect rendered media preview: %w", err)
	}
	if detected := http.DetectContentType(magic[:count]); detected != spec.mimeType {
		return application.RenditionFile{}, ErrUnsupportedMedia
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return application.RenditionFile{}, fmt.Errorf("hash rendered media preview: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return application.RenditionFile{}, fmt.Errorf("rewind rendered media preview: %w", err)
	}
	prepared := &application.Downloaded{
		File: file, Size: info.Size(), MIMEType: spec.mimeType, SHA256: hex.EncodeToString(hash.Sum(nil)),
	}
	verified, err := (FFProber{}).Probe(ctx, prepared)
	if err != nil || verified.Width == nil || verified.Height == nil {
		return application.RenditionFile{}, ErrUnsupportedMedia
	}
	keep = true
	return application.RenditionFile{
		Kind: spec.kind, Result: prepared, Width: *verified.Width, Height: *verified.Height, Ext: spec.ext,
	}, nil
}

// limitedWriter prevents verbose ffmpeg diagnostics from consuming worker memory.
type limitedWriter struct{ bytes.Buffer }

func (w *limitedWriter) Write(value []byte) (int, error) {
	const maxDiagnosticBytes = 4_096
	if w.Len() < maxDiagnosticBytes {
		_, _ = w.Buffer.Write(value[:min(len(value), maxDiagnosticBytes-w.Len())])
	}
	return len(value), nil
}
