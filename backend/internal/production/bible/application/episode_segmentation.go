package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

type EpisodeSegmentationCommand struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	DocumentRevisionID, DocumentRevisionHash         string
	EvidenceCandidateRevisionID                      string
	EvidenceCandidateRevisionHash                    string
	BibleVersionID, MaterializationHash              string
	BibleVersion                                     int
}

type EpisodeSegmentationState struct {
	Status                                     string
	ManifestID, ManifestHash                   string
	ManifestVersion                            int64
	CandidateRevisionID, CandidateRevisionHash string
	CandidateRevisionNo                        int64
}

type EpisodeSegmentationEvidenceSeed struct {
	ShardKey, CandidateRevisionID, CandidateRevisionHash string
	SourceInvocationID, SourceResultHash                 string
	LogicalStart, LogicalEnd                             int
	Candidate                                            domain.SourceEvidenceCandidate
}

type EpisodeSegmentationSeed struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	DocumentLogicalID, DocumentRevisionID            string
	DocumentRevision                                 int64
	NormalizedText, NormalizedHash                   string
	TargetDurationMS                                 int
	BibleVersionID, BibleContentHash                 string
	BibleVersion                                     int
	MaterializationHash                              string
	EvidenceAggregateRevisionID                      string
	EvidenceAggregateRevisionHash                    string
	Evidence                                         []EpisodeSegmentationEvidenceSeed
	Markers                                          []domain.EpisodeSegmentationMarker
}

type EpisodeSegmentationPreparation struct {
	Command    EpisodeSegmentationCommand
	Seed       EpisodeSegmentationSeed
	CreatedAt  time.Time
	Manifest   domain.EpisodeSegmentationManifest
	Invocation domain.Invocation
}

type EpisodeSegmentationRepository interface {
	LoadEpisodeSegmentationSeed(context.Context, EpisodeSegmentationCommand) (EpisodeSegmentationSeed, error)
	EnsureEpisodeSegmentation(context.Context, EpisodeSegmentationPreparation) (EpisodeSegmentationState, error)
}

type EpisodeSegmentationConfig struct {
	Now   func() time.Time
	NewID func() string
}

type EpisodeSegmentationService struct {
	repository EpisodeSegmentationRepository
	config     EpisodeSegmentationConfig
}

var ErrEpisodeSegmentationCandidateInvalid = errors.New("Episode segmentation candidate is invalid")

func NewEpisodeSegmentationService(
	repository EpisodeSegmentationRepository,
	config EpisodeSegmentationConfig,
) *EpisodeSegmentationService {
	return &EpisodeSegmentationService{repository: repository, config: config}
}

func (service *EpisodeSegmentationService) Ensure(
	ctx context.Context,
	command EpisodeSegmentationCommand,
) (EpisodeSegmentationState, error) {
	if service == nil || service.repository == nil || service.config.Now == nil || service.config.NewID == nil {
		return EpisodeSegmentationState{}, errors.New("Episode segmentation service is unavailable")
	}
	for _, identifier := range []string{
		command.WorkspaceID, command.ProjectID, command.WorkflowRunID, command.NodeRunID,
		command.DocumentRevisionID, command.EvidenceCandidateRevisionID, command.BibleVersionID,
	} {
		if _, err := uuid.Parse(identifier); err != nil {
			return EpisodeSegmentationState{}, errors.New("invalid Episode segmentation workflow identity")
		}
	}
	if command.BibleVersion < 1 || !sourceEvidenceHashPattern.MatchString(command.DocumentRevisionHash) ||
		!sourceEvidenceHashPattern.MatchString(command.EvidenceCandidateRevisionHash) ||
		!sourceEvidenceHashPattern.MatchString(command.MaterializationHash) {
		return EpisodeSegmentationState{}, errors.New("invalid Episode segmentation exact input")
	}
	seed, err := service.repository.LoadEpisodeSegmentationSeed(ctx, command)
	if err != nil {
		return EpisodeSegmentationState{}, err
	}
	rootInputHash, err := EpisodeSegmentationRootInputHash(seed)
	if err != nil {
		return EpisodeSegmentationState{}, err
	}
	manifest, err := domain.BuildEpisodeSegmentationManifest(domain.EpisodeSegmentationManifestInput{
		ManifestID: service.config.NewID(), WorkspaceID: seed.WorkspaceID,
		WorkflowRunID: seed.WorkflowRunID, NodeRunID: seed.NodeRunID,
		RootInputHash: rootInputHash, SourceCodePoints: len([]rune(seed.NormalizedText)),
	})
	if err != nil {
		return EpisodeSegmentationState{}, err
	}
	createdAt := service.config.Now().UTC()
	invocation, err := buildEpisodeSegmentationInvocation(
		manifest, seed, agentcontract.StoryGraphDefinition().ExecutionPolicy(), service.config.NewID(), createdAt,
	)
	if err != nil {
		return EpisodeSegmentationState{}, err
	}
	return service.repository.EnsureEpisodeSegmentation(ctx, EpisodeSegmentationPreparation{
		Command: command, Seed: seed, CreatedAt: createdAt, Manifest: manifest, Invocation: invocation,
	})
}

