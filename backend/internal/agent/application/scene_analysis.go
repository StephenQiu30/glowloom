package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

var ErrNotFound = errors.New("scene Analysis record not found")

type Error struct {
	Code    string
	Message string
}

func (value *Error) Error() string { return value.Message }

func ErrorCode(err error) string {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}

type SourceInput struct {
	WorkspaceID          string
	ProjectID            string
	OwnerKind            string
	LogicalID            string
	VersionID            string
	Revision             int64
	ContentHash          string
	CreatedAt            time.Time
	NormalizedText       string
	NewlineNormalization string
	CodepointIndexRule   string
}

type Candidate struct {
	ID                    string          `json:"id"`
	WorkspaceID           string          `json:"workspace_id"`
	ProjectID             string          `json:"project_id"`
	StageKey              string          `json:"stage_key"`
	ProfileKey            string          `json:"profile_key"`
	StageInstanceKey      string          `json:"stage_instance_key"`
	Revision              int64           `json:"revision"`
	CandidateType         string          `json:"candidate_type"`
	Candidate             json.RawMessage `json:"candidate"`
	CandidateContentHash  string          `json:"candidate_content_hash"`
	CandidateRevisionHash string          `json:"candidate_revision_hash"`
	SourceInvocationID    string          `json:"source_invocation_id"`
	SourceResultID        string          `json:"source_result_id"`
	SourceResultHash      string          `json:"source_result_hash"`
	CreatedAt             time.Time       `json:"created_at"`
}

type ExecuteCommand struct {
	WorkflowRunID                   string
	NodeRunID                       string
	StageKey                        string
	Source                          SourceInput
	Upstreams                       []Candidate
	DeterministicIssues             []contract.CandidateReviewIssue
	Repair                          *contract.StructureIdentityRepairDirective
	ProductionRepair                *contract.ProductionWorldRepairDirective
	StructureIdentitySetVersionID   string
	StructureIdentitySetVersionHash string
	StructureIdentitySet            json.RawMessage
}

type ReleaseRecord struct {
	ID              string
	Identity        contract.SceneAnalysisReleaseIdentity
	Variant         contract.SceneAnalysisStageVariant
	LoadedResources []string
	CreatedAt       time.Time
	InitialControl  contract.SceneAnalysisControlProof
}

type ManifestRecord struct {
	ID, WorkspaceID, WorkflowRunID, NodeRunID string
	StageKey, RootInputHash                   string
	Shards                                    json.RawMessage
	CoverageHash, ManifestHash                string
	CreatedAt                                 time.Time
}

type InvocationRecord struct {
	Invocation                                                  contract.SceneAnalysisInvocation
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID, ReleaseID string
	SourceVersionID, SourceHash                                 string
	Manifest                                                    ManifestRecord
	CreatedAt                                                   time.Time
}

type AttemptRecord struct {
	ID, InvocationID, ControlHash, AgentImageDigest string
	ClaimVersion, ReleaseFence                      int64
	DispatchedAt                                    time.Time
}

type DispatchAuthorizationRecord struct {
	AttemptID, AuthorizationHash string
	ExpiresAt, IssuedAt          time.Time
}

type ResultAcceptance struct {
	ResultID, CandidateID string
	Invocation            contract.SceneAnalysisInvocation
	Result                contract.SceneAnalysisAttemptResult
	AcceptedAt            time.Time
}

type Repository interface {
	EnsureRelease(context.Context, ReleaseRecord) (contract.SceneAnalysisControlProof, error)
	EnsureManifest(context.Context, ManifestRecord) error
	FindInvocation(context.Context, string, string, string, string) (InvocationRecord, error)
	CreateInvocation(context.Context, InvocationRecord) error
	FindCandidateByInvocation(context.Context, string) (Candidate, error)
	CountAttempts(context.Context, string) (int64, error)
	CreateAttempt(context.Context, AttemptRecord) error
	CreateDispatchAuthorization(context.Context, DispatchAuthorizationRecord) error
	AcceptResult(context.Context, ResultAcceptance) (Candidate, error)
	RecordFailedResult(context.Context, ResultAcceptance) error
	GetCandidate(context.Context, string, string) (Candidate, error)
}

type Transactions interface {
	WithinSceneAnalysisTransaction(context.Context, func(Repository) error) error
}

type Runtime interface {
	InvokeSceneAnalysis(
		context.Context,
		contract.SceneAnalysisInvocation,
		contract.SceneAnalysisDispatchAuthorization,
	) (contract.SceneAnalysisAttemptResult, error)
}

type DispatchAuthorizer interface {
	IssueSceneAnalysisDispatchAuthorization(
		contract.SceneAnalysisInvocation,
		int64,
	) (contract.SceneAnalysisDispatchAuthorization, error)
}

type SceneAnalysisConfig struct {
	Now              func() time.Time
	NewID            func() string
	AgentImageDigest string
	Budget           contract.SceneAnalysisExecutionBudget
}

type SceneAnalysisService struct {
	transactions Transactions
	runtime      Runtime
	authorizer   DispatchAuthorizer
	config       SceneAnalysisConfig
}

