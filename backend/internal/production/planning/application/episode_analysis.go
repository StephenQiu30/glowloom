package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	bibledomain "github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

type EpisodeAnalysisCommand struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	EpisodeSetID, EpisodeSetHash                     string
	BibleVersionID                                   string
	BibleVersion                                     int
	MaterializationHash                              string
}

type EpisodeAnalysisState struct {
	Status                                     string
	CandidateRevisionID, CandidateRevisionHash string
	CandidateRevisionNo                        int64
}

type EpisodeAnalysisEpisodeSeed struct {
	Source             domain.EpisodeAnalysisSource
	ScriptVersionNo    int
	DocumentRevisionID string
}

type EpisodeAnalysisSeed struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	EpisodeSetID, EpisodeSetHash                     string
	Episodes                                         []EpisodeAnalysisEpisodeSeed
	BibleVersionID, BibleContentHash                 string
	BibleVersion                                     int
	BibleSnapshotHash                                string
	BibleSnapshot                                    json.RawMessage
	MaterializationHash                              string
	KnownIdentities                                  []agentcontract.EpisodeKnownIdentity
}

type EpisodeAnalysisPreparation struct {
	Command           EpisodeAnalysisCommand
	Seed              EpisodeAnalysisSeed
	CreatedAt         time.Time
	AnalyzeManifest   domain.EpisodeAnalysisManifest
	ReconcileManifest domain.EpisodeReconcileManifest
	Invocations       []bibledomain.Invocation
}

type EpisodeAnalysisRepository interface {
	LoadEpisodeAnalysisSeed(context.Context, EpisodeAnalysisCommand) (EpisodeAnalysisSeed, error)
	EnsureEpisodeAnalysis(context.Context, EpisodeAnalysisPreparation) (EpisodeAnalysisState, error)
}

type EpisodeAnalysisConfig struct {
	Now                                   func() time.Time
	NewID                                 func() string
	MaxShardCodePoints, OverlapCodePoints int
	AdjacentCodePoints, FanIn             int
}

type EpisodeAnalysisService struct {
	repository EpisodeAnalysisRepository
	config     EpisodeAnalysisConfig
}

var (
	ErrEpisodeAnalysisUpstreamStale = errors.New("Episode analysis upstream reference is stale")
	ErrEpisodeAnalysisManifestStale = errors.New("Episode analysis Shard Manifest is stale")
	episodeAnalysisHashPattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func NewEpisodeAnalysisService(
	repository EpisodeAnalysisRepository,
	config EpisodeAnalysisConfig,
) *EpisodeAnalysisService {
	if config.MaxShardCodePoints == 0 {
		config.MaxShardCodePoints = 8_000
	}
	if config.OverlapCodePoints == 0 {
		config.OverlapCodePoints = 200
	}
	if config.AdjacentCodePoints == 0 {
		config.AdjacentCodePoints = 200
	}
	if config.FanIn == 0 {
		config.FanIn = 2
	}
	return &EpisodeAnalysisService{repository: repository, config: config}
}

func (service *EpisodeAnalysisService) Ensure(
	ctx context.Context,
	command EpisodeAnalysisCommand,
) (EpisodeAnalysisState, error) {
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.NewID == nil {
		return EpisodeAnalysisState{}, errors.New("Episode analysis service is unavailable")
	}
	for _, identifier := range []string{
		command.WorkspaceID, command.ProjectID, command.WorkflowRunID, command.NodeRunID,
		command.EpisodeSetID, command.BibleVersionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return EpisodeAnalysisState{}, errors.New("invalid Episode analysis workflow identity")
		}
	}
	if command.BibleVersion < 1 || service.config.MaxShardCodePoints < 1 ||
		service.config.OverlapCodePoints < 0 || service.config.AdjacentCodePoints < 1 || service.config.FanIn != 2 ||
		!episodeAnalysisHashPattern.MatchString(command.EpisodeSetHash) ||
		!episodeAnalysisHashPattern.MatchString(command.MaterializationHash) {
		return EpisodeAnalysisState{}, errors.New("invalid Episode analysis exact input")
	}
	seed, err := service.repository.LoadEpisodeAnalysisSeed(ctx, command)
	if err != nil {
		return EpisodeAnalysisState{}, err
	}
	if seed.WorkspaceID != command.WorkspaceID || seed.ProjectID != command.ProjectID ||
		seed.WorkflowRunID != command.WorkflowRunID || seed.NodeRunID != command.NodeRunID ||
		seed.EpisodeSetID != command.EpisodeSetID || seed.EpisodeSetHash != command.EpisodeSetHash ||
		seed.BibleVersionID != command.BibleVersionID || seed.BibleVersion != command.BibleVersion ||
		seed.MaterializationHash != command.MaterializationHash {
		return EpisodeAnalysisState{}, ErrEpisodeAnalysisUpstreamStale
	}
	rootInputHash, err := EpisodeAnalysisRootInputHash(seed)
	if err != nil {
		return EpisodeAnalysisState{}, err
	}
	sources := make([]domain.EpisodeAnalysisSource, len(seed.Episodes))
	for index := range seed.Episodes {
		sources[index] = seed.Episodes[index].Source
	}
	analyze, reconcile, err := domain.BuildEpisodeAnalysisManifests(domain.EpisodeAnalysisManifestInput{
		AnalyzeManifestID:   episodeAnalysisManifestID(seed.NodeRunID, domain.AnalyzeEpisodeStage),
		ReconcileManifestID: episodeAnalysisManifestID(seed.NodeRunID, domain.ReconcileEpisodeStage),
		WorkspaceID:         seed.WorkspaceID, WorkflowRunID: seed.WorkflowRunID, NodeRunID: seed.NodeRunID,
		RootInputHash: rootInputHash, MaxShardCodePoints: service.config.MaxShardCodePoints,
		OverlapCodePoints: service.config.OverlapCodePoints, FanIn: service.config.FanIn,
		Episodes: sources,
	})
	if err != nil {
		return EpisodeAnalysisState{}, err
	}
	createdAt := service.config.Now().UTC()
	invocations, err := buildEpisodeAnalysisInvocations(
		analyze, seed, service.config.AdjacentCodePoints, service.config.NewID, createdAt,
	)
	if err != nil {
		return EpisodeAnalysisState{}, err
	}
	return service.repository.EnsureEpisodeAnalysis(ctx, EpisodeAnalysisPreparation{
		Command: command, Seed: seed, CreatedAt: createdAt,
		AnalyzeManifest: analyze, ReconcileManifest: reconcile, Invocations: invocations,
	})
}

