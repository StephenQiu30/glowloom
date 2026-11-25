package providerhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go/v2"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/v2/option"
	openai "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
)

type textDoer struct {
	key      string
	client   *Client
	config   catalog.ProviderConfig
	limit    int64
	progress func()
}

// Do preserves the entire address/DNS/redirect fence, not just its Transport.
func (d textDoer) Do(r *http.Request) (*http.Response, error) {
	r.Header = http.Header{"Content-Type": []string{"application/json"}, "Accept": []string{"application/json"}}
	if d.config.AdapterKey == "anthropic" {
		r.Header.Set("X-Api-Key", d.key)
		r.Header.Set("Anthropic-Version", "2023-06-01")
	} else {
		r.Header.Set("Authorization", "Bearer "+d.key)
	}
	if d.config.AdapterKey == "openrouter" {
		r.Header.Set("HTTP-Referer", "https://glowloom.com")
		r.Header.Set("X-Title", "Glowloom")
	}
	response, err := d.client.do(d.config, r)
	if err != nil {
		return nil, err
	}
	if d.progress != nil {
		d.progress()
	}
	response.Body = &boundedTextBody{ReadCloser: response.Body, remaining: d.limit, progress: d.progress}
	return response, nil
}

type boundedTextBody struct {
	io.ReadCloser
	remaining int64
	progress  func()
}

func (b *boundedTextBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, io.ErrShortBuffer
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	if n > 0 && b.progress != nil {
		b.progress()
	}
	if b.remaining < 0 {
		return n, io.ErrShortBuffer
	}
	return n, err
}

// TextRequest executes one native protocol request with the official SDK. The
// operation owner has already selected the route, budget and credential. No SDK
// retry, redirect, environment-selected endpoint or implicit protocol conversion
// may change that identity. Raw JSON retains reviewed provider-specific fields.
func (c *Client) TextRequest(ctx context.Context, config catalog.ProviderConfig, key, path string, payload any, limit int64, progress func()) ([]byte, int, string, error) {
	if config.Validate() != nil || !validKey(key) || limit < 1 || (path != "/chat/completions" && path != "/responses" && path != "/messages") {
		return nil, 0, "", errors.New("invalid text protocol request")
	}
	if (config.AdapterKey == "anthropic") != (path == "/messages") {
		return nil, 0, "", errors.New("unsupported native protocol")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, "", err
	}
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var connected atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }})
	var response *http.Response
	doer := textDoer{key: key, client: c, config: config, limit: limit, progress: progress}
	base := strings.TrimRight(config.BaseURL, "/") + "/"
	if path == "/messages" {
		sdk := anthropic.NewClient(anthropicoption.WithBaseURL(base), anthropicoption.WithAPIKey(key), anthropicoption.WithMaxRetries(0), anthropicoption.WithHTTPClient(doer))
		err = sdk.Post(ctx, strings.TrimPrefix(path, "/"), json.RawMessage(body), &response)
	} else {
		sdk := openai.NewClient(openaioption.WithBaseURL(base), openaioption.WithAPIKey(key), openaioption.WithMaxRetries(0), openaioption.WithHTTPClient(doer))
		err = sdk.Post(ctx, strings.TrimPrefix(path, "/"), json.RawMessage(body), &response)
	}
	if response == nil {
		var networkErr *net.OpError
		if ctx.Err() == nil && !connected.Load() && errors.As(err, &networkErr) && networkErr.Op == "dial" {
			err = errDialNotSent
		}
		return nil, 0, "", err
	}
	defer func() { _ = response.Body.Close() }()
	id := response.Header.Get("X-Request-Id")
	if id == "" {
		id = response.Header.Get("X-Tt-Logid")
	}
	if !application.ValidProviderRequestID(id) {
		id = ""
	}
	// SDK error bodies are bounded by textDoer too. Preserve status/error JSON for
	// existing outcome classification; an HTTP error never authorizes a resend.
	if err != nil && response.StatusCode < 400 {
		return nil, response.StatusCode, id, err
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if readErr != nil {
		return nil, response.StatusCode, id, readErr
	}
	if int64(len(data)) > limit {
		return nil, response.StatusCode, id, io.ErrShortBuffer
	}
	return data, response.StatusCode, id, nil
}