func NewSceneAnalysisService(
	transactions Transactions,
	runtime Runtime,
	authorizer DispatchAuthorizer,
	config SceneAnalysisConfig,
) (*SceneAnalysisService, error) {
	if transactions == nil || runtime == nil || authorizer == nil ||
		!strings.HasPrefix(config.AgentImageDigest, "sha256:") {
		return nil, errors.New("scene Analysis dependencies are required")
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.NewID == nil {
		config.NewID = uuid.NewString
	}
	if config.Budget == (contract.SceneAnalysisExecutionBudget{}) {
		config.Budget = contract.SceneAnalysisExecutionBudget{
			MaxAttempts: 2, MaxModelCalls: 1, MaxExecutionSeconds: 120, MaxOutputBytes: 131072,
		}
	}
	if config.Budget.Validate() != nil {
		return nil, errors.New("scene Analysis execution budget is invalid")
	}
	return &SceneAnalysisService{
		transactions: transactions, runtime: runtime, authorizer: authorizer, config: config,
	}, nil
}

func (service *SceneAnalysisService) Execute(ctx context.Context, command ExecuteCommand) (Candidate, error) {
	if err := validateExecuteCommand(command); err != nil {
		return Candidate{}, err
	}
	now := service.config.Now().UTC()
	release, err := service.release(command.StageKey, now)
	if err != nil {
		return Candidate{}, err
	}
	manifest, err := buildManifest(command, now)
	if err != nil {
		return Candidate{}, err
	}

	var invocation contract.SceneAnalysisInvocation
	var claimVersion int64
	var authorization contract.SceneAnalysisDispatchAuthorization
	var completed Candidate
	err = service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
		control, ensureErr := repo.EnsureRelease(ctx, release)
		if ensureErr != nil {
			return ensureErr
		}
		if control.Status != "approved" {
			return &Error{Code: "release_not_executable", Message: "Scene Analysis release is not approved"}
		}
		if ensureErr = repo.EnsureManifest(ctx, manifest); ensureErr != nil {
			return ensureErr
		}
		proposedInvocationID, proposedAttemptID := service.config.NewID(), service.config.NewID()
		proposed, buildErr := buildInvocation(command, release, control, manifest, proposedInvocationID, proposedAttemptID, service.config.Budget)
		if buildErr != nil {
			return buildErr
		}
		existing, findErr := repo.FindInvocation(ctx, command.WorkflowRunID, command.NodeRunID, command.StageKey, proposed.InputHash)
		invocationID := proposedInvocationID
		if findErr == nil {
			invocationID = existing.Invocation.InvocationID
			if existing.Invocation.StageRelease != release.Identity || existing.Invocation.Control != control ||
				existing.Invocation.InputHash != proposed.InputHash {
				return &Error{Code: "invocation_fence_drift", Message: "Scene Analysis invocation fence drifted"}
			}
			candidate, candidateErr := repo.FindCandidateByInvocation(ctx, invocationID)
			if candidateErr == nil {
				completed = candidate
				return nil
			}
			if !errors.Is(candidateErr, ErrNotFound) {
				return candidateErr
			}
			if existing.Invocation.InputHash != proposed.InputHash {
				return &Error{Code: "invocation_input_drift", Message: "Scene Analysis invocation input drifted"}
			}
		} else if !errors.Is(findErr, ErrNotFound) {
			return findErr
		}
		attempts, countErr := repo.CountAttempts(ctx, invocationID)
		if countErr != nil && !errors.Is(countErr, ErrNotFound) {
			return countErr
		}
		if attempts >= int64(service.config.Budget.MaxAttempts) {
			return &Error{Code: "attempt_budget_exhausted", Message: "Scene Analysis attempt budget is exhausted"}
		}
		claimVersion = attempts + 1
		attemptID := service.config.NewID()
		invocation, buildErr = buildInvocation(command, release, control, manifest, invocationID, attemptID, service.config.Budget)
		if buildErr != nil {
			return buildErr
		}
		if errors.Is(findErr, ErrNotFound) {
			if createErr := repo.CreateInvocation(ctx, InvocationRecord{
				Invocation: invocation, WorkspaceID: command.Source.WorkspaceID, ProjectID: command.Source.ProjectID,
				WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID, ReleaseID: release.ID,
				SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
				Manifest: manifest, CreatedAt: now,
			}); createErr != nil {
				return createErr
			}
		}
		if createErr := repo.CreateAttempt(ctx, AttemptRecord{
			ID: attemptID, InvocationID: invocationID, ClaimVersion: claimVersion,
			ControlHash: control.ControlHash, ReleaseFence: control.ReleaseFence,
			AgentImageDigest: release.Identity.AgentImageDigest, DispatchedAt: now,
		}); createErr != nil {
			return createErr
		}
		authorization, buildErr = service.authorizer.IssueSceneAnalysisDispatchAuthorization(
			invocation,
			claimVersion,
		)
		if buildErr != nil {
			return buildErr
		}
		return repo.CreateDispatchAuthorization(ctx, DispatchAuthorizationRecord{
			AttemptID: attemptID, AuthorizationHash: authorization.Hash,
			ExpiresAt: authorization.ExpiresAt, IssuedAt: now,
		})
	})
	if err != nil || completed.ID != "" {
		return completed, err
	}

	result, invokeErr := service.runtime.InvokeSceneAnalysis(ctx, invocation, authorization)
	if invokeErr != nil || result.ValidateFor(invocation, claimVersion, authorization.Hash) != nil {
		resultErrorCode := "agent_outcome_unknown"
		resultSummary := "Scene Analysis outcome could not be confirmed"
		if errors.Is(invokeErr, contract.ErrSkillBundleUnavailable) {
			resultErrorCode = "skill_bundle_unavailable"
			resultSummary = "Frozen Scene Analysis skill bundle is unavailable"
		}
		result, err = outcomeUnknownResult(
			invocation,
			authorization,
			service.config.Now().UTC(),
			resultErrorCode,
			resultSummary,
		)
		if err != nil {
			return Candidate{}, err
		}
		acceptance := ResultAcceptance{
			ResultID: service.config.NewID(), CandidateID: service.config.NewID(), Invocation: invocation,
			Result: result, AcceptedAt: result.CompletedAt,
		}
		err = service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
			return repo.RecordFailedResult(ctx, acceptance)
		})
		if err != nil {
			return Candidate{}, err
		}
		return Candidate{}, &Error{Code: resultErrorCode, Message: resultSummary}
	}
	acceptance := ResultAcceptance{
		ResultID: service.config.NewID(), CandidateID: service.config.NewID(), Invocation: invocation,
		Result: result, AcceptedAt: service.config.Now().UTC(),
	}
	if result.Status != "accepted" {
		recordErr := service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
			return repo.RecordFailedResult(ctx, acceptance)
		})
		if recordErr != nil {
			return Candidate{}, recordErr
		}
		return Candidate{}, &Error{Code: result.Error.Code, Message: result.Error.SafeSummary}
	}
	err = service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
		var acceptErr error
		completed, acceptErr = repo.AcceptResult(ctx, acceptance)
		return acceptErr
	})
	return completed, err
}

