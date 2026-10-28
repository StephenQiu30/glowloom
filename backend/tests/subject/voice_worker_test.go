package subject_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

type voiceWorkMemory struct {
	w                subject.VoiceWork
	sends, publishes int
	publishFail      bool
	unknown          bool
}

func (f *voiceWorkMemory) Work(context.Context, uuid.UUID) (subject.VoiceWork, error) {
	return f.w, nil
}
func (f *voiceWorkMemory) Claim(_ context.Context, _ uuid.UUID, e uuid.UUID) (subject.VoiceWork, bool, error) {
	if f.w.DispatchStartedAt != nil {
		return f.w, false, nil
	}
	now := time.Now()
	f.w.DispatchStartedAt = &now
	f.w.ExecutionID = e
	f.w.Job.Status = "dispatching"
	return f.w, true, nil
}
func (f *voiceWorkMemory) Credential(context.Context, subject.VoiceWork) (subject.VoiceCredential, error) {
	return subject.VoiceCredential{}, nil
}
func (f *voiceWorkMemory) RecordReceipt(_ context.Context, _ subject.VoiceWork, r subject.VoiceReceipt) (subject.VoiceWork, error) {
	if r.Validate(f.w) != nil {
		return f.w, subject.ErrVoiceExecutionUnknown
	}
	f.w.Receipt = &r
	f.w.Job.Status = "ingesting"
	return f.w, nil
}
func (f *voiceWorkMemory) MarkUnknown(context.Context, subject.VoiceWork, string) (subject.VoiceJob, error) {
	f.w.Job.Status = "unknown"
	return f.w.Job, nil
}
func (f *voiceWorkMemory) MarkFailed(context.Context, subject.VoiceWork, string) (subject.VoiceJob, error) {
	f.w.Job.Status = "failed"
	return f.w.Job, nil
}
func (f *voiceWorkMemory) Synthesize(_ context.Context, in subject.VoiceSynthesisInput, _ string) (subject.VoiceSynthesisResult, error) {
	f.sends++
	if in.RequestID != f.w.Job.ID {
		panic("unstable supplier request identity")
	}
	if f.unknown {
		return subject.VoiceSynthesisResult{}, subject.ErrVoiceExecutionUnknown
	}
	return subject.VoiceSynthesisResult{Audio: []byte("synthetic provider codec test"), MIMEType: "audio/mpeg", TerminalCode: 20000000}, nil
}
func (f *voiceWorkMemory) Save(_ context.Context, w subject.VoiceWork, r subject.VoiceSynthesisResult) (subject.VoiceReceipt, error) {
	digest := sha256.Sum256(r.Audio)
	return subject.VoiceReceipt{SchemaVersion: 1, JobID: w.Job.ID, ActorID: w.Job.CurrentActorID, OwnerScopeID: w.Owner.ScopeID, PersonalOrgID: w.Owner.PersonalOrgID, AssetID: w.AssetID, ReleaseID: w.Job.ReleaseID, ModelVersionID: w.Job.ModelVersionID, ProviderRequestID: w.Job.ID, RequestSHA256: w.RequestSHA256, AudioLocationID: uuid.New(), AudioKey: subject.VoiceAudioKey(w), AudioSHA256: hex.EncodeToString(digest[:]), AudioByteSize: int64(len(r.Audio)), MIMEType: r.MIMEType, TerminalCode: r.TerminalCode}, nil
}
func (f *voiceWorkMemory) Read(_ context.Context, w subject.VoiceWork) (subject.VoiceReceipt, bool, error) {
	if w.Receipt == nil {
		return subject.VoiceReceipt{}, false, nil
	}
	return *w.Receipt, true, nil
}
func (f *voiceWorkMemory) Publish(context.Context, subject.VoiceWork, subject.VoiceReceipt) (subject.VoiceJob, error) {
	f.publishes++
	if f.publishFail {
		return f.w.Job, errors.New("temporary media ingest unavailable")
	}
	f.w.Job.Status = "succeeded"
	return f.w.Job, nil
}

type voiceTestKeys struct{}

