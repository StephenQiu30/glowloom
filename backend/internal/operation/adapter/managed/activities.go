// Package managed executes immutable catalog releases inside the provider worker.
package managed

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// Store is consumed only by the credential-owning worker.
type Store interface {
	application.ManagedResultRepository
	LoadWorkflowOperation(context.Context, uuid.UUID) (application.WorkflowOperation, error)
	ClaimProviderDispatch(context.Context, application.ProviderDispatchIdentity) (application.ProviderDispatchResult, error)
	LoadManagedCredential(context.Context, uuid.UUID) (application.ManagedCredential, error)
	ManagedMediaKey(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (string, error)
}

// Unsealer authenticates a saved credential's connection identity.
type Unsealer interface {
	Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error)
}

// Signer issues a temporary URL for a project-authorized media object.
type Signer interface {
	PresignGet(context.Context, string, time.Duration) (string, error)
}

// SubjectInputSigner signs only an exact frozen operation input through its
// binding owner, including the media's saved physical storage location.
type SubjectInputSigner interface {
	ManagedSubjectMediaReference(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (application.ProviderReference, error)
}

// VideoReader downloads approved content with the original credential when needed.
type VideoReader interface {
	Read(context.Context, catalogdomain.ProviderConfig, string, string, string) ([]byte, int64, error)
}

// ProjectInputSigner preserves exact same-project media and its physical location.
type ProjectInputSigner interface {
	ManagedProjectMediaReference(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (application.ProviderReference, bool, error)
}

// Activities has no environment-based supplier credentials.
type Activities struct {
	store   Store
	keys    Unsealer
	client  application.GenerationProvider
	results *application.ManagedResults
	signer  Signer
	video   VideoReader
}

// NewActivities explicitly injects private persistence, HTTP and media dependencies.
func NewActivities(store Store, keys Unsealer, client application.GenerationProvider, results *application.ManagedResults, signer Signer, video VideoReader) *Activities {
	return &Activities{store: store, keys: keys, client: client, results: results, signer: signer, video: video}
}

// Step never resubmits a claimed operation, even after a timeout or worker crash.
func (a *Activities) Step(ctx context.Context, input application.ManagedStep) (output application.ManagedProgress, resultErr error) {
	defer func() { resultErr = executionAuthorityError(resultErr) }()
	unknown := application.ManagedProgress{State: "unknown"}
	if input.OperationID == uuid.Nil || (input.Action != "submit" && input.Action != "query" && input.Action != "recover") {
		return unknown, application.ErrInvalidProviderCall
	}
	loaded, err := a.store.LoadWorkflowOperation(ctx, input.OperationID)
	if err != nil {
		return unknown, err
	}
	op := loaded.Operation
	if !catalogdomain.IsManagedCloud(loaded.Provider.AdapterKey) || op.ModelProfileVersionID == nil || op.PriceRuleVersionID == nil || op.Status.IsTerminal() {
		return unknown, application.ErrInvalidProviderCall
	}
	identity := application.ProviderDispatchIdentity{ProjectID: op.ProjectID, OperationID: op.ID, ModelProfileVersionID: *op.ModelProfileVersionID, PriceRuleVersionID: *op.PriceRuleVersionID, Action: "submit", Attempt: 1, RequestKey: loaded.ProviderRequestKey}
	saved, err := a.results.Recover(ctx, identity)
	if err != nil {
		return unknown, err
	}
	if saved != nil && (saved.State == "succeeded" || input.Action != "query") {
		return progress(*saved), nil
	}
	if input.Action == "recover" {
		return unknown, nil
	}
	if input.Action == "query" && (saved == nil || saved.TaskID == "") {
		return unknown, nil
	}
	if input.Action == "submit" {
		claim, claimErr := a.store.ClaimProviderDispatch(ctx, identity)
		if errors.Is(claimErr, application.ErrManagedCapacity) {
			return application.ManagedProgress{State: "waiting"}, nil
		}
		if errors.Is(claimErr, application.ErrProviderDispatchCancelled) {
			return application.ManagedProgress{State: "not_submitted", ErrorCode: "provider_dispatch_cancelled"}, nil
		}
		if errors.Is(claimErr, application.ErrWorkflowModelUnavailable) {
			return application.ManagedProgress{State: "not_submitted", ErrorCode: "provider_unavailable"}, nil
		}
		if claimErr != nil {
			return unknown, claimErr
		}
		if !claim.Claimed {
			return unknown, nil
		}
	}
	credential, err := a.store.LoadManagedCredential(ctx, op.ID)
	if err != nil {
		return unknown, err
	}
	var config catalogdomain.ProviderConfig
	if json.Unmarshal(credential.Config, &config) != nil || config.Validate() != nil {
		return unknown, application.ErrInvalidProviderCall
	}
	secret, err := a.keys.Open(credential.KeyID, credential.ProviderID, credential.CredentialID, credential.Ciphertext)
	if err != nil {
		return unknown, application.ErrWorkflowModelUnavailable
	}
	defer clear(secret)
	providerKey, keyErr := config.CredentialValue(secret)
	if keyErr != nil {
		return unknown, application.ErrWorkflowModelUnavailable
	}
	var result application.GenerationResult
	if input.Action == "submit" {
		request, requestErr := a.request(ctx, loaded, config)
		if requestErr != nil {
			return application.ManagedProgress{State: "not_submitted", ErrorCode: "input_unavailable"}, nil
		}
		result, err = a.client.Submit(ctx, request, providerKey)
	} else {
		result, err = a.client.Query(ctx, config, saved.TaskID, providerKey)
	}
	if err != nil {
		return unknown, nil
	}
	switch result.Outcome {
	case "rejected", "not_submitted":
		return application.ManagedProgress{State: result.Outcome, ErrorCode: result.ErrorCode}, nil
	case "unknown":
		return unknown, nil
	case "accepted":
		evidence := application.ManagedResult{Identity: identity, State: "accepted", TaskID: result.TaskID}
		if err := a.results.Save(ctx, evidence); err != nil {
			return unknown, err
		}
		return progress(evidence), nil
	}
	if result.State != "succeeded" {
		switch result.State {
		case "pending", "running", "failed":
			return application.ManagedProgress{State: result.State, TaskID: saved.TaskID, ErrorCode: result.ErrorCode}, nil
		default:
			return unknown, nil
		}
	}
	evidence := application.ManagedResult{Identity: identity, State: "succeeded", TaskID: result.TaskID, Text: result.Text, SupplierUsage: result.Usage, Subtitle: result.Subtitle}
	var duration int64
	for index, item := range result.Images {
		output, e := a.results.StageArtifact(ctx, identity, index+1, item.Bytes, item.MediaType)
		if e != nil {
			return unknown, e
		}
		evidence.Artifacts = append(evidence.Artifacts, output)
	}
	for index, item := range result.Audio {
		if op.Capability != "audio.generate" || len(result.Images) != 0 || result.Text != "" {
			return unknown, application.ErrInvalidProviderCall
		}
		output, e := a.results.StageArtifact(ctx, identity, index+1, item.Bytes, item.MediaType)
		if e != nil {
			return unknown, e
		}
		evidence.Artifacts = append(evidence.Artifacts, output)
	}
	if op.Capability == "audio.generate" && config.AdapterKey == "volcengine_music" {
		reader, ok := a.video.(interface {
			ReadAudio(context.Context, catalogdomain.ProviderConfig, string, string) ([]byte, int64, error)
		})
		if !ok || len(result.URLs) != 1 || len(evidence.Artifacts) != 0 {
			return unknown, application.ErrInvalidProviderCall
		}
		data, ms, e := reader.ReadAudio(ctx, config, result.TaskID, result.URLs[0])
		if e != nil {
			return unknown, e
		}
		duration = ms
		output, e := a.results.StageArtifact(ctx, identity, 1, data, "audio/mpeg")
		if e != nil {
			return unknown, e
		}
		evidence.Artifacts = append(evidence.Artifacts, output)
	}
	if op.Capability == "video.generate" {
		if len(result.URLs) != 1 || a.video == nil {
			return unknown, application.ErrInvalidProviderCall
		}
		data, ms, e := a.video.Read(ctx, config, result.TaskID, result.URLs[0], providerKey)
		if e != nil {
			return unknown, e
		}
		duration = ms
		output, e := a.results.StageArtifact(ctx, identity, 1, data, "video/mp4")
		if e != nil {
			return unknown, e
		}
		evidence.Artifacts = append(evidence.Artifacts, output)
	}
	count := int64(len(evidence.Artifacts))
	if evidence.Text != "" {
		count = 1
	}
	if count != int64(op.OutputCount) {
		return unknown, application.ErrInvalidProviderCall
	}
	usage := map[string]int64{"billable_requests": 1, "output_count": count}
	if config.AdapterKey == "volcengine_music" {
		var receipt struct {
			Duration float64 `json:"duration_seconds"`
		}
		if json.Unmarshal(result.Usage, &receipt) != nil || receipt.Duration <= 0 || receipt.Duration > 240 {
			return unknown, application.ErrInvalidProviderCall
		}
		duration = int64(math.Ceil(receipt.Duration * 1000))
	}
	if config.AdapterKey == "volcengine_speech" && op.Mode == "omni_audio" {
		var receipt struct {
			Duration float64 `json:"original_duration"`
		}
		if json.Unmarshal(result.Usage, &receipt) != nil || receipt.Duration <= 0 || receipt.Duration > 120 {
			return unknown, application.ErrInvalidProviderCall
		}
		duration = int64(math.Ceil(receipt.Duration * 1000))
	}
	if duration > 0 {
		usage["billed_duration_ms"] = duration
	}
	var tokens map[string]json.RawMessage
	if json.Unmarshal(result.Usage, &tokens) == nil {
		if op.Capability == "audio.generate" {
			var characters int64
			if raw, ok := tokens["text_words"]; ok && json.Unmarshal(raw, &characters) == nil && characters >= 0 {
				usage["character_count"] = characters
			}
		}
		for _, pair := range [][2]string{{"prompt_tokens", "input_tokens"}, {"completion_tokens", "output_tokens"}, {"input_tokens", "input_tokens"}, {"output_tokens", "output_tokens"}} {
			var value int64
			if raw, ok := tokens[pair[0]]; ok && json.Unmarshal(raw, &value) == nil && value >= 0 {
				usage[pair[1]] = value
			}
		}
	}
	evidence.Usage, err = json.Marshal(usage)
	if err != nil {
		return unknown, err
	}
	if err := a.results.Save(ctx, evidence); err != nil {
		return unknown, err
	}
	return progress(evidence), nil
}

func progress(r application.ManagedResult) application.ManagedProgress {
	count := len(r.Artifacts)
	if r.Text != "" {
		count = 1
	}
	return application.ManagedProgress{State: r.State, TaskID: r.TaskID, OutputCount: count}
}
func (a *Activities) request(ctx context.Context, loaded application.WorkflowOperation, config catalogdomain.ProviderConfig) (application.GenerationRequest, error) {
	op := loaded.Operation
	request := application.GenerationRequest{Config: config, ModelID: loaded.Provider.ProviderModelID, EndpointTag: loaded.Provider.EndpointTag, Capability: op.Capability, Mode: op.Mode, OutputCount: int(op.OutputCount)}
	if json.Unmarshal(op.Params, &request.Params) != nil {
		return request, application.ErrInvalidProviderCall
	}
	var texts []string
	for _, input := range loaded.Inputs {
		if input.TextValue != nil {
			texts = append(texts, *input.TextValue)
		}
		if input.MediaAssetID != nil {
			if input.RefType == "subject_media" {
				owner, ok := a.store.(SubjectInputSigner)
				if !ok {
					return request, application.ErrWorkflowInputNotReady
				}
				ref, err := owner.ManagedSubjectMediaReference(ctx, op.ID, op.ProjectID, input.ID)
				if err != nil {
					return request, err
				}
				if ref.Role != input.Role || ref.MediaKind == "" {
					return request, application.ErrWorkflowInputNotReady
				}
				request.References = append(request.References, ref)
				continue
			}
			if owner, ok := a.store.(ProjectInputSigner); ok {
				ref, handled, err := owner.ManagedProjectMediaReference(ctx, op.ID, op.ProjectID, input.ID)
				if handled {
					if err != nil {
						return request, err
					}
					if ref.Role != input.Role || ref.MediaKind == "" {
						return request, application.ErrWorkflowInputNotReady
					}
					request.References = append(request.References, ref)
					continue
				}
			}
			var url string
			var err error
			var key string
			key, err = a.store.ManagedMediaKey(ctx, op.ID, op.ProjectID, *input.MediaAssetID)
			if err == nil {
				url, err = a.signer.PresignGet(ctx, key, 15*time.Minute)
			}
			if err != nil {
				return request, err
			}
			request.References = append(request.References, application.ProviderReference{Role: input.Role, URL: url})
		}
		if input.MaskAssetID != nil {
			return request, application.ErrInvalidProviderCall
		}
	}
	request.Prompt = strings.Join(texts, "\n")
	return request, nil
}
