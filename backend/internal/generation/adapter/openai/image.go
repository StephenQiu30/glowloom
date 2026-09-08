package openai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"strings"
	"time"

	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

const maxResponseBytes = 24 << 20

type ObjectWriter interface {
	EnsurePrivateObject(context.Context, string, []byte, string, string) error
}
type Factory struct {
	client  *http.Client
	objects ObjectWriter
	now     func() time.Time
}

func NewFactory(client *http.Client, objects ObjectWriter, now func() time.Time) *Factory {
	c := http.Client{Timeout: 3 * time.Minute}
	if client != nil {
		c = *client
		if c.Timeout <= 0 {
			c.Timeout = 3 * time.Minute
		}
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if now == nil {
		now = time.Now
	}
	return &Factory{client: &c, objects: objects, now: now}
}
func (*Factory) Descriptor() app.MediaFactoryDescriptor {
	return app.MediaFactoryDescriptor{ProviderKey: domain.MediaProviderOpenAI, Modality: domain.MediaModalityImage, AdapterContractVersion: "openai-image-api"}
}
func (f *Factory) NewRuntime(config app.ProviderRuntimeConfig) (app.ProviderGateway, error) {
	if f == nil || f.objects == nil || config.Connection.ProviderKey != domain.MediaProviderOpenAI || config.Connection.AdapterContractVersion != "openai-image-api" || len(config.Connection.ResolvedConfig) != 0 || config.Profile.ProviderKey != domain.MediaProviderOpenAI || config.Profile.Modality != domain.MediaModalityImage || config.Profile.ExternalModelID != "gpt-image-2" || config.Profile.AdapterTransportContract != "openai-image-api-nonstreaming" || len(config.Profile.Defaults) != 0 {
		return nil, errors.New("unsupported openai runtime configuration")
	}
	// Keep only the invocation-scoped byte slice, never credentials in a shared client.
	return &runtime{factory: f, config: config}, nil
}

type runtime struct {
	factory *Factory
	config  app.ProviderRuntimeConfig
}
type imageRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	Size         string `json:"size"`
	N            int    `json:"n"`
	OutputFormat string `json:"output_format"`
}

