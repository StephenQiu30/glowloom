package httpclient

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"resty.dev/v3"
)

const defaultTimeout = 30 * time.Second

// ErrResponseBodyTooLarge is returned by Resty clients when a response exceeds
// the configured WithResponseBodyLimit bound.
var ErrResponseBodyTooLarge = resty.ErrReadExceedsThresholdLimit

// Option configures an outbound HTTP or Resty client built by this package.
type Option func(*options)

type options struct {
	baseClient        *http.Client
	transport         http.RoundTripper
	timeout           time.Duration
	checkRedirect     func(*http.Request, []*http.Request) error
	responseBodyLimit int64
	retryCount        int
	retryWaitTime     time.Duration
	retryMaxWaitTime  time.Duration
	disableOTel       bool
}

// WithBaseClient clones transport and settings from an existing *http.Client
// (for example, an injected test client) before applying further options.
func WithBaseClient(client *http.Client) Option {
	return func(o *options) { o.baseClient = client }
}

// WithTransport overrides the underlying http.RoundTripper.
func WithTransport(rt http.RoundTripper) Option {
	return func(o *options) { o.transport = rt }
}

// WithTimeout sets the request timeout. Non-positive values keep the default (30s).
func WithTimeout(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.timeout = d
		}
	}
}

// WithNoTimeout disables the whole-request http.Client.Timeout for streaming
// transports (for example SSE), relying on per-request context cancellation and
// transport ResponseHeaderTimeout instead.
func WithNoTimeout() Option {
	return func(o *options) { o.timeout = 0 }
}

// WithCheckRedirect overrides the default RejectRedirects policy.
func WithCheckRedirect(fn func(*http.Request, []*http.Request) error) Option {
	return func(o *options) { o.checkRedirect = fn }
}

// WithResponseBodyLimit sets Resty's maximum buffered response body size in bytes.
func WithResponseBodyLimit(limit int64) Option {
	return func(o *options) { o.responseBodyLimit = limit }
}

// WithRetry enables bounded exponential-backoff retries on Resty clients.
// Callers must only enable retries for idempotent operations; paid AI submissions
// must keep the default RetryCount of 0.
func WithRetry(count int, wait, maxWait time.Duration) Option {
	return func(o *options) {
		o.retryCount = count
		o.retryWaitTime = wait
		o.retryMaxWaitTime = maxWait
	}
}

// WithoutOTel skips wrapping the transport with otelhttp (useful when a caller
// already supplies an instrumented transport).
func WithoutOTel() Option {
	return func(o *options) { o.disableOTel = true }
}

// NewTransport wraps base (or http.DefaultTransport when nil) with OpenTelemetry
// outbound HTTP instrumentation.
func NewTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return otelhttp.NewTransport(base)
}

// NewHTTPClient builds a bounded, redirect-disabled, OTel-instrumented *http.Client
// suitable for official SDKs (OpenAI, Anthropic, COS) and low-level callers.
func NewHTTPClient(opts ...Option) *http.Client {
	cfg := options{
		timeout:       defaultTimeout,
		checkRedirect: RejectRedirects,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	var client http.Client
	if cfg.baseClient != nil {
		client = *cfg.baseClient
	}
	if cfg.transport != nil {
		client.Transport = cfg.transport
	}
	if !cfg.disableOTel {
		client.Transport = NewTransport(client.Transport)
	}
	client.Timeout = cfg.timeout
	if cfg.checkRedirect != nil {
		client.CheckRedirect = cfg.checkRedirect
	}
	return &client
}

// NewResty builds a Resty v3 client backed by NewHTTPClient.
// By default redirects are rejected and automatic retries are disabled (RetryCount=0).
func NewResty(opts ...Option) *resty.Client {
	cfg := options{
		timeout:       defaultTimeout,
		checkRedirect: RejectRedirects,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	hc := NewHTTPClient(opts...)
	rc := resty.NewWithClient(hc)
	rc.SetTimeout(hc.Timeout)
	if cfg.responseBodyLimit > 0 {
		rc.SetResponseBodyLimit(cfg.responseBodyLimit)
	}
	if cfg.retryCount > 0 {
		rc.SetRetryCount(cfg.retryCount)
		if cfg.retryWaitTime > 0 {
			rc.SetRetryWaitTime(cfg.retryWaitTime)
		}
		if cfg.retryMaxWaitTime > 0 {
			rc.SetRetryMaxWaitTime(cfg.retryMaxWaitTime)
		}
	}
	return rc
}
