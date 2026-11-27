// Package provider executes native protocols through official SDKs and a new network fence.
package provider

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go/v2"
	ao "github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/google/uuid"
	openai "github.com/openai/openai-go/v3"
	oo "github.com/openai/openai-go/v3/option"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpclient"
)

// Keys is the independently injectable credential cryptography boundary.
type Keys interface {
	Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error)
}

// Client owns timeouts and address verification. The injected transport supports isolated tests.
type Client struct {
	keys            Keys
	client          http.Client
	lookup          func(context.Context, string, string) ([]netip.Addr, error)
	mediaReferences MediaReferences
}

// New copies the HTTP settings and enforces bounded requests without redirects.
func New(keys Keys, client *http.Client) *Client {
	c := httpclient.NewHTTPClient(
		httpclient.WithBaseClient(client),
		httpclient.WithTimeout(180*time.Second),
		httpclient.WithoutOTel(),
	)
	return &Client{keys: keys, client: *c, lookup: net.DefaultResolver.LookupNetIP}
}

func public(a netip.Addr) bool {
	return httpclient.IsPublicAddr(a)
}

type fenced struct {
	client       *Client
	route        domain.Route
	key          string
	mediaAdapter string
}

func (f fenced) Do(r *http.Request) (*http.Response, error) {
	base, err := url.Parse(f.route.BaseURL)
	if err != nil || base.Scheme != "https" || base.Host == "" || r.URL.Scheme != "https" || r.URL.User != nil || r.URL.Opaque != "" || r.URL.Fragment != "" {
		return nil, application.ErrNotSent
	}
	// The old custom gateway driver may already pin the URL to a public IP.
	// Only its exact original route Host can authorize that representation.
	pinned := r.URL.Host != base.Host
	if r.Host != "" && r.Host != base.Host || pinned && (f.mediaAdapter == "" || r.Host != base.Host || effectivePort(r.URL) != effectivePort(base)) {
		return nil, application.ErrNotSent
	}
	addresses, err := f.client.lookup(r.Context(), "ip", base.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, application.ErrNotSent
	}
	for _, a := range addresses {
		if !public(a) {
			return nil, application.ErrNotSent
		}
	}
	if pinned {
		address, err := netip.ParseAddr(r.URL.Hostname())
		matched := false
		for _, allowed := range addresses {
			matched = matched || allowed.Unmap() == address.Unmap()
		}
		if err != nil || !public(address) || !matched {
			return nil, application.ErrNotSent
		}
	}
	clone := r.Clone(r.Context())
	clone.Header = http.Header{"Content-Type": []string{"application/json"}, "Accept": []string{"application/json, text/event-stream"}}
	switch {
	case f.mediaAdapter == "volcengine_speech":
		clone.Header.Set("X-Api-Key", f.key)
		for _, name := range []string{"X-Api-Resource-Id", "X-Api-Request-Id", "X-Control-Require-Usage-Tokens-Return"} {
			if value := r.Header.Get(name); value != "" {
				clone.Header.Set(name, value)
			}
		}
	case f.mediaAdapter == "volcengine_music":
		// These headers were signed by the installed music driver for this
		// immutable, validated endpoint. No caller headers are forwarded.
		for _, name := range []string{"Authorization", "X-Date", "X-Content-Sha256"} {
			clone.Header.Set(name, r.Header.Get(name))
		}
	case f.mediaAdapter == "vercel" && r.URL.Path == "/v4/ai/image-model":
		clone.Header.Set("Authorization", "Bearer "+f.key)
		clone.Header.Set("ai-gateway-protocol-version", "0.0.1")
		clone.Header.Set("ai-gateway-auth-method", "api-key")
		clone.Header.Set("ai-image-model-specification-version", "4")
		clone.Header.Set("ai-model-id", f.route.UpstreamModel)
	case f.route.Protocol == "messages":
		clone.Header.Set("X-Api-Key", f.key)
		clone.Header.Set("Anthropic-Version", "2023-06-01")
	default:
		clone.Header.Set("Authorization", "Bearer "+f.key)
	}
	clone.Host = base.Host
	port := effectivePort(base)
	clone.URL.Host = net.JoinHostPort(addresses[0].Unmap().String(), port)
	c := f.client.client
	transport := c.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if original, ok := transport.(*http.Transport); ok {
		bounded := original.Clone()
		bounded.Proxy = nil
		bounded.DisableKeepAlives = true
		if bounded.TLSClientConfig == nil {
			bounded.TLSClientConfig = &tls.Config{}
		}
		{
			// HTTP/2 may replay REFUSED_STREAM/GOAWAY inside one RoundTrip.
			// Fresh HTTP/1 connections keep both submissions and queries single-send.
			bounded.Protocols = &http.Protocols{}
			bounded.Protocols.SetHTTP1(true)
			bounded.ForceAttemptHTTP2 = false
			bounded.TLSNextProto = nil
			bounded.TLSClientConfig.NextProtos = []string{"http/1.1"}
		}
		bounded.TLSClientConfig.ServerName = base.Hostname()
		c.Transport = bounded
		defer bounded.CloseIdleConnections()
	}
	res, err := c.Do(clone)
	if res != nil && res.Body != nil {
		res.Body = &boundedBody{ReadCloser: res.Body, left: 16 << 20}
	}
	return res, err
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	return "443"
}