func EpisodeSegmentationRootInputHash(seed EpisodeSegmentationSeed) (string, error) {
	leaves := append([]EpisodeSegmentationEvidenceSeed(nil), seed.Evidence...)
	slices.SortFunc(leaves, func(left, right EpisodeSegmentationEvidenceSeed) int {
		return strings.Compare(left.ShardKey, right.ShardKey)
	})
	type leafRef struct {
		ShardKey              string `json:"shard_key"`
		CandidateRevisionID   string `json:"candidate_revision_id"`
		CandidateRevisionHash string `json:"candidate_revision_hash"`
	}
	references := make([]leafRef, 0, len(leaves))
	for _, leaf := range leaves {
		references = append(references, leafRef{leaf.ShardKey, leaf.CandidateRevisionID, leaf.CandidateRevisionHash})
	}
	encoded, err := json.Marshal(struct {
		SchemaVersion               string    `json:"schema_version"`
		DocumentRevisionID          string    `json:"document_revision_id"`
		NormalizedHash              string    `json:"normalized_hash"`
		EvidenceAggregateRevisionID string    `json:"evidence_aggregate_revision_id"`
		EvidenceAggregateHash       string    `json:"evidence_aggregate_hash"`
		BibleVersionID              string    `json:"bible_version_id"`
		BibleContentHash            string    `json:"bible_content_hash"`
		MaterializationHash         string    `json:"materialization_hash"`
		BibleVersion                int       `json:"bible_version"`
		TargetDurationMS            int       `json:"target_duration_ms"`
		SourceCodePoints            int       `json:"source_code_points"`
		EvidenceLeaves              []leafRef `json:"evidence_leaves"`
	}{
		SchemaVersion: "episode-segmentation-input", DocumentRevisionID: seed.DocumentRevisionID,
		NormalizedHash: seed.NormalizedHash, EvidenceAggregateRevisionID: seed.EvidenceAggregateRevisionID,
		EvidenceAggregateHash: seed.EvidenceAggregateRevisionHash, BibleVersionID: seed.BibleVersionID,
		BibleVersion: seed.BibleVersion, BibleContentHash: seed.BibleContentHash,
		MaterializationHash: seed.MaterializationHash, TargetDurationMS: seed.TargetDurationMS,
		SourceCodePoints: len([]rune(seed.NormalizedText)), EvidenceLeaves: references,
	})
	if err != nil {
		return "", err
	}
	return agentcontract.CanonicalHash(encoded)
}

