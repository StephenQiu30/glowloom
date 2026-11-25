package workflow

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	stdhtml "html"
	"io"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// VideoLinkResolverOptions changes only trusted process composition. Source
// origin overrides require literal loopback TLS and are never enabled by HTTP.
type VideoLinkResolverOptions struct {
	SourceOrigins        []string
	Resolver             IPResolver
	TLSRootCAs           *x509.CertPool
	AllowTestLoopbackTLS bool
}

// PublicVideoLinkResolver reads bounded public metadata and verified video bytes.
type PublicVideoLinkResolver struct {
	pages   *SecureDownloader
	options VideoLinkResolverOptions
}

// NewPublicVideoLinkResolver fixes public share origins to the three platforms.
func NewPublicVideoLinkResolver(options VideoLinkResolverOptions) (*PublicVideoLinkResolver, error) {
	origins := []string{"https://douyin.com", "https://www.douyin.com", "https://v.douyin.com", "https://iesdouyin.com", "https://www.iesdouyin.com", "https://toutiao.com", "https://www.toutiao.com", "https://m.toutiao.com", "https://v.toutiao.com", "https://ixigua.com", "https://www.ixigua.com", "https://m.ixigua.com", "https://v.ixigua.com"}
	if len(options.SourceOrigins) > 0 {
		if !options.AllowTestLoopbackTLS {
			return nil, ErrUnsafeResultURL
		}
		for _, origin := range options.SourceOrigins {
			u, err := url.Parse(origin)
			if err != nil || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
				return nil, ErrUnsafeResultURL
			}
		}
		origins = options.SourceOrigins
	}
	pages, err := NewSecureDownloader(DownloadPolicy{AllowedOrigins: origins, Resolver: options.Resolver, TLSRootCAs: options.TLSRootCAs, MaxBytes: 2 << 20, AllowTestLoopbackTLS: options.AllowTestLoopbackTLS})
	if err != nil {
		return nil, err
	}
	return &PublicVideoLinkResolver{pages: pages, options: options}, nil
}

// Resolve reads public metadata without executing site JavaScript or supplying
// login cookies. Missing/guarded pages remain explicit failures.
func (r *PublicVideoLinkResolver) Resolve(ctx context.Context, raw string, maxBytes int64) (*application.Downloaded, error) {
	if r == nil || r.pages == nil || maxBytes < 1 || maxBytes > application.MaxUploadVideoBytes {
		return nil, application.ErrInvalidVideoLink
	}
	if !r.options.AllowTestLoopbackTLS {
		canonical, err := application.CanonicalVideoLinkURL(raw)
		if err != nil {
			return nil, err
		}
		raw = canonical
	}
	body, base, err := r.pages.ReadPublicPage(ctx, raw)
	if err != nil {
		return nil, videoLinkError(err)
	}
	candidates := publicVideoURLs(body)
	if len(candidates) == 0 {
		return nil, application.ErrVideoLinkUnresolvable
	}
	last := application.ErrVideoLinkUnresolvable
	for _, candidate := range candidates {
		u, err := url.Parse(candidate)
		if err != nil {
			continue
		}
		u = base.ResolveReference(u)
		if u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Hostname() == "" || !r.options.AllowTestLoopbackTLS && u.Port() != "" && u.Port() != "443" {
			continue
		}
		download, err := NewSecureDownloader(DownloadPolicy{AllowedOrigins: []string{"https://" + u.Host}, Resolver: r.options.Resolver, TLSRootCAs: r.options.TLSRootCAs, MaxBytes: maxBytes, AllowTestLoopbackTLS: r.options.AllowTestLoopbackTLS})
		if err != nil {
			last = videoLinkError(err)
			continue
		}
		file, err := download.Download(ctx, u.String())
		if err != nil {
			last = videoLinkError(err)
			continue
		}
		if file.MIMEType != "video/mp4" {
			_ = file.Close()
			continue
		}
		// Formal UploadPersonal repeats the actual ffprobe/container/duration and
		// publication checks. The resolver never returns a preview URL as an asset.
		return file, nil
	}
	return nil, last
}

func videoLinkError(err error) error {
	switch {
	case errors.Is(err, ErrUnsafeResultURL):
		return application.ErrInvalidVideoLink
	case errors.Is(err, ErrResultTooLarge):
		return application.ErrUploadTooLarge
	case errors.Is(err, ErrResultExpired), errors.Is(err, ErrUnsupportedMedia):
		return application.ErrVideoLinkUnresolvable
	default:
		return application.ErrVideoLinkUnavailable
	}
}

func publicVideoURLs(body []byte) []string {
	urls := []string{}
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw != "" && len(raw) <= 8192 && !seen[raw] && len(urls) < 4 {
			seen[raw] = true
			urls = append(urls, raw)
		}
	}
	var walk func(any, int)
	walk = func(value any, depth int) {
		if depth > 64 || len(urls) >= 4 {
			return
		}
		switch v := value.(type) {
		case map[string]any:
			if play, ok := v["play_addr"].(map[string]any); ok {
				if list, ok := play["url_list"].([]any); ok {
					for _, raw := range list {
						if text, ok := raw.(string); ok {
							add(text)
						}
					}
				}
			}
			for _, child := range v {
				walk(child, depth+1)
			}
		case []any:
			for _, child := range v {
				walk(child, depth+1)
			}
		}
	}
	z := html.NewTokenizer(strings.NewReader(string(body)))
	script := false
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				break
			}
			return urls
		}
		token := z.Token()
		switch kind {
		case html.StartTagToken, html.SelfClosingTagToken:
			if token.Data == "meta" {
				name, content := "", ""
				for _, attr := range token.Attr {
					switch strings.ToLower(attr.Key) {
					case "property", "name":
						name = strings.ToLower(attr.Val)
					case "content":
						content = attr.Val
					}
				}
				switch name {
				case "og:video", "og:video:url", "og:video:secure_url", "twitter:player:stream":
					add(content)
				}
			}
			script = token.Data == "script"
		case html.EndTagToken:
			if token.Data == "script" {
				script = false
			}
		case html.TextToken:
			if script {
				text := strings.TrimSpace(stdhtml.UnescapeString(token.Data))
				if decoded, err := url.QueryUnescape(text); err == nil && strings.Contains(decoded, "play_addr") {
					text = decoded
				}
				start, end := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
				if start >= 0 && end >= start {
					var value any
					if json.Unmarshal([]byte(text[start:end+1]), &value) == nil {
						walk(value, 0)
					}
				}
			}
		}
	}
	return urls
}