// Event retains a protocol-native SSE event and its exact data fragment.
type Event struct {
	Event string `json:"event,omitempty"`
	Data  string `json:"data"`
}

// Send never changes model, key, protocol or route after dispatch begins.
func (c *Client) Send(ctx context.Context, d application.Dispatch, emit func(json.RawMessage) error) (domain.Receipt, error) {
	if c.keys == nil || d.Route.Protocol != d.Call.Model.Protocol {
		return domain.Receipt{}, domain.ErrInvalid
	}
	secret, err := c.keys.Open(d.Route.KeyID, d.Route.Supplier, d.Route.Credential, d.Route.Ciphertext)
	if err != nil {
		return domain.Receipt{}, mediaPreflightError(d)
	}
	defer clear(secret)
	var envelope struct {
		APIKey string `json:"api_key"`
	}
	if json.Unmarshal(secret, &envelope) != nil || envelope.APIKey == "" || strings.ContainsAny(envelope.APIKey, "\r\n\x00") {
		return domain.Receipt{}, mediaPreflightError(d)
	}
	if d.Route.Protocol == domain.EmbeddingProtocol {
		return c.sendEmbedding(ctx, d, envelope.APIKey)
	}
	if d.Route.Protocol == "media" {
		return c.sendMedia(ctx, d, envelope.APIKey)
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(d.Call.Raw, &payload) != nil {
		return domain.Receipt{}, application.ErrNotSent
	}
	payload["model"], _ = json.Marshal(d.Route.UpstreamModel)
	if tariff := d.Route.Purchase.TextTariff; tariff != nil && tariff.VercelProvider != "" {
		if !d.Route.AcceptsTextTariffRoute() {
			return domain.Receipt{}, application.ErrNotSent
		}
		payload["providerOptions"], _ = json.Marshal(map[string]any{
			"gateway": map[string]any{"only": []string{tariff.VercelProvider}},
		})
	}
	if d.Call.Stream && d.Route.Protocol == "chat" {
		payload["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	}
	raw, _ := json.Marshal(payload)
	base := strings.TrimRight(d.Route.BaseURL, "/") + "/"
	path := "chat/completions"
	if d.Route.Protocol == "responses" {
		path = "responses"
	}
	if d.Route.Protocol == "messages" {
		path = "messages"
	}
	var connected atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }})
	var response *http.Response
	startedAt := time.Now().UTC()
	if !d.Route.Purchase.AvailableAt(startedAt) || !d.Route.Purchase.AvailableAt(d.Call.Deadline) {
		return domain.Receipt{}, application.ErrNotSent
	}
	if d.Route.Protocol == "messages" {
		sdk := anthropic.NewClient(ao.WithAPIKey(envelope.APIKey), ao.WithBaseURL(base), ao.WithMaxRetries(0), ao.WithHTTPClient(fenced{client: c, route: d.Route, key: envelope.APIKey}))
		err = sdk.Post(ctx, path, json.RawMessage(raw), &response)
	} else {
		sdk := openai.NewClient(oo.WithAPIKey(envelope.APIKey), oo.WithBaseURL(base), oo.WithMaxRetries(0), oo.WithHTTPClient(fenced{client: c, route: d.Route, key: envelope.APIKey}))
		err = sdk.Post(ctx, path, json.RawMessage(raw), &response)
	}
	if err != nil || response == nil {
		var op *net.OpError
		if !connected.Load() && (errors.Is(err, application.ErrNotSent) || errors.As(err, &op) && op.Op == "dial") {
			return domain.Receipt{}, application.ErrNotSent
		}
		return domain.Receipt{}, domain.ErrUnknown
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.Receipt{}, domain.ErrUnknown
	}
	receipt := domain.Receipt{UpstreamID: response.Header.Get("X-Request-Id")}
	if !d.Call.Stream {
		raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20+1))
		if err != nil || len(raw) > 16<<20 {
			return receipt, domain.ErrUnknown
		}
		if err := verifyResponse(d.Route.Protocol, raw, d.Route.UpstreamModel); err != nil {
			return receipt, err
		}
		usage, err := Usage(d.Route.Protocol, raw)
		if err != nil {
			return receipt, err
		}
		receipt.Body = raw
		receipt.Usage = usage
	} else {
		var used int
		scan := bufio.NewScanner(response.Body)
		scan.Buffer(make([]byte, 4096), 1<<20)
		event := Event{}
		evidence := newEvidence(d.Route.Protocol, d.Route.UpstreamModel)
		state := streamState{protocol: d.Route.Protocol, expected: d.Route.UpstreamModel}
		flush := func() error {
			if event.Data == "" {
				event = Event{}
				return nil
			}
			used += len(event.Data)
			if used > 16<<20 {
				return domain.ErrUnknown
			}
			raw, _ := json.Marshal(event)
			receipt.Events = append(receipt.Events, json.RawMessage(raw))
			if err := evidence.consume(event); err != nil {
				return err
			}
			if err := state.consume(event); err != nil {
				return err
			}
			// Terminal events are persisted and settled before being emitted by the HTTP boundary.
			if !terminal(event) && emit != nil {
				if err := emit(raw); err != nil {
					return err
				}
			}
			event = Event{}
			return nil
		}
		for scan.Scan() {
			line := scan.Text()
			if line == "" {
				if err := flush(); err != nil {
					return receipt, domain.ErrUnknown
				}
				continue
			}
			if strings.HasPrefix(line, "event:") {
				event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			}
			if strings.HasPrefix(line, "data:") {
				if event.Data != "" {
					event.Data += "\n"
				}
				event.Data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		if scan.Err() != nil || flush() != nil || !state.complete || !state.known || !state.modelKnown {
			return receipt, domain.ErrUnknown
		}
		receipt.Usage = state.usage
		receipt.Body, err = evidence.response()
		if err != nil {
			return receipt, err
		}
	}
	if err := verifyToolNames(d.Route.Protocol, d.Call.Raw, receipt.Body); err != nil {
		return receipt, err
	}
	if receipt.Usage.Input > d.Call.Model.MaxInput || receipt.Usage.Output > d.Call.Model.MaxOutput {
		return receipt, domain.ErrUnknown
	}
	if routeTariff, saleTariff := d.Route.Purchase.TextTariff, d.Call.Model.Price.TextTariff; routeTariff != nil && routeTariff.Schedule != "" || saleTariff != nil && saleTariff.Schedule != "" {
		receipt.Usage.BillingStartedAt = startedAt
		receipt.Usage.BillingReceivedAt = time.Now().UTC()
		if _, err := d.Call.Model.Price.Cost(receipt.Usage); err != nil {
			return receipt, err
		}
	}
	receipt.PurchaseMicros, err = d.Route.Purchase.Cost(receipt.Usage)
	if err != nil {
		return receipt, err
	}
	return receipt, nil
}
func terminal(e Event) bool {
	return e.Data == "[DONE]" || e.Event == "message_stop" || e.Event == "response.completed" || e.Event == "response.incomplete"
}

// Terminal exposes the persistence barrier to a protocol-native HTTP writer.
func Terminal(raw json.RawMessage) bool {
	var e Event
	return json.Unmarshal(raw, &e) == nil && terminal(e)
}

// boundedBody also limits SDK error-body reads before application parsing.
type boundedBody struct {
	io.ReadCloser
	left int64
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.left < 0 {
		return 0, io.ErrShortBuffer
	}
	if int64(len(p)) > b.left+1 {
		p = p[:b.left+1]
	}
	n, err := b.ReadCloser.Read(p)
	b.left -= int64(n)
	if b.left < 0 {
		return n, io.ErrShortBuffer
	}
	return n, err
}
