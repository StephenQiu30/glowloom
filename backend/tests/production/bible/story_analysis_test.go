package bible_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestStoryAnalysisBuildsDeterministicBoundedMapAndReduceTree(t *testing.T) {
	fragments := make([]domain.StoryAnalysisEvidenceFragment, 5)
	for index := range fragments {
		fragments[index] = domain.StoryAnalysisEvidenceFragment{
			ShardKey:              fmt.Sprintf("source:%04d", index),
			LogicalStart:          index * 10,
			LogicalEnd:            (index + 1) * 10,
			CandidateRevisionID:   uuid.NewSHA1(uuid.NameSpaceOID, []byte{byte(index)}).String(),
			CandidateRevisionHash: strings.Repeat(string(rune('a'+index)), 64),
		}
	}
	input := domain.StoryAnalysisManifestInput{
		AnalyzeManifestID: uuid.NewString(), ReconcileManifestID: uuid.NewString(),
		WorkspaceID: uuid.NewString(), WorkflowRunID: uuid.NewString(), NodeRunID: uuid.NewString(),
		RootInputHash: strings.Repeat("f", 64), FanIn: 2, EvidenceFragments: fragments,
	}
	analyze, reconcile, err := domain.BuildStoryAnalysisManifests(input)
	if err != nil {
		t.Fatal(err)
	}
	if analyze.Stage != domain.AnalyzeStoryStage || len(analyze.Shards) != 5 {
		t.Fatalf("unexpected analyze manifest: %#v", analyze)
	}
	if reconcile.Stage != domain.ReconcileStoryStage || len(reconcile.Shards) != 6 {
		t.Fatalf("unexpected reconcile manifest: %#v", reconcile)
	}
	rootCount := 0
	for _, shard := range reconcile.Shards {
		if len(shard.Children) < 1 || len(shard.Children) > 2 {
			t.Fatalf("reduce shard exceeded the fixed fan-in: %#v", shard)
		}
		if shard.ParentKey == "" {
			rootCount++
		}
	}
	if rootCount != 1 {
		t.Fatalf("reduce tree roots=%d", rootCount)
	}
	if err = domain.ValidateStoryAnalysisManifests(analyze, reconcile); err != nil {
		t.Fatalf("built manifests are invalid: %v", err)
	}

	reversed := append([]domain.StoryAnalysisEvidenceFragment(nil), fragments...)
	slices.Reverse(reversed)
	input.AnalyzeManifestID = uuid.NewString()
	input.ReconcileManifestID = uuid.NewString()
	input.EvidenceFragments = reversed
	secondAnalyze, secondReconcile, err := domain.BuildStoryAnalysisManifests(input)
	if err != nil {
		t.Fatal(err)
	}
	if analyze.ManifestHash != secondAnalyze.ManifestHash ||
		reconcile.ManifestHash != secondReconcile.ManifestHash ||
		analyze.CoverageHash != secondAnalyze.CoverageHash ||
		reconcile.CoverageHash != secondReconcile.CoverageHash {
		t.Fatalf("map/reduce topology drifted by input order: first=%s/%s second=%s/%s",
			analyze.ManifestHash, reconcile.ManifestHash,
			secondAnalyze.ManifestHash, secondReconcile.ManifestHash,
		)
	}
}

func TestStoryCandidatesRejectEvidenceOutsideExactUpstreamSet(t *testing.T) {
	allowed := domain.Evidence{
		SourceStart: 3, SourceEnd: 5, TextHash: domain.SourceTextHash("林一"), ExactAnchor: "林一",
	}
	valid := domain.StoryAnalysisCandidate{
		Entities: []domain.StoryEntityCandidate{{
			EntityKey: "character:lin-yi", Kind: "character", CanonicalName: "林一",
			NormalizedName: "林一", Aliases: []string{}, StableSpec: domain.AssetSpecCandidate{
				Temperament: []string{}, Goals: []string{}, Relationships: []string{},
				VisualElements: []string{}, NegativeConstraints: []string{}, PerformanceTraits: []string{}, AllowedUsage: []string{},
			}, EpisodeNumbers: []int{1}, Evidence: []domain.Evidence{allowed},
			States: []domain.StoryEntityStateCandidate{}, Ambiguities: []string{},
		}},
		WorldEntries: []domain.StoryWorldEntryCandidate{}, Claims: []domain.StoryClaimCandidate{},
		Arcs: []domain.StoryArcCandidate{}, ReviewIssues: []domain.ReviewIssue{},
	}
	if err := domain.ValidateStoryAnalysisCandidate(valid, []domain.Evidence{allowed}); err != nil {
		t.Fatalf("valid exact evidence was rejected: %v", err)
	}
	forged := valid
	forged.Entities = append([]domain.StoryEntityCandidate(nil), valid.Entities...)
	forged.Entities[0].Evidence = []domain.Evidence{{
		SourceStart: 9, SourceEnd: 11, TextHash: domain.SourceTextHash("伪造"), ExactAnchor: "伪造",
	}}
	if err := domain.ValidateStoryAnalysisCandidate(forged, []domain.Evidence{allowed}); err == nil {
		t.Fatal("story analysis accepted evidence absent from its exact upstream Candidate Revision")
	}
}
