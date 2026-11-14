package providerhttp

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"time"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ReachabilityResult is address evidence only; no key or model is tested.
type ReachabilityResult struct {
	Result     string `json:"result"`
	ErrorCode  string `json:"error_code,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	ElapsedMS  int64  `json:"elapsed_ms"`
	Attempts   int    `json:"attempts"`
}

// ProbeReachability sends no credentials and reads only response headers. Redirects
// remain unfollowed; only a timeout receives one additional attempt.
func (c *Client) ProbeReachability(ctx context.Context, config catalog.ProviderConfig) ReachabilityResult {
	result := ReachabilityResult{Result: "unsupported"}
	if config.Validate() != nil {
		return result
	}
	for attempt := 1; attempt <= 2; attempt++ {
		result.Attempts = attempt
		bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
		request, err := http.NewRequestWithContext(bounded, http.MethodGet, config.BaseURL, nil)
		if err != nil {
			cancel()
			return result
		}
		start := time.Now()
		response, err := c.do(config, request)
		result.ElapsedMS = time.Since(start).Milliseconds()
		if response != nil {
			_ = response.Body.Close()
		}
		cancel()
		if err == nil && response != nil {
			result.Result = "reachable"
			result.HTTPStatus = response.StatusCode
			result.ErrorCode = ""
			return result
		}
		result.Result = "unreachable"
		result.ErrorCode = "connection_failed"
		var network net.Error
		var dns *net.DNSError
		var cert x509.UnknownAuthorityError
		switch {
		case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout():
			result.ErrorCode = "timeout"
		case errors.As(err, &dns):
			result.ErrorCode = "dns_failed"
		case errors.As(err, &cert):
			result.ErrorCode = "tls_failed"
		}
		if result.ErrorCode != "timeout" || ctx.Err() != nil {
			return result
		}
	}
	return result
}
