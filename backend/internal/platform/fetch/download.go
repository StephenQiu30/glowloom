// Package fetch implements bounded HTTPS file transfer with pinned DNS and explicit caller policy.
package fetch

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

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpclient"
)

const (
	maxResultBytes  int64 = 2_000_000_000
	maxRedirects          = 3
	downloadTimeout       = 10 * time.Minute
)

var (
	// ErrUnsafeResultURL rejects an origin, redirect or address outside explicit policy.
	ErrUnsafeResultURL = errors.New("unsafe media result URL")
	// ErrResultTooLarge rejects a file exceeding the injected byte bound.
	ErrResultTooLarge = errors.New("media result is too large")
	// ErrUnsupportedMedia rejects a detected MIME outside the caller's explicit set.
	ErrUnsupportedMedia = errors.New("unsupported media")
	// ErrResultUnavailable means no usable remote response was established.
	ErrResultUnavailable = errors.New("media result unavailable")
	// ErrResultExpired means a remote signed result is inaccessible.
	ErrResultExpired = errors.New("media result URL expired or inaccessible")
)

// IPResolver is a small DNS port; production uses net.DefaultResolver.
type IPResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// DownloadPolicy supplies exact origins, detected MIME types and resource limits.
// Empty AcceptedMIME disables binary downloads while permitting ReadPublicPage.
type DownloadPolicy struct {
	AllowedOrigins       []string
	AcceptedMIME         []string
	Resolver             IPResolver
	TLSRootCAs           *x509.CertPool
	MaxBytes             int64
	AllowTestLoopbackTLS bool
}

// Downloaded owns one bounded temporary byte source and its independently computed SHA.
type Downloaded struct {
	File             *os.File
	Size             int64
	MIMEType, SHA256 string
}

// Close closes and removes the owned file; repeated calls are safe.
func (d *Downloaded) Close() error {
	if d == nil || d.File == nil {
		return nil
	}
	name := d.File.Name()
	closeErr := d.File.Close()
	removeErr := os.Remove(name)
	d.File = nil
	return errors.Join(closeErr, removeErr)
}

// SecureDownloader pins each connection to already checked public IP addresses.
type SecureDownloader struct {
	allowed, acceptedMIME map[string]struct{}
	resolver              IPResolver
	maxBytes              int64
	allowTestLoopback     bool
	client                *http.Client
}

// NewSecureDownloader validates explicit policy without reading routes or credentials.
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
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
			return nil, fmt.Errorf("configure media result allowlist: %w", ErrUnsafeResultURL)
		}
		allowed[strings.ToLower(u.Host)] = struct{}{}
	}
	accepted := make(map[string]struct{}, len(policy.AcceptedMIME))
	for _, value := range policy.AcceptedMIME {
		parsed, _, err := mime.ParseMediaType(value)
		if err != nil || parsed != value {
			return nil, ErrUnsupportedMedia
		}
		accepted[value] = struct{}{}
	}
	resolver := policy.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	d := &SecureDownloader{allowed: allowed, acceptedMIME: accepted, resolver: resolver, maxBytes: maxBytes, allowTestLoopback: policy.AllowTestLoopbackTLS}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: policy.TLSRootCAs}, ResponseHeaderTimeout: 30 * time.Second, DialContext: d.dialChecked}
	d.client = httpclient.NewHTTPClient(
		httpclient.WithTransport(transport),
		httpclient.WithTimeout(downloadTimeout),
		httpclient.WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return ErrUnsafeResultURL
			}
			req.Header.Del("Referer")
			return d.validateURL(req.URL)
		}),
	)
	return d, nil
}

func (d *SecureDownloader) validateURL(u *url.URL) error {
	if u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
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
	return httpclient.IsPublicAddr(addr)
}

// Download verifies the final body, declared MIME, byte bound and actual SHA.
// Error messages omit remote URLs, which may contain signed query credentials.
func (d *SecureDownloader) Download(ctx context.Context, rawURL string) (*Downloaded, error) {
	return d.download(ctx, rawURL, "")
}

// DownloadAuthenticated uses checked DNS/TLS and never follows a redirect carrying credentials.
func (d *SecureDownloader) DownloadAuthenticated(ctx context.Context, rawURL, authorization string) (*Downloaded, error) {
	if authorization == "" || len(authorization) > 16384 || strings.ContainsAny(authorization, "\r\n") {
		return nil, ErrUnsafeResultURL
	}
	return d.download(ctx, rawURL, authorization)
}

func (d *SecureDownloader) download(ctx context.Context, rawURL, authorization string) (*Downloaded, error) {
	u, err := url.Parse(rawURL)
	if err != nil || d.validateURL(u) != nil {
		return nil, ErrUnsafeResultURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, ErrUnsafeResultURL
	}
	client := d.client
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
		copyClient := *d.client
		copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &copyClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, ErrUnsafeResultURL) {
			return nil, ErrUnsafeResultURL
		}
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
	file, err := os.CreateTemp("", "glowloom-media-result-*")
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
	if _, ok := d.acceptedMIME[detected]; !ok {
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

// ReadPublicPage uses the same pinned transport with a separate bounded HTML envelope.
func (d *SecureDownloader) ReadPublicPage(ctx context.Context, rawURL string) ([]byte, *url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil || d.validateURL(u) != nil {
		return nil, nil, ErrUnsafeResultURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, ErrUnsafeResultURL
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := d.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrUnsafeResultURL) {
			return nil, nil, ErrUnsafeResultURL
		}
		return nil, nil, ErrResultUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusGone:
			return nil, nil, ErrResultExpired
		}
		return nil, nil, ErrResultUnavailable
	}
	limit := min(d.maxBytes, int64(2<<20))
	if resp.ContentLength > limit {
		return nil, nil, ErrResultTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, nil, ErrResultUnavailable
	}
	if len(body) == 0 || int64(len(body)) > limit {
		return nil, nil, ErrResultTooLarge
	}
	declared, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if declared != "" && declared != "text/html" && declared != "application/xhtml+xml" {
		return nil, nil, ErrUnsupportedMedia
	}
	return body, resp.Request.URL, nil
}
