package operation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
	artifact "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/fetch"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
)

// GeneratedVideos preserves the existing legacy supplier transfer contract.
// Processing policy belongs to Media; native Execution uses owned artifact staging.
type GeneratedVideos struct{ client *http.Client }

// NewGeneratedVideos constructs the actual legacy bridge with redirects disabled.
func NewGeneratedVideos() *GeneratedVideos {
	return &GeneratedVideos{client: &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Read fetches only the original approved content URL and delegates video probing.
func (v *GeneratedVideos) Read(ctx context.Context, c catalog.ProviderConfig, taskID, raw, key string) ([]byte, int64, error) {
	if c.Validate() != nil || taskID == "" {
		return nil, 0, application.ErrInvalidProviderCall
	}
	var downloaded *fetch.Downloaded
	var err error
	if c.AdapterKey == "openrouter" {
		if raw != c.BaseURL+"/videos/"+taskID+"/content?index=0" || strings.ContainsAny(taskID, "/?#\\") {
			return nil, 0, mediaflow.ErrUnsafeResultURL
		}
		request, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if e != nil {
			return nil, 0, mediaflow.ErrUnsafeResultURL
		}
		request.Header.Set("Authorization", "Bearer "+key)
		response, e := v.client.Do(request)
		if e != nil {
			return nil, 0, mediaflow.ErrResultUnavailable
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, (512<<20)+1))
		closeErr := response.Body.Close()
		if response.StatusCode != http.StatusOK || readErr != nil || closeErr != nil || len(data) > 512<<20 {
			return nil, 0, mediaflow.ErrResultUnavailable
		}
		downloaded, err = generatedTempArtifact(data, "video/mp4")
	} else {
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || (!strings.HasSuffix(u.Hostname(), ".tos-cn-beijing.volces.com") && !catalog.ValidCreationResultURL(c.AdapterKey, raw)) {
			return nil, 0, mediaflow.ErrUnsafeResultURL
		}
		downloader, e := mediaflow.NewSecureDownloader(mediaflow.DownloadPolicy{AllowedOrigins: []string{"https://" + u.Host}, MaxBytes: 512 << 20})
		if e != nil {
			return nil, 0, e
		}
		downloaded, err = downloader.Download(ctx, raw)
	}
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = downloaded.Close() }()
	return processedGeneratedBytes(ctx, downloaded, "video_probe", 512<<20)
}

// ReadAudio retains the original music URL and input limits before Media processing.
func (v *GeneratedVideos) ReadAudio(ctx context.Context, config catalog.ProviderConfig, taskID, raw string) ([]byte, int64, error) {
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
	return processedGeneratedBytes(ctx, input, "music_mp3", 32<<20)
}

func processedGeneratedBytes(ctx context.Context, input *fetch.Downloaded, processing string, limit int64) ([]byte, int64, error) {
	request := artifact.Request{ByteSize: input.Size, SHA256: input.SHA256, MIME: input.MIMEType, Processing: processing}
	output, duration, err := (mediaflow.ProviderArtifactProcessor{}).ProcessProviderArtifact(ctx, request, input)
	if errors.Is(err, mediaflow.ErrGeneratedAudioInvalid) {
		return nil, 0, application.ErrInvalidProviderCall
	}
	if err != nil {
		return nil, 0, err
	}
	if output != input {
		defer func() { _ = output.Close() }()
	}
	if _, err := output.File.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	data, err := io.ReadAll(io.LimitReader(output.File, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, 0, mediaflow.ErrResultTooLarge
	}
	return data, duration, nil
}

func generatedTempArtifact(data []byte, mime string) (*fetch.Downloaded, error) {
	file, err := os.CreateTemp("", "glowloom-managed-*")
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	result := &fetch.Downloaded{File: file, Size: int64(len(data)), MIMEType: mime, SHA256: hex.EncodeToString(digest[:])}
	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		_ = result.Close()
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = result.Close()
		return nil, err
	}
	return result, nil
}
