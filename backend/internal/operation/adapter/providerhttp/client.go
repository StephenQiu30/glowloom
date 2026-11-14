// Package providerhttp implements wire protocols for supported AI model providers.
// It uses strategy drivers for protocol dispatch, never retries a paid submission,
// and disables redirects that could leak API keys.
package providerhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// Client uses one bounded HTTP client without an implicit retry middleware,
// delegating wire execution to protocol strategy drivers.
type Client struct {
	client   http.Client
	drivers  map[string]ProtocolDriver
	lookupIP func(context.Context, string, string) ([]netip.Addr, error)
}

// NewClient copies transport settings and disables redirects that could leak Keys.
func NewClient(client *http.Client) *Client {
	var copied http.Client
	if client != nil {
		copied = *client
	}
	copied.Timeout = 300 * time.Second
	copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	registryMu.RLock()
	drivers := make(map[string]ProtocolDriver, len(defaultRegistry))
	for k, v := range defaultRegistry {
		drivers[k] = v
	}
	registryMu.RUnlock()

	return &Client{
		client:   copied,
		drivers:  drivers,
		lookupIP: net.DefaultResolver.LookupNetIP,
	}
}

// SetDriver allows injecting or overriding a protocol driver for a given adapter key.
func (c *Client) SetDriver(adapterKey string, driver ProtocolDriver) {
	if c.drivers == nil {
		c.drivers = make(map[string]ProtocolDriver)
	}
	c.drivers[adapterKey] = driver
}

func (c *Client) request(ctx context.Context, config catalogdomain.ProviderConfig, key, method, path string, payload any, limit int64) ([]byte, int, error) {
	data, status, _, err := c.requestWithID(ctx, config, key, method, path, payload, limit)
	return data, status, err
}

func (c *Client) requestWithID(ctx context.Context, config catalogdomain.ProviderConfig, key, method, path string, payload any, limit int64) ([]byte, int, string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, "", err
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(body)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(timeoutCtx, method, strings.TrimRight(config.BaseURL, "/")+path, reader)
	if err != nil {
		return nil, 0, "", err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	if config.AdapterKey == "openrouter" {
		request.Header.Set("HTTP-Referer", "https://glowloom.com")
		request.Header.Set("X-Title", "Glowloom")
	}
	var connected atomic.Bool
	if method == http.MethodPost {
		trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }}
		request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	}
	response, err := c.do(config, request)
	if err != nil {
		var networkErr *net.OpError
		if method == http.MethodPost && ctx.Err() == nil && !connected.Load() && errors.As(err, &networkErr) && networkErr.Op == "dial" {
			return nil, 0, "", errDialNotSent
		}
		return nil, 0, "", err
	}
	defer func() { _ = response.Body.Close() }()
	requestID := response.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = response.Header.Get("X-Tt-Logid")
	}
	if !application.ValidProviderRequestID(requestID) {
		requestID = ""
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, response.StatusCode, requestID, err
	}
	if int64(len(data)) > limit {
		return nil, response.StatusCode, requestID, io.ErrShortBuffer
	}
	return data, response.StatusCode, requestID, nil
}

// Submit delegates generation request execution to the registered protocol strategy driver.
func (c *Client) Submit(ctx context.Context, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	driver, ok := c.drivers[input.Config.AdapterKey]
	if !ok {
		return invalidSubmissionResult(), nil
	}
	return driver.Submit(ctx, c, input, key)
}

// Query delegates asynchronous task polling to the registered protocol strategy driver.
func (c *Client) Query(ctx context.Context, config catalogdomain.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	driver, ok := c.drivers[config.AdapterKey]
	if !ok {
		return unknownResult(), nil
	}
	return driver.Query(ctx, c, config, taskID, key)
}

// Cancel deliberately does not map DELETE to cancellation: Ark DELETE may delete
// a completed task after a queued-to-completed race, and OpenRouter has no reviewed
// cancellation contract here. Model configuration must advertise supports_cancel=false.
func (c *Client) Cancel(context.Context, catalogdomain.ProviderConfig, string, string) (application.GenerationResult, error) {
	return application.GenerationResult{Outcome: application.ProviderOutcomeNotCancelled, ErrorCode: "cancel_unsupported"}, nil
}

var _ application.GenerationProvider = (*Client)(nil)

var errDialNotSent = errors.New("provider connection was never established")

func submissionTransportFailure(err error) application.GenerationResult {
	if errors.Is(err, errDialNotSent) {
		return application.GenerationResult{Outcome: application.ProviderOutcomeNotSubmitted, ErrorCode: application.DialNotSent}
	}
	return application.GenerationResult{Outcome: application.ProviderOutcomeUnknown, ErrorCode: "transport_unknown"}
}
