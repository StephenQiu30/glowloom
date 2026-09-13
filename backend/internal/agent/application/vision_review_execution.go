package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/google/uuid"
)

type ExecuteVisionReviewCommand struct {
	WorkflowRunID, NodeRunID, UserID string
	TokenVersion                     int
	Input                            contract.VisionReviewInput
}

type VisionReviewInvocationRecord struct {
	Invocation                          contract.VisionReviewInvocation
	WorkflowRunID, NodeRunID, ReleaseID string
	Manifest                            ManifestRecord
	CreatedAt                           time.Time
}

type VisionReviewExecutionState struct {
	Record        VisionReviewInvocationRecord
	Authorization DispatchAuthorizationRecord
	Status        string
	Result        *contract.VisionReviewAttemptResult
	Candidate     Candidate
}

type VisionReviewResultAcceptance struct {
	Command               ExecuteVisionReviewCommand
	Record                VisionReviewInvocationRecord
	Result                contract.VisionReviewAttemptResult
	ResultID, CandidateID string
	AcceptedAt            time.Time
}

type VisionReviewRepository interface {
	EnsureRelease(context.Context, ReleaseRecord) (contract.SceneAnalysisControlProof, error)
	EnsureManifest(context.Context, ManifestRecord) error
	ValidateVisionReviewInput(context.Context, ExecuteVisionReviewCommand) error
	FindVisionReviewExecution(context.Context, string, string) (VisionReviewExecutionState, error)
	CreateVisionReviewExecution(context.Context, VisionReviewInvocationRecord, DispatchAuthorizationRecord) error
	CompleteVisionReviewExecution(context.Context, VisionReviewResultAcceptance) (VisionReviewExecutionState, error)
}
type VisionReviewTransactions interface {
	WithinVisionReviewTransaction(context.Context, func(VisionReviewRepository) error) error
}
type VisionReviewMediaLoader interface {
	LoadVisionReviewMedia(context.Context, string, int, contract.VisionReviewInput) ([][]byte, error)
}
type VisionReviewInvoker interface {
	InvokeVisionReview(context.Context, contract.VisionReviewInvocation, contract.SceneAnalysisDispatchAuthorization, [][]byte) (contract.VisionReviewAttemptResult, error)
}
type VisionReviewAuthorizer interface {
	IssueVisionReviewDispatchAuthorization(contract.VisionReviewInvocation, int64) (contract.SceneAnalysisDispatchAuthorization, error)
}
type VisionReviewExecutionConfig struct {
	Now              func() time.Time
	NewID            func() string
	AgentImageDigest string
	Budget           contract.SceneAnalysisExecutionBudget
}
type VisionReviewExecutionService struct {
	transactions VisionReviewTransactions
	media        VisionReviewMediaLoader
	invoker      VisionReviewInvoker
	authorizer   VisionReviewAuthorizer
	config       VisionReviewExecutionConfig
}

func NewVisionReviewExecutionService(transactions VisionReviewTransactions, media VisionReviewMediaLoader, invoker VisionReviewInvoker, authorizer VisionReviewAuthorizer, config VisionReviewExecutionConfig) (*VisionReviewExecutionService, error) {
	if transactions == nil || media == nil || invoker == nil || authorizer == nil {
		return nil, errors.New("Vision Review execution dependencies are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.NewID == nil {
		config.NewID = uuid.NewString
	}
	if config.Budget == (contract.SceneAnalysisExecutionBudget{}) {
		config.Budget = contract.SceneAnalysisExecutionBudget{MaxAttempts: 1, MaxModelCalls: 1, MaxExecutionSeconds: 120, MaxOutputBytes: 131072}
	}
	if config.Budget.Validate() != nil || config.Budget.MaxAttempts != 1 || config.Budget.MaxModelCalls != 1 ||
		config.Budget.MaxExecutionSeconds > 120 || config.Budget.MaxOutputBytes > 131072 {
		return nil, errors.New("Vision Review requires a bounded single-attempt budget")
	}
	if _, err := BuildStageReleaseRecord(contract.VisionReviewStageKey, config.AgentImageDigest, config.Now().UTC()); err != nil {
		return nil, err
	}
	return &VisionReviewExecutionService{transactions: transactions, media: media, invoker: invoker, authorizer: authorizer, config: config}, nil
}

func (service *VisionReviewExecutionService) Execute(ctx context.Context, command ExecuteVisionReviewCommand) (VisionReviewExecutionState, error) {
	if err := ctx.Err(); err != nil {
		return VisionReviewExecutionState{}, err
	}
	for _, id := range []string{command.WorkflowRunID, command.NodeRunID, command.UserID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return VisionReviewExecutionState{}, errors.New("invalid Vision Review execution identity")
		}
	}
	if command.TokenVersion < 1 {
		return VisionReviewExecutionState{}, errors.New("invalid Vision Review actor")
	}
	raw, err := json.Marshal(command.Input)
	if err != nil {
		return VisionReviewExecutionState{}, err
	}
	command.Input, err = contract.DecodeVisionReviewInput(raw)
	if err != nil {
		return VisionReviewExecutionState{}, err
	}
	release, err := BuildStageReleaseRecord(contract.VisionReviewStageKey, service.config.AgentImageDigest, service.config.Now().UTC())
	if err != nil {
		return VisionReviewExecutionState{}, err
	}
	if release.Identity.StageReleaseHash != command.Input.Subject.StageReleaseHash {
		return VisionReviewExecutionState{}, errors.New("Vision Review input release is not executable")
	}
	state, _, err := service.prepare(ctx, command, release, false)
	if err != nil || state.Status != "" {
		return state, err
	}
	images, err := service.media.LoadVisionReviewMedia(ctx, command.UserID, command.TokenVersion, command.Input)
	defer func() {
		for _, contents := range images {
			clear(contents)
		}
	}()
	if err != nil {
		return VisionReviewExecutionState{}, fmt.Errorf("load Vision Review media: %w", err)
	}
	if len(images) != len(command.Input.Attachments) {
		return VisionReviewExecutionState{}, errors.New("Vision Review media set is incomplete")
	}
	for i, image := range images {
		digest := sha256.Sum256(image)
		if int64(len(image)) != command.Input.Attachments[i].ByteLength || hex.EncodeToString(digest[:]) != command.Input.Attachments[i].Slot.SHA256 {
			return VisionReviewExecutionState{}, errors.New("Vision Review media changed before dispatch")
		}
	}
	if err := ctx.Err(); err != nil {
		return VisionReviewExecutionState{}, err
	}
	state, authorization, err := service.prepare(ctx, command, release, true)
	if err != nil || authorization.Value == "" {
		return state, err
	}
	// The transaction returned successfully before this single external call.
	result, invokeErr := service.invoker.InvokeVisionReview(ctx, state.Record.Invocation, authorization, images)
	if err := ctx.Err(); err != nil {
		return VisionReviewExecutionState{}, err
	}
	if invokeErr != nil || result.ValidateFor(state.Record.Invocation, 1, authorization.Hash) != nil {
		result, err = visionReviewUnknownResult(state, service.config.Now().UTC())
		if err != nil {
			return VisionReviewExecutionState{}, err
		}
	}
	return service.complete(ctx, command, state.Record, result)
}

