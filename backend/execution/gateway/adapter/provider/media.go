package provider

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
	"slices"
	"strings"
	"sync/atomic"

	catalog "github.com/StephenQiu30/glowloom-server/backend/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
	"github.com/StephenQiu30/glowloom-server/backend/execution/operation/adapter/providerhttp"
	operation "github.com/StephenQiu30/glowloom-server/backend/execution/operation/application"
)

// MediaReferences resolves owned inputs to their exact original bytes or signed URL.
// The credential-owning worker injects ownership and version validation here.
type MediaReferences func(context.Context, application.Dispatch, []domain.Reference) ([]operation.ProviderReference, error)

// WithMediaReferences returns a copy with the trusted reference resolver installed.
func (c *Client) WithMediaReferences(resolve MediaReferences) *Client {
	configured := *c
	configured.mediaReferences = resolve
	return &configured
}

// WithLookup injects DNS resolution while retaining all public-address checks.
func (c *Client) WithLookup(lookup func(context.Context, string, string) ([]netip.Addr, error)) *Client {
	configured := *c
	if lookup != nil {
		configured.lookup = lookup
	}
	return &configured
}

// mediaTransport fences every installed driver's HTTP request, including drivers
// that call http.Client directly. A dispatch can never issue a second request.
type mediaTransport struct {
	fence   fenced
	calls   atomic.Int32
	notSent atomic.Bool
}

func (t *mediaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.calls.Add(1) != 1 {
		t.notSent.Store(false)
		return nil, domain.ErrUnknown
	}
	var connected atomic.Bool
	ctx := httptrace.WithClientTrace(r.Context(), &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }})
	response, err := t.fence.Do(r.WithContext(ctx))
	var networkErr *net.OpError
	if err != nil && !connected.Load() && (errors.Is(err, application.ErrNotSent) || errors.As(err, &networkErr) && networkErr.Op == "dial") {
		t.notSent.Store(true)
	}
	return response, err
}

func (c *Client) sendMedia(ctx context.Context, d application.Dispatch, key string) (domain.Receipt, error) {
	var config catalog.ProviderConfig
	if json.Unmarshal(d.Route.DriverConfig, &config) != nil || config.Validate() != nil || config.BaseURL != d.Route.BaseURL || !slices.Contains([]string{"image.generate", "video.generate", "audio.generate"}, d.Call.Model.Capability) || !slices.Contains(config.Capabilities, strings.TrimSuffix(d.Call.Model.Capability, ".generate")) || !fixedMediaPrice(d.Route.Purchase) || !fixedMediaPrice(d.Call.Model.Price) || d.Call.Stream {
		return domain.Receipt{}, mediaPreflightError(d)
	}
	if _, installed := providerhttp.GetDriver(config.AdapterKey); !installed {
		return domain.Receipt{}, mediaPreflightError(d)
	}
	transport := &mediaTransport{fence: fenced{client: c, route: d.Route, key: key, mediaAdapter: config.AdapterKey}}
	driver := providerhttp.NewClient(&http.Client{Transport: transport})
	var result operation.GenerationResult
	var err error
	if d.PollTask != "" {
		result, err = driver.Query(ctx, config, d.PollTask, key)
	} else {
		input, inputErr := c.mediaRequest(ctx, d, config)
		if inputErr != nil {
			return domain.Receipt{}, application.ErrNotSent
		}
		result, err = driver.Submit(ctx, input, key)
		if transport.calls.Load() == 0 || transport.notSent.Load() {
			return domain.Receipt{}, application.ErrNotSent
		}
	}
	if err != nil {
		return domain.Receipt{}, domain.ErrUnknown
	}
	return mediaReceipt(d, result)
}

func mediaPreflightError(d application.Dispatch) error {
	// A failed poll says nothing about whether the earlier paid submission ran.
	if d.PollTask != "" {
		return domain.ErrUnknown
	}
	return application.ErrNotSent
}

func (c *Client) mediaRequest(ctx context.Context, d application.Dispatch, config catalog.ProviderConfig) (operation.GenerationRequest, error) {
	var in domain.MediaInput
	decoder := json.NewDecoder(bytes.NewReader(d.Call.Raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF || in.Model != d.Call.Model.Name || in.Capability != d.Call.Model.Capability || !slices.Contains(d.Call.Model.Modes, in.Mode) || in.OutputCount != 1 || len(in.References) > 16 {
		return operation.GenerationRequest{}, application.ErrNotSent
	}
	input := operation.GenerationRequest{Config: config, ModelID: d.Route.UpstreamModel, Capability: in.Capability, Mode: in.Mode, Prompt: in.Prompt, Params: in.Params, OutputCount: 1, ProviderRequestID: d.Call.ID.String(), StrictSpeechTerminal: true}
	if len(in.References) > 0 {
		if c.mediaReferences == nil {
			return input, application.ErrNotSent
		}
		var err error
		input.References, err = c.mediaReferences(ctx, d, in.References)
		if err != nil || len(input.References) != len(in.References) {
			return input, application.ErrNotSent
		}
		for i, ref := range input.References {
			if ref.Role != in.References[i].Role || ref.URL == "" && len(ref.ImageBytes) == 0 {
				return input, application.ErrNotSent
			}
		}
	}
	return input, nil
}

func fixedMediaPrice(p domain.Price) bool {
	return p.Validate() == nil && p.Request > 0 && p.Input == 0 && p.CacheRead == 0 && p.CacheWrite == 0 && p.Output == 0
}

func mediaReceipt(d application.Dispatch, result operation.GenerationResult) (domain.Receipt, error) {
	receipt := domain.Receipt{UpstreamID: result.TaskID}
	if d.PollTask != "" && result.TaskID != d.PollTask {
		return receipt, domain.ErrUnknown
	}
	switch result.State {
	case operation.ProviderTaskQueued, operation.ProviderTaskPending, operation.ProviderTaskRunning:
		if result.TaskID == "" || d.PollTask == "" && result.Outcome != operation.ProviderOutcomeAccepted {
			return receipt, domain.ErrUnknown
		}
		receipt.Pending = true
		result.Outcome = operation.ProviderOutcomeAccepted
	case operation.ProviderTaskSucceeded:
		if d.PollTask == "" && result.Outcome != operation.ProviderOutcomeCompleted || len(result.Images)+len(result.Audio)+len(result.URLs) != 1 {
			return receipt, domain.ErrUnknown
		}
		result.Outcome = operation.ProviderOutcomeCompleted
	default:
		return receipt, domain.ErrUnknown
	}
	var err error
	receipt.Body, err = json.Marshal(result)
	if err != nil {
		return receipt, domain.ErrUnknown
	}
	if receipt.UpstreamID == "" {
		receipt.UpstreamID = result.ProviderLogID
	}
	if !receipt.Pending {
		// Media v1 is priced explicitly per request. Provider-native usage is
		// retained in Body for audit and is never fabricated into token usage.
		receipt.PurchaseMicros, err = d.Route.Purchase.Cost(domain.Usage{})
	}
	return receipt, err
}
