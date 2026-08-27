package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformcommand "github.com/StephenQiu30/lanverse/backend/internal/platform/command"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type StoryAnalysisCommand struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	EvidenceCandidateRevisionID                      string
	EvidenceCandidateRevisionHash                    string
}

var (
	ErrStoryAnalysisUpstreamStale = errors.New("Story analysis upstream Candidate Revision is stale")
	ErrStoryAnalysisManifestStale = errors.New("Story analysis Shard Manifest is stale")
)

type StoryAnalysisState struct {
	Status                                     string
	CandidateRevisionID, CandidateRevisionHash string
	CandidateRevisionNo                        int64
}

type StoryAnalysisEvidenceSeed struct {
	Fragment  domain.StoryAnalysisEvidenceFragment
	Candidate domain.SourceEvidenceCandidate
	SourceRef agentcontract.StageSourceRef
	Upstream  agentcontract.StageUpstreamCandidateRef
}

type StoryAnalysisSeed struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	RootInputHash                                    string
	Evidence                                         []StoryAnalysisEvidenceSeed
}

type StoryAnalysisPreparation struct {
	Command           StoryAnalysisCommand
	CreatedAt         time.Time
	AnalyzeManifest   domain.StoryAnalysisManifest
	ReconcileManifest domain.StoryReconcileManifest
	Invocations       []domain.Invocation
}

type StoryAnalysisRepository interface {
	LoadStoryAnalysisSeed(context.Context, StoryAnalysisCommand) (StoryAnalysisSeed, error)
	EnsureStoryAnalysis(context.Context, StoryAnalysisPreparation) (StoryAnalysisState, error)
}

const StoryAnalysisRecoveryOperation = "story_analysis.recover"

type StoryAnalysisRecoveryCommand struct {
	WorkflowRunID, NodeRunID, IdempotencyKey string
}

type StoryAnalysisRecovery struct {
	ReceiptID            string `json:"receipt_id"`
	WorkflowRunID        string `json:"workflow_run_id"`
	NodeRunID            string `json:"node_run_id"`
	InvocationID         string `json:"invocation_id"`
	Stage                string `json:"stage"`
	ShardKey             string `json:"shard_key"`
	Status               string `json:"status"`
	FailureCode          string `json:"failure_code"`
	PreviousClaimVersion int    `json:"previous_claim_version"`
}

type StoryAnalysisRecoveryPreparation struct {
	Command   StoryAnalysisRecoveryCommand
	InputHash string
	ReceiptID string
	CreatedAt time.Time
}

type StoryAnalysisRecoveryRepository interface {
	RecoverStoryAnalysis(context.Context, Actor, StoryAnalysisRecoveryPreparation) (StoryAnalysisRecovery, error)
}

type StoryAnalysisConfig struct {
	Now   func() time.Time
	NewID func() string
	FanIn int
}

type StoryAnalysisService struct {
	repository StoryAnalysisRepository
	config     StoryAnalysisConfig
}

func NewStoryAnalysisService(repository StoryAnalysisRepository, config StoryAnalysisConfig) *StoryAnalysisService {
	if config.FanIn == 0 {
		config.FanIn = 2
	}
	return &StoryAnalysisService{repository: repository, config: config}
}

func (service *StoryAnalysisService) Ensure(ctx context.Context, command StoryAnalysisCommand) (StoryAnalysisState, error) {
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.NewID == nil {
		return StoryAnalysisState{}, errors.New("Story analysis service is unavailable")
	}
	for _, identifier := range []string{
		command.WorkspaceID, command.ProjectID, command.WorkflowRunID, command.NodeRunID,
		command.EvidenceCandidateRevisionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return StoryAnalysisState{}, errors.New("invalid Story analysis workflow identity")
		}
	}
	if service.config.FanIn != 2 || !storyAnalysisHashPattern.MatchString(command.EvidenceCandidateRevisionHash) {
		return StoryAnalysisState{}, errors.New("invalid Story analysis input")
	}
	seed, err := service.repository.LoadStoryAnalysisSeed(ctx, command)
	if err != nil {
		return StoryAnalysisState{}, err
	}
	fragments := make([]domain.StoryAnalysisEvidenceFragment, len(seed.Evidence))
	for index := range seed.Evidence {
		fragments[index] = seed.Evidence[index].Fragment
		fragments[index].CandidateItemCount = domain.SourceEvidenceCandidateItemCount(seed.Evidence[index].Candidate)
	}
	analyze, reconcile, err := domain.BuildStoryAnalysisManifests(domain.StoryAnalysisManifestInput{
		AnalyzeManifestID: service.config.NewID(), ReconcileManifestID: service.config.NewID(),
		WorkspaceID: command.WorkspaceID, WorkflowRunID: command.WorkflowRunID,
		NodeRunID: command.NodeRunID, RootInputHash: seed.RootInputHash,
		FanIn: service.config.FanIn, EvidenceFragments: fragments,
	})
	if err != nil {
		return StoryAnalysisState{}, err
	}
	createdAt := service.config.Now().UTC()
	invocations, err := buildStoryAnalysisInvocations(analyze, seed, service.config.NewID, createdAt)
	if err != nil {
		return StoryAnalysisState{}, err
	}
	return service.repository.EnsureStoryAnalysis(ctx, StoryAnalysisPreparation{
		Command: command, CreatedAt: createdAt, AnalyzeManifest: analyze,
		ReconcileManifest: reconcile, Invocations: invocations,
	})
}