func outcomeUnknownResult(
	invocation contract.SceneAnalysisInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
	completedAt time.Time,
	errorCode string,
	safeSummary string,
) (contract.SceneAnalysisAttemptResult, error) {
	diagnostics := []contract.SceneAnalysisDiagnostic{{
		Code: errorCode, Summary: safeSummary,
	}}
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(encoded)
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	result := contract.SceneAnalysisAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID, Kind: "storygraph_stage",
		WireSchemaVersion: invocation.WireSchemaVersion, Variant: invocation.Payload.Variant,
		StageRelease: invocation.StageRelease, Control: invocation.Control,
		ClaimVersion: authorization.ClaimVersion, DispatchAuthorizationHash: authorization.Hash,
		Status: "outcome_unknown",
		CandidateType: map[string]string{
			"propose_script_spans":             "script_span_candidate",
			"extract_scene_facts":              "scene_fact_candidate",
			"resolve_identities":               "identity_resolution_candidate",
			"review_candidate":                 "structure_identity_review_candidate",
			"derive_production_entities":       "production_entity_fragment_candidate",
			"bind_scene_occurrences":           "scene_binding_fragment_candidate",
			"reconcile_interaction_continuity": "continuity_fragment_candidate",
		}[invocation.Payload.Variant.StageKey],
		InputHash: invocation.InputHash, Diagnostics: diagnostics, DiagnosticHash: diagnosticHash,
		CompletedAt: completedAt,
		Executor: contract.SceneAnalysisExecutor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "scene-analysis-harness", Model: "unconfirmed",
		},
		Error: &contract.SceneAnalysisResultError{
			Code: errorCode, SafeSummary: safeSummary, RetryClass: "same_release",
		},
	}
	result.ResultHash, err = result.ComputeResultHash()
	if err != nil {
		return contract.SceneAnalysisAttemptResult{}, err
	}
	return result, result.ValidateFor(invocation, authorization.ClaimVersion, authorization.Hash)
}

func (service *SceneAnalysisService) GetCandidate(ctx context.Context, projectID, candidateID string) (Candidate, error) {
	var candidate Candidate
	err := service.transactions.WithinSceneAnalysisTransaction(ctx, func(repo Repository) error {
		var queryErr error
		candidate, queryErr = repo.GetCandidate(ctx, projectID, candidateID)
		return queryErr
	})
	return candidate, err
}

