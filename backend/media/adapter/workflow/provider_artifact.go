package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/fetch"
)

// ErrGeneratedAudioInvalid identifies an invalid canonical audio probe.
var ErrGeneratedAudioInvalid = errors.New("generated audio probe invalid")

// ProviderArtifactProcessor owns actual supplier-byte probing and music encoding.
// It borrows the original file and never receives provider keys, URLs or a DB.
type ProviderArtifactProcessor struct{}

// ProcessProviderArtifact returns an owned canonical file or the borrowed original.
// The caller closes a returned file only when it differs from the original.
func (ProviderArtifactProcessor) ProcessProviderArtifact(ctx context.Context, req contract.Request, input *fetch.Downloaded) (*fetch.Downloaded, int64, error) {
	if input == nil || input.File == nil || input.Size != req.ByteSize || input.SHA256 != req.SHA256 || input.MIMEType != req.MIME {
		return nil, 0, ErrUnsupportedMedia
	}
	switch req.Processing {
	case "", "original":
		return input, 0, nil
	case "video_probe":
		if input.Size > 512<<20 || input.MIMEType != "video/mp4" {
			return nil, 0, ErrUnsupportedMedia
		}
		meta, err := (FFProber{}).Probe(ctx, input)
		if err != nil || meta.Kind != "video" || meta.DurationMS == nil || *meta.DurationMS <= 0 {
			return nil, 0, ErrUnsupportedMedia
		}
		return input, int64(*meta.DurationMS), nil
	case "music_mp3":
		if input.Size > 128<<20 {
			return nil, 0, ErrResultTooLarge
		}
		switch input.MIMEType {
		case "audio/mpeg", "audio/wave", "audio/x-wav", "video/mp4":
		default:
			return nil, 0, ErrUnsupportedMedia
		}
	default:
		return nil, 0, ErrUnsupportedMedia
	}
	output, err := os.CreateTemp("", "glowloom-music-*.mp3")
	if err != nil {
		return nil, 0, err
	}
	name := output.Name()
	if err := output.Close(); err != nil {
		_ = os.Remove(name)
		return nil, 0, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-protocol_whitelist", "file,pipe", "-i", input.File.Name(), "-map", "0:a:0", "-vn", "-c:a", "libmp3lame", "-b:a", "192k", "-threads", "1", "-fs", "33554432", "-y", name)
	if err := cmd.Run(); err != nil {
		return nil, 0, ErrUnsupportedMedia
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, 0, err
	}
	result := &fetch.Downloaded{File: file, MIMEType: "audio/mpeg"}
	keep := false
	defer func() {
		if !keep {
			_ = result.Close()
		}
	}()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, (32<<20)+1))
	if err != nil {
		return nil, 0, err
	}
	if size < 1 || size > 32<<20 {
		return nil, 0, ErrResultTooLarge
	}
	result.Size, result.SHA256 = size, hex.EncodeToString(hash.Sum(nil))
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	meta, err := (FFProber{}).Probe(ctx, result)
	if err != nil || meta.Kind != "audio" || meta.DurationMS == nil || *meta.DurationMS <= 0 || *meta.DurationMS > 241000 {
		return nil, 0, errors.Join(ErrUnsupportedMedia, ErrGeneratedAudioInvalid)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	cleanup = false
	keep = true
	return result, int64(*meta.DurationMS), nil
}
