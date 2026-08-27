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
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type SourceEvidenceCommand struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	DocumentLogicalID, DocumentRevisionID            string
	DocumentRevision                                 int64
	NormalizedText, NormalizedHash                   string
}

type SourceEvidenceState struct {
	Status                                     string
	ManifestID, ManifestHash                   string
	ManifestVersion                            int64
	CandidateRevisionID, CandidateRevisionHash string
	CandidateRevisionNo                        int64
}

type SourceEvidencePreparation struct {
	ProjectID          string
	DocumentLogicalID  string
	DocumentRevisionID string
	DocumentRevision   int64
	NormalizedText     string
	CreatedAt          time.Time
	Manifest           domain.SourceEvidenceManifest
	Invocations        []domain.Invocation
}

type SourceEvidenceRepository interface {
	EnsureSourceEvidence(context.Context, SourceEvidencePreparation) (SourceEvidenceState, error)
}

type SourceEvidenceConfig struct {
	Now                func() time.Time
	NewID              func() string
	MaxShardCodePoints int
	OverlapCodePoints  int
}

type SourceEvidenceService struct {
	repository SourceEvidenceRepository
	config     SourceEvidenceConfig
}

func NewSourceEvidenceService(repository SourceEvidenceRepository, config SourceEvidenceConfig) *SourceEvidenceService {
	if config.MaxShardCodePoints == 0 {
		config.MaxShardCodePoints = 8_000
	}
	if config.OverlapCodePoints == 0 {
		config.OverlapCodePoints = 200
	}
	return &SourceEvidenceService{repository: repository, config: config}
}

func (service *SourceEvidenceService) Ensure(
	ctx context.Context,
	command SourceEvidenceCommand,
) (SourceEvidenceState, error) {
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.NewID == nil {
		return SourceEvidenceState{}, errors.New("source Evidence service is unavailable")
	}
	for _, identifier := range []string{
		command.WorkspaceID, command.ProjectID, command.WorkflowRunID, command.NodeRunID,
		command.DocumentLogicalID, command.DocumentRevisionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return SourceEvidenceState{}, errors.New("invalid source Evidence workflow identity")
		}
	}
	if command.DocumentRevision < 1 || !sourceEvidenceHashPattern.MatchString(command.NormalizedHash) ||
		strings.TrimSpace(command.NormalizedText) == "" || service.config.MaxShardCodePoints < 1 ||
		service.config.OverlapCodePoints < 0 {
		return SourceEvidenceState{}, errors.New("invalid source Evidence source revision")
	}
	manifest, err := domain.BuildSourceEvidenceManifest(domain.SourceEvidenceManifestInput{
		ManifestID: service.config.NewID(), WorkspaceID: command.WorkspaceID,
		WorkflowRunID: command.WorkflowRunID, NodeRunID: command.NodeRunID,
		RootInputHash: command.NormalizedHash, NormalizedText: command.NormalizedText,
		MaxShardCodePoints: service.config.MaxShardCodePoints,
		OverlapCodePoints:  service.config.OverlapCodePoints,
	})
	if err != nil {
		return SourceEvidenceState{}, err
	}
	createdAt := service.config.Now().UTC()
	invocations, err := buildSourceEvidenceInvocations(
		manifest,
		SourceEvidenceSource{
			ProjectID: command.ProjectID, DocumentLogicalID: command.DocumentLogicalID,
			DocumentRevisionID: command.DocumentRevisionID, DocumentRevision: command.DocumentRevision,
			NormalizedText: command.NormalizedText, NormalizedHash: command.NormalizedHash,
		},
		agentcontract.StoryGraphDefinition().ExecutionPolicy(),
		service.config.NewID,
		createdAt,
	)
	if err != nil {
		return SourceEvidenceState{}, err
	}
	return service.repository.EnsureSourceEvidence(ctx, SourceEvidencePreparation{
		ProjectID: command.ProjectID, DocumentLogicalID: command.DocumentLogicalID,
		DocumentRevisionID: command.DocumentRevisionID, DocumentRevision: command.DocumentRevision,
		NormalizedText: command.NormalizedText, CreatedAt: createdAt,
		Manifest: manifest, Invocations: invocations,
	})
}

type SourceEvidenceSource struct {
	ProjectID, DocumentLogicalID, DocumentRevisionID string
	DocumentRevision                                 int64
	NormalizedText, NormalizedHash                   string
}