func episodeAnalysisManifestID(nodeRunID, stage string) string {
	return uuid.NewSHA1(uuid.MustParse(nodeRunID), []byte("episode-analysis-manifest-v1\x00"+stage)).String()
}

func EpisodeAnalysisRootInputHash(seed EpisodeAnalysisSeed) (string, error) {
	episodes := append([]EpisodeAnalysisEpisodeSeed(nil), seed.Episodes...)
	slices.SortFunc(episodes, func(left, right EpisodeAnalysisEpisodeSeed) int {
		return left.Source.EpisodePosition - right.Source.EpisodePosition
	})
	type episodeRef struct {
		EpisodeID, ScriptVersionID, DocumentRevisionID, ContentHash string
		EpisodePosition, ScriptVersionNo, SourceStart, SourceEnd    int
	}
	references := make([]episodeRef, len(episodes))
	for index, episode := range episodes {
		references[index] = episodeRef{
			episode.Source.EpisodeID, episode.Source.ScriptVersionID,
			episode.DocumentRevisionID, episode.Source.ContentHash,
			episode.Source.EpisodePosition, episode.ScriptVersionNo,
			episode.Source.SourceStart, episode.Source.SourceEnd,
		}
	}
	encoded, err := json.Marshal(struct {
		Schema              string       `json:"schema"`
		EpisodeSetID        string       `json:"episode_set_id"`
		EpisodeSetHash      string       `json:"episode_set_hash"`
		BibleVersionID      string       `json:"bible_version_id"`
		BibleContentHash    string       `json:"bible_content_hash"`
		BibleVersion        int          `json:"bible_version"`
		BibleSnapshotHash   string       `json:"bible_snapshot_hash"`
		MaterializationHash string       `json:"materialization_hash"`
		Episodes            []episodeRef `json:"episodes"`
	}{
		"episode-analysis-input-v1", seed.EpisodeSetID, seed.EpisodeSetHash,
		seed.BibleVersionID, seed.BibleContentHash, seed.BibleVersion,
		seed.BibleSnapshotHash, seed.MaterializationHash, references,
	})
	if err != nil {
		return "", err
	}
	return agentcontract.CanonicalHash(encoded)
}

