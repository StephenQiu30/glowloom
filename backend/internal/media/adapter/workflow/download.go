// Package workflow adapts media ingestion to the Temporal activity boundary.
package workflow

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

const (
	maxResultBytes  int64 = 2_000_000_000
	maxRedirects          = 3
	downloadTimeout       = 10 * time.Minute
)

var (
	// ErrUnsafeResultURL means the result URL, its redirect, or its resolved IP
	// violates the provider result download policy.
	ErrUnsafeResultURL = errors.New("unsafe media result URL")
	// ErrResultTooLarge means the body exceeded the allowed byte count.
	ErrResultTooLarge = errors.New("media result is too large")
	// ErrUnsupportedMedia means the downloaded bytes are not an accepted media type.
	ErrUnsupportedMedia = errors.New("unsupported media")
	// ErrResultUnavailable means the remote server did not return a usable result.
	ErrResultUnavailable = errors.New("media result unavailable")
	// ErrResultExpired means the signed result URL can no longer be fetched.
	ErrResultExpired = errors.New("media result URL expired or inaccessible")
)

// IPResolver is injected only to make DNS safety checks deterministic in tests.
// Production uses net.DefaultResolver.
type IPResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// DownloadPolicy fixes a provider's approved HTTPS result origins. An origin
// has a scheme and host (including any port), but no path or credentials.
type DownloadPolicy struct {
	AllowedOrigins       []string
	Resolver             IPResolver
	TLSRootCAs           *x509.CertPool
	MaxBytes             int64
	AllowTestLoopbackTLS bool
}

// SecureDownloader validates every request and pins each connection to the
// previously checked IP, so the HTTP transport cannot perform a second DNS
// lookup after validation.
type SecureDownloader struct {
	allowed           map[string]struct{}
	resolver          IPResolver
	maxBytes          int64
	allowTestLoopback bool
	client            *http.Client
}

// Downloaded is the application's bounded temporary media result.
type Downloaded = application.Downloaded

// NewSecureDownloader treats an empty allowlist as a disabled download path.
// Malformed entries still fail startup so a configured provider cannot silently
// bypass its expected result origin.
func NewSecureDownloader(policy DownloadPolicy) (*SecureDownloader, error) {
	maxBytes := policy.MaxBytes
	if maxBytes == 0 {
		maxBytes = maxResultBytes
	}
	if maxBytes < 1 || maxBytes > maxResultBytes {
		return nil, fmt.Errorf("configure media result size: %w", ErrResultTooLarge)
	}
	allowed := make(map[string]struct{}, len(policy.AllowedOrigins))
	for _, raw := range policy.AllowedOrigins {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
			u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
			return nil, fmt.Errorf("configure media result allowlist: %w", ErrUnsafeResultURL)
		}
		allowed[strings.ToLower(u.Host)] = struct{}{}
	}
	resolver := policy.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	d := &SecureDownloader{
		allowed: allowed, resolver: resolver, maxBytes: maxBytes,
		allowTestLoopback: policy.AllowTestLoopbackTLS,
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DisableKeepAlives:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: policy.TLSRootCAs},
		ResponseHeaderTimeout: 30 * time.Second,
		DialContext:           d.dialChecked,
	}
	d.client = &http.Client{
		Transport: transport,
		Timeout:   downloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return ErrUnsafeResultURL
			}
			// net/http otherwise copies the previous signed URL, including
			// its query token, into Referer before invoking this callback.
			req.Header.Del("Referer")
			return d.validateURL(req.URL)
		},
	}
	return d, nil
}

func (d *SecureDownloader) validateURL(u *url.URL) error {
	if u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Opaque != "" || u.Fragment != "" {
		return ErrUnsafeResultURL
	}
	if _, ok := d.allowed[strings.ToLower(u.Host)]; !ok {
		return ErrUnsafeResultURL
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !d.permittedIP(ip, u.Hostname()) {
		return ErrUnsafeResultURL
	}
	return nil
}

func (d *SecureDownloader) dialChecked(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrUnsafeResultURL
	}
	var addresses []net.IPAddr
	if ip := net.ParseIP(host); ip != nil {
		addresses = []net.IPAddr{{IP: ip}}
	} else {
		addresses, err = d.resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve result host: %w", err)
		}
	}
	if len(addresses) == 0 {
		return nil, ErrResultUnavailable
	}
	for _, address := range addresses {
		if !d.permittedIP(address.IP, host) {
			return nil, ErrUnsafeResultURL
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
}

func (d *SecureDownloader) permittedIP(ip net.IP, host string) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if d.allowTestLoopback && addr.IsLoopback() && net.ParseIP(host) != nil {
		return true
	}
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return false
	}
	for _, prefix := range reservedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// Download saves a bounded, checked HTTPS result to a temporary file. Errors
// deliberately omit the provider URL, which may contain signed query tokens.
func (d *SecureDownloader) Download(ctx context.Context, rawURL string) (*Downloaded, error) {
	u, err := url.Parse(rawURL)
	if err != nil || d.validateURL(u) != nil {
		return nil, ErrUnsafeResultURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, ErrUnsafeResultURL
	}
	resp, err := d.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrUnsafeResultURL) {
			return nil, ErrUnsafeResultURL
		}
		// net/http includes the signed URL in url.Error; do not wrap it.
		return nil, ErrResultUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusGone:
			return nil, fmt.Errorf("download media result status %d: %w", resp.StatusCode, ErrResultExpired)
		}
		return nil, fmt.Errorf("download media result status %d: %w", resp.StatusCode, ErrResultUnavailable)
	}
	if resp.ContentLength > d.maxBytes {
		return nil, ErrResultTooLarge
	}
	file, err := os.CreateTemp("", "lanverse-media-result-*")
	if err != nil {
		return nil, fmt.Errorf("create media result temp file: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = file.Close()
			_ = os.Remove(file.Name())
		}
	}()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, d.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("copy media result: %w", err)
	}
	if size > d.maxBytes {
		return nil, ErrResultTooLarge
	}
	if size == 0 {
		return nil, ErrUnsupportedMedia
	}
	var head [512]byte
	count, err := file.ReadAt(head[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("inspect media result: %w", err)
	}
	detected := http.DetectContentType(head[:count])
	if !supportedMIME(detected) {
		return nil, ErrUnsupportedMedia
	}
	declared, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if declared != "" && declared != "application/octet-stream" && declared != detected {
		return nil, ErrUnsupportedMedia
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind media result: %w", err)
	}
	cleanup = false
	return &Downloaded{File: file, Size: size, MIMEType: detected, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func supportedMIME(value string) bool {
	switch value {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "video/mp4", "audio/mpeg", "audio/wave", "audio/x-wav", "audio/ogg":
		return true
	default:
		return false
	}
}
