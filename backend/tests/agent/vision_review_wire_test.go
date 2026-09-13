package agent_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func validVisionReviewInvocation(t *testing.T) contract.VisionReviewInvocation {
	t.Helper()
	raw, err := os.ReadFile("testdata/vision_review_input.json")
	if err != nil {
		t.Fatal(err)
	}
	input, err := contract.DecodeVisionReviewInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	brief := validReferenceBriefInvocation(t)
	release := brief.StageRelease
	release.StageReleaseHash = input.Subject.StageReleaseHash
	payload := contract.VisionReviewPayload{
		Variant:    contract.VisionReviewStageVariant{StageKey: contract.VisionReviewStageKey, ProfileKey: "default", LaneKey: "primary", OutputSchemaVersion: contract.VisionReviewCandidateContractID},
		Scope:      contract.VisionReviewScope{WorkspaceID: input.Subject.WorkspaceID, ProjectID: input.Subject.ProjectID, BundleInputID: input.Subject.BundleInputRef.ID},
		Shard:      contract.VisionReviewShard{ManifestID: brief.Payload.Shard.ManifestID, ManifestHash: brief.Payload.Shard.ManifestHash, ShardKey: "vision_bundle:" + input.Subject.BundleInputRef.ID, ImpactClosureHash: brief.Payload.Shard.ImpactClosureHash},
		StageInput: input,
	}
	value, err := contract.NewVisionReviewInvocation(brief.InvocationID, brief.AttemptID, release, brief.Control, brief.Budget, payload)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestVisionReviewInvocationSeparatesContentAndEnvelopeHashes(t *testing.T) {
	value := validVisionReviewInvocation(t)
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contract.DecodeVisionReviewInvocation(raw)
	if err != nil || decoded.InputHash != value.InputHash || value.InputHash == value.Payload.StageInput.Subject.InputHash {
		t.Fatalf("invalid review envelope: %v", err)
	}
	if value.StageInstanceKey() == "" {
		t.Fatal("missing review instance identity")
	}
}
