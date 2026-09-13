package agent_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	app "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
)

func visionExecutionFixture(t *testing.T) (*app.VisionReviewExecutionService, *visionExecutionStore, *visionExecutionRuntime, app.ExecuteVisionReviewCommand, *time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	invocation, images := visionInvocationWithImages(t)
	release, err := app.BuildStageReleaseRecord(contract.VisionReviewStageKey, invocation.StageRelease.AgentImageDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	input := invocation.Payload.StageInput
	input.Subject.StageReleaseHash = release.Identity.StageReleaseHash
	input, err = contract.BuildVisionReviewInput(input)
	if err != nil {
		t.Fatal(err)
	}
	command := app.ExecuteVisionReviewCommand{
		WorkflowRunID: "00000000-0000-4000-8000-000000000001",
		NodeRunID:     "00000000-0000-4000-8000-000000000002",
		UserID:        "00000000-0000-4000-8000-000000000003", TokenVersion: 1, Input: input,
	}
	signer, err := grant.NewSigner("synthetic-vision-execution-secret-not-a-credential", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	store := &visionExecutionStore{release: release.InitialControl}
	runtime := &visionExecutionRuntime{t: t, store: store, images: images}
	service, err := app.NewVisionReviewExecutionService(store, runtime, runtime, signer,
		app.VisionReviewExecutionConfig{Now: func() time.Time { return now }, AgentImageDigest: invocation.StageRelease.AgentImageDigest})
	if err != nil {
		t.Fatal(err)
	}
	return service, store, runtime, command, &now
}

func TestVisionReviewExecutionCommitsBeforeCallingAndReplaysWithoutSending(t *testing.T) {
	service, _, runtime, command, _ := visionExecutionFixture(t)
	got, err := service.Execute(context.Background(), command)
	if err != nil || got.Status != "accepted" || runtime.calls != 1 || runtime.loads != 1 {
		t.Fatalf("review execution: status=%s calls=%d loads=%d err=%v", got.Status, runtime.calls, runtime.loads, err)
	}
	repeated, err := service.Execute(context.Background(), command)
	if err != nil || !reflect.DeepEqual(got, repeated) || runtime.calls != 1 || runtime.loads != 1 {
		t.Fatalf("persisted review replay drifted: %v", err)
	}
	for _, image := range runtime.lastBuffers {
		if strings.Trim(string(image), "\x00") != "" {
			t.Fatal("owned media buffer not cleared")
		}
	}
}

func TestVisionReviewExecutionFailuresNeverResend(t *testing.T) {
	for _, fault := range []string{"preflight_commit", "dispatch_commit", "media", "media_changed", "stale_after_media", "revoked_after_media", "invoke", "invalid_result", "cancel_after_dispatch", "result_commit", "stale_after_invoke"} {
		t.Run(fault, func(t *testing.T) {
			service, store, runtime, command, now := visionExecutionFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch fault {
			case "preflight_commit":
				store.failCommit = true
			case "dispatch_commit":
				runtime.afterLoad = func() { store.failCommit = true }
			case "media":
				runtime.loadErr = errors.New("object unavailable")
			case "media_changed":
				runtime.afterLoad = func() { runtime.lastBuffers[0][0] ^= 1 }
			case "stale_after_media":
				runtime.afterLoad = func() { store.stale = true }
			case "revoked_after_media":
				runtime.afterLoad = func() { store.release.Status = "revoked" }
			case "invoke":
				runtime.invokeErr = errors.New("response lost")
			case "invalid_result":
				runtime.invalidResult = true
			case "cancel_after_dispatch":
				runtime.afterInvoke = cancel
			case "result_commit":
				runtime.afterInvoke = func() { store.failCommit = true }
			case "stale_after_invoke":
				runtime.afterInvoke = func() { store.stale = true }
			}
			got, err := service.Execute(ctx, command)
			if fault == "invoke" || fault == "invalid_result" {
				if err != nil || got.Status != "outcome_unknown" || got.Candidate.ID != "" {
					t.Fatalf("unconfirmed result accepted: %s %v", got.Status, err)
				}
			} else if err == nil {
				t.Fatal("fault did not fail closed")
			}
			sent := fault == "invoke" || fault == "invalid_result" || fault == "cancel_after_dispatch" || fault == "result_commit" || fault == "stale_after_invoke"
			if !sent && (runtime.calls != 0 || store.state.Status != "") {
				t.Fatal("failed preparation acquired dispatch rights")
			}
			if sent {
				store.failCommit, store.stale = false, false
				*now = now.Add(3 * time.Minute)
				replay, err := service.Execute(context.Background(), command)
				if err != nil || replay.Status != "outcome_unknown" || runtime.calls != 1 || runtime.loads != 1 {
					t.Fatalf("recovery resent or accepted unknown: %+v %v", replay, err)
				}
			}
			for _, b := range runtime.lastBuffers {
				if strings.Trim(string(b), "\x00") != "" {
					t.Fatal("media buffer leaked")
				}
			}
		})
	}
}

func TestVisionReviewExecutionConcurrentObserverDoesNotSend(t *testing.T) {
	service, _, runtime, command, _ := visionExecutionFixture(t)
	runtime.afterInvoke = func() {
		observed, err := service.Execute(context.Background(), command)
		if err != nil || observed.Status != "running" || runtime.calls != 1 || runtime.loads != 1 {
			t.Fatalf("inflight observer resent: %v", err)
		}
	}
	if got, err := service.Execute(context.Background(), command); err != nil || got.Status != "accepted" {
		t.Fatalf("original dispatch: %v", err)
	}
}

type visionExecutionStore struct {
	state      app.VisionReviewExecutionState
	release    contract.SceneAnalysisControlProof
	active     bool
	failCommit bool
	stale      bool
}

func (s *visionExecutionStore) WithinVisionReviewTransaction(ctx context.Context, fn func(app.VisionReviewRepository) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	before := s.state
	s.active = true
	defer func() { s.active = false }()
	if err := fn(s); err != nil {
		s.state = before
		return err
	}
	if s.failCommit {
		s.state = before
		return errors.New("injected commit failure")
	}
	return nil
}
func (s *visionExecutionStore) EnsureRelease(context.Context, app.ReleaseRecord) (contract.SceneAnalysisControlProof, error) {
	return s.release, nil
}
func (s *visionExecutionStore) EnsureManifest(context.Context, app.ManifestRecord) error { return nil }
func (s *visionExecutionStore) ValidateVisionReviewInput(context.Context, app.ExecuteVisionReviewCommand) error {
	if s.stale {
		return errors.New("current facts changed")
	}
	return nil
}
func (s *visionExecutionStore) FindVisionReviewExecution(context.Context, string, string) (app.VisionReviewExecutionState, error) {
	if s.state.Status == "" {
		return app.VisionReviewExecutionState{}, app.ErrNotFound
	}
	return s.state, nil
}
func (s *visionExecutionStore) CreateVisionReviewExecution(_ context.Context, record app.VisionReviewInvocationRecord, auth app.DispatchAuthorizationRecord) error {
	s.state = app.VisionReviewExecutionState{Record: record, Authorization: auth, Status: "running"}
	return nil
}
func (s *visionExecutionStore) CompleteVisionReviewExecution(_ context.Context, value app.VisionReviewResultAcceptance) (app.VisionReviewExecutionState, error) {
	if s.stale {
		return app.VisionReviewExecutionState{}, errors.New("current facts changed")
	}
	s.state.Status = value.Result.Status
	s.state.Result = &value.Result
	if value.Result.Status == "accepted" {
		s.state.Candidate = app.Candidate{ID: value.CandidateID, Candidate: value.Result.Candidate}
	}
	return s.state, nil
}

type visionExecutionRuntime struct {
	t                      *testing.T
	store                  *visionExecutionStore
	images                 [][]byte
	lastBuffers            [][]byte
	calls, loads           int
	afterLoad, afterInvoke func()
	loadErr, invokeErr     error
	invalidResult          bool
}

func (r *visionExecutionRuntime) LoadVisionReviewMedia(context.Context, string, int, contract.VisionReviewInput) ([][]byte, error) {
	if r.store.active {
		r.t.Fatal("media IO inside SQL transaction")
	}
	r.loads++
	r.lastBuffers = make([][]byte, len(r.images))
	for i := range r.images {
		r.lastBuffers[i] = append([]byte(nil), r.images[i]...)
	}
	if r.afterLoad != nil {
		r.afterLoad()
	}
	return r.lastBuffers, r.loadErr
}
func (r *visionExecutionRuntime) InvokeVisionReview(_ context.Context, invocation contract.VisionReviewInvocation, auth contract.SceneAnalysisDispatchAuthorization, images [][]byte) (contract.VisionReviewAttemptResult, error) {
	if r.store.active || r.store.state.Status != "running" || r.store.state.Authorization.AuthorizationHash != auth.Hash {
		r.t.Fatal("model called before committed dispatch authorization")
	}
	if !reflect.DeepEqual(images, r.images) {
		r.t.Fatal("model received wrong images")
	}
	r.calls++
	if r.afterInvoke != nil {
		r.afterInvoke()
	}
	if r.invokeErr != nil {
		return contract.VisionReviewAttemptResult{}, r.invokeErr
	}
	if r.invalidResult {
		return contract.VisionReviewAttemptResult{}, nil
	}
	fixture := readVisionReviewWireFixture(r.t)
	result, err := contract.DecodeVisionReviewAttemptResult(fixture.AcceptedResult)
	if err != nil {
		r.t.Fatal(err)
	}
	candidate, _, err := contract.DecodeVisionReviewCandidate(result.Candidate)
	if err != nil {
		r.t.Fatal(err)
	}
	candidate.Subject = invocation.Payload.StageInput.Subject
	result.Candidate = mustJSON(r.t, candidate)
	hash, err := contract.ProductionCanonicalHash(result.Candidate)
	if err != nil {
		r.t.Fatal(err)
	}
	result.OutputHash = &hash
	result.InvocationID, result.AttemptID = invocation.InvocationID, invocation.AttemptID
	result.StageRelease, result.Control = invocation.StageRelease, invocation.Control
	result.InputHash, result.DispatchAuthorizationHash = invocation.InputHash, auth.Hash
	result.ClaimVersion = 1
	result.ResultHash, err = result.ComputeResultHash()
	return result, err
}