func validateExecuteCommand(command ExecuteCommand) error {
	for _, identifier := range []string{
		command.WorkflowRunID, command.NodeRunID, command.Source.WorkspaceID, command.Source.ProjectID,
		command.Source.LogicalID, command.Source.VersionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return &Error{Code: "invalid_scene_analysis_command", Message: "Scene Analysis command has an invalid identity"}
		}
	}
	if command.Source.OwnerKind != "production/script" || command.Source.Revision < 1 ||
		command.Source.CreatedAt.IsZero() || command.Source.NewlineNormalization != "lf" ||
		command.Source.CodepointIndexRule != "unicode-code-point" || command.Source.NormalizedText == "" ||
		strings.Contains(command.Source.NormalizedText, "\r") || !utf8.ValidString(command.Source.NormalizedText) ||
		len(command.Source.ContentHash) != 64 {
		return &Error{Code: "invalid_scene_analysis_source", Message: "Scene Analysis source is invalid"}
	}
	if command.Repair != nil && command.Repair.ValidateFor(command.StageKey) != nil {
		return &Error{Code: "invalid_scene_analysis_repair", Message: "Scene Analysis repair directive is invalid"}
	}
	if command.ProductionRepair != nil &&
		(command.Repair != nil || command.ProductionRepair.ValidateFor(command.StageKey) != nil) {
		return &Error{Code: "invalid_scene_analysis_repair", Message: "Production World repair directive is invalid"}
	}
	if command.StageKey == "propose_script_spans" {
		if len(command.Upstreams) != 0 {
			return &Error{Code: "unexpected_upstream_candidate", Message: "ScriptSpan stage cannot read an upstream candidate"}
		}
		return nil
	}
	if command.StageKey == "extract_scene_facts" {
		if len(command.Upstreams) != 1 || command.Upstreams[0].ProjectID != command.Source.ProjectID ||
			command.Upstreams[0].CandidateType != "script_span_candidate" ||
			contract.ValidateScriptSpanCandidate(command.Upstreams[0].Candidate, command.Source.NormalizedText) != nil {
			return &Error{Code: "invalid_upstream_candidate", Message: "SceneFact stage requires one exact ScriptSpan candidate"}
		}
		return nil
	}
	if command.StageKey == "resolve_identities" {
		if len(command.Upstreams) != 1 || command.Upstreams[0].ProjectID != command.Source.ProjectID ||
			command.Upstreams[0].CandidateType != "scene_fact_candidate" {
			return &Error{Code: "invalid_upstream_candidate", Message: "IdentityResolution stage requires one exact SceneFact candidate"}
		}
		return nil
	}
	if command.StageKey == "derive_production_entities" {
		if len(command.Upstreams) != 1 || command.Upstreams[0].ProjectID != command.Source.ProjectID ||
			command.Upstreams[0].CandidateType != "scene_fact_candidate" ||
			command.Upstreams[0].StageKey != "extract_scene_facts" {
			return &Error{Code: "invalid_upstream_candidate", Message: "Production Entity stage requires one exact SceneFact candidate"}
		}
		input := contract.ProductionEntityDerivationInput{
			SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
			NormalizedText:                  command.Source.NormalizedText,
			StructureIdentitySetVersionID:   command.StructureIdentitySetVersionID,
			StructureIdentitySetVersionHash: command.StructureIdentitySetVersionHash,
			SceneFactCandidateRevisionID:    command.Upstreams[0].ID,
			SceneFactCandidateRevisionHash:  command.Upstreams[0].CandidateRevisionHash,
			SceneFactCandidate:              command.Upstreams[0].Candidate,
		}
		var decodeErr error
		input.StructureIdentitySet, decodeErr = contract.DecodeFrozenStructureIdentitySet(command.StructureIdentitySet)
		if decodeErr != nil || input.Validate() != nil {
			return &Error{Code: "invalid_formal_identity_input", Message: "Production Entity formal identity input is invalid"}
		}
		return nil
	}
	if command.StageKey == "bind_scene_occurrences" {
		if len(command.Upstreams) != 2 {
			return &Error{Code: "invalid_upstream_candidate", Message: "Scene binding stage requires exact SceneFact and Production Entity candidates"}
		}
		byStage := make(map[string]Candidate, len(command.Upstreams))
		for _, upstream := range command.Upstreams {
			if upstream.ProjectID != command.Source.ProjectID {
				return &Error{Code: "invalid_upstream_candidate", Message: "Scene binding candidate project drifted"}
			}
			byStage[upstream.StageKey] = upstream
		}
		facts, factsFound := byStage["extract_scene_facts"]
		entities, entitiesFound := byStage["derive_production_entities"]
		if !factsFound || !entitiesFound || facts.CandidateType != "scene_fact_candidate" ||
			entities.CandidateType != "production_entity_fragment_candidate" {
			return &Error{Code: "invalid_upstream_candidate", Message: "Scene binding Candidate set drifted"}
		}
		identities, decodeErr := contract.DecodeFrozenStructureIdentitySet(command.StructureIdentitySet)
		input := contract.SceneOccurrenceBindingInput{
			ProductionEntityDerivationInput: contract.ProductionEntityDerivationInput{
				SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
				NormalizedText:                  command.Source.NormalizedText,
				StructureIdentitySetVersionID:   command.StructureIdentitySetVersionID,
				StructureIdentitySetVersionHash: command.StructureIdentitySetVersionHash,
				StructureIdentitySet:            identities,
				SceneFactCandidateRevisionID:    facts.ID,
				SceneFactCandidateRevisionHash:  facts.CandidateRevisionHash,
				SceneFactCandidate:              facts.Candidate,
			},
			ProductionEntityCandidateRevisionID:   entities.ID,
			ProductionEntityCandidateRevisionHash: entities.CandidateRevisionHash,
			ProductionEntityCandidate:             entities.Candidate,
		}
		if decodeErr != nil || input.Validate() != nil {
			return &Error{Code: "invalid_formal_scene_binding_input", Message: "Scene binding frozen input is invalid"}
		}
		return nil
	}
	if command.StageKey == "reconcile_interaction_continuity" {
		if len(command.Upstreams) != 3 {
			return &Error{Code: "invalid_upstream_candidate", Message: "Interaction/Continuity stage requires exact SceneFact, Production Entity, and Scene Binding candidates"}
		}
		byStage := make(map[string]Candidate, len(command.Upstreams))
		for _, upstream := range command.Upstreams {
			if upstream.ProjectID != command.Source.ProjectID {
				return &Error{Code: "invalid_upstream_candidate", Message: "Interaction/Continuity candidate project drifted"}
			}
			byStage[upstream.StageKey] = upstream
		}
		facts, factsFound := byStage["extract_scene_facts"]
		entities, entitiesFound := byStage["derive_production_entities"]
		bindings, bindingsFound := byStage["bind_scene_occurrences"]
		if !factsFound || !entitiesFound || !bindingsFound ||
			facts.CandidateType != "scene_fact_candidate" ||
			entities.CandidateType != "production_entity_fragment_candidate" ||
			bindings.CandidateType != "scene_binding_fragment_candidate" {
			return &Error{Code: "invalid_upstream_candidate", Message: "Interaction/Continuity Candidate set drifted"}
		}
		identities, decodeErr := contract.DecodeFrozenStructureIdentitySet(command.StructureIdentitySet)
		input := contract.InteractionContinuityInput{
			SceneOccurrenceBindingInput: contract.SceneOccurrenceBindingInput{
				ProductionEntityDerivationInput: contract.ProductionEntityDerivationInput{
					SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
					NormalizedText:                  command.Source.NormalizedText,
					StructureIdentitySetVersionID:   command.StructureIdentitySetVersionID,
					StructureIdentitySetVersionHash: command.StructureIdentitySetVersionHash,
					StructureIdentitySet:            identities,
					SceneFactCandidateRevisionID:    facts.ID,
					SceneFactCandidateRevisionHash:  facts.CandidateRevisionHash,
					SceneFactCandidate:              facts.Candidate,
				},
				ProductionEntityCandidateRevisionID:   entities.ID,
				ProductionEntityCandidateRevisionHash: entities.CandidateRevisionHash,
				ProductionEntityCandidate:             entities.Candidate,
			},
			SceneBindingCandidateRevisionID:   bindings.ID,
			SceneBindingCandidateRevisionHash: bindings.CandidateRevisionHash,
			SceneBindingCandidate:             bindings.Candidate,
		}
		if decodeErr != nil || input.Validate() != nil {
			return &Error{Code: "invalid_interaction_continuity_input", Message: "Interaction/Continuity frozen input is invalid"}
		}
		return nil
	}
	if command.StageKey != "review_candidate" || len(command.Upstreams) != 3 ||
		command.DeterministicIssues == nil {
		return &Error{Code: "invalid_upstream_candidate", Message: "StructureIdentityReview stage requires three exact candidates"}
	}
	expected := map[string]string{
		"script_span_candidate":         "propose_script_spans",
		"scene_fact_candidate":          "extract_scene_facts",
		"identity_resolution_candidate": "resolve_identities",
	}
	for _, upstream := range command.Upstreams {
		stage, exists := expected[upstream.CandidateType]
		if !exists || upstream.StageKey != stage || upstream.ProjectID != command.Source.ProjectID {
			return &Error{Code: "invalid_upstream_candidate", Message: "StructureIdentityReview candidate set drifted"}
		}
		delete(expected, upstream.CandidateType)
	}
	if len(expected) != 0 {
		return &Error{Code: "invalid_upstream_candidate", Message: "StructureIdentityReview candidate set is incomplete"}
	}
	return nil
}

