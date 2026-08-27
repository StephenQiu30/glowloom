package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type StoryAnalysisCommand struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	EvidenceCandidateRevisionID                      string
	EvidenceCandidateRevisionHash                    string
}

var ErrStoryAnalysisUpstreamStale = errors.New("Story analysis upstream Candidate Revision is stale")

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
		evidence, exists := byRevision[shard.UpstreamCandidateRevision]
		if !exists || evidence.Fragment.ShardKey != shard.EvidenceShardKey ||
			evidence.Fragment.CandidateRevisionHash != shard.UpstreamCandidateHash {
			return nil, errors.New("Story analysis manifest lost its Evidence seed")
		}
		evidenceJSON, err := json.Marshal(evidence.Candidate)
		if err != nil {
			return nil, err
		}
		stageInput, err := json.Marshal(agentcontract.StoryAnalysisStageInput{
			EvidenceShardKey:              shard.EvidenceShardKey,
			EvidenceCandidateRevisionID:   shard.UpstreamCandidateRevision,
			EvidenceCandidateRevisionHash: shard.UpstreamCandidateHash,
			LogicalStart:                  shard.LogicalStart, LogicalEnd: shard.LogicalEnd,
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