func buildEpisodeAnalysisInvocations(
	manifest domain.EpisodeAnalysisManifest,
	seed EpisodeAnalysisSeed,
	adjacentCodePoints int,
	newID func() string,
	createdAt time.Time,
) ([]bibledomain.Invocation, error) {
	episodes := append([]EpisodeAnalysisEpisodeSeed(nil), seed.Episodes...)
	slices.SortFunc(episodes, func(left, right EpisodeAnalysisEpisodeSeed) int {
		return left.Source.EpisodePosition - right.Source.EpisodePosition
	})
	byEpisode := make(map[string]int, len(episodes))
	for index, episode := range episodes {
		byEpisode[episode.Source.EpisodeID] = index
	}
	policy := agentcontract.StoryGraphDefinition().ExecutionPolicy()
	result := make([]bibledomain.Invocation, 0, len(manifest.Shards))
	for _, shard := range manifest.Shards {
		index, exists := byEpisode[shard.EpisodeID]
		if !exists || shard.Status != "active" {
			return nil, errors.New("Episode analysis manifest lost its published Episode")
		}
		episode := episodes[index]
		content := []rune(episode.Source.Content)
		contextStart := shard.ContextStart - episode.Source.SourceStart
		contextEnd := shard.ContextEnd - episode.Source.SourceStart
		logicalStart := shard.LogicalStart - episode.Source.SourceStart
		logicalEnd := shard.LogicalEnd - episode.Source.SourceStart
		markers := make([]agentcontract.EpisodeSceneMarkerHint, 0, len(episode.Source.SceneMarkers))
		for _, marker := range episode.Source.SceneMarkers {
			if marker.AbsoluteStart >= shard.ContextStart && marker.AbsoluteEnd <= shard.ContextEnd {
				markers = append(markers, agentcontract.EpisodeSceneMarkerHint{
					Label: marker.Label, AbsoluteStart: marker.AbsoluteStart, AbsoluteEnd: marker.AbsoluteEnd,
				})
			}
		}
		adjacent := episodeAdjacentContexts(episodes, index, adjacentCodePoints)
		stageInput, err := json.Marshal(agentcontract.EpisodeAnalysisStageInput{
			EpisodeID: episode.Source.EpisodeID, EpisodePosition: episode.Source.EpisodePosition,
			ScriptVersionID: episode.Source.ScriptVersionID, ScriptVersionNo: episode.ScriptVersionNo,
			DocumentRevisionID: episode.DocumentRevisionID,
			EpisodeSourceStart: episode.Source.SourceStart, EpisodeSourceEnd: episode.Source.SourceEnd,
			ScriptContentHash: episode.Source.ContentHash,
			LogicalStart:      shard.LogicalStart, LogicalEnd: shard.LogicalEnd,
			ContextStart: shard.ContextStart, ContextEnd: shard.ContextEnd,
			ContextText:      string(content[contextStart:contextEnd]),
			LogicalTextHash:  bibledomain.SourceTextHash(string(content[logicalStart:logicalEnd])),
			SceneMarkerHints: markers, AdjacentEpisodes: adjacent,
			BibleVersionID: seed.BibleVersionID, BibleVersion: seed.BibleVersion,
			BibleContentHash: seed.BibleContentHash, BibleSnapshotHash: seed.BibleSnapshotHash,
			BibleSnapshot: seed.BibleSnapshot, MaterializationHash: seed.MaterializationHash,
			KnownIdentities: seed.KnownIdentities,
		})
		if err != nil {
			return nil, err
		}
		sourceRefs := []agentcontract.StageSourceRef{
			{OwnerKind: "production/episode-script", OwnerLogicalID: episode.Source.EpisodeID,
				OwnerVersionID: episode.Source.ScriptVersionID, Revision: int64(episode.ScriptVersionNo),
				ContentHash: episode.Source.ContentHash},
			{OwnerKind: "production/bible-version", OwnerLogicalID: seed.BibleVersionID,
				OwnerVersionID: seed.BibleVersionID, Revision: int64(seed.BibleVersion), ContentHash: seed.BibleContentHash},
			{OwnerKind: "production/bible-materialization", OwnerLogicalID: seed.BibleVersionID,
				OwnerVersionID: seed.BibleVersionID, Revision: int64(seed.BibleVersion), ContentHash: seed.MaterializationHash},
		}
		for _, neighbor := range adjacent {
			sourceRefs = append(sourceRefs, agentcontract.StageSourceRef{
				OwnerKind: "production/episode-script", OwnerLogicalID: neighbor.EpisodeID,
				OwnerVersionID: neighbor.ScriptVersionID, Revision: int64(neighbor.ScriptVersionNo),
				ContentHash: neighbor.ContentHash,
			})
		}
		start, end := shard.LogicalStart, shard.LogicalEnd
		invocationID := newID()
		request, err := agentcontract.NewStageInvocation(
			invocationID,
			policy,
			agentcontract.StageInvocationPayload{
				Stage: domain.AnalyzeEpisodeStage, ShardKey: shard.Key,
				WorkspaceID: seed.WorkspaceID, ProjectID: seed.ProjectID,
				SourceRefs: sourceRefs, UpstreamCandidates: nil,
				ShardManifestRef: agentcontract.ShardManifestRef{
					ManifestID: manifest.ManifestID, Version: manifest.Version, Hash: manifest.ManifestHash,
				},
				Shard: agentcontract.InvocationShard{
					Kind: shard.Kind, Key: shard.Key, TreePath: shard.TreePath, ParentKey: shard.ParentKey,
					AbsoluteStart: &start, AbsoluteEnd: &end,
				},
				StageInput: stageInput,
			},
		)
		if err != nil {
			return nil, err
		}
		if err = agentcontract.ValidateEpisodeAnalysisInvocation(request); err != nil {
			return nil, err
		}
		policyJSON, err := json.Marshal(request.ExecutionPolicy)
		if err != nil {
			return nil, err
		}
		payloadJSON, err := json.Marshal(request.Payload)
		if err != nil {
			return nil, err
		}
		stageKey, err := request.StageInstanceKey()
		if err != nil {
			return nil, err
		}
		result = append(result, bibledomain.Invocation{
			ID: invocationID, WorkspaceID: seed.WorkspaceID,
			RequestType: "episode_analysis_shard", RequestID: invocationID,
			WorkflowRunID: seed.WorkflowRunID, NodeRunID: seed.NodeRunID,
			ManifestID: manifest.ManifestID, ManifestVersion: manifest.Version,
			Kind: "storygraph_stage", Stage: domain.AnalyzeEpisodeStage, ShardKey: shard.Key,
			InputHash: request.InputHash, StageInstanceKey: stageKey,
			ManifestHash: manifest.ManifestHash, ExecutionPolicy: policyJSON, Payload: payloadJSON,
			Status: "queued", CreatedAt: createdAt,
		})
	}
	return result, nil
}

