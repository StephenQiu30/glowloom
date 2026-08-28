package runware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
)

const maximumRunwareRedirects = 5

type PrivateObjectStore interface {
	EnsurePrivateObject(context.Context, string, []byte, string, string) error
}

type ImageStagerConfig struct {
	ObjectStore     PrivateObjectStore
	Transport       http.RoundTripper
	ResolveIP       func(context.Context, string) ([]net.IP, error)
	DownloadTimeout time.Duration
	MaxImageBytes   int64
	MaxPixels       int64
}

type PrivateImageStager struct {
	objects         PrivateObjectStore
	client          *http.Client
	resolveIP       func(context.Context, string) ([]net.IP, error)
	downloadTimeout time.Duration
	maxImageBytes   int64
	maxPixels       int64
}

func NewImageStager(config ImageStagerConfig) (*PrivateImageStager, error) {
	if config.ObjectStore == nil || config.DownloadTimeout <= 0 || config.DownloadTimeout > maximumRequestTimeout ||
		config.MaxImageBytes < 1 || config.MaxPixels < 1 {
		return nil, errors.New("invalid Runware image stager configuration")
	}
	resolver := config.ResolveIP
	if resolver == nil {
		resolver = func(ctx context.Context, host string) ([]net.IP, error) {
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			values := make([]net.IP, len(addresses))
			for index := range addresses {
				values[index] = addresses[index].IP
			}
			return values, nil
		}
	}
	transport := config.Transport
	if transport == nil {
		transport = safeRunwareImageTransport(resolver, config.DownloadTimeout)
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &PrivateImageStager{
		objects: config.ObjectStore, client: client, resolveIP: resolver,
		downloadTimeout: config.DownloadTimeout, maxImageBytes: config.MaxImageBytes, maxPixels: config.MaxPixels,
	}, nil
}

func (stager *PrivateImageStager) StageImage(
	ctx context.Context,
	request StageImageRequest,
) (application.ProviderOutput, error) {
	if stager == nil || stager.objects == nil || stager.client == nil || stager.resolveIP == nil ||
		!validStageImageRequest(request) {
		return application.ProviderOutput{}, errors.New("invalid Runware image staging request")
	}
	downloadContext, cancel := context.WithTimeout(ctx, stager.downloadTimeout)
	defer cancel()
	contents, err := stager.download(downloadContext, request.ImageURL)
	if err != nil {
		return application.ProviderOutput{}, err
	}
	configuration, err := png.DecodeConfig(bytes.NewReader(contents))
	if err != nil || configuration.Width != request.Width || configuration.Height != request.Height ||
		configuration.Width < 1 || configuration.Height < 1 ||
		int64(configuration.Width)*int64(configuration.Height) > stager.maxPixels {
		return application.ProviderOutput{}, errors.New("Runware output PNG bounds drifted")
	}
	if _, err = png.Decode(bytes.NewReader(contents)); err != nil {
		return application.ProviderOutput{}, errors.New("Runware output PNG decode failed")
	}
	digest := sha256.Sum256(contents)
	sha256Hex := hex.EncodeToString(digest[:])
	objectKey := "staging/" + request.WorkspaceID + "/" + request.ProviderJobID + "/" + request.ImageUUID + ".png"
	if err = stager.objects.EnsurePrivateObject(downloadContext, objectKey, contents, "image/png", sha256Hex); err != nil {
		return application.ProviderOutput{}, errors.New("private Runware output staging failed")
	}
	return application.ProviderOutput{
		OutputKey: request.ImageUUID, StagingObjectKey: objectKey, SHA256: sha256Hex,
		Bytes: int64(len(contents)), MediaType: "image/png", Width: configuration.Width, Height: configuration.Height,
	}, nil
}

func (stager *PrivateImageStager) download(ctx context.Context, rawURL string) ([]byte, error) {
	current, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("invalid Runware output URL")
	}
	for redirects := 0; redirects <= maximumRunwareRedirects; redirects++ {
		if err = validateRunwareImageURL(ctx, current, stager.resolveIP); err != nil {
			return nil, err
		}
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, current.String(), nil)
		if requestErr != nil {
			return nil, errors.New("build Runware output request")
		}
		request.Header.Set("Accept", "image/png")
		response, requestErr := stager.client.Do(request)
		if requestErr != nil {
			return nil, errors.New("Runware output download failed")
		}
		if redirectStatus(response.StatusCode) {
			location, locationErr := response.Location()
			closeRunwareResponse(response)
			if locationErr != nil || redirects == maximumRunwareRedirects {
				return nil, errors.New("invalid Runware output redirect")
			}
			current = location
			continue
		}
		if response.StatusCode != http.StatusOK {
			closeRunwareResponse(response)
			return nil, fmt.Errorf("Runware output returned HTTP status %d", response.StatusCode)
		}
		mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if mediaErr != nil || mediaType != "image/png" || response.ContentLength > stager.maxImageBytes {
			closeRunwareResponse(response)
			return nil, errors.New("Runware output media declaration is invalid")
		}
		contents, readErr := io.ReadAll(io.LimitReader(response.Body, stager.maxImageBytes+1))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || len(contents) == 0 || int64(len(contents)) > stager.maxImageBytes ||
			(response.ContentLength >= 0 && response.ContentLength != int64(len(contents))) {
			return nil, errors.New("Runware output body is invalid")
		}
		return contents, nil
	}
	return nil, errors.New("invalid Runware output redirect")
}

