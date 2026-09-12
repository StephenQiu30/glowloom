package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/agent/grant"
)

func TestReferenceBriefExecutionCompletesAcceptedCandidateAndReplays(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	fixture := validReferenceBriefInvocation(t)
	agentImageDigest := "sha256:" + referencePlanHash("reference-brief-runtime")
	release, err := agentapp.BuildStageReleaseRecord(contract.ReferenceBriefStageKey, agentImageDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Payload.StageInput.StageRelease.StageReleaseHash = release.Identity.StageReleaseHash
	repository := &referenceBriefExecutionRepository{}
	runtime := &referenceBriefExecutionRuntime{t: t}
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	service, err := agentapp.NewReferenceBriefExecutionService(
		repository,
		runtime,
		signer,
		agentapp.ReferenceBriefExecutionConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: agentImageDigest,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	command := agentapp.ExecuteReferenceBriefCommand{
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), Input: fixture.Payload.StageInput,
	}

	created, err := service.Execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if created.StageKey != contract.ReferenceBriefStageKey || created.CandidateType != "reference_brief_candidate" ||
		created.ProjectID != command.Input.ProjectID || created.CandidateContentHash == "" ||
		created.CandidateRevisionHash == "" || repository.validateCalls != 2 ||
		len(repository.attempts) != 1 || len(repository.results) != 1 || runtime.calls != 1 {
		t.Fatalf("accepted Reference Brief execution = %#v repo=%#v calls=%d", created, repository, runtime.calls)
	}

	replayed, err := service.Execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != created.ID || replayed.CandidateRevisionHash != created.CandidateRevisionHash ||
		len(repository.attempts) != 1 || len(repository.results) != 1 || runtime.calls != 1 {
		t.Fatalf("replayed Reference Brief execution = %#v want=%#v repo=%#v calls=%d", replayed, created, repository, runtime.calls)
	}
}

func TestReferenceBriefExecutionRecordsUnknownWithoutCandidate(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	fixture := validReferenceBriefInvocation(t)
	agentImageDigest := "sha256:" + referencePlanHash("reference-brief-runtime")
	release, err := agentapp.BuildStageReleaseRecord(contract.ReferenceBriefStageKey, agentImageDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Payload.StageInput.StageRelease.StageReleaseHash = release.Identity.StageReleaseHash
	repository := &referenceBriefExecutionRepository{}
	runtime := &referenceBriefExecutionRuntime{t: t, err: errors.New("connection closed after dispatch")}
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	service, err := agentapp.NewReferenceBriefExecutionService(
		repository,
		runtime,
		signer,
		agentapp.ReferenceBriefExecutionConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: agentImageDigest,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Execute(context.Background(), agentapp.ExecuteReferenceBriefCommand{
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), Input: fixture.Payload.StageInput,
	})
	if agentapp.ErrorCode(err) != "agent_outcome_unknown" || len(repository.results) != 1 ||
		repository.results[0].Result.Status != "outcome_unknown" || repository.candidate.ID != "" {
		t.Fatalf("unknown Reference Brief execution: err=%v repo=%#v", err, repository)
	}
}

func TestReferenceBriefExecutionRejectsFactsDriftBeforeAccept(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	fixture := validReferenceBriefInvocation(t)
	agentImageDigest := "sha256:" + referencePlanHash("reference-brief-runtime")
	release, err := agentapp.BuildStageReleaseRecord(contract.ReferenceBriefStageKey, agentImageDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Payload.StageInput.StageRelease.StageReleaseHash = release.Identity.StageReleaseHash
	repository := &referenceBriefExecutionRepository{validateErrorAt: 2}
	runtime := &referenceBriefExecutionRuntime{t: t}
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	service, err := agentapp.NewReferenceBriefExecutionService(
		repository,
		runtime,
		signer,
		agentapp.ReferenceBriefExecutionConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: agentImageDigest,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Execute(context.Background(), agentapp.ExecuteReferenceBriefCommand{
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), Input: fixture.Payload.StageInput,
	})
	if err == nil || created.ID != "" || repository.validateCalls != 2 ||
		repository.candidate.ID != "" || len(repository.results) != 0 || runtime.calls != 1 {
		t.Fatalf("facts drift before accept: candidate=%#v repo=%#v calls=%d err=%v", created, repository, runtime.calls, err)
	}
}

type referenceBriefExecutionRuntime struct {
	t     *testing.T
	calls int
	err   error
}

func (runtime *referenceBriefExecutionRuntime) InvokeReferenceBrief(
	_ context.Context,
	invocation contract.ReferenceBriefInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
) (contract.ReferenceBriefAttemptResult, error) {
	runtime.calls++
	if runtime.err != nil {
		return contract.ReferenceBriefAttemptResult{}, runtime.err
	}
	candidate := referenceBriefCandidateForInput(runtime.t, invocation.Payload.StageInput)
	outputHash, err := contract.ProductionCanonicalHash(candidate)
	if err != nil {
		runtime.t.Fatal(err)
	}
	diagnostics := []contract.SceneAnalysisDiagnostic{}
	diagnosticBytes, err := json.Marshal(diagnostics)
	if err != nil {
		runtime.t.Fatal(err)
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(diagnosticBytes)
	if err != nil {
		runtime.t.Fatal(err)
	}
	result := contract.ReferenceBriefAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: invocation.WireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease, Control: invocation.Control,
		ClaimVersion: authorization.ClaimVersion, DispatchAuthorizationHash: authorization.Hash,
		Status: "accepted", CandidateType: "reference_brief_candidate", Candidate: candidate,
		InputHash: invocation.InputHash, OutputHash: &outputHash, Diagnostics: diagnostics,
		DiagnosticHash: diagnosticHash, CompletedAt: time.Date(2026, 9, 12, 12, 0, 1, 0, time.UTC),
		Executor: contract.ReferenceBriefExecutor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "reference-brief-harness", Model: "codex-cli-default",
		},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		runtime.t.Fatal(err)
	}
	return result, nil
}

type referenceBriefExecutionRepository struct {
	release         agentapp.ReleaseRecord
	control         contract.SceneAnalysisControlProof
	manifest        agentapp.ManifestRecord
	invocation      agentapp.ReferenceBriefInvocationRecord
	attempts        []agentapp.AttemptRecord
	authorization   agentapp.DispatchAuthorizationRecord
	results         []agentapp.ReferenceBriefResultAcceptance
	candidate       agentapp.Candidate
	validateCalls   int
	validateErrorAt int
}

func (repository *referenceBriefExecutionRepository) WithinReferenceBriefTransaction(
	_ context.Context,
	operation func(agentapp.ReferenceBriefRepository) error,
) error {
	return operation(repository)
}

func (repository *referenceBriefExecutionRepository) ValidateReferenceBriefInput(context.Context, contract.ReferenceBriefInput) error {
	repository.validateCalls++
	if repository.validateCalls == repository.validateErrorAt {
		return errors.New("Reference Brief facts drifted")
	}
	return nil
}

func (repository *referenceBriefExecutionRepository) EnsureReferenceBriefRelease(
	_ context.Context,
	release agentapp.ReleaseRecord,
) (contract.SceneAnalysisControlProof, error) {
	if repository.release.ID == "" {
		repository.release = release
		repository.control = release.InitialControl
	}
	return repository.control, nil
}

func (repository *referenceBriefExecutionRepository) EnsureReferenceBriefManifest(
	_ context.Context,
	manifest agentapp.ManifestRecord,
) error {
	if repository.manifest.ID == "" {
		repository.manifest = manifest
	}
	return nil
}

func (repository *referenceBriefExecutionRepository) FindReferenceBriefInvocation(
	context.Context,
	string,
	string,
	string,
) (agentapp.ReferenceBriefInvocationRecord, error) {
	if repository.invocation.Invocation.InvocationID == "" {
		return agentapp.ReferenceBriefInvocationRecord{}, agentapp.ErrNotFound
	}
	return repository.invocation, nil
}

func (repository *referenceBriefExecutionRepository) CreateReferenceBriefInvocation(
	_ context.Context,
	invocation agentapp.ReferenceBriefInvocationRecord,
) error {
	repository.invocation = invocation
	return nil
}

func (repository *referenceBriefExecutionRepository) FindReferenceBriefCandidateByInvocation(context.Context, string) (agentapp.Candidate, error) {
	if repository.candidate.ID == "" {
		return agentapp.Candidate{}, agentapp.ErrNotFound
	}
	return repository.candidate, nil
}

func (repository *referenceBriefExecutionRepository) CountReferenceBriefAttempts(context.Context, string) (int64, error) {
	return int64(len(repository.attempts)), nil
}

func (repository *referenceBriefExecutionRepository) CreateReferenceBriefAttempt(
	_ context.Context,
	attempt agentapp.AttemptRecord,
) error {
	repository.attempts = append(repository.attempts, attempt)
	return nil
}

func (repository *referenceBriefExecutionRepository) CreateReferenceBriefDispatchAuthorization(
	_ context.Context,
	authorization agentapp.DispatchAuthorizationRecord,
) error {
	repository.authorization = authorization
	return nil
}

func (repository *referenceBriefExecutionRepository) AcceptReferenceBriefResult(
	_ context.Context,
	acceptance agentapp.ReferenceBriefResultAcceptance,
) (agentapp.Candidate, error) {
	repository.results = append(repository.results, acceptance)
	repository.candidate = agentapp.Candidate{
		ID: acceptance.CandidateID, WorkspaceID: acceptance.Invocation.Invocation.Payload.Scope.WorkspaceID,
		ProjectID: acceptance.Invocation.Invocation.Payload.Scope.ProjectID,
		StageKey:  contract.ReferenceBriefStageKey, ProfileKey: "default",
		StageInstanceKey: acceptance.Invocation.Invocation.StageInstanceKey(), Revision: 1,
		CandidateType: acceptance.Result.CandidateType, Candidate: acceptance.Result.Candidate,
		CandidateContentHash:  *acceptance.Result.OutputHash,
		CandidateRevisionHash: referencePlanHash("reference-brief-revision"),
		SourceInvocationID:    acceptance.Invocation.Invocation.InvocationID, SourceResultID: acceptance.ResultID,
		SourceResultHash: acceptance.Result.ResultHash, CreatedAt: acceptance.AcceptedAt,
	}
	return repository.candidate, nil
}

func (repository *referenceBriefExecutionRepository) RecordFailedReferenceBriefResult(
	_ context.Context,
	acceptance agentapp.ReferenceBriefResultAcceptance,
) error {
	repository.results = append(repository.results, acceptance)
	return nil
}

var _ agentapp.ReferenceBriefTransactions = (*referenceBriefExecutionRepository)(nil)
var _ agentapp.ReferenceBriefRepository = (*referenceBriefExecutionRepository)(nil)
