package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/google/uuid"
)

// VoiceWorker separates irreversible supplier submission from replayable ingest.
type VoiceWorker struct {
	store     VoiceWorkerStore
	keys      VoiceUnsealer
	speech    VoiceSynthesizer
	artifacts VoiceArtifacts
	output    VoiceOutputPublisher
}

// NewVoiceWorker injects existing credential sealing and actual media publication.
func NewVoiceWorker(store VoiceWorkerStore, keys VoiceUnsealer, speech VoiceSynthesizer, artifacts VoiceArtifacts, output VoiceOutputPublisher) *VoiceWorker {
	return &VoiceWorker{store: store, keys: keys, speech: speech, artifacts: artifacts, output: output}
}

// Run always attempts manifest recovery before considering the one dispatch.
// Once a claim exists no retry path can call the paid synthesizer again.
func (w *VoiceWorker) Run(ctx context.Context, id VoiceWorkID) (VoiceJob, error) {
	if w == nil || w.store == nil || w.keys == nil || w.speech == nil || w.artifacts == nil || w.output == nil || id.JobID == uuid.Nil {
		return VoiceJob{}, ErrVoiceUnavailable
	}
	work, err := w.store.Work(ctx, id.JobID)
	if err != nil {
		return VoiceJob{}, err
	}
	if work.Job.Status == "succeeded" {
		return work.Job, nil
	}
	receipt, found, err := w.artifacts.Read(ctx, work)
	if err != nil {
		return work.Job, err
	}
	if found {
		return w.publish(ctx, work, receipt)
	}
	if work.DispatchStartedAt != nil && work.Job.Status == "dispatching" {
		return w.store.MarkUnknown(ctx, work, "speech_execution_interrupted")
	}
	if work.DispatchStartedAt != nil || work.Job.Status == "unknown" || work.Job.Status == "failed" || id.RecoveryOnly {
		return work.Job, nil
	}
	if preflight, ok := w.artifacts.(interface {
		PrepareSubmission(context.Context, VoiceWork) error
	}); ok {
		if err := preflight.PrepareSubmission(ctx, work); err != nil {
			return work.Job, err
		}
	}
	work, claimed, err := w.store.Claim(ctx, id.JobID, uuid.New())
	if err != nil || !claimed {
		return work.Job, err
	}
	credential, err := w.store.Credential(ctx, work)
	if err != nil {
		return w.store.MarkFailed(ctx, work, "speech_credential_unavailable")
	}
	secret, err := w.keys.Open(credential.KeyID, credential.ProviderID, credential.CredentialID, credential.Ciphertext)
	if err != nil {
		return w.store.MarkFailed(ctx, work, "speech_credential_unavailable")
	}
	defer clear(secret)
	var key struct {
		APIKey string `json:"api_key"`
	}
	decoder := json.NewDecoder(bytes.NewReader(secret))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&key) != nil || decoder.Decode(new(any)) != io.EOF || key.APIKey == "" {
		return w.store.MarkFailed(ctx, work, "speech_credential_unavailable")
	}
	generated, err := w.speech.Synthesize(ctx, VoiceSynthesisInput{Config: credential.Config, RequestID: work.Job.ID, ResourceID: work.ResourceID, Text: work.Job.Text, VoiceKey: work.Job.VoiceKey, SpeechRate: work.SpeechRate}, key.APIKey)
	if err != nil {
		if errors.Is(err, ErrVoiceProviderRejected) {
			return w.store.MarkFailed(ctx, work, "speech_provider_rejected")
		}
		return w.store.MarkUnknown(ctx, work, "speech_execution_unknown")
	}
	receipt, err = w.artifacts.Save(ctx, work, generated)
	if err != nil {
		return work.Job, err
	}
	return w.publish(ctx, work, receipt)
}

func (w *VoiceWorker) publish(ctx context.Context, work VoiceWork, receipt VoiceReceipt) (VoiceJob, error) {
	if err := receipt.Validate(work); err != nil {
		return work.Job, err
	}
	current, err := w.store.RecordReceipt(ctx, work, receipt)
	if err != nil {
		return work.Job, err
	}
	if current.Receipt == nil {
		return current.Job, ErrVoiceExecutionUnknown
	}
	return w.output.Publish(ctx, current, *current.Receipt)
}

// Interrupt records orchestration loss without reopening a paid dispatch. A
// durable success manifest stays ingesting and can be resumed by a new delivery.
func (w *VoiceWorker) Interrupt(ctx context.Context, id VoiceWorkID) (VoiceJob, error) {
	if w == nil || w.store == nil || w.artifacts == nil {
		return VoiceJob{}, ErrVoiceUnavailable
	}
	work, err := w.store.Work(ctx, id.JobID)
	if err != nil {
		return VoiceJob{}, err
	}
	if work.Job.Status == "succeeded" || work.Job.Status == "failed" || work.DispatchStartedAt == nil {
		return work.Job, nil
	}
	receipt, found, err := w.artifacts.Read(ctx, work)
	if err == nil && found {
		if receipt.Validate(work) != nil {
			return w.store.MarkUnknown(ctx, work, "speech_receipt_invalid")
		}
		current, err := w.store.RecordReceipt(ctx, work, receipt)
		return current.Job, err
	}
	return w.store.MarkUnknown(ctx, work, "speech_execution_interrupted")
}