func validStageImageRequest(request StageImageRequest) bool {
	for _, identifier := range []string{request.WorkspaceID, request.ProjectID, request.ProviderJobID, request.ImageUUID} {
		if _, err := uuid.Parse(strings.TrimSpace(identifier)); err != nil {
			return false
		}
	}
	return request.Width > 0 && request.Height > 0 && request.OutputFormat == "PNG" && strings.TrimSpace(request.ImageURL) != ""
}

func validateRunwareImageURL(
	ctx context.Context,
	value *url.URL,
	resolveIP func(context.Context, string) ([]net.IP, error),
) error {
	if value == nil || value.Scheme != "https" || value.Hostname() != runwareOutputHost ||
		(value.Port() != "" && value.Port() != "443") || value.User != nil || value.RawQuery != "" ||
		value.Fragment != "" || value.Path == "" {
		return errors.New("invalid Runware output URL")
	}
	addresses, err := resolveIP(ctx, value.Hostname())
	if err != nil || len(addresses) == 0 {
		return errors.New("Runware output host resolution failed")
	}
	for _, address := range addresses {
		if !publicRunwareIP(address) {
			return errors.New("Runware output host is not public")
		}
	}
	return nil
}

func publicRunwareIP(value net.IP) bool {
	return value != nil && value.IsGlobalUnicast() && !value.IsPrivate() && !value.IsLoopback() &&
		!value.IsLinkLocalUnicast() && !value.IsLinkLocalMulticast() && !value.IsUnspecified()
}

func safeRunwareImageTransport(
	resolveIP func(context.Context, string) ([]net.IP, error),
	timeout time.Duration,
) *http.Transport {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || host != runwareOutputHost || port != "443" {
				return nil, errors.New("invalid Runware output dial address")
			}
			addresses, err := resolveIP(ctx, host)
			if err != nil || len(addresses) == 0 {
				return nil, errors.New("Runware output host resolution failed")
			}
			for _, candidate := range addresses {
				if !publicRunwareIP(candidate) {
					return nil, errors.New("Runware output host is not public")
				}
			}
			slices.SortFunc(addresses, func(left, right net.IP) int {
				return strings.Compare(left.String(), right.String())
			})
			var dialErr error
			for _, candidate := range addresses {
				connection, candidateErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
				if candidateErr == nil {
					return connection, nil
				}
				dialErr = candidateErr
			}
			return nil, fmt.Errorf("dial Runware output host: %w", dialErr)
		},
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   4,
	}
}

func redirectStatus(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func closeRunwareResponse(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	_ = response.Body.Close()
}
