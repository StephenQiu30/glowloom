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

var _ app.ReferenceImageExecutableFactory = (*Factory)(nil)

type referenceImageRuntime struct {
	factory *Factory
	config  app.ProviderRuntimeConfig
}

func (factory *Factory) NewReferenceImageRuntime(config app.ProviderRuntimeConfig) (app.ReferenceImageRuntime, error) {
	if factory == nil || factory.client == nil || factory.objects == nil || config.Connection.ProviderKey != domain.MediaProviderOpenAI || config.Connection.AdapterContractVersion != "openai-image-api" || len(config.Connection.ResolvedConfig) != 0 || config.Profile.ProviderKey != domain.MediaProviderOpenAI || config.Profile.Modality != domain.MediaModalityImage || config.Profile.ExternalModelID != "gpt-image-2" || config.Profile.AdapterTransportContract != "openai-image-api-nonstreaming" || len(config.Profile.Defaults) != 0 {
		return nil, errors.New("unsupported Reference image runtime configuration")
	}
	return &referenceImageRuntime{factory: factory, config: config}, nil
}

func (runtime *referenceImageRuntime) Preflight(ctx context.Context, input app.ReferenceImageSubmission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, value := range []string{input.WorkspaceID, input.ProjectID} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil || id.String() != value {
			return errors.New("invalid Reference image scope")
		}
	}
	raw, err := json.Marshal(input.Call)
	if err != nil {
		return errors.New("invalid Reference image call")
	}
	if _, err = domain.DecodeReferenceProviderCall(raw); err != nil {
		return errors.New("invalid Reference image call")
	}
	if input.Request.BundleIndex != input.Call.BundleIndex || input.Request.SlotKey != input.Call.SlotKey || input.Slot.SlotKey != input.Call.SlotKey || input.Slot.ViewRole != input.Slot.SlotKey || !input.Slot.Required || input.Request.ContentHash != input.Call.CompiledRequestHash {
		return errors.New("Reference image slot or request identity differs from call")
	}
	limits := domain.DefaultReferenceGenerationLimits()
	if input.Slot.MaxBytes < 1 || input.Slot.MaxBytes > limits.MaxOutputBytes || validateReferenceImageSize(input.Slot) != nil {
		return errors.New("invalid Reference image output policy")
	}
	// The compiler emits a small canonical body. Reject drift before parsing or IO.
	if len(input.Request.Body) > referenceImageMaxPromptBytes*6+1024 {
		return errors.New("Reference image request exceeds byte budget")
	}
	canonicalBody, err := canonical.JSON(input.Request.Body)
	if err != nil || !bytes.Equal(canonicalBody, input.Request.Body) {
		return errors.New("Reference image request is not canonical")
	}
	hash, err := canonical.Hash(input.Request.Body)
	if err != nil || hash != input.Request.ContentHash {
		return errors.New("Reference image request hash differs from call")
	}
	var body referenceImageBody
	if canonical.Decode(input.Request.Body, &body) != nil || body.Model != runtime.config.Profile.ExternalModelID || strings.TrimSpace(body.Prompt) == "" || len(body.Prompt) > referenceImageMaxPromptBytes || body.Size != fmt.Sprintf("%dx%d", input.Slot.MinWidth, input.Slot.MinHeight) || body.N != 1 || body.Quality != "high" || body.OutputFormat != "png" || body.Stream {
		return errors.New("unsupported Reference image request parameters")
	}
	// Require every compiler field, including explicit stream=false, not just
	// equivalent zero values produced by an omitted field or JSON null.
	raw, err = json.Marshal(body)
	if err != nil {
		return errors.New("invalid Reference image request")
	}
	expected, err := canonical.JSON(raw)
	if err != nil || !bytes.Equal(expected, input.Request.Body) {
		return errors.New("Reference image request fields differ from compiler contract")
	}
	_, err = runtime.apiKey()
	return err
}

func (runtime *referenceImageRuntime) apiKey() (string, error) {
	var credential struct {
		APIKey string `json:"api_key"`
	}
	if len(runtime.config.Credentials) > 64<<10 || canonical.Decode(runtime.config.Credentials, &credential) != nil || credential.APIKey == "" || strings.TrimSpace(credential.APIKey) != credential.APIKey {
		return "", errors.New("invalid Reference image credential")
	}
	for _, c := range credential.APIKey {
		if c < 33 || c > 126 {
			return "", errors.New("invalid Reference image credential")
		}
	}
	return credential.APIKey, nil
}

