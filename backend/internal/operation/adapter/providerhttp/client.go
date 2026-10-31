// Package providerhttp implements wire protocols for supported AI model providers.
// It uses strategy drivers for protocol dispatch, never retries a paid submission,
// and disables redirects that could leak API keys.
package providerhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// Client uses one bounded HTTP client without an implicit retry middleware,
// delegating wire execution to protocol strategy drivers.
type Client struct {
	client  http.Client
	drivers map[string]ProtocolDriver
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
		client:  copied,
		drivers: drivers,
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
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(body)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(timeoutCtx, method, config.BaseURL+path, reader)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, response.StatusCode, err
	}
	if int64(len(data)) > limit {
		return nil, response.StatusCode, io.ErrShortBuffer
	}
	return data, response.StatusCode, nil
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
	return application.GenerationResult{Outcome: "not_cancelled", ErrorCode: "cancel_unsupported"}, nil
}

var _ application.GenerationProvider = (*Client)(nil)