func (voiceTestKeys) Open(string, uuid.UUID, uuid.UUID, []byte) ([]byte, error) {
	return []byte(`{"api_key":"synthetic-test-only"}`), nil
}
func voiceMemory() *voiceWorkMemory {
	return &voiceWorkMemory{w: subject.VoiceWork{Job: subject.VoiceJob{ID: uuid.New(), CurrentActorID: uuid.New(), ReleaseID: uuid.New(), ModelVersionID: uuid.New(), CreatedAt: time.Now().UTC(), Status: "queued"}, Owner: subject.VoiceOwner{ScopeID: uuid.New(), PersonalOrgID: uuid.New()}, AssetID: uuid.New(), RequestSHA256: "frozen-request", ResourceID: "seed-tts-2.0"}}
}
func TestVoiceWorkerRecoversIngestWithoutSecondPaidSubmission(t *testing.T) {
	f := voiceMemory()
	f.publishFail = true
	worker := subject.NewVoiceWorker(f, voiceTestKeys{}, f, f, f)
	id := subject.VoiceWorkID{JobID: f.w.Job.ID}
	if _, err := worker.Run(t.Context(), id); err == nil || f.w.Job.Status != "ingesting" || f.sends != 1 {
		t.Fatal("success receipt must survive local ingest failure", err)
	}
	f.publishFail = false
	id.RecoveryOnly = true
	job, err := worker.Run(t.Context(), id)
	if err != nil || job.Status != "succeeded" || f.sends != 1 || f.publishes != 2 {
		t.Fatal("same-job import recovery repeated supplier submission", err)
	}
	if _, err := worker.Run(t.Context(), id); err != nil || f.sends != 1 || f.publishes != 2 {
		t.Fatal("successful replay performed work", err)
	}
}
func TestVoiceWorkerUnknownAndRecoveryOnlyNeverResubmit(t *testing.T) {
	f := voiceMemory()
	f.unknown = true
	worker := subject.NewVoiceWorker(f, voiceTestKeys{}, f, f, f)
	id := subject.VoiceWorkID{JobID: f.w.Job.ID}
	job, err := worker.Run(t.Context(), id)
	if err != nil || job.Status != "unknown" || f.sends != 1 {
		t.Fatal("unknown not durable", err)
	}
	for range 3 {
		if _, err := worker.Run(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	if f.sends != 1 || f.publishes != 0 {
		t.Fatal("unknown submission repeated")
	}
	fresh := voiceMemory()
	worker = subject.NewVoiceWorker(fresh, voiceTestKeys{}, fresh, fresh, fresh)
	if _, err := worker.Run(t.Context(), subject.VoiceWorkID{JobID: fresh.w.Job.ID, RecoveryOnly: true}); err != nil || fresh.sends != 0 {
		t.Fatal("recovery-only delivery submitted", err)
	}
}

func TestVoiceReceiptSurvivesJSONBKeyNormalization(t *testing.T) {
	f := voiceMemory()
	r, err := f.Save(t.Context(), f.w, subject.VoiceSynthesisResult{Audio: []byte("test"), MIMEType: "audio/mpeg", TerminalCode: 20000000})
	if err != nil {
		t.Fatal(err)
	}
	r.Usage = []byte(`{"text_words":12,"a":1}`)
	before := r.Digest()
	r.Usage = []byte(`{ "a": 1, "text_words": 12 }`)
	if before == "" || before != r.Digest() {
		t.Fatal("PostgreSQL jsonb normalization changed immutable receipt identity")
	}
	r.ProviderLogID = "private\u0085control"
	if r.Validate(f.w) == nil {
		t.Fatal("provider control character entered receipt")
	}
}

type voiceBlockedOutput struct{ *voiceWorkMemory }

func (voiceBlockedOutput) PrepareSubmission(context.Context, subject.VoiceWork) error {
	return subject.ErrVoiceUnavailable
}
func TestVoiceWorkerRejectsMissingOutputRouteBeforePaidClaim(t *testing.T) {
	f := voiceMemory()
	worker := subject.NewVoiceWorker(f, voiceTestKeys{}, f, voiceBlockedOutput{f}, f)
	if _, err := worker.Run(t.Context(), subject.VoiceWorkID{JobID: f.w.Job.ID}); !errors.Is(err, subject.ErrVoiceUnavailable) || f.sends != 0 || f.w.DispatchStartedAt != nil {
		t.Fatal("unavailable output route consumed paid dispatch", err)
	}
}