func (service *SceneAnalysisService) release(stageKey string, now time.Time) (ReleaseRecord, error) {
	stageReleases, err := contract.BuildSceneAnalysisStageReleases(service.config.AgentImageDigest)
	if err != nil {
		return ReleaseRecord{}, err
	}
	stageIndex := slices.IndexFunc(stageReleases, func(candidate contract.SceneAnalysisStageRelease) bool {
		return candidate.VariantKey.StageKey == stageKey
	})
	if stageIndex < 0 {
		return ReleaseRecord{}, errors.New("unsupported Scene Analysis stage")
	}
	stageRelease := stageReleases[stageIndex]
	stageReleaseHashes := make([]string, len(stageReleases))
	for index, release := range stageReleases {
		stageReleaseHashes[index] = release.StageReleaseHash
	}
	core, _, err := contract.BuildSceneAnalysisDefinitionCore()
	if err != nil {
		return ReleaseRecord{}, err
	}
	skillMaterial, err := json.Marshal(struct {
		ContractID         string   `json:"contract_id"`
		DefinitionCoreHash string   `json:"definition_core_hash"`
		BundleContentHash  string   `json:"bundle_content_hash"`
		RuntimeImageDigest string   `json:"runtime_image_digest"`
		WireSchemaHash     string   `json:"wire_schema_hash"`
		StageReleaseHashes []string `json:"stage_release_hashes"`
	}{
		ContractID: "scene-analysis-skill-release-production", DefinitionCoreHash: core.DefinitionCoreHash,
		BundleContentHash: stageRelease.BundleContentHash, RuntimeImageDigest: stageRelease.RuntimeImageDigest,
		WireSchemaHash: stageRelease.WireSchemaHash, StageReleaseHashes: stageReleaseHashes,
	})
	if err != nil {
		return ReleaseRecord{}, err
	}
	skillHash, err := platformcanonical.Hash(skillMaterial)
	if err != nil {
		return ReleaseRecord{}, err
	}
	skillID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:scene-analysis:skill:"+skillHash)).String()
	stageHash := stageRelease.StageReleaseHash
	releaseID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:scene-analysis:stage:"+stageHash)).String()
	controlRecordID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("lanverse:scene-analysis:control:"+stageHash)).String()
	controlMaterial, _ := json.Marshal(map[string]any{
		"contract_id": "scene-analysis-control", "control_record_id": controlRecordID,
		"release_id": releaseID, "stage_release_hash": stageHash, "control_revision": 1,
		"status": "approved", "release_fence": 0,
	})
	controlHash, err := platformcanonical.Hash(controlMaterial)
	if err != nil {
		return ReleaseRecord{}, err
	}
	loadedResources, err := contract.SceneAnalysisLoadedResourcePaths(stageRelease)
	if err != nil {
		return ReleaseRecord{}, err
	}
	return ReleaseRecord{
		ID: releaseID,
		Identity: contract.SceneAnalysisReleaseIdentity{
			SkillReleaseID: skillID, SkillReleaseHash: skillHash, StageReleaseHash: stageHash,
			BundleContentHash: stageRelease.BundleContentHash, AgentImageDigest: stageRelease.RuntimeImageDigest,
		},
		Variant: stageRelease.VariantKey, LoadedResources: loadedResources, CreatedAt: now,
		InitialControl: contract.SceneAnalysisControlProof{
			ControlRecordID: controlRecordID, ControlRevision: 1, Status: "approved",
			ControlHash: controlHash, ReleaseFence: 0,
		},
	}, nil
}