func (service *VisionReviewExecutionService) prepare(ctx context.Context, command ExecuteVisionReviewCommand, release ReleaseRecord, dispatch bool) (VisionReviewExecutionState, contract.SceneAnalysisDispatchAuthorization, error) {
	var state VisionReviewExecutionState
	var authorization contract.SceneAnalysisDispatchAuthorization
	err := service.transactions.WithinVisionReviewTransaction(ctx, func(repo VisionReviewRepository) error {
		control, err := repo.EnsureRelease(ctx, release)
		if err != nil {
			return err
		}
		if control.Status != "approved" {
			return &Error{Code: "release_not_executable", Message: "Vision Review release is not approved"}
		}
		if err := repo.ValidateVisionReviewInput(ctx, command); err != nil {
			return err
		}
		existing, err := repo.FindVisionReviewExecution(ctx, command.WorkflowRunID, command.NodeRunID)
		if err == nil {
			invocation := existing.Record.Invocation
			if invocation.Validate() != nil || invocation.StageRelease != release.Identity || invocation.Control != control ||
				invocation.Budget != service.config.Budget || !reflect.DeepEqual(invocation.Payload.StageInput, command.Input) {
				return &Error{Code: "invocation_fence_drift", Message: "Vision Review invocation changed its frozen input"}
			}
			state = existing
			if state.Status == "running" && !service.config.Now().Before(visionReviewDeadline(state)) {
				result, err := visionReviewUnknownResult(state, service.config.Now().UTC())
				if err != nil {
					return err
				}
				state, err = repo.CompleteVisionReviewExecution(ctx, service.acceptance(command, state.Record, result))
				return err
			}
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		if !dispatch {
			return nil
		}
		now := service.config.Now().UTC()
		record, err := service.newInvocation(command, release, control, now)
		if err != nil {
			return err
		}
		if err := repo.EnsureManifest(ctx, record.Manifest); err != nil {
			return err
		}
		authorization, err = service.authorizer.IssueVisionReviewDispatchAuthorization(record.Invocation, 1)
		if err != nil {
			return err
		}
		if err := authorization.Validate(); err != nil || !authorization.ExpiresAt.After(now) {
			return errors.New("invalid Vision Review dispatch authorization")
		}
		auth := DispatchAuthorizationRecord{AttemptID: record.Invocation.AttemptID, AuthorizationHash: authorization.Hash, ExpiresAt: authorization.ExpiresAt, IssuedAt: now}
		if err := repo.CreateVisionReviewExecution(ctx, record, auth); err != nil {
			return err
		}
		state = VisionReviewExecutionState{Record: record, Authorization: auth, Status: "running"}
		return nil
	})
	if err != nil {
		return VisionReviewExecutionState{}, contract.SceneAnalysisDispatchAuthorization{}, err
	}
	return state, authorization, nil
}

func (service *VisionReviewExecutionService) complete(ctx context.Context, command ExecuteVisionReviewCommand, record VisionReviewInvocationRecord, result contract.VisionReviewAttemptResult) (VisionReviewExecutionState, error) {
	var state VisionReviewExecutionState
	err := service.transactions.WithinVisionReviewTransaction(ctx, func(repo VisionReviewRepository) error {
		// The repository locks Control first, then validates current Owner facts.
		var err error
		state, err = repo.CompleteVisionReviewExecution(ctx, service.acceptance(command, record, result))
		return err
	})
	if err != nil {
		return VisionReviewExecutionState{}, err
	}
	return state, err
}

func (service *VisionReviewExecutionService) acceptance(command ExecuteVisionReviewCommand, record VisionReviewInvocationRecord, result contract.VisionReviewAttemptResult) VisionReviewResultAcceptance {
	return VisionReviewResultAcceptance{Command: command, Record: record, Result: result, ResultID: service.config.NewID(), CandidateID: service.config.NewID(), AcceptedAt: service.config.Now().UTC()}
}

func (service *VisionReviewExecutionService) newInvocation(command ExecuteVisionReviewCommand, release ReleaseRecord, control contract.SceneAnalysisControlProof, now time.Time) (VisionReviewInvocationRecord, error) {
	subject := command.Input.Subject
	root := subject.InputHash
	manifestID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:vision-review:manifest:"+command.NodeRunID+":"+root)).String()
	shardKey := "vision_bundle:" + subject.BundleInputRef.ID
	shards, err := json.Marshal([]map[string]string{{"shard_key": shardKey, "impact_closure_hash": root}})
	if err != nil {
		return VisionReviewInvocationRecord{}, err
	}
	coverageHash, err := contract.ProductionCanonicalHash(shards)
	if err != nil {
		return VisionReviewInvocationRecord{}, err
	}
	material, err := json.Marshal(map[string]any{"contract_id": "vision-review-shard-manifest-production",
		"manifest_id": manifestID, "workflow_run_id": command.WorkflowRunID, "node_run_id": command.NodeRunID,
		"stage_key": contract.VisionReviewStageKey, "root_input_hash": root, "shards": json.RawMessage(shards), "coverage_hash": coverageHash})
	if err != nil {
		return VisionReviewInvocationRecord{}, err
	}
	manifestHash, err := contract.ProductionCanonicalHash(material)
	if err != nil {
		return VisionReviewInvocationRecord{}, err
	}
	manifest := ManifestRecord{ID: manifestID, WorkspaceID: subject.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
		NodeRunID: command.NodeRunID, StageKey: contract.VisionReviewStageKey, RootInputHash: root,
		Shards: shards, CoverageHash: coverageHash, ManifestHash: manifestHash, CreatedAt: now}
	invocation, err := contract.NewVisionReviewInvocation(service.config.NewID(), service.config.NewID(), release.Identity, control, service.config.Budget,
		contract.VisionReviewPayload{
			Variant:    contract.VisionReviewStageVariant{StageKey: contract.VisionReviewStageKey, ProfileKey: "default", LaneKey: "primary", OutputSchemaVersion: contract.VisionReviewCandidateContractID},
			Scope:      contract.VisionReviewScope{WorkspaceID: subject.WorkspaceID, ProjectID: subject.ProjectID, BundleInputID: subject.BundleInputRef.ID},
			Shard:      contract.VisionReviewShard{ManifestID: manifestID, ManifestHash: manifestHash, ShardKey: shardKey, ImpactClosureHash: root},
			StageInput: command.Input,
		})
	return VisionReviewInvocationRecord{Invocation: invocation, WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID, ReleaseID: release.ID, Manifest: manifest, CreatedAt: now}, err
}

func visionReviewDeadline(state VisionReviewExecutionState) time.Time {
	return state.Record.CreatedAt.Add(time.Duration(state.Record.Invocation.Budget.MaxExecutionSeconds)*time.Second + 30*time.Second)
}

func visionReviewUnknownResult(state VisionReviewExecutionState, now time.Time) (contract.VisionReviewAttemptResult, error) {
	invocation := state.Record.Invocation
	diagnostics := []contract.SceneAnalysisDiagnostic{{Code: "vision_review_outcome_unknown", Summary: "Vision Review outcome could not be confirmed; do not automatically resend"}}
	raw, err := json.Marshal(diagnostics)
	if err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(raw)
	if err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	result := contract.VisionReviewAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: invocation.WireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease, Control: invocation.Control,
		ClaimVersion: 1, DispatchAuthorizationHash: state.Authorization.AuthorizationHash, Status: "outcome_unknown",
		CandidateType: "vision_review_candidate", InputHash: invocation.InputHash, Diagnostics: diagnostics,
		DiagnosticHash: diagnosticHash, CompletedAt: now,
		Executor: contract.VisionReviewExecutor{RuntimeClass: "vision", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest, HarnessVersion: "vision-review-harness", Model: "unconfirmed"},
		Error:    &contract.SceneAnalysisResultError{Code: diagnostics[0].Code, SafeSummary: diagnostics[0].Summary, RetryClass: "same_release"},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		return contract.VisionReviewAttemptResult{}, err
	}
	return result, result.ValidateFor(invocation, 1, state.Authorization.AuthorizationHash)
}