func buildEpisodeSegmentationInvocation(
	manifest domain.EpisodeSegmentationManifest,
	seed EpisodeSegmentationSeed,
	policy agentcontract.StageExecutionPolicy,
	invocationID string,
	createdAt time.Time,
) (domain.Invocation, error) {
	if err := domain.ValidateEpisodeSegmentationManifest(manifest); err != nil {
		return domain.Invocation{}, err
	}
	leaves := append([]EpisodeSegmentationEvidenceSeed(nil), seed.Evidence...)
	slices.SortFunc(leaves, func(left, right EpisodeSegmentationEvidenceSeed) int {
		return strings.Compare(left.ShardKey, right.ShardKey)
	})
	inputLeaves := make([]agentcontract.EpisodeSegmentationEvidenceLeaf, 0, len(leaves))
	upstreams := make([]agentcontract.StageUpstreamCandidateRef, 0, len(leaves))
	for _, leaf := range leaves {
		inputLeaves = append(inputLeaves, agentcontract.EpisodeSegmentationEvidenceLeaf{
			ShardKey: leaf.ShardKey, CandidateRevisionID: leaf.CandidateRevisionID,
			CandidateRevisionHash: leaf.CandidateRevisionHash,
		})
		upstreams = append(upstreams, agentcontract.StageUpstreamCandidateRef{
			Stage: domain.SourceEvidenceStage, ShardKey: leaf.ShardKey,
			CandidateRevisionID: leaf.CandidateRevisionID, CandidateRevisionHash: leaf.CandidateRevisionHash,
			SourceInvocationID: leaf.SourceInvocationID, SourceResultHash: leaf.SourceResultHash,
		})
	}
	markers, index, err := buildEpisodeSegmentationEvidenceIndex(seed, leaves)
	if err != nil {
		return domain.Invocation{}, err
	}
	stageInput, err := json.Marshal(agentcontract.EpisodeSegmentationStageInput{
		DocumentRevisionID: seed.DocumentRevisionID, NormalizedHash: seed.NormalizedHash,
		SourceCodePoints: len([]rune(seed.NormalizedText)), TargetDurationMS: seed.TargetDurationMS,
		BibleVersionID: seed.BibleVersionID, BibleVersion: seed.BibleVersion,
		BibleContentHash: seed.BibleContentHash, MaterializationHash: seed.MaterializationHash,
		EvidenceAggregateRevisionID:   seed.EvidenceAggregateRevisionID,
		EvidenceAggregateRevisionHash: seed.EvidenceAggregateRevisionHash,
		EvidenceLeaves:                inputLeaves, MarkerHints: markers, EvidenceIndex: index,
	})
	if err != nil {
		return domain.Invocation{}, err
	}
	start, end := manifest.Shard.AbsoluteStart, manifest.Shard.AbsoluteEnd
	request, err := agentcontract.NewStageInvocation(invocationID, policy, agentcontract.StageInvocationPayload{
		Stage: domain.EpisodeSegmentationStage, ShardKey: manifest.Shard.Key,
		WorkspaceID: seed.WorkspaceID, ProjectID: seed.ProjectID,
		SourceRefs: []agentcontract.StageSourceRef{
			{OwnerKind: "production/script", OwnerLogicalID: seed.DocumentLogicalID, OwnerVersionID: seed.DocumentRevisionID, Revision: seed.DocumentRevision, ContentHash: seed.NormalizedHash},
			{OwnerKind: "production/bible-materialization", OwnerLogicalID: seed.BibleVersionID, OwnerVersionID: seed.BibleVersionID, Revision: int64(seed.BibleVersion), ContentHash: seed.MaterializationHash},
		},
		UpstreamCandidates: upstreams,
		ShardManifestRef:   agentcontract.ShardManifestRef{ManifestID: manifest.ManifestID, Version: manifest.Version, Hash: manifest.ManifestHash},
		Shard:              agentcontract.InvocationShard{Kind: manifest.Shard.Kind, Key: manifest.Shard.Key, TreePath: manifest.Shard.TreePath, AbsoluteStart: &start, AbsoluteEnd: &end},
		StageInput:         stageInput,
	})
	if err != nil {
		return domain.Invocation{}, err
	}
	if err = agentcontract.ValidateEpisodeSegmentationInvocation(request); err != nil {
		return domain.Invocation{}, err
	}
	policyJSON, err := json.Marshal(request.ExecutionPolicy)
	if err != nil {
		return domain.Invocation{}, err
	}
	payloadJSON, err := json.Marshal(request.Payload)
	if err != nil {
		return domain.Invocation{}, err
	}
	stageKey, err := request.StageInstanceKey()
	if err != nil {
		return domain.Invocation{}, err
	}
	return domain.Invocation{
		ID: invocationID, WorkspaceID: seed.WorkspaceID,
		RequestType: "episode_segmentation", RequestID: invocationID,
		WorkflowRunID: seed.WorkflowRunID, NodeRunID: seed.NodeRunID,
		ManifestID: manifest.ManifestID, ManifestVersion: manifest.Version,
		Kind: "storygraph_stage", Stage: domain.EpisodeSegmentationStage, ShardKey: manifest.Shard.Key,
		InputHash: request.InputHash, StageInstanceKey: stageKey, ManifestHash: manifest.ManifestHash,
		ExecutionPolicy: policyJSON, Payload: payloadJSON, Status: "queued", CreatedAt: createdAt,
	}, nil
}