func buildManifest(command ExecuteCommand, now time.Time) (ManifestRecord, error) {
	rootInputHash := command.Source.ContentHash
	if len(command.Upstreams) == 1 {
		rootInputHash = command.Upstreams[0].CandidateRevisionHash
	} else if len(command.Upstreams) > 1 {
		roots := make([]string, len(command.Upstreams))
		for index, upstream := range command.Upstreams {
			roots[index] = upstream.StageKey + "\x00" + upstream.CandidateRevisionHash
		}
		slices.Sort(roots)
		encoded, _ := json.Marshal(roots)
		var hashErr error
		rootInputHash, hashErr = platformcanonical.Hash(encoded)
		if hashErr != nil {
			return ManifestRecord{}, hashErr
		}
	}
	if command.StageKey == "derive_production_entities" || command.StageKey == "bind_scene_occurrences" ||
		command.StageKey == "reconcile_interaction_continuity" {
		encoded, marshalErr := json.Marshal(struct {
			SceneFactHash, StructureIdentitySetHash string
		}{rootInputHash, command.StructureIdentitySetVersionHash})
		if marshalErr != nil {
			return ManifestRecord{}, marshalErr
		}
		var hashErr error
		rootInputHash, hashErr = platformcanonical.Hash(encoded)
		if hashErr != nil {
			return ManifestRecord{}, hashErr
		}
	}
	if command.Repair != nil {
		encoded, marshalErr := json.Marshal(struct {
			RootInputHash string                                     `json:"root_input_hash"`
			Repair        *contract.StructureIdentityRepairDirective `json:"repair"`
		}{RootInputHash: rootInputHash, Repair: command.Repair})
		if marshalErr != nil {
			return ManifestRecord{}, marshalErr
		}
		var hashErr error
		rootInputHash, hashErr = platformcanonical.Hash(encoded)
		if hashErr != nil {
			return ManifestRecord{}, hashErr
		}
	}
	if command.ProductionRepair != nil {
		encoded, marshalErr := json.Marshal(struct {
			RootInputHash string                                   `json:"root_input_hash"`
			Repair        *contract.ProductionWorldRepairDirective `json:"production_world_repair"`
		}{RootInputHash: rootInputHash, Repair: command.ProductionRepair})
		if marshalErr != nil {
			return ManifestRecord{}, marshalErr
		}
		var hashErr error
		rootInputHash, hashErr = platformcanonical.Hash(encoded)
		if hashErr != nil {
			return ManifestRecord{}, hashErr
		}
	}
	manifestID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf(
		"lanverse:scene-analysis:manifest:%s:%s:%s", command.NodeRunID, command.StageKey, rootInputHash,
	))).String()
	shardKey := "script:full"
	shards, _ := json.Marshal([]map[string]any{{
		"shard_key": shardKey, "codepoint_start": 0, "codepoint_end": utf8.RuneCountInString(command.Source.NormalizedText),
	}})
	coverageHash, err := platformcanonical.Hash(shards)
	if err != nil {
		return ManifestRecord{}, err
	}
	material, _ := json.Marshal(map[string]any{
		"contract_id": "scene-analysis-shard-manifest", "manifest_id": manifestID,
		"workflow_run_id": command.WorkflowRunID, "node_run_id": command.NodeRunID,
		"stage_key": command.StageKey, "root_input_hash": rootInputHash,
		"shards": json.RawMessage(shards), "coverage_hash": coverageHash,
	})
	manifestHash, err := platformcanonical.Hash(material)
	if err != nil {
		return ManifestRecord{}, err
	}
	return ManifestRecord{
		ID: manifestID, WorkspaceID: command.Source.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
		NodeRunID: command.NodeRunID, StageKey: command.StageKey, RootInputHash: rootInputHash,
		Shards: shards, CoverageHash: coverageHash, ManifestHash: manifestHash, CreatedAt: now,
	}, nil
}