func (service *StoryAnalysisService) Recover(
	ctx context.Context,
	actor Actor,
	command StoryAnalysisRecoveryCommand,
) (StoryAnalysisRecovery, error) {
	command.WorkflowRunID = strings.TrimSpace(command.WorkflowRunID)
	command.NodeRunID = strings.TrimSpace(command.NodeRunID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	actor.UserID = strings.TrimSpace(actor.UserID)
	if service == nil || service.config.Now == nil || service.config.NewID == nil || actor.UserID == "" ||
		actor.TokenVersion < 1 || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 200 {
		return StoryAnalysisRecovery{}, invalid("Invalid Story analysis recovery request")
	}
	if _, err := uuid.Parse(command.WorkflowRunID); err != nil {
		return StoryAnalysisRecovery{}, invalid("Invalid Story analysis recovery request")
	}
	if _, err := uuid.Parse(command.NodeRunID); err != nil {
		return StoryAnalysisRecovery{}, invalid("Invalid Story analysis recovery request")
	}
	repository, supported := service.repository.(StoryAnalysisRecoveryRepository)
	if !supported {
		return StoryAnalysisRecovery{}, errors.New("Story analysis recovery service is unavailable")
	}
	inputHash, err := platformcommand.InputHash(command)
	if err != nil {
		return StoryAnalysisRecovery{}, err
	}
	result, err := repository.RecoverStoryAnalysis(ctx, actor, StoryAnalysisRecoveryPreparation{
		Command: command, InputHash: inputHash,
		ReceiptID: service.config.NewID(), CreatedAt: service.config.Now().UTC(),
	})
	if errors.Is(err, ErrNotFound) {
		return StoryAnalysisRecovery{}, &Error{
			Code: "not_found", Message: "Story analysis recovery target not found", Status: 404,
		}
	}
	return result, err
}

func buildStoryAnalysisInvocations(
	manifest domain.StoryAnalysisManifest,
	seed StoryAnalysisSeed,
	newID func() string,
	createdAt time.Time,
) ([]domain.Invocation, error) {
	byRevision := make(map[string]StoryAnalysisEvidenceSeed, len(seed.Evidence))
	for _, evidence := range seed.Evidence {
		byRevision[evidence.Fragment.CandidateRevisionID] = evidence
	}
	result := make([]domain.Invocation, 0, len(manifest.Shards))
	policy := agentcontract.StoryGraphDefinition().ExecutionPolicy()
	for _, shard := range manifest.Shards {
		if shard.Status != "active" {
			continue
		}
		evidence, exists := byRevision[shard.UpstreamCandidateRevision]
		if !exists || evidence.Fragment.ShardKey != shard.EvidenceShardKey ||
			evidence.Fragment.CandidateRevisionHash != shard.UpstreamCandidateHash {
			return nil, errors.New("Story analysis manifest lost its Evidence seed")
		}
		candidate, err := domain.SliceSourceEvidenceCandidate(
			evidence.Candidate, shard.CandidateItemStart, shard.CandidateItemEnd,
		)
		if err != nil {
			return nil, err
		}
		evidenceJSON, err := json.Marshal(candidate)
		if err != nil {
			return nil, err
		}
		stageInput, err := json.Marshal(agentcontract.StoryAnalysisStageInput{
			EvidenceShardKey:              shard.EvidenceShardKey,
			EvidenceCandidateRevisionID:   shard.UpstreamCandidateRevision,
			EvidenceCandidateRevisionHash: shard.UpstreamCandidateHash,
			LogicalStart:                  shard.LogicalStart, LogicalEnd: shard.LogicalEnd,
			CandidateItemStart: shard.CandidateItemStart, CandidateItemEnd: shard.CandidateItemEnd,
			EvidenceCandidate: evidenceJSON,
		})
		if err != nil {
			return nil, err
		}
		start, end := shard.LogicalStart, shard.LogicalEnd
		invocationID := newID()
		invocation, err := agentcontract.NewStageInvocation(invocationID, policy, agentcontract.StageInvocationPayload{
			Stage: domain.AnalyzeStoryStage, ShardKey: shard.Key,
			WorkspaceID: seed.WorkspaceID, ProjectID: seed.ProjectID,
			SourceRefs:         []agentcontract.StageSourceRef{evidence.SourceRef},
			UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{evidence.Upstream},
			ShardManifestRef: agentcontract.ShardManifestRef{
				ManifestID: manifest.ManifestID, Version: manifest.Version, Hash: manifest.ManifestHash,
			},
			Shard: agentcontract.InvocationShard{
				Kind: shard.Kind, Key: shard.Key, TreePath: shard.TreePath, ParentKey: shard.ParentKey,
				AbsoluteStart: &start, AbsoluteEnd: &end,
			},
			StageInput: stageInput,
		})
		if err != nil {
			return nil, err
		}
		if err = agentcontract.ValidateStoryAnalysisInvocation(invocation); err != nil {
			return nil, err
		}
		policyJSON, _ := json.Marshal(invocation.ExecutionPolicy)
		payloadJSON, _ := json.Marshal(invocation.Payload)
		stageKey, err := invocation.StageInstanceKey()
		if err != nil {
			return nil, err
		}
		result = append(result, domain.Invocation{
			ID: invocationID, WorkspaceID: seed.WorkspaceID,
			RequestType: "story_analysis_shard", RequestID: invocationID,
			WorkflowRunID: seed.WorkflowRunID, NodeRunID: seed.NodeRunID,
			ManifestID: manifest.ManifestID, ManifestVersion: manifest.Version,
			Kind: "storygraph_stage", Stage: domain.AnalyzeStoryStage, ShardKey: shard.Key,
			InputHash: invocation.InputHash, StageInstanceKey: stageKey,
			ManifestHash: manifest.ManifestHash, ExecutionPolicy: policyJSON, Payload: payloadJSON,
			Status: "queued", CreatedAt: createdAt,
		})
	}
	return result, nil
}

var storyAnalysisHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type StoryAnalysisReshardSeed struct {
	Stage, ShardKey         string
	ProjectID               string
	AnalyzeManifest         domain.StoryAnalysisManifest
	ReconcileManifest       domain.StoryReconcileManifest
	Evidence                []StoryAnalysisEvidenceSeed
	ReconcileCandidateSizes []domain.StoryReconcileCandidateSize
}

type StoryAnalysisReshardPreparation struct {
	InvocationID, ErrorCode, ErrorSummary string
	ClaimVersion                          int
	CreatedAt                             time.Time
	PreviousAnalyzeManifestHash           string
	PreviousReconcileManifestHash         string
	AnalyzeManifest                       *domain.StoryAnalysisManifest
	ReconcileManifest                     domain.StoryReconcileManifest
	Invocations                           []domain.Invocation
}

type StoryAnalysisReshardRepository interface {
	LoadStoryAnalysisReshardSeed(context.Context, string, int, time.Time) (StoryAnalysisReshardSeed, error)
	ApplyStoryAnalysisReshard(context.Context, StoryAnalysisReshardPreparation) (bool, error)
}

func (service *StoryAnalysisService) ReshardBudgetExceeded(
	ctx context.Context,
	invocationID string,
	claimVersion int,
	summary string,
) (bool, error) {
	if service == nil || service.config.Now == nil || service.config.NewID == nil {
		return false, errors.New("Story analysis reshard service is unavailable")
	}
	repository, ok := service.repository.(StoryAnalysisReshardRepository)
	if !ok {
		return false, errors.New("Story analysis reshard service is unavailable")
	}
	if _, err := uuid.Parse(invocationID); err != nil || claimVersion < 1 {
		return false, errors.New("invalid Story analysis reshard identity")
	}
	createdAt := service.config.Now().UTC()
	seed, err := repository.LoadStoryAnalysisReshardSeed(ctx, invocationID, claimVersion, createdAt)
	if err != nil {
		return false, err
	}
	preparation := StoryAnalysisReshardPreparation{
		InvocationID: invocationID, ClaimVersion: claimVersion,
		ErrorCode: "execution_budget_exceeded", ErrorSummary: summary, CreatedAt: createdAt,
		PreviousAnalyzeManifestHash:   seed.AnalyzeManifest.ManifestHash,
		PreviousReconcileManifestHash: seed.ReconcileManifest.ManifestHash,
	}
	switch seed.Stage {
	case domain.AnalyzeStoryStage:
		nextAnalyze, nextReconcile, buildErr := domain.ReshardStoryAnalysisMap(
			seed.AnalyzeManifest, seed.ReconcileManifest, seed.ShardKey,
		)
		if buildErr != nil {
			return false, buildErr
		}
		invocations, buildErr := buildStoryAnalysisInvocations(
			nextAnalyze,
			StoryAnalysisSeed{
				WorkspaceID: nextAnalyze.WorkspaceID, WorkflowRunID: nextAnalyze.WorkflowRunID,
				NodeRunID: nextAnalyze.NodeRunID, RootInputHash: nextAnalyze.RootInputHash,
				Evidence:  seed.Evidence,
				ProjectID: seed.ProjectID,
			},
			service.config.NewID,
			createdAt,
		)
		if buildErr != nil {
			return false, buildErr
		}
		preparation.AnalyzeManifest = &nextAnalyze
		preparation.ReconcileManifest = nextReconcile
		preparation.Invocations = invocations
	case domain.ReconcileStoryStage:
		nextReconcile, buildErr := domain.ReshardStoryReconcile(
			seed.ReconcileManifest, seed.ShardKey, seed.ReconcileCandidateSizes,
		)
		if buildErr != nil {
			return false, buildErr
		}
		preparation.ReconcileManifest = nextReconcile
	default:
		return false, errors.New("unsupported Story analysis reshard stage")
	}
	return repository.ApplyStoryAnalysisReshard(ctx, preparation)
}