func (runtime *referenceImageRuntime) Submit(ctx context.Context, input app.ReferenceImageSubmission, dispatch domain.ReferenceCallDispatch) (app.ReferenceImageObservation, error) {
	if err := runtime.Preflight(ctx, input); err != nil {
		return app.ReferenceImageObservation{}, err
	}
	pending, err := domain.NewReferenceCallState(input.Call.CallKey)
	if err != nil {
		return app.ReferenceImageObservation{}, err
	}
	claimed, _, err := domain.ClaimReferenceCall(pending, dispatch)
	if err != nil {
		return app.ReferenceImageObservation{}, err
	}
	// This validates the invocation envelope, not the database send-right commit.
	now := runtime.factory.now().UTC()
	if claimed.Dispatch == nil || now.Before(dispatch.DispatchedAt) || !now.Before(dispatch.DeadlineAt) || dispatch.DeadlineAt.Sub(dispatch.DispatchedAt) != time.Duration(domain.DefaultReferenceGenerationLimits().SubmitTimeoutSeconds)*time.Second {
		return app.ReferenceImageObservation{}, errors.New("invalid Reference image dispatch deadline")
	}
	ctx, cancel := context.WithDeadline(ctx, dispatch.DeadlineAt)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return app.ReferenceImageObservation{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/images/generations", bytes.NewReader(input.Request.Body))
	if err != nil {
		return app.ReferenceImageObservation{}, errors.New("invalid Reference image HTTP request")
	}
	// net/http must not replay a non-idempotent generation body on a reused
	// connection. We deliberately do not set any idempotency or retry header.
	request.GetBody = nil
	key, err := runtime.apiKey()
	if err != nil {
		return app.ReferenceImageObservation{}, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	key = ""
	response, err := runtime.factory.client.Do(request)
	request.Header.Del("Authorization")
	observation := app.ReferenceImageObservation{CallKey: input.Call.CallKey, SubmissionToken: dispatch.SubmissionToken}
	finish := func(status, reason string) (app.ReferenceImageObservation, error) {
		observation.Status, observation.ReasonCode = status, reason
		observation.ObservedAt = runtime.factory.now().UTC().Truncate(time.Microsecond)
		return observation, nil
	}
	if err != nil {
		return finish(app.ReferenceImageOutcomeUnknown, "transport_failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return finish(app.ReferenceImageOutcomeUnknown, "unexpected_http_status")
	}
	// A base64 image plus bounded additive JSON metadata; no URL fetching.
	responseLimit := int64(base64.StdEncoding.EncodedLen(int(input.Slot.MaxBytes))) + 64<<10
	if response.ContentLength > responseLimit {
		return finish(app.ReferenceImageOutputRejected, "response_byte_limit")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil {
		return finish(app.ReferenceImageOutcomeUnknown, "response_read_failed")
	}
	if int64(len(raw)) > responseLimit {
		return finish(app.ReferenceImageOutputRejected, "response_byte_limit")
	}
	contents, usage, reason := decodeReferenceImageResponse(raw, input.Slot)
	if reason != "" {
		return finish(app.ReferenceImageOutputRejected, reason)
	}
	if ctx.Err() != nil {
		return finish(app.ReferenceImageOutcomeUnknown, "dispatch_deadline_reached")
	}
	digest := sha256.Sum256(contents)
	hash := hex.EncodeToString(digest[:])
	objectKey := "staging/reference/" + input.WorkspaceID + "/" + input.ProjectID + "/" + input.Call.ExecutionRef.ID + "/" + input.Call.CallKey + "/" + dispatch.SubmissionToken + "/image.png"
	if err = runtime.factory.objects.EnsurePrivateObject(ctx, objectKey, contents, "image/png", hash); err != nil {
		return finish(app.ReferenceImageOutcomeUnknown, "staging_failed")
	}
	if ctx.Err() != nil {
		return finish(app.ReferenceImageOutcomeUnknown, "dispatch_deadline_reached")
	}
	observation.Output = &domain.ProviderOutput{OutputKey: "image", StagingObjectKey: objectKey, SHA256: hash, MediaType: "image/png", Bytes: int64(len(contents)), Width: input.Slot.MinWidth, Height: input.Slot.MinHeight}
	observation.Usage = usage
	return finish(app.ReferenceImageStaged, "")
}

func decodeReferenceImageResponse(raw []byte, slot domain.ReferenceOutputSlot) ([]byte, domain.ProviderUsageObservation, string) {
	var noUsage domain.ProviderUsageObservation
	if _, err := canonical.JSON(raw); err != nil {
		return nil, noUsage, "invalid_response_json"
	}
	var response struct {
		Data []struct {
			Base64 string `json:"b64_json"`
			URL    string `json:"url"`
		} `json:"data"`
		Usage struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
			Total  int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return nil, noUsage, "invalid_response_json"
	}
	if len(response.Data) != 1 {
		return nil, noUsage, "provider_output_cardinality_mismatch"
	}
	value := response.Data[0]
	if value.URL != "" || value.Base64 == "" || len(value.Base64) > base64.StdEncoding.EncodedLen(int(slot.MaxBytes)) || strings.ContainsAny(value.Base64, "\r\n") {
		return nil, noUsage, "invalid_image_encoding"
	}
	contents, err := base64.StdEncoding.Strict().DecodeString(value.Base64)
	if err != nil || len(contents) == 0 || int64(len(contents)) > slot.MaxBytes {
		return nil, noUsage, "invalid_image_encoding"
	}
	// DecodeConfig bounds allocation before full CRC/deflate/image validation.
	size, err := png.DecodeConfig(bytes.NewReader(contents))
	if err != nil || size.Width != slot.MinWidth || size.Height != slot.MinHeight {
		return nil, noUsage, "image_dimensions_mismatch"
	}
	reader := bytes.NewReader(contents)
	if _, err = png.Decode(reader); err != nil || reader.Len() != 0 {
		return nil, noUsage, "invalid_png_contents"
	}
	if response.Usage.Input < 0 || response.Usage.Output < 0 || response.Usage.Total < 0 {
		return nil, noUsage, "invalid_usage_observation"
	}
	return contents, domain.ProviderUsageObservation{InputTokens: response.Usage.Input, OutputTokens: response.Usage.Output, TotalTokens: response.Usage.Total, ImageCount: 1}, ""
}
