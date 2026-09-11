package agent_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
)

func TestVisualFoundationExecutionCompletesAcceptedCandidateAndReplays(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	fixture := validVisualFoundationInvocation(t)
	repository := &visualFoundationExecutionRepository{}
	runtime := &visualFoundationExecutionRuntime{t: t}
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	service, err := agentapp.NewVisualFoundationExecutionService(
		repository,
		runtime,
		signer,
		agentapp.VisualFoundationExecutionConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + visualFoundationHash("visual-foundation-runtime"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	command := agentapp.ExecuteVisualFoundationCommand{
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		Input: fixture.Payload.StageInput, MediaAttachments: fixture.Payload.MediaAttachments,
	}

	created, err := service.Execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if created.StageKey != contract.VisualFoundationStageKey || created.CandidateType != "visual_foundation_candidate" ||
		created.ProjectID != command.Input.ProjectID || created.CandidateContentHash == "" ||
		created.CandidateRevisionHash == "" || repository.validateCalls != 2 ||
		len(repository.attempts) != 1 || len(repository.results) != 1 || runtime.calls != 1 {
		t.Fatalf("accepted Visual Foundation execution = %#v repo=%#v calls=%d", created, repository, runtime.calls)
	}

	replayed, err := service.Execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != created.ID || replayed.CandidateRevisionHash != created.CandidateRevisionHash ||
		len(repository.attempts) != 1 || len(repository.results) != 1 || runtime.calls != 1 {
		t.Fatalf("replayed Visual Foundation execution = %#v want=%#v repo=%#v calls=%d", replayed, created, repository, runtime.calls)
	}
}

func TestVisualFoundationExecutionRecordsUnknownWithoutCandidate(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	fixture := validVisualFoundationInvocation(t)
	repository := &visualFoundationExecutionRepository{}
	runtime := &visualFoundationExecutionRuntime{t: t, err: errors.New("connection closed after dispatch")}
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	service, err := agentapp.NewVisualFoundationExecutionService(
		repository,
		runtime,
		signer,
		agentapp.VisualFoundationExecutionConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + visualFoundationHash("visual-foundation-runtime"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Execute(context.Background(), agentapp.ExecuteVisualFoundationCommand{
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		Input: fixture.Payload.StageInput, MediaAttachments: fixture.Payload.MediaAttachments,
	})
	if agentapp.ErrorCode(err) != "agent_outcome_unknown" || len(repository.results) != 1 ||
		repository.results[0].Result.Status != "outcome_unknown" || repository.candidate.ID != "" {
		t.Fatalf("unknown Visual Foundation execution: err=%v repo=%#v", err, repository)
	}
}

type visualFoundationExecutionRuntime struct {
	t     *testing.T
	calls int
	err   error
}

func (runtime *visualFoundationExecutionRuntime) Invoke(
	_ context.Context,
	invocation contract.VisualFoundationInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
) (contract.VisualFoundationAttemptResult, error) {
	runtime.calls++
	if runtime.err != nil {
		return contract.VisualFoundationAttemptResult{}, runtime.err
	}
	return validVisualFoundationResult(runtime.t, invocation, authorization), nil
}

type visualFoundationExecutionRepository struct {
	release       agentapp.ReleaseRecord
	control       contract.SceneAnalysisControlProof
	manifest      agentapp.ManifestRecord
	invocation    agentapp.VisualFoundationInvocationRecord
	attempts      []agentapp.AttemptRecord
	authorization agentapp.DispatchAuthorizationRecord
	results       []agentapp.VisualFoundationResultAcceptance
	candidate     agentapp.Candidate
	validateCalls int
}

func (repository *visualFoundationExecutionRepository) WithinVisualFoundationTransaction(
	_ context.Context,
	operation func(agentapp.VisualFoundationRepository) error,
) error {
	return operation(repository)
}

func (repository *visualFoundationExecutionRepository) ValidateVisualFoundationInput(
	context.Context,
	contract.VisualFoundationInput,
) error {
	repository.validateCalls++
	return nil
}

func (repository *visualFoundationExecutionRepository) EnsureVisualFoundationRelease(
	_ context.Context,
	release agentapp.ReleaseRecord,
) (contract.SceneAnalysisControlProof, error) {
	if repository.release.ID == "" {
		repository.release = release
		repository.control = release.InitialControl
	}
	return repository.control, nil
}

func (repository *visualFoundationExecutionRepository) EnsureVisualFoundationManifest(
	_ context.Context,
	manifest agentapp.ManifestRecord,
) error {
	if repository.manifest.ID == "" {
		repository.manifest = manifest
	}
	return nil
}

func (repository *visualFoundationExecutionRepository) FindVisualFoundationInvocation(
	context.Context,
	string,
	string,
	string,
) (agentapp.VisualFoundationInvocationRecord, error) {
	if repository.invocation.Invocation.InvocationID == "" {
		return agentapp.VisualFoundationInvocationRecord{}, agentapp.ErrNotFound
	}
	return repository.invocation, nil
}

func (repository *visualFoundationExecutionRepository) CreateVisualFoundationInvocation(
	_ context.Context,
	invocation agentapp.VisualFoundationInvocationRecord,
) error {
	repository.invocation = invocation
	return nil
}

func (repository *visualFoundationExecutionRepository) FindVisualFoundationCandidateByInvocation(
	context.Context,
	string,
) (agentapp.Candidate, error) {
	if repository.candidate.ID == "" {
		return agentapp.Candidate{}, agentapp.ErrNotFound
	}
	return repository.candidate, nil
}

func (repository *visualFoundationExecutionRepository) CountVisualFoundationAttempts(context.Context, string) (int64, error) {
	return int64(len(repository.attempts)), nil
}

func (repository *visualFoundationExecutionRepository) CreateVisualFoundationAttempt(
	_ context.Context,
	attempt agentapp.AttemptRecord,
) error {
	repository.attempts = append(repository.attempts, attempt)
	return nil
}

func (repository *visualFoundationExecutionRepository) CreateVisualFoundationDispatchAuthorization(
	_ context.Context,
	authorization agentapp.DispatchAuthorizationRecord,
) error {
	repository.authorization = authorization
	return nil
}

func (repository *visualFoundationExecutionRepository) AcceptVisualFoundationResult(
	_ context.Context,
	acceptance agentapp.VisualFoundationResultAcceptance,
) (agentapp.Candidate, error) {
	repository.results = append(repository.results, acceptance)
	repository.candidate = agentapp.Candidate{
		ID: acceptance.CandidateID, WorkspaceID: acceptance.Invocation.Invocation.Payload.Scope.WorkspaceID,
		ProjectID: acceptance.Invocation.Invocation.Payload.Scope.ProjectID,
		StageKey:  contract.VisualFoundationStageKey, ProfileKey: "default",
		StageInstanceKey: acceptance.Invocation.Invocation.StageInstanceKey(), Revision: 1,
		CandidateType: acceptance.Result.CandidateType, Candidate: acceptance.Result.Candidate,
		CandidateContentHash:  *acceptance.Result.OutputHash,
		CandidateRevisionHash: visualFoundationHash("candidate-revision"),
		SourceInvocationID:    acceptance.Invocation.Invocation.InvocationID,
		SourceResultID:        acceptance.ResultID, SourceResultHash: acceptance.Result.ResultHash,
		CreatedAt: acceptance.AcceptedAt,
	}
	return repository.candidate, nil
}

func (repository *visualFoundationExecutionRepository) RecordFailedVisualFoundationResult(
	_ context.Context,
	acceptance agentapp.VisualFoundationResultAcceptance,
) error {
	repository.results = append(repository.results, acceptance)
	return nil
}