func (r *runtime) input(s app.ProviderSubmission) (imageRequest, error) {
	if err := domain.ValidateGenerationTarget(s.Target); err != nil {
		return imageRequest{}, fmt.Errorf("invalid formal generation target: %w", err)
	}
	if s.WorkspaceID != s.Target.WorkspaceID || s.ProjectID != s.Target.ProjectID {
		return imageRequest{}, errors.New("generation target scope differs from submission")
	}
	if s.ProviderKey != domain.MediaProviderOpenAI || s.ExternalModelID != r.config.Profile.ExternalModelID || s.RequestedOutputCount != 1 {
		return imageRequest{}, errors.New("invalid openai image submission")
	}
	for _, id := range []string{s.WorkspaceID, s.ProviderJobID, s.ProviderCallID} {
		if _, err := uuid.Parse(id); err != nil {
			return imageRequest{}, errors.New("invalid image staging identity")
		}
	}
	// Formal reference images require the image-edit adapter; never discard them.
	if s.Target.Kind != domain.GenerationTargetReferenceAsset || s.Target.ReferenceAsset == nil || s.Target.ShotFrame != nil {
		return imageRequest{}, errors.New("openai text image adapter does not support this target")
	}
	t := s.Target.ReferenceAsset
	if strings.TrimSpace(t.PositivePrompt) == "" || len(t.PositivePrompt) > 32000 || t.OutputFormat != "PNG" {
		return imageRequest{}, errors.New("unsupported openai image parameters")
	}
	size := fmt.Sprintf("%dx%d", t.Width, t.Height)
	// Explicit initial capability subset, independently of broader upstream limits.
	if size != "1024x1024" && size != "1536x1024" && size != "1024x1536" {
		return imageRequest{}, errors.New("unsupported openai image size")
	}
	return imageRequest{Model: s.ExternalModelID, Prompt: "Generation requirements:\n" + t.PositivePrompt + "\n\nExclude from the generated image:\n" + t.NegativePrompt, Size: size, N: 1, OutputFormat: "png"}, nil
}
func (r *runtime) Preflight(ctx context.Context, s app.ProviderSubmission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := r.input(s); err != nil {
		return err
	}
	var credential struct {
		APIKey string `json:"api_key"`
	}
	if canonical.Decode(r.config.Credentials, &credential) != nil || strings.TrimSpace(credential.APIKey) == "" || strings.ContainsAny(credential.APIKey, "\r\n") {
		return errors.New("invalid openai credential")
	}
	return nil
}
func (r *runtime) Submit(ctx context.Context, s app.ProviderSubmission) (app.ProviderOutcome, error) {
	if err := r.Preflight(ctx, s); err != nil {
		return app.ProviderOutcome{}, err
	}
	input, err := r.input(s)
	if err != nil {
		return app.ProviderOutcome{}, err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return app.ProviderOutcome{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return app.ProviderOutcome{}, err
	}
	var credential struct {
		APIKey string `json:"api_key"`
	}
	if err = canonical.Decode(r.config.Credentials, &credential); err != nil {
		return app.ProviderOutcome{}, errors.New("invalid openai credential")
	}
	request.Header.Set("Authorization", "Bearer "+credential.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := r.factory.client.Do(request)
	request.Header.Del("Authorization")
	credential.APIKey = ""
	if err != nil {
		return app.ProviderOutcome{}, fmt.Errorf("openai image transport: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return app.ProviderOutcome{}, fmt.Errorf("openai image response status %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return app.ProviderOutcome{}, fmt.Errorf("read openai image response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return app.ProviderOutcome{}, errors.New("openai image response exceeds limit")
	}
	// Reject duplicate keys but allow additive upstream fields.
	if _, err = canonical.JSON(raw); err != nil {
		return app.ProviderOutcome{}, errors.New("invalid openai image response")
	}
	var value struct {
		Data []struct {
			Base64 string `json:"b64_json"`
		} `json:"data"`
		Usage struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
			Total  int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &value) != nil || len(value.Data) != 1 {
		return app.ProviderOutcome{}, errors.New("openai image response must contain one output")
	}
	contents, err := base64.StdEncoding.Strict().DecodeString(value.Data[0].Base64)
	if err != nil || len(contents) == 0 || len(contents) > 16<<20 {
		return app.ProviderOutcome{}, errors.New("invalid openai image bytes")
	}
	dimensions, err := png.DecodeConfig(bytes.NewReader(contents))
	if err != nil || dimensions.Width != s.Target.ReferenceAsset.Width || dimensions.Height != s.Target.ReferenceAsset.Height {
		return app.ProviderOutcome{}, errors.New("openai image dimensions differ from target")
	}
	if _, err = png.Decode(bytes.NewReader(contents)); err != nil {
		return app.ProviderOutcome{}, errors.New("invalid openai png contents")
	}
	if value.Usage.Input < 0 || value.Usage.Output < 0 || value.Usage.Total < 0 {
		return app.ProviderOutcome{}, errors.New("invalid openai usage")
	}
	digest := sha256.Sum256(contents)
	hash := hex.EncodeToString(digest[:])
	key := "staging/" + s.WorkspaceID + "/" + s.ProviderJobID + "/" + s.ProviderCallID + "/image.png"
	if err = r.factory.objects.EnsurePrivateObject(ctx, key, contents, "image/png", hash); err != nil {
		return app.ProviderOutcome{}, fmt.Errorf("stage openai image: %w", err)
	}
	return app.ProviderOutcome{Status: app.ProviderOutcomeSucceeded, ProviderEventID: "image-" + s.ProviderCallID, OccurredAt: r.factory.now().UTC(), Output: &domain.ProviderOutput{OutputKey: "image", StagingObjectKey: key, SHA256: hash, MediaType: "image/png", Bytes: int64(len(contents)), Width: dimensions.Width, Height: dimensions.Height}, ProviderUsageObservation: domain.ProviderUsageObservation{InputTokens: value.Usage.Input, OutputTokens: value.Usage.Output, TotalTokens: value.Usage.Total, ImageCount: 1}}, nil
}

type unqueryable struct{}

func (unqueryable) Error() string { return "synchronous image outcome cannot be queried" }
func (unqueryable) ProviderQueryFailureKind() string {
	return app.ProviderQueryFailureIdentityUnrecoverable
}
func (r *runtime) Query(ctx context.Context, _ app.ProviderSubmission) (app.ProviderOutcome, error) {
	if err := ctx.Err(); err != nil {
		return app.ProviderOutcome{}, err
	}
	return app.ProviderOutcome{}, unqueryable{}
}
