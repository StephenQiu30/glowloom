package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg" // Register JPEG dimension decoding.
	_ "image/png"  // Register PNG dimension decoding.
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"slices"
	"strings"
	"sync/atomic"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/execution/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/adapter/providerhttp"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
)

// MediaReferences resolves owned inputs to their exact original bytes or signed URL.
// The credential-owning worker injects ownership and version validation here.
type MediaReferences func(context.Context, application.Dispatch, []domain.Reference) ([]operation.ProviderReference, error)

// MediaDriverOption describes non-secret settings for an installed media driver.
type MediaDriverOption struct {
	Key        string              `json:"key"`
	Name       string              `json:"name"`
	BaseURL    string              `json:"base_url"`
	Region     catalog.Region      `json:"region"`
	Parameters map[string][]string `json:"parameters"`
	CustomURL  bool                `json:"custom_url"`
}

// MediaDriverOptions projects the catalog used by media dispatch into the admin form.
func MediaDriverOptions() []MediaDriverOption {
	options := []MediaDriverOption{}
	for _, spec := range catalog.AdapterSpecifications() {
		if _, ok := providerhttp.GetDriver(spec.AdapterKey); !ok || spec.AllowedBaseURL == "" {
			continue
		}
		parameters := make(map[string][]string)
		for _, capability := range []string{"image", "video", "audio"} {
			if slices.Contains(spec.Capabilities, capability) {
				key := capability + ".generate"
				parameters[key] = append([]string{}, spec.ParameterFields[key]...)
			}
		}
		if len(parameters) > 0 {
			options = append(options, MediaDriverOption{
				Key: spec.AdapterKey, Name: spec.DisplayName, BaseURL: spec.AllowedBaseURL,
				Region: spec.AllowedRegion, Parameters: parameters, CustomURL: spec.AdapterKey == "openai",
			})
		}
	}
	return options
}

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
	model := d.Call.Model
	if json.Unmarshal(d.Route.DriverConfig, &config) != nil || config.Validate() != nil || config.BaseURL != d.Route.BaseURL || !slices.Contains([]string{"image.generate", "video.generate", "audio.generate"}, model.Capability) || !slices.Contains(config.Capabilities, strings.TrimSuffix(model.Capability, ".generate")) || !model.AcceptsMediaPrice(d.Route.Purchase) || !model.AcceptsMediaPrice(model.Price) ||
		model.VideoMetering != nil && config.AdapterKey != "volcengine_ark" || d.Call.Stream {
		return domain.Receipt{}, mediaPreflightError(d)
	}
	if model.Capability == "audio.generate" && slices.Contains(model.Modes, "text2audio") && config.AdapterKey != "volcengine_speech" {
		return domain.Receipt{}, mediaPreflightError(d)
	}
	if model.Price.MediaTariff != nil && !model.Price.MediaTariff.MatchesAdapter(config.AdapterKey) {
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
		if tariff := model.Price.MediaTariff; tariff != nil && tariff.Meter == domain.ReportedImageAmount && tariff.Adapter == "vercel" {
			result, err = submitVercelImage(ctx, transport, input)
		} else {
			result, err = driver.Submit(ctx, input, key)
		}
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
	// The installed Ark driver forces adaptive for this native model/mode.
	// Refuse a conflicting confirmed ratio before resolving or sending inputs.
	if config.AdapterKey == "volcengine_ark" && strings.HasPrefix(d.Route.UpstreamModel, "doubao-seedance-2-5-") && in.Capability == "video.generate" && in.Mode == "image2video" {
		for _, name := range []string{"ratio", "aspect_ratio"} {
			if value, present := in.Params[name]; present && value != "adaptive" {
				return operation.GenerationRequest{}, application.ErrNotSent
			}
		}
	}
	input := operation.GenerationRequest{Config: config, ModelID: d.Route.UpstreamModel, Capability: in.Capability, Mode: in.Mode, Prompt: in.Prompt, Params: in.Params, OutputCount: 1, ProviderRequestID: d.Call.ID.String(), StrictSpeechTerminal: true, RequireSpeechTerminalUsage: d.Call.Model.Price.MediaTariff != nil && d.Call.Model.Price.MediaTariff.Meter == domain.SpeechCharacters}
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
			if d.Call.Model.VideoMetering != nil && ref.MediaKind != "image" {
				return input, application.ErrNotSent
			}
		}
	}
	return input, nil
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
	case operation.ProviderTaskFailed, operation.ProviderTaskCancelled, operation.ProviderTaskExpired:
		if !d.Call.Model.UsesVideoDeliveryMeter() || d.PollTask == "" {
			return receipt, domain.ErrUnknown
		}
		receipt.DeliveryFailed = true
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
		if d.Call.Model.UsesOutputVideoDuration() {
			receipt.PurchasePending = true
			return receipt, nil
		}
		if d.Call.Model.VideoMetering != nil {
			var usage struct {
				CompletionTokens int64 `json:"completion_tokens"`
			}
			if json.Unmarshal(result.Usage, &usage) == nil {
				receipt.Usage = domain.Usage{Output: usage.CompletionTokens}
			}
			if !receipt.Usage.IsVideoUsage() {
				receipt.Usage = domain.Usage{}
				receipt.PurchasePending = true
				return receipt, nil
			}
		}
		if tariff := d.Call.Model.Price.MediaTariff; tariff != nil {
			if tariff.UsesAuthorizedCeiling() {
				usage, actualErr := imageUsage(tariff.Meter, result.Usage)
				if actualErr != nil {
					receipt.PurchasePending = true
					return receipt, nil
				}
				receipt.Usage = usage
			}
			var usage map[string]json.RawMessage
			if !tariff.UsesAuthorizedCeiling() && tariff.Meter != domain.ArkImagePixels && json.Unmarshal(result.Usage, &usage) != nil {
				return receipt, domain.ErrUnknown
			}
			switch tariff.Meter {
			case domain.ArkImagePixels:
				var intent domain.MediaInput
				if len(result.Images) != 1 || json.Unmarshal(d.Call.Raw, &intent) != nil {
					return receipt, domain.ErrUnknown
				}
				config, _, err := image.DecodeConfig(bytes.NewReader(result.Images[0].Bytes))
				if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 32768 || config.Height > 32768 {
					return receipt, domain.ErrUnknown
				}
				receipt.Usage.Pixels = int64(config.Width) * int64(config.Height)
				receipt.Usage.ReferenceImages = int64(len(intent.References))
			case domain.SpeechCharacters:
				if result.ProviderTerminalCode != 20000000 || json.Unmarshal(usage["text_words"], &receipt.Usage.Characters) != nil {
					return receipt, domain.ErrUnknown
				}
			case domain.OriginalAudioSeconds:
				seconds, err := operation.ParseAudioDurationSeconds(usage["original_duration"], tariff.MaxQuantity)
				if err != nil {
					return receipt, domain.ErrUnknown
				}
				receipt.Usage.OriginalSeconds = seconds
			}
		}
		receipt.PurchaseMicros, err = d.Route.Purchase.Cost(receipt.Usage)
		if err == nil && d.Call.Model.Price.MediaTariff != nil {
			_, err = d.Call.Model.Price.Cost(receipt.Usage)
		}
		if err != nil && d.Call.Model.Price.MediaTariff != nil && d.Call.Model.Price.MediaTariff.UsesAuthorizedCeiling() {
			receipt.Usage = domain.Usage{}
			receipt.PurchasePending = true
			receipt.PurchaseMicros = 0
			return receipt, nil
		}
	}
	return receipt, err
}
