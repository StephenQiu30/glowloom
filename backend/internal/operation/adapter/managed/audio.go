package managed

import (
	"context"
	"io"
	"net/url"
	"os"
	"os/exec"
	"time"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// ReadAudio normalizes music results to a verified MP3. The supplier documents
// WAV and MP4 wrappers even when audio was requested; MIME is never relabelled.
func (v *Videos) ReadAudio(ctx context.Context, config catalog.ProviderConfig, taskID, raw string) ([]byte, int64, error) {
	if config.Validate() != nil || taskID == "" || !catalog.ValidCreationResultURL(config.AdapterKey, raw) {
		return nil, 0, mediaflow.ErrUnsafeResultURL
	}
	u, _ := url.Parse(raw)
	downloader, err := mediaflow.NewSecureDownloader(mediaflow.DownloadPolicy{AllowedOrigins: []string{"https://" + u.Host}, MaxBytes: 128 << 20})
	if err != nil {
		return nil, 0, err
	}
	input, err := downloader.Download(ctx, raw)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = input.Close() }()
	return normalizeMusicAudio(ctx, input)
}

func normalizeMusicAudio(ctx context.Context, input *mediaapp.Downloaded) ([]byte, int64, error) {
	output, err := os.CreateTemp("", "glowloom-music-*.mp3")
	if err != nil {
		return nil, 0, err
	}
	name := output.Name()
	_ = output.Close()
	defer func() { _ = os.Remove(name) }()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// No remote protocols are available to the decoder. Only the downloaded,
	// bounded file may be opened, and a missing audio stream fails explicitly.
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-protocol_whitelist", "file,pipe", "-i", input.File.Name(), "-map", "0:a:0", "-vn", "-c:a", "libmp3lame", "-b:a", "192k", "-threads", "1", "-fs", "33554432", "-y", name)
	if err := cmd.Run(); err != nil {
		return nil, 0, mediaflow.ErrUnsupportedMedia
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, (32<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 32<<20 {
		return nil, 0, mediaflow.ErrResultTooLarge
	}
	verified, err := tempArtifact(data, "audio/mpeg")
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = verified.Close() }()
	meta, err := (mediaflow.FFProber{}).Probe(ctx, verified)
	if err != nil || meta.Kind != "audio" || meta.DurationMS == nil || *meta.DurationMS <= 0 || *meta.DurationMS > 241000 {
		return nil, 0, application.ErrInvalidProviderCall
	}
	return data, int64(*meta.DurationMS), nil
}
