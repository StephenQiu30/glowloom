// Package runtimeclient implements the authenticated HTTP boundary to the independent Agent runtime.
package runtimeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client only accesses operation paths generated from the TypeBox contract.
type Client struct {
	origin *url.URL
	token  string
	http   *http.Client
}

// New creates a runtime client with the supplied origin and direction credential.
func New(origin, token string) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(token) < 32 {
		return nil, fmt.Errorf("invalid Agent service configuration")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost = 32
	transport.ResponseHeaderTimeout = 15 * time.Second
	return &Client{origin: u, token: token, http: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Request sends a bounded authenticated request to a named runtime operation.
func (c *Client) Request(ctx context.Context, operation string, params map[string]string, query url.Values, body any, lastEventID string) (*http.Response, error) {
	spec, ok := OperationPaths[operation]
	if !ok {
		return nil, fmt.Errorf("unknown Agent operation %q", operation)
	}
	method, path, _ := strings.Cut(spec, " ")
	for name, value := range params {
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(value))
	}
	if strings.Contains(path, "{") {
		return nil, fmt.Errorf("missing Agent operation parameters")
	}
	var input io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		input = bytes.NewReader(b)
	}
	endpoint := *c.origin
	endpoint.Path = strings.TrimRight(c.origin.Path, "/") + path
	endpoint.RawQuery = query.Encode()
	requestCtx := ctx
	cancel := func() {}
	if operation != "subscribeAgentEvents" {
		requestCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
	}
	req, err := http.NewRequestWithContext(requestCtx, method, endpoint.String(), input)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Agent-Audience", "agent")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	response.Body = &cancelBody{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error { defer b.cancel(); return b.ReadCloser.Close() }