func buildSourceEvidenceInvocations(
	manifest domain.SourceEvidenceManifest,
	source SourceEvidenceSource,
	policy agentcontract.StageExecutionPolicy,
	newID func() string,
	createdAt time.Time,
) ([]domain.Invocation, error) {
	if err := domain.ValidateSourceEvidenceManifest(manifest, source.NormalizedText); err != nil {
		return nil, err
	}
	text := []rune(source.NormalizedText)
	result := make([]domain.Invocation, 0, len(manifest.Shards))
	for _, shard := range manifest.Shards {
		if shard.Status != "active" {
			continue
		}
		invocationID := newID()
		markerHints := make([]agentcontract.SourceEvidenceEpisodeMarkerHint, 0, len(shard.EpisodeMarkerHints))
		for _, marker := range shard.EpisodeMarkerHints {
			markerHints = append(markerHints, agentcontract.SourceEvidenceEpisodeMarkerHint{
				EpisodeNumber: marker.EpisodeNumber, Label: marker.Label,
				AbsoluteStart: marker.AbsoluteStart, AbsoluteEnd: marker.AbsoluteEnd,
			})
		}
		stageInput, err := json.Marshal(agentcontract.SourceEvidenceStageInput{
			DocumentRevisionID: source.DocumentRevisionID, NormalizedHash: source.NormalizedHash,
			LogicalSourceHash: shard.SourceHashes[0],
			LogicalStart:      shard.LogicalStart, LogicalEnd: shard.LogicalEnd,
			ContextStart: shard.ContextStart, ContextEnd: shard.ContextEnd,
			NormalizedText:     string(text[shard.ContextStart:shard.ContextEnd]),
			EpisodeMarkerHints: markerHints,
		})
		if err != nil {
			return nil, err
		}
		start, end := shard.LogicalStart, shard.LogicalEnd
		invocation, err := agentcontract.NewStageInvocation(invocationID, policy, agentcontract.StageInvocationPayload{
			Stage: domain.SourceEvidenceStage, ShardKey: shard.Key,
			WorkspaceID: manifest.WorkspaceID, ProjectID: source.ProjectID,
			SourceRefs: []agentcontract.StageSourceRef{{
				OwnerKind: "production/script", OwnerLogicalID: source.DocumentLogicalID,
				OwnerVersionID: source.DocumentRevisionID, Revision: source.DocumentRevision,
				ContentHash: source.NormalizedHash,
			}},
			UpstreamCandidates: []agentcontract.StageUpstreamCandidateRef{},
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
		policyJSON, err := json.Marshal(invocation.ExecutionPolicy)
		if err != nil {
			return nil, err
		}
		payloadJSON, err := json.Marshal(invocation.Payload)
		if err != nil {
			return nil, err
		}
		stageInstanceKey, err := invocation.StageInstanceKey()
		if err != nil {
			return nil, err
		}
		result = append(result, domain.Invocation{
			ID: invocationID, WorkspaceID: manifest.WorkspaceID,
			RequestType: "source_evidence_shard", RequestID: invocationID,
			WorkflowRunID: manifest.WorkflowRunID, NodeRunID: manifest.NodeRunID,
			ManifestID: manifest.ManifestID, ManifestVersion: manifest.Version,
			Kind: "storygraph_stage", Stage: domain.SourceEvidenceStage, ShardKey: shard.Key,
			InputHash: invocation.InputHash, StageInstanceKey: stageInstanceKey,
			ManifestHash: manifest.ManifestHash, ExecutionPolicy: policyJSON, Payload: payloadJSON,
			Status: "queued", CreatedAt: createdAt,
		})
	}
	if len(result) == 0 {
		return nil, errors.New("source Evidence manifest produced no active invocations")
	}
	return result, nil
}

var sourceEvidenceHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var ErrSourceEvidenceShardCannotSplit = errors.New("source Evidence shard cannot be split further")

type SourceEvidenceReshardSeed struct {
	Manifest       domain.SourceEvidenceManifest
	ParentShardKey string
	Source         SourceEvidenceSource
}

type SourceEvidenceReshardPreparation struct {
	InvocationID, ErrorCode, ErrorSummary string
	ClaimVersion                          int
	CreatedAt                             time.Time
	PreviousManifestHash                  string
	Manifest                              domain.SourceEvidenceManifest
	Invocations                           []domain.Invocation
}

type SourceEvidenceReshardRepository interface {
	LoadSourceEvidenceReshardSeed(context.Context, string, int, time.Time) (SourceEvidenceReshardSeed, error)
	ApplySourceEvidenceReshard(context.Context, SourceEvidenceReshardPreparation) (bool, error)
}

func (service *SourceEvidenceService) ReshardBudgetExceeded(
	ctx context.Context,
	invocationID string,
	claimVersion int,
	summary string,
) (bool, error) {
	if service == nil || service.config.Now == nil || service.config.NewID == nil {
		return false, errors.New("source Evidence reshard service is unavailable")
	}
	repository, ok := service.repository.(SourceEvidenceReshardRepository)
	if !ok {
		return false, errors.New("source Evidence reshard service is unavailable")
	}
	if _, err := uuid.Parse(invocationID); err != nil || claimVersion < 1 {
		return false, errors.New("invalid source Evidence reshard identity")
	}
	createdAt := service.config.Now().UTC()
	seed, err := repository.LoadSourceEvidenceReshardSeed(ctx, invocationID, claimVersion, createdAt)
	if err != nil {
		return false, err
	}
	parentLength := 0
	for _, shard := range seed.Manifest.Shards {
		if shard.Key == seed.ParentShardKey && shard.Status == "active" {
			parentLength = shard.LogicalEnd - shard.LogicalStart
			break
		}
	}
	if parentLength <= 1 {
		return false, ErrSourceEvidenceShardCannotSplit
	}
	maxChildCodePoints := (parentLength + 1) / 2
	next, err := domain.ReshardSourceEvidenceManifest(
		seed.Manifest,
		seed.ParentShardKey,
		seed.Source.NormalizedText,
		maxChildCodePoints,
		service.config.OverlapCodePoints,
	)
	if err != nil {
		return false, err
	}
	invocations, err := buildSourceEvidenceInvocations(
		next,
		seed.Source,
		agentcontract.StoryGraphDefinition().ExecutionPolicy(),
		service.config.NewID,
		createdAt,
	)
	if err != nil {
		return false, err
	}
	return repository.ApplySourceEvidenceReshard(ctx, SourceEvidenceReshardPreparation{
		InvocationID: invocationID, ClaimVersion: claimVersion,
		ErrorCode: "execution_budget_exceeded", ErrorSummary: summary,
		CreatedAt: createdAt, PreviousManifestHash: seed.Manifest.ManifestHash,
		Manifest: next, Invocations: invocations,
	})
}
