package managed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// Videos isolates authenticated OpenRouter downloads from unsigned Ark storage.
type Videos struct{ client *http.Client }

// NewVideos never follows redirects while carrying an API Key.
func NewVideos() *Videos {
	return &Videos{client: &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Read fetches only the exact expected content URL and verifies the video bytes.
func (v *Videos) Read(ctx context.Context, c catalogdomain.ProviderConfig, taskID, raw, key string) ([]byte, int64, error) {
	if c.Validate() != nil || taskID == "" {
		return nil, 0, application.ErrInvalidProviderCall
	}
	var downloaded *mediaapp.Downloaded
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
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 512<<20+1))
		closeErr := response.Body.Close()
		if response.StatusCode != 200 || readErr != nil || closeErr != nil || len(data) > 512<<20 {
			return nil, 0, mediaflow.ErrResultUnavailable
		}
		downloaded, err = tempArtifact(data, "video/mp4")
	} else {
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !strings.HasSuffix(u.Hostname(), ".tos-cn-beijing.volces.com") {
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
	metadata, err := (mediaflow.FFProber{}).Probe(ctx, downloaded)
	if err != nil || string(metadata.Kind) != "video" || metadata.DurationMS == nil || *metadata.DurationMS <= 0 {
		return nil, 0, mediaflow.ErrUnsupportedMedia
	}
	if _, err := downloaded.File.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	data, err := io.ReadAll(io.LimitReader(downloaded.File, 512<<20+1))
	if err != nil || len(data) > 512<<20 {
		return nil, 0, mediaflow.ErrResultTooLarge
	}
	return data, int64(*metadata.DurationMS), nil
}
func tempArtifact(data []byte, mime string) (*mediaapp.Downloaded, error) {
	file, err := os.CreateTemp("", "glowloom-managed-*")
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	result := &mediaapp.Downloaded{File: file, Size: int64(len(data)), MIMEType: mime, SHA256: hex.EncodeToString(digest[:])}
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
