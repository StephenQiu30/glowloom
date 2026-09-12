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

func TestReferencePlanExecutionCompletesAcceptedCandidateAndReplays(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	fixture := validReferencePlanInvocation(t)
	repository := &referencePlanExecutionRepository{}
	runtime := &referencePlanExecutionRuntime{t: t}
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	service, err := agentapp.NewReferencePlanExecutionService(
		repository,
		runtime,
		signer,
		agentapp.ReferencePlanExecutionConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + referencePlanHash("reference-plan-runtime"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	command := agentapp.ExecuteReferencePlanCommand{
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), Input: fixture.Payload.StageInput,
	}

	created, err := service.Execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if created.StageKey != contract.ReferencePlanStageKey || created.CandidateType != "reference_plan_candidate" ||
		created.ProjectID != command.Input.ProjectID || created.CandidateContentHash == "" ||
		created.CandidateRevisionHash == "" || repository.validateCalls != 2 ||
		len(repository.attempts) != 1 || len(repository.results) != 1 || runtime.calls != 1 {
		t.Fatalf("accepted Reference Plan execution = %#v repo=%#v calls=%d", created, repository, runtime.calls)
	}

	replayed, err := service.Execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != created.ID || replayed.CandidateRevisionHash != created.CandidateRevisionHash ||
		len(repository.attempts) != 1 || len(repository.results) != 1 || runtime.calls != 1 {
		t.Fatalf("replayed Reference Plan execution = %#v want=%#v repo=%#v calls=%d", replayed, created, repository, runtime.calls)
	}
}

func TestReferencePlanExecutionRecordsUnknownWithoutCandidate(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	fixture := validReferencePlanInvocation(t)
	repository := &referencePlanExecutionRepository{}
	runtime := &referencePlanExecutionRuntime{t: t, err: errors.New("connection closed after dispatch")}
	signer, err := grant.NewSigner("a-strong-agent-execution-secret-123", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	service, err := agentapp.NewReferencePlanExecutionService(
		repository,
		runtime,
		signer,
		agentapp.ReferencePlanExecutionConfig{
			Now: func() time.Time { return now }, NewID: uuid.NewString,
			AgentImageDigest: "sha256:" + referencePlanHash("reference-plan-runtime"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Execute(context.Background(), agentapp.ExecuteReferencePlanCommand{
		WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(), Input: fixture.Payload.StageInput,
	})
	if agentapp.ErrorCode(err) != "agent_outcome_unknown" || len(repository.results) != 1 ||
		repository.results[0].Result.Status != "outcome_unknown" || repository.candidate.ID != "" {
		t.Fatalf("unknown Reference Plan execution: err=%v repo=%#v", err, repository)
	}
}

type referencePlanExecutionRuntime struct {
	t     *testing.T
	calls int
	err   error
}

func (runtime *referencePlanExecutionRuntime) InvokeReferencePlan(
	_ context.Context,
	invocation contract.ReferencePlanInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
) (contract.ReferencePlanAttemptResult, error) {
	runtime.calls++
	if runtime.err != nil {
		return contract.ReferencePlanAttemptResult{}, runtime.err
	}
	candidate := mustJSON(runtime.t, referencePlanCandidate(invocation.Payload.StageInput))
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
	result := contract.ReferencePlanAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: invocation.WireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease, Control: invocation.Control,
		ClaimVersion: authorization.ClaimVersion, DispatchAuthorizationHash: authorization.Hash,
		Status: "accepted", CandidateType: "reference_plan_candidate", Candidate: candidate,
		InputHash: invocation.InputHash, OutputHash: &outputHash, Diagnostics: diagnostics,
		DiagnosticHash: diagnosticHash, CompletedAt: time.Date(2026, 9, 12, 12, 0, 1, 0, time.UTC),
		Executor: contract.ReferencePlanExecutor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "reference-plan-harness", Model: "codex-cli-default",
		},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		runtime.t.Fatal(err)
	}
	return result, nil
}

type referencePlanExecutionRepository struct {
	release       agentapp.ReleaseRecord
	control       contract.SceneAnalysisControlProof
	manifest      agentapp.ManifestRecord
	invocation    agentapp.ReferencePlanInvocationRecord
	attempts      []agentapp.AttemptRecord
	authorization agentapp.DispatchAuthorizationRecord
	results       []agentapp.ReferencePlanResultAcceptance
	candidate     agentapp.Candidate
	validateCalls int
}

func (repository *referencePlanExecutionRepository) WithinReferencePlanTransaction(
	_ context.Context,
	operation func(agentapp.ReferencePlanRepository) error,
) error {
	return operation(repository)
}

func (repository *referencePlanExecutionRepository) ValidateReferencePlanInput(context.Context, contract.ReferencePlanInput) error {
	repository.validateCalls++
	return nil
}

func (repository *referencePlanExecutionRepository) EnsureReferencePlanRelease(
	_ context.Context,
	release agentapp.ReleaseRecord,
) (contract.SceneAnalysisControlProof, error) {
	if repository.release.ID == "" {
		repository.release = release
		repository.control = release.InitialControl
	}
	return repository.control, nil
}

func (repository *referencePlanExecutionRepository) EnsureReferencePlanManifest(
	_ context.Context,
	manifest agentapp.ManifestRecord,
) error {
	if repository.manifest.ID == "" {
		repository.manifest = manifest
	}
	return nil
}

func (repository *referencePlanExecutionRepository) FindReferencePlanInvocation(
	context.Context,
	string,
	string,
	string,
) (agentapp.ReferencePlanInvocationRecord, error) {
	if repository.invocation.Invocation.InvocationID == "" {
		return agentapp.ReferencePlanInvocationRecord{}, agentapp.ErrNotFound
	}
	return repository.invocation, nil
}

func (repository *referencePlanExecutionRepository) CreateReferencePlanInvocation(
	_ context.Context,
	invocation agentapp.ReferencePlanInvocationRecord,
) error {
	repository.invocation = invocation
	return nil
}

func (repository *referencePlanExecutionRepository) FindReferencePlanCandidateByInvocation(context.Context, string) (agentapp.Candidate, error) {
	if repository.candidate.ID == "" {
		return agentapp.Candidate{}, agentapp.ErrNotFound
	}
	return repository.candidate, nil
}

func (repository *referencePlanExecutionRepository) CountReferencePlanAttempts(context.Context, string) (int64, error) {
	return int64(len(repository.attempts)), nil
}

func (repository *referencePlanExecutionRepository) CreateReferencePlanAttempt(_ context.Context, attempt agentapp.AttemptRecord) error {
	repository.attempts = append(repository.attempts, attempt)
	return nil
}

func (repository *referencePlanExecutionRepository) CreateReferencePlanDispatchAuthorization(
	_ context.Context,
	authorization agentapp.DispatchAuthorizationRecord,
) error {
	repository.authorization = authorization
	return nil
}

func (repository *referencePlanExecutionRepository) AcceptReferencePlanResult(
	_ context.Context,
	acceptance agentapp.ReferencePlanResultAcceptance,
) (agentapp.Candidate, error) {
	repository.results = append(repository.results, acceptance)
	repository.candidate = agentapp.Candidate{
		ID: acceptance.CandidateID, WorkspaceID: acceptance.Invocation.Invocation.Payload.Scope.WorkspaceID,
		ProjectID: acceptance.Invocation.Invocation.Payload.Scope.ProjectID,
		StageKey:  contract.ReferencePlanStageKey, ProfileKey: "default",
		StageInstanceKey: acceptance.Invocation.Invocation.StageInstanceKey(), Revision: 1,
		CandidateType: acceptance.Result.CandidateType, Candidate: acceptance.Result.Candidate,
		CandidateContentHash:  *acceptance.Result.OutputHash,
		CandidateRevisionHash: referencePlanHash("reference-plan-revision"),
		SourceInvocationID:    acceptance.Invocation.Invocation.InvocationID, SourceResultID: acceptance.ResultID,
		SourceResultHash: acceptance.Result.ResultHash, CreatedAt: acceptance.AcceptedAt,
	}
	return repository.candidate, nil
}

func (repository *referencePlanExecutionRepository) RecordFailedReferencePlanResult(
	_ context.Context,
	acceptance agentapp.ReferencePlanResultAcceptance,
) error {
	repository.results = append(repository.results, acceptance)
	return nil
}

var _ agentapp.ReferencePlanTransactions = (*referencePlanExecutionRepository)(nil)
var _ agentapp.ReferencePlanRepository = (*referencePlanExecutionRepository)(nil)
