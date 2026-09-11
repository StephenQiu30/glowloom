package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestVisualFoundationInvocationFreezesProjectMediaAndInputHash(t *testing.T) {
	invocation := validVisualFoundationInvocation(t)
	if err := invocation.Validate(); err != nil {
		t.Fatal(err)
	}
	computed, err := invocation.ComputeInputHash()
	if err != nil {
		t.Fatal(err)
	}
	if invocation.InputHash != computed ||
		invocation.InputHash != "c61f2dc1a56173a34ff17b7042f25ad4f3fa2ef198a520fb250e0a244ca8609c" ||
		invocation.StageInstanceKey() != "cd6f973a527f5dd7c13a4fb759cd7edc591a37bec1778e7ce740d942eee4e8d5" {
		t.Fatal("Visual Foundation invocation identity is not deterministic")
	}
	_, current, _, _ := runtime.Caller(0)
	encoded, err := os.ReadFile(filepath.Join(
		filepath.Dir(current),
		"..",
		"fixtures",
		"agent",
		"storygraph-visual-foundation-invocation.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contract.DecodeVisualFoundationInvocation(encoded)
	if err != nil || !reflect.DeepEqual(decoded, invocation) {
		t.Fatalf("Visual Foundation invocation does not round trip: %v", err)
	}
}

func TestVisualFoundationInvocationRejectsUnknownOrDriftingInput(t *testing.T) {
	invocation := validVisualFoundationInvocation(t)
	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	payload["unexpected"] = true
	if _, err = contract.DecodeVisualFoundationInvocation(mustJSON(t, payload)); err == nil {
		t.Fatal("unknown Visual Foundation invocation field must fail closed")
	}

	invocation = validVisualFoundationInvocation(t)
	invocation.Payload.MediaAttachments[0].ContentHash = visualFoundationHash("drift")
	if _, err = contract.NewVisualFoundationInvocation(
		invocation.InvocationID,
		invocation.AttemptID,
		invocation.StageRelease,
		invocation.Control,
		invocation.Budget,
		invocation.Payload,
	); err == nil {
		t.Fatal("Visual Foundation media drift must fail closed")
	}

	invocation = validVisualFoundationInvocation(t)
	invocation.Payload.MediaAttachments[0].ByteLength = 10*1024*1024 + 1
	if _, err = contract.NewVisualFoundationInvocation(
		invocation.InvocationID,
		invocation.AttemptID,
		invocation.StageRelease,
		invocation.Control,
		invocation.Budget,
		invocation.Payload,
	); err == nil {
		t.Fatal("Visual Foundation media budget overflow must fail closed")
	}
}

func validVisualFoundationInvocation(t *testing.T) contract.VisualFoundationInvocation {
	t.Helper()
	input, _, err := contract.DecodeVisualFoundationInput(visualFoundationInputJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := contract.NewVisualFoundationInvocation(
		"00000000-0000-0000-0000-000000000080",
		"00000000-0000-0000-0000-000000000090",
		contract.SceneAnalysisReleaseIdentity{
			SkillReleaseID:    "66666666-6666-4666-8666-666666666666",
			SkillReleaseHash:  "1111111111111111111111111111111111111111111111111111111111111111",
			StageReleaseHash:  "2222222222222222222222222222222222222222222222222222222222222222",
			BundleContentHash: contract.StoryGraphSkillBundleHash,
			AgentImageDigest:  "sha256:4444444444444444444444444444444444444444444444444444444444444444",
		},
		contract.SceneAnalysisControlProof{
			ControlRecordID: "77777777-7777-4777-8777-777777777777",
			ControlRevision: 1,
			Status:          "approved",
			ControlHash:     "5555555555555555555555555555555555555555555555555555555555555555",
			ReleaseFence:    0,
		},
		contract.SceneAnalysisExecutionBudget{
			MaxAttempts:         3,
			MaxModelCalls:       1,
			MaxExecutionSeconds: 120,
			MaxOutputBytes:      131072,
		},
		contract.VisualFoundationPayload{
			Variant: contract.VisualFoundationStageVariant{
				StageKey:            "resolve_visual_foundation",
				ProfileKey:          "default",
				LaneKey:             "primary",
				OutputSchemaVersion: "visual-foundation-candidate-production",
			},
			Scope: contract.VisualFoundationScope{
				WorkspaceID: input.WorkspaceID,
				ProjectID:   input.ProjectID,
			},
			Shard: contract.VisualFoundationShard{
				ManifestID:        "00000000-0000-0000-0000-000000000040",
				ManifestHash:      visualFoundationHash("visual-shard"),
				ShardKey:          "project:" + input.ProjectID,
				ImpactClosureHash: visualFoundationHash("visual-impact"),
			},
			MediaAttachments: []contract.VisualFoundationMediaAttachment{{
				AttachmentID:   input.ReferenceAttachments[0].AttachmentID,
				MediaObjectID:  "00000000-0000-0000-0000-000000000031",
				VersionNo:      1,
				Purpose:        "style_reference",
				ObjectKey:      input.ReferenceAttachments[0].ObjectKey,
				ContentHash:    input.ReferenceAttachments[0].ContentHash,
				MediaType:      input.ReferenceAttachments[0].MediaType,
				ByteLength:     1024,
				PixelWidth:     1024,
				PixelHeight:    1024,
				PageCount:      1,
				FrameCount:     1,
				RightsBasis:    input.ReferenceAttachments[0].RightsBasis,
				RightsRefHash:  input.ReferenceAttachments[0].RightsRefHash,
				LineageRefHash: visualFoundationHash("visual-lineage"),
			}},
			StageInput: input,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}