func buildInvocation(
	command ExecuteCommand,
	release ReleaseRecord,
	control contract.SceneAnalysisControlProof,
	manifest ManifestRecord,
	invocationID, attemptID string,
	budget contract.SceneAnalysisExecutionBudget,
) (contract.SceneAnalysisInvocation, error) {
	text := command.Source.NormalizedText
	payload := contract.SceneAnalysisPayload{
		Variant: release.Variant,
		Scope:   contract.SceneAnalysisScope{WorkspaceID: command.Source.WorkspaceID, ProjectID: command.Source.ProjectID},
		SourceRefs: []contract.ScriptSourceVersionIdentity{{
			OwnerKind: command.Source.OwnerKind, LogicalID: command.Source.LogicalID, VersionID: command.Source.VersionID,
			Revision: command.Source.Revision, ContentHash: command.Source.ContentHash, CreatedAt: command.Source.CreatedAt.UTC(),
		}},
		Shard: contract.SceneAnalysisShard{
			ManifestID: manifest.ID, ManifestHash: manifest.ManifestHash, ShardKey: "script:full",
			CodepointStart: 0, CodepointEnd: utf8.RuneCountInString(text),
		},
		ProductionRepair: command.ProductionRepair,
	}
	if command.StageKey == "propose_script_spans" {
		payload.UpstreamCandidates = []contract.SceneAnalysisCandidateRevisionIdentity{}
		payload.StageInput, _ = json.Marshal(contract.ScriptSpanProposalInput{
			SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
			NormalizedText: text, CodepointCount: utf8.RuneCountInString(text), NewlineNormalization: "lf",
			Repair: command.Repair,
		})
	} else if command.StageKey == "extract_scene_facts" {
		upstream := command.Upstreams[0]
		payload.UpstreamCandidates = []contract.SceneAnalysisCandidateRevisionIdentity{{
			StageKey: upstream.StageKey, ShardKey: "script:full",
			CandidateRevisionID: upstream.ID, CandidateRevisionHash: upstream.CandidateRevisionHash,
			SourceInvocationID: upstream.SourceInvocationID, SourceResultHash: upstream.SourceResultHash,
		}}
		payload.StageInput, _ = json.Marshal(contract.SceneFactExtractionInput{
			SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
			NormalizedText: text, SpanCandidateRevisionID: upstream.ID,
			SpanCandidateRevisionHash: upstream.CandidateRevisionHash,
			SpanCandidate:             upstream.Candidate,
		})
	} else if command.StageKey == "resolve_identities" {
		upstream := command.Upstreams[0]
		payload.UpstreamCandidates = []contract.SceneAnalysisCandidateRevisionIdentity{{
			StageKey: upstream.StageKey, ShardKey: "script:full",
			CandidateRevisionID: upstream.ID, CandidateRevisionHash: upstream.CandidateRevisionHash,
			SourceInvocationID: upstream.SourceInvocationID, SourceResultHash: upstream.SourceResultHash,
		}}
		payload.StageInput, _ = json.Marshal(contract.IdentityResolutionInput{
			SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
			NormalizedText: text, SceneFactCandidateRevisionID: upstream.ID,
			SceneFactCandidateRevisionHash: upstream.CandidateRevisionHash,
			SceneFactCandidate:             upstream.Candidate, AllowedReuseIdentityKeys: []string{},
			Repair: command.Repair,
		})
	} else if command.StageKey == "derive_production_entities" {
		upstream := command.Upstreams[0]
		payload.UpstreamCandidates = []contract.SceneAnalysisCandidateRevisionIdentity{{
			StageKey: upstream.StageKey, ShardKey: "script:full",
			CandidateRevisionID: upstream.ID, CandidateRevisionHash: upstream.CandidateRevisionHash,
			SourceInvocationID: upstream.SourceInvocationID, SourceResultHash: upstream.SourceResultHash,
		}}
		identities, err := contract.DecodeFrozenStructureIdentitySet(command.StructureIdentitySet)
		if err != nil {
			return contract.SceneAnalysisInvocation{}, err
		}
		payload.StageInput, _ = json.Marshal(contract.ProductionEntityDerivationInput{
			SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
			NormalizedText:                  text,
			StructureIdentitySetVersionID:   command.StructureIdentitySetVersionID,
			StructureIdentitySetVersionHash: command.StructureIdentitySetVersionHash,
			StructureIdentitySet:            identities,
			SceneFactCandidateRevisionID:    upstream.ID,
			SceneFactCandidateRevisionHash:  upstream.CandidateRevisionHash,
			SceneFactCandidate:              upstream.Candidate,
		})
	} else if command.StageKey == "bind_scene_occurrences" {
		byStage := make(map[string]Candidate, len(command.Upstreams))
		payload.UpstreamCandidates = make([]contract.SceneAnalysisCandidateRevisionIdentity, 0, len(command.Upstreams))
		for _, upstream := range command.Upstreams {
			byStage[upstream.StageKey] = upstream
			payload.UpstreamCandidates = append(payload.UpstreamCandidates, contract.SceneAnalysisCandidateRevisionIdentity{
				StageKey: upstream.StageKey, ShardKey: "script:full",
				CandidateRevisionID: upstream.ID, CandidateRevisionHash: upstream.CandidateRevisionHash,
				SourceInvocationID: upstream.SourceInvocationID, SourceResultHash: upstream.SourceResultHash,
			})
		}
		facts, entities := byStage["extract_scene_facts"], byStage["derive_production_entities"]
		identities, err := contract.DecodeFrozenStructureIdentitySet(command.StructureIdentitySet)
		if err != nil {
			return contract.SceneAnalysisInvocation{}, err
		}
		payload.StageInput, _ = json.Marshal(contract.SceneOccurrenceBindingInput{
			ProductionEntityDerivationInput: contract.ProductionEntityDerivationInput{
				SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
				NormalizedText:                  text,
				StructureIdentitySetVersionID:   command.StructureIdentitySetVersionID,
				StructureIdentitySetVersionHash: command.StructureIdentitySetVersionHash,
				StructureIdentitySet:            identities,
				SceneFactCandidateRevisionID:    facts.ID,
				SceneFactCandidateRevisionHash:  facts.CandidateRevisionHash,
				SceneFactCandidate:              facts.Candidate,
			},
			ProductionEntityCandidateRevisionID:   entities.ID,
			ProductionEntityCandidateRevisionHash: entities.CandidateRevisionHash,
			ProductionEntityCandidate:             entities.Candidate,
		})
	} else if command.StageKey == "reconcile_interaction_continuity" {
		byStage := make(map[string]Candidate, len(command.Upstreams))
		payload.UpstreamCandidates = make([]contract.SceneAnalysisCandidateRevisionIdentity, 0, len(command.Upstreams))
		for _, upstream := range command.Upstreams {
			byStage[upstream.StageKey] = upstream
			payload.UpstreamCandidates = append(payload.UpstreamCandidates, contract.SceneAnalysisCandidateRevisionIdentity{
				StageKey: upstream.StageKey, ShardKey: "script:full",
				CandidateRevisionID: upstream.ID, CandidateRevisionHash: upstream.CandidateRevisionHash,
				SourceInvocationID: upstream.SourceInvocationID, SourceResultHash: upstream.SourceResultHash,
			})
		}
		facts := byStage["extract_scene_facts"]
		entities := byStage["derive_production_entities"]
		bindings := byStage["bind_scene_occurrences"]
		identities, err := contract.DecodeFrozenStructureIdentitySet(command.StructureIdentitySet)
		if err != nil {
			return contract.SceneAnalysisInvocation{}, err
		}
		payload.StageInput, _ = json.Marshal(contract.InteractionContinuityInput{
			SceneOccurrenceBindingInput: contract.SceneOccurrenceBindingInput{
				ProductionEntityDerivationInput: contract.ProductionEntityDerivationInput{
					SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
					NormalizedText:                  text,
					StructureIdentitySetVersionID:   command.StructureIdentitySetVersionID,
					StructureIdentitySetVersionHash: command.StructureIdentitySetVersionHash,
					StructureIdentitySet:            identities,
					SceneFactCandidateRevisionID:    facts.ID,
					SceneFactCandidateRevisionHash:  facts.CandidateRevisionHash,
					SceneFactCandidate:              facts.Candidate,
				},
				ProductionEntityCandidateRevisionID:   entities.ID,
				ProductionEntityCandidateRevisionHash: entities.CandidateRevisionHash,
				ProductionEntityCandidate:             entities.Candidate,
			},
			SceneBindingCandidateRevisionID:   bindings.ID,
			SceneBindingCandidateRevisionHash: bindings.CandidateRevisionHash,
			SceneBindingCandidate:             bindings.Candidate,
		})
	} else {
		byStage := make(map[string]Candidate, len(command.Upstreams))
		payload.UpstreamCandidates = make([]contract.SceneAnalysisCandidateRevisionIdentity, 0, len(command.Upstreams))
		for _, upstream := range command.Upstreams {
			byStage[upstream.StageKey] = upstream
			payload.UpstreamCandidates = append(payload.UpstreamCandidates, contract.SceneAnalysisCandidateRevisionIdentity{
				StageKey: upstream.StageKey, ShardKey: "script:full",
				CandidateRevisionID: upstream.ID, CandidateRevisionHash: upstream.CandidateRevisionHash,
				SourceInvocationID: upstream.SourceInvocationID, SourceResultHash: upstream.SourceResultHash,
			})
		}
		spans, facts, identities := byStage["propose_script_spans"], byStage["extract_scene_facts"], byStage["resolve_identities"]
		payload.StageInput, _ = json.Marshal(contract.StructureIdentityReviewInput{
			SourceVersionID: command.Source.VersionID, SourceHash: command.Source.ContentHash,
			NormalizedText:          text,
			SpanCandidateRevisionID: spans.ID, SpanCandidateRevisionHash: spans.CandidateRevisionHash,
			SpanCandidate:                spans.Candidate,
			SceneFactCandidateRevisionID: facts.ID, SceneFactCandidateRevisionHash: facts.CandidateRevisionHash,
			SceneFactCandidate:          facts.Candidate,
			IdentityCandidateRevisionID: identities.ID, IdentityCandidateRevisionHash: identities.CandidateRevisionHash,
			IdentityCandidate:   identities.Candidate,
			DeterministicIssues: command.DeterministicIssues,
		})
	}
	return contract.NewSceneAnalysisInvocation(invocationID, attemptID, release.Identity, control, budget, payload)
}