func buildEpisodeSegmentationEvidenceIndex(
	seed EpisodeSegmentationSeed,
	leaves []EpisodeSegmentationEvidenceSeed,
) ([]agentcontract.EpisodeSegmentationMarkerHint, []agentcontract.EpisodeSegmentationEvidenceIndexItem, error) {
	markerValues := append([]domain.EpisodeSegmentationMarker(nil), seed.Markers...)
	slices.SortFunc(markerValues, func(left, right domain.EpisodeSegmentationMarker) int {
		return left.Evidence.SourceStart - right.Evidence.SourceStart
	})
	markers := make([]agentcontract.EpisodeSegmentationMarkerHint, 0, len(markerValues))
	index := make([]agentcontract.EpisodeSegmentationEvidenceIndexItem, 0, 512)
	seen := map[string]struct{}{}
	appendItem := func(kind, label string, evidence domain.Evidence, leaf EpisodeSegmentationEvidenceSeed) {
		key := fmt.Sprintf("%s:%d:%d:%s", leaf.ShardKey, evidence.SourceStart, evidence.SourceEnd, evidence.TextHash)
		if _, exists := seen[key]; exists || len(index) >= 512 {
			return
		}
		seen[key] = struct{}{}
		index = append(index, agentcontract.EpisodeSegmentationEvidenceIndexItem{
			IndexKey: fmt.Sprintf("%s:%04d", kind, len(index)), Kind: kind, Label: label,
			ShardKey: leaf.ShardKey, CandidateRevisionID: leaf.CandidateRevisionID,
			CandidateRevisionHash: leaf.CandidateRevisionHash, Evidence: episodeSegmentationContractEvidence(evidence),
		})
	}
	for _, marker := range markerValues {
		leaf, ok := episodeSegmentationLeafAt(leaves, marker.Evidence.SourceStart)
		if !ok {
			return nil, nil, errors.New("explicit Episode marker has no exact Evidence leaf")
		}
		markers = append(markers, agentcontract.EpisodeSegmentationMarkerHint{
			EpisodeNumber: marker.EpisodeNumber, Label: marker.Label,
			Evidence: episodeSegmentationContractEvidence(marker.Evidence),
		})
		appendItem("marker", marker.Label, marker.Evidence, leaf)
	}
	for _, leaf := range leaves {
		for _, observation := range leaf.Candidate.Observations {
			kind := observation.Kind
			if kind != "marker" && kind != "event" {
				kind = "evidence"
			}
			for _, evidence := range observation.Evidence {
				appendItem(kind, observation.Label, evidence, leaf)
			}
		}
	}
	if len(index) == 0 {
		return nil, nil, errors.New("Episode segmentation has no bounded Evidence index")
	}
	return markers, index, nil
}

func episodeSegmentationLeafAt(leaves []EpisodeSegmentationEvidenceSeed, position int) (EpisodeSegmentationEvidenceSeed, bool) {
	for _, leaf := range leaves {
		if position >= leaf.LogicalStart && position < leaf.LogicalEnd {
			return leaf, true
		}
	}
	return EpisodeSegmentationEvidenceSeed{}, false
}

func episodeSegmentationContractEvidence(value domain.Evidence) agentcontract.EpisodeSegmentationEvidence {
	return agentcontract.EpisodeSegmentationEvidence{
		SourceStart: value.SourceStart, SourceEnd: value.SourceEnd, TextHash: value.TextHash,
		ExactAnchor: value.ExactAnchor, EpisodeNumber: value.EpisodeNumber,
	}
}
