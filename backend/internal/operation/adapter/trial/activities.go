package trial

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	_ "image/jpeg" // Register formats accepted by development image trials.
	_ "image/png"
	"io"
	"time"

	_ "golang.org/x/image/webp" // Register the accepted WebP decoder.

	"go.temporal.io/sdk/activity"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialseal"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/managed"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// Bindings rechecks current authority before exposing a frozen provider binding.
type Bindings interface {
	Authorize(context.Context, identityapp.Principal) error
	Binding(context.Context, application.TrialInput) (Binding, error)
}

// Results stores and reads verified private experiment output.
type Results interface {
	PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error
	Stat(context.Context, string) (objectstorage.ObjectInfo, error)
	Get(context.Context, string) (io.ReadCloser, error)
}

// Runner performs one bounded dispatch or a read-only recovery.
type Runner struct {
	StorageID string
	Store     Bindings
	Objects   Results
	Launcher  codex.Launcher
	WorkRoot  string
	Keys      *credentialseal.Unsealer
	Cloud     application.GenerationProvider
}

// Run rechecks authority and never retries a generation submission.
func (r *Runner) Run(ctx context.Context, input application.TrialInput) (application.TrialProgress, error) {
	result := application.TrialProgress{State: "failed", ErrorCode: "trial_unavailable", TaskID: input.TaskID, ConfigVersionID: input.ConfigVersionID, CredentialID: input.CredentialID}
	if input.Action == "recover" || input.Action == "query" {
		result.State = "unknown"
	}
	if input.Request.Validate() != nil || r.Store.Authorize(ctx, input.Actor) != nil {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 9*time.Minute)
	defer cancel()
	if activity.IsActivity(ctx) {
		done := make(chan struct{})
		finished := make(chan struct{})
		defer func() { close(done); <-finished }()
		go func() {
			defer close(finished)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					activity.RecordHeartbeat(ctx)
				}
			}
		}()
	}
	if input.Action == "recover" {
		info, e := r.Objects.Stat(ctx, application.TrialObjectKey(input, 1))
		if e == nil && info.Size > 0 && len(info.SHA256) == 64 && (info.ContentType == "image/png" || info.ContentType == "image/jpeg" || info.ContentType == "image/webp" || info.ContentType == "video/mp4") {
			return application.TrialProgress{State: "succeeded", Artifacts: []application.TrialArtifact{{Sequence: 1, MIME: info.ContentType, Size: info.Size, SHA256: info.SHA256, StorageID: r.StorageID}}}, nil
		}
		if input.Request.Kind != "video" || input.TaskID == "" {
			return application.TrialProgress{State: "unknown", ErrorCode: "trial_result_unavailable"}, nil
		}
		input.Action = "query"
	}
	if input.Request.Source == "codex" {
		return r.local(ctx, input), nil
	}
	binding, err := r.Store.Binding(ctx, input)
	if err != nil || r.Keys == nil {
		return result, nil
	}
	result.ConfigVersionID = binding.ConfigID
	result.CredentialID = binding.CredentialID
	raw, err := r.Keys.Open(binding.KeyID, input.Request.ProviderID, binding.CredentialID, binding.Ciphertext)
	if err != nil {
		return result, nil
	}
	defer clear(raw)
	var key struct {
		APIKey string `json:"api_key"`
	}
	if json.Unmarshal(raw, &key) != nil {
		return result, nil
	}
	if r.Store.Authorize(ctx, input.Actor) != nil {
		return result, nil
	}
	var generated application.GenerationResult
	if input.Action == "query" {
		generated, err = r.Cloud.Query(ctx, binding.Config, input.TaskID, key.APIKey)
	} else {
		mode := input.Request.Mode
		if mode == "" {
			mode = "text2image"
			if input.Request.Kind == "video" {
				mode = "text2video"
			}
		}
		generated, err = r.Cloud.Submit(ctx, application.GenerationRequest{Config: binding.Config, ModelID: input.Request.Model, Capability: input.Request.Kind + ".generate", Mode: mode, Prompt: input.Request.Prompt, Params: input.Request.Params, References: input.Request.References, OutputCount: 1}, key.APIKey)
	}
	if err != nil {
		result.State = "unknown"
		return result, nil
	}
	result.TaskID = generated.TaskID
	if result.TaskID == "" {
		result.TaskID = input.TaskID
	}
	result.ErrorCode = generated.ErrorCode
	switch {
	case generated.Outcome == "accepted" || generated.State == "pending" || generated.State == "running":
		result.State = "pending"
	case generated.Outcome == "unknown" || generated.State == "unknown" || generated.State == "not_found":
		result.State = "unknown"
		if input.Action == "query" && generated.ErrorCode == "query_unreachable" {
			result.State = "pending"
		}
	case generated.Outcome == "completed" || generated.State == "succeeded":
		var data []byte
		var mime string
		switch {
		case len(generated.Images) == 1:
			data = generated.Images[0].Bytes
			mime = generated.Images[0].MediaType
			err = validateImage(data, mime)
		case len(generated.URLs) == 1:
			data, _, err = managed.NewVideos().Read(ctx, binding.Config, result.TaskID, generated.URLs[0], key.APIKey)
			mime = "video/mp4"
		default:
			err = application.ErrInvalidProviderCall
		}
		if err != nil || len(data) == 0 {
			result.State = "unknown"
			result.ErrorCode = "trial_result_unavailable"
			break
		}
		artifact, e := r.save(ctx, input, data, mime)
		if e != nil {
			result.State = "unknown"
			result.ErrorCode = "trial_transfer_unconfirmed"
			break
		}
		result.State = "succeeded"
		result.Artifacts = []application.TrialArtifact{artifact}
	default:
		result.State = "failed"
	}
	return result, nil
}
func (r *Runner) local(ctx context.Context, input application.TrialInput) application.TrialProgress {
	result := application.TrialProgress{State: "failed", ErrorCode: "codex_unavailable"}
	if r.Launcher == nil {
		return result
	}
	req := input.Request
	if req.Kind == "probe" {
		status, err := codex.Probe(ctx, r.Launcher, r.WorkRoot)
		if err != nil {
			return result
		}
		return application.TrialProgress{State: "succeeded", Models: status.Models, Authenticated: status.Authenticated, ImageGeneration: status.ImageGeneration}
	}
	if req.Kind == "chat" {
		var session *codex.ChatSession
		var err error
		if input.ThreadID == "" {
			session, err = codex.OpenChat(ctx, r.Launcher, r.WorkRoot, req.Model)
		} else {
			session, err = codex.ResumeChat(ctx, r.Launcher, r.WorkRoot, req.Model, input.ThreadID)
		}
		if err != nil {
			return result
		}
		defer func() { _ = session.Close() }()
		generated, err := session.Turn(ctx, req.Prompt, nil)
		if err != nil {
			return application.TrialProgress{State: "unknown", ErrorCode: "codex_turn_unknown"}
		}
		state := "failed"
		switch generated.Outcome {
		case "completed":
			state = "succeeded"
		case "unknown":
			state = "unknown"
		}
		encodedText, _ := json.Marshal(generated.Text)
		if len(encodedText) > 64*1024 {
			return application.TrialProgress{State: "unknown", ErrorCode: "trial_response_too_large"}
		}
		return application.TrialProgress{State: state, Text: generated.Text, ThreadID: session.ThreadID(), ErrorCode: generated.ErrorCode}
	}
	image, outcome, err := codex.TrialImage(ctx, r.Launcher, r.WorkRoot, req.Model, req.Prompt)
	if err != nil {
		if outcome == "unknown" {
			result.State = "unknown"
		}
		return result
	}
	artifact, err := r.save(ctx, input, image.Bytes, image.MediaType)
	if err != nil {
		return application.TrialProgress{State: "unknown", ErrorCode: "trial_transfer_unconfirmed"}
	}
	return application.TrialProgress{State: "succeeded", Artifacts: []application.TrialArtifact{artifact}}
}
func (r *Runner) save(ctx context.Context, input application.TrialInput, data []byte, mime string) (application.TrialArtifact, error) {
	h := sha256.Sum256(data)
	digest := hex.EncodeToString(h[:])
	artifact := application.TrialArtifact{Sequence: 1, StorageID: r.StorageID, MIME: mime, Size: int64(len(data)), SHA256: digest}
	key := application.TrialObjectKey(input, 1)
	if err := r.Objects.PutIfAbsent(ctx, key, bytes.NewReader(data), int64(len(data)), mime, digest); err != nil {
		// A transfer retry may read an already committed immutable result, never generate again.
		info, e := r.Objects.Stat(ctx, key)
		if e != nil || info.Size != artifact.Size || info.SHA256 != digest || info.ContentType != mime {
			return artifact, err
		}
	}
	return artifact, nil
}

// ReadArtifact resolves only a persisted result selected by workflow identity.
func ReadArtifact(ctx context.Context, objects Results, input application.TrialInput, artifact application.TrialArtifact) (io.ReadCloser, error) {
	key := application.TrialObjectKey(input, artifact.Sequence)
	info, err := objects.Stat(ctx, key)
	if err != nil {
		return nil, err
	}
	if info.Size != artifact.Size || info.SHA256 != artifact.SHA256 || info.ContentType != artifact.MIME {
		return nil, application.ErrInvalidTrial
	}
	return objects.Get(ctx, key)
}

func validateImage(data []byte, mime string) error {
	if len(data) == 0 || len(data) > 32<<20 {
		return application.ErrInvalidTrial
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return application.ErrInvalidTrial
	}
	if map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"}[format] != mime {
		return application.ErrInvalidTrial
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return application.ErrInvalidTrial
	}
	return nil
}