func episodeAdjacentContexts(
	episodes []EpisodeAnalysisEpisodeSeed,
	index int,
	maximum int,
) []agentcontract.EpisodeAdjacentContext {
	result := make([]agentcontract.EpisodeAdjacentContext, 0, 2)
	if index > 0 {
		previous := episodes[index-1]
		runes := []rune(previous.Source.Content)
		length := min(maximum, len(runes))
		result = append(result, agentcontract.EpisodeAdjacentContext{
			Side: "previous", EpisodeID: previous.Source.EpisodeID,
			EpisodePosition: previous.Source.EpisodePosition,
			ScriptVersionID: previous.Source.ScriptVersionID, ScriptVersionNo: previous.ScriptVersionNo,
			SourceStart: previous.Source.SourceStart, SourceEnd: previous.Source.SourceEnd,
			ContentHash:  previous.Source.ContentHash,
			ExcerptStart: previous.Source.SourceEnd - length, ExcerptEnd: previous.Source.SourceEnd,
			Excerpt:     string(runes[len(runes)-length:]),
			ExcerptHash: bibledomain.SourceTextHash(string(runes[len(runes)-length:])),
		})
	}
	if index+1 < len(episodes) {
		next := episodes[index+1]
		runes := []rune(next.Source.Content)
		length := min(maximum, len(runes))
		result = append(result, agentcontract.EpisodeAdjacentContext{
			Side: "next", EpisodeID: next.Source.EpisodeID, EpisodePosition: next.Source.EpisodePosition,
			ScriptVersionID: next.Source.ScriptVersionID, ScriptVersionNo: next.ScriptVersionNo,
			SourceStart: next.Source.SourceStart, SourceEnd: next.Source.SourceEnd,
			ContentHash:  next.Source.ContentHash,
			ExcerptStart: next.Source.SourceStart, ExcerptEnd: next.Source.SourceStart + length,
			Excerpt: string(runes[:length]), ExcerptHash: bibledomain.SourceTextHash(string(runes[:length])),
		})
	}
	return result
}
