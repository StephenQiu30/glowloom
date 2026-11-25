package managed

import (
	"context"
	"io"
	"net/url"
	"strings"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/fetch"
)

// GeneratedFiles downloads original supplier bytes. It has no decoder or object store.
type GeneratedFiles interface {
	Music(context.Context, catalog.ProviderConfig, string, string) (*fetch.Downloaded, error)
	Video(context.Context, catalog.ProviderConfig, string, string, string) (*fetch.Downloaded, error)
}

// SupplierFiles preserves provider-specific URL and credential rules in Execution.
// Media owns the subsequent probe, normalization and physical byte responsibility.
type SupplierFiles struct{}

// NewSupplierFiles constructs the authenticated raw-file adapter without credentials.
func NewSupplierFiles() *SupplierFiles { return &SupplierFiles{} }

// Music downloads the original WAV, MP4 or MP3; it never relabels or decodes bytes.
func (*SupplierFiles) Music(ctx context.Context, c catalog.ProviderConfig, taskID, raw string) (*fetch.Downloaded, error) {
	if c.Validate() != nil || taskID == "" || !catalog.ValidCreationResultURL(c.AdapterKey, raw) {
		return nil, fetch.ErrUnsafeResultURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return nil, fetch.ErrUnsafeResultURL
	}
	downloader, err := fetch.NewSecureDownloader(fetch.DownloadPolicy{AllowedOrigins: []string{"https://" + u.Host}, AcceptedMIME: []string{"audio/mpeg", "audio/wave", "audio/x-wav", "video/mp4"}, MaxBytes: 128 << 20})
	if err != nil {
		return nil, err
	}
	return downloader.Download(ctx, raw)
}

// Video uses a credential only for the exact original OpenRouter content endpoint.
func (*SupplierFiles) Video(ctx context.Context, c catalog.ProviderConfig, taskID, raw, key string) (*fetch.Downloaded, error) {
	if c.Validate() != nil || taskID == "" {
		return nil, application.ErrInvalidProviderCall
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return nil, fetch.ErrUnsafeResultURL
	}
	authorization := ""
	if c.AdapterKey == "openrouter" {
		if raw != c.BaseURL+"/videos/"+taskID+"/content?index=0" || strings.ContainsAny(taskID, "/?#\\") || key == "" {
			return nil, fetch.ErrUnsafeResultURL
		}
		authorization = "Bearer " + key
	} else if !strings.HasSuffix(u.Hostname(), ".tos-cn-beijing.volces.com") && !catalog.ValidCreationResultURL(c.AdapterKey, raw) {
		return nil, fetch.ErrUnsafeResultURL
	}
	downloader, err := fetch.NewSecureDownloader(fetch.DownloadPolicy{AllowedOrigins: []string{"https://" + u.Host}, AcceptedMIME: []string{"video/mp4"}, MaxBytes: 512 << 20})
	if err != nil {
		return nil, err
	}
	if authorization != "" {
		return downloader.DownloadAuthenticated(ctx, raw, authorization)
	}
	return downloader.Download(ctx, raw)
}

func (a *Activities) stageDownloaded(ctx context.Context, i application.ProviderDispatchIdentity, file *fetch.Downloaded) (application.ManagedArtifact, error) {
	if file == nil || file.File == nil {
		return application.ManagedArtifact{}, application.ErrInvalidProviderCall
	}
	defer func() { _ = file.Close() }()
	if _, err := file.File.Seek(0, 0); err != nil {
		return application.ManagedArtifact{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file.File, (512<<20)+1))
	if err != nil || int64(len(data)) != file.Size {
		return application.ManagedArtifact{}, application.ErrInvalidProviderCall
	}
	return a.results.StageArtifact(ctx, i, 1, data, file.MIMEType)
}

var _ GeneratedFiles = (*SupplierFiles)(nil)
