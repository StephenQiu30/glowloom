package bible_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestEpisodeSegmentationManifestAndCandidatePreserveExactSourceCoverage(t *testing.T) {
	text := "第一集\n甲出发。\n第二集\n乙抵达。"
	runes := []rune(text)
	secondStart := strings.Index(text, "第二集")
	secondStart = len([]rune(text[:secondStart]))
	firstEvidence := domain.Evidence{SourceStart: 0, SourceEnd: 3, TextHash: domain.SourceTextHash("第一集"), ExactAnchor: "第一集", EpisodeNumber: episodeIntPointer(1)}
	secondEvidence := domain.Evidence{SourceStart: secondStart, SourceEnd: secondStart + 3, TextHash: domain.SourceTextHash("第二集"), ExactAnchor: "第二集", EpisodeNumber: episodeIntPointer(2)}

	manifest, err := domain.BuildEpisodeSegmentationManifest(domain.EpisodeSegmentationManifestInput{
		ManifestID: uuid.NewString(), WorkspaceID: uuid.NewString(), WorkflowRunID: uuid.NewString(),
		NodeRunID: uuid.NewString(), RootInputHash: strings.Repeat("a", 64), SourceCodePoints: len(runes),
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Stage != domain.EpisodeSegmentationStage || manifest.Shard.AbsoluteStart != 0 || manifest.Shard.AbsoluteEnd != len(runes) {
		t.Fatalf("unexpected segmentation manifest: %#v", manifest)
	}

	raw, err := json.Marshal(domain.EpisodeSegmentationCandidate{
		Boundaries: []domain.EpisodeBoundary{
			{BoundaryKey: "episode:0001", EpisodeOrder: 1, Title: "第一集", AbsoluteStart: 0, AbsoluteEnd: secondStart, Evidence: []domain.Evidence{firstEvidence}},
			{BoundaryKey: "episode:0002", EpisodeOrder: 2, Title: "第二集", AbsoluteStart: secondStart, AbsoluteEnd: len(runes), Evidence: []domain.Evidence{secondEvidence}},
		},
		ReviewIssues: []domain.ReviewIssue{},
	})
	if err != nil {
		t.Fatal(err)
	}
	markers := []domain.EpisodeSegmentationMarker{{EpisodeNumber: 1, Label: "第一集", Evidence: firstEvidence}, {EpisodeNumber: 2, Label: "第二集", Evidence: secondEvidence}}
	if _, err = domain.DecodeEpisodeSegmentationCandidate(raw, text, []domain.Evidence{firstEvidence, secondEvidence}, markers); err != nil {
		t.Fatalf("valid full-coverage candidate rejected: %v", err)
	}

	var drifted domain.EpisodeSegmentationCandidate
	if err = json.Unmarshal(raw, &drifted); err != nil {
		t.Fatal(err)
	}
	drifted.Boundaries[1].AbsoluteStart++
	raw, _ = json.Marshal(drifted)
	if _, err = domain.DecodeEpisodeSegmentationCandidate(raw, text, []domain.Evidence{firstEvidence, secondEvidence}, markers); err == nil {
		t.Fatal("candidate with a source coverage gap was accepted")
	}

	drifted.Boundaries[1].AbsoluteStart = secondStart
	drifted.Boundaries[1].Evidence = []domain.Evidence{firstEvidence}
	raw, _ = json.Marshal(drifted)
	if _, err = domain.DecodeEpisodeSegmentationCandidate(raw, text, []domain.Evidence{firstEvidence, secondEvidence}, markers); err == nil {
		t.Fatal("candidate that overrode an explicit episode marker was accepted")
	}
}

func episodeIntPointer(value int) *int { return &value }

func TestEpisodeSegmentationServiceFreezesExactUpstreamsWithoutWritingEpisodes(t *testing.T) {
	workspaceID, projectID := uuid.NewString(), uuid.NewString()
	runID, nodeID := uuid.NewString(), uuid.NewString()
	documentID, revisionID := uuid.NewString(), uuid.NewString()
	aggregateID, leafID, invocationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	bibleVersionID := uuid.NewString()
	text := "第一集\n甲出发。"
	markerEvidence := domain.Evidence{
		SourceStart: 0, SourceEnd: 3, TextHash: domain.SourceTextHash("第一集"), ExactAnchor: "第一集", EpisodeNumber: episodeIntPointer(1),
	}
	repository := &episodeSegmentationRepository{seed: application.EpisodeSegmentationSeed{
		WorkspaceID: workspaceID, ProjectID: projectID, WorkflowRunID: runID, NodeRunID: nodeID,
		DocumentLogicalID: documentID, DocumentRevisionID: revisionID, DocumentRevision: 1,
		NormalizedText: text, NormalizedHash: domain.SourceTextHash(text), TargetDurationMS: 90_000,
		BibleVersionID: bibleVersionID, BibleVersion: 1, BibleContentHash: strings.Repeat("b", 64),
		MaterializationHash: strings.Repeat("c", 64), EvidenceAggregateRevisionID: aggregateID,
		EvidenceAggregateRevisionHash: strings.Repeat("d", 64),
		Evidence: []application.EpisodeSegmentationEvidenceSeed{{
			ShardKey: "source:00000000:00000008", CandidateRevisionID: leafID,
			CandidateRevisionHash: strings.Repeat("e", 64), SourceInvocationID: invocationID,
			SourceResultHash: strings.Repeat("f", 64), LogicalStart: 0, LogicalEnd: len([]rune(text)),
			Candidate: domain.SourceEvidenceCandidate{Observations: []domain.SourceObservation{{
				ObservationKey: "marker:first", Kind: "marker", ProposedKey: "episode:1", Label: "第一集",
				Facts: []string{}, Evidence: []domain.Evidence{markerEvidence}, Ambiguities: []string{},
			}}, ReviewIssues: []domain.SourceEvidenceIssue{}},
		}},
		Markers: []domain.EpisodeSegmentationMarker{{EpisodeNumber: 1, Label: "第一集", Evidence: markerEvidence}},
	}}
	ids := []string{uuid.NewString(), uuid.NewString()}
	service := application.NewEpisodeSegmentationService(repository, application.EpisodeSegmentationConfig{
		Now: func() time.Time { return time.Date(2026, time.August, 28, 0, 0, 0, 0, time.UTC) },
		NewID: func() string {
			value := ids[0]
			ids = ids[1:]
			return value
		},
	})
	state, err := service.Ensure(context.Background(), application.EpisodeSegmentationCommand{
		WorkspaceID: workspaceID, ProjectID: projectID, WorkflowRunID: runID, NodeRunID: nodeID,
		DocumentRevisionID: revisionID, DocumentRevisionHash: repository.seed.NormalizedHash,
		EvidenceCandidateRevisionID:   aggregateID,
		EvidenceCandidateRevisionHash: repository.seed.EvidenceAggregateRevisionHash,
		BibleVersionID:                bibleVersionID, BibleVersion: 1, MaterializationHash: repository.seed.MaterializationHash,
	})
	if err != nil || state.Status != "pending" {
		t.Fatalf("ensure Episode segmentation: state=%#v err=%v", state, err)
	}
	var payload contract.StageInvocationPayload
	if err = json.Unmarshal(repository.preparation.Invocation.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Stage != domain.EpisodeSegmentationStage || len(payload.SourceRefs) != 2 ||
		len(payload.UpstreamCandidates) != 1 || payload.Shard.AbsoluteEnd == nil ||
		*payload.Shard.AbsoluteEnd != len([]rune(text)) {
		t.Fatalf("frozen Episode segmentation invocation=%#v", payload)
	}
}

type episodeSegmentationRepository struct {
	seed        application.EpisodeSegmentationSeed
	preparation application.EpisodeSegmentationPreparation
}

func (repository *episodeSegmentationRepository) LoadEpisodeSegmentationSeed(
	context.Context,
	application.EpisodeSegmentationCommand,
) (application.EpisodeSegmentationSeed, error) {
	return repository.seed, nil
}

func (repository *episodeSegmentationRepository) EnsureEpisodeSegmentation(
	_ context.Context,
	preparation application.EpisodeSegmentationPreparation,
) (application.EpisodeSegmentationState, error) {
	repository.preparation = preparation
	return application.EpisodeSegmentationState{
		Status: "pending", ManifestID: preparation.Manifest.ManifestID,
		ManifestVersion: preparation.Manifest.Version, ManifestHash: preparation.Manifest.ManifestHash,
	}, nil
}
