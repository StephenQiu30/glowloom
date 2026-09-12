package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

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
		invocation.InputHash != "2f29e94e5f29d194dc0dea83587b26fd6a93c7b445198640428ef8368cbc33c4" ||
		invocation.StageInstanceKey() != "7b98988445792eb8d7ff01d71e6fa85a38a713945dc78de579a8a5cf6ee578bc" {
		t.Fatalf("Visual Foundation invocation identity is not deterministic: input=%s stage=%s", invocation.InputHash, invocation.StageInstanceKey())
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

func TestVisualFoundationDispatchAuthorizationBindsAttemptAndExpiry(t *testing.T) {
	invocation := validVisualFoundationInvocation(t)
	claims := contract.SceneAnalysisDispatchAuthorizationClaims{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		InputHash: invocation.InputHash, SkillReleaseID: invocation.StageRelease.SkillReleaseID,
		SkillReleaseHash:  invocation.StageRelease.SkillReleaseHash,
		StageReleaseHash:  invocation.StageRelease.StageReleaseHash,
		BundleContentHash: invocation.StageRelease.BundleContentHash,
		ControlHash:       invocation.Control.ControlHash, ReleaseFence: invocation.Control.ReleaseFence,
		ClaimVersion: 1, AgentImageDigest: invocation.StageRelease.AgentImageDigest, ExpiresAt: 200,
	}
	if err := claims.ValidateForVisualFoundation(invocation, 1, 100); err != nil {
		t.Fatal(err)
	}
	if err := claims.ValidateForVisualFoundation(invocation, 1, 200); err == nil {
		t.Fatal("expired Visual Foundation authorization was accepted")
	}
}

func TestVisualFoundationAttemptResultValidatesCandidateAndTerminalStates(t *testing.T) {
	invocation := validVisualFoundationInvocation(t)
	candidate := visualFoundationCandidateJSON(t, invocation.Payload.StageInput)
	outputHash, err := contract.ProductionCanonicalHash(candidate)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := []contract.SceneAnalysisDiagnostic{}
	diagnosticJSON, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticHash, err := contract.ProductionCanonicalHash(diagnosticJSON)
	if err != nil {
		t.Fatal(err)
	}
	authorizationHash := visualFoundationHash("visual-dispatch")
	accepted := contract.VisualFoundationAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: contract.SceneAnalysisWireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease,
		Control: invocation.Control, ClaimVersion: 1, DispatchAuthorizationHash: authorizationHash,
		Status: "accepted", CandidateType: "visual_foundation_candidate", Candidate: candidate,
		InputHash: invocation.InputHash, OutputHash: &outputHash, Diagnostics: diagnostics,
		DiagnosticHash: diagnosticHash, CompletedAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		Executor: contract.VisualFoundationExecutor{
			RuntimeClass: "vision", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "visual-foundation-harness", Model: "codex-cli-default",
		},
	}
	accepted.ResultHash, err = accepted.ComputeResultHash()
	if err != nil {
		t.Fatal(err)
	}
	if outputHash != "3c89f9503800ac661d25bc41ee15127ee392ad32601329442c04177ac192c7eb" ||
		accepted.ResultHash != "76abedd1750ffb9b9326941e3fa58445f6463028a7162a3f677261f7e6850cfd" {
		t.Fatal("Visual Foundation output or result hash drifted across runtimes")
	}
	if err = accepted.ValidateFor(invocation, 1, authorizationHash); err != nil {
		t.Fatal(err)
	}
	_, current, _, _ := runtime.Caller(0)
	fixtureJSON, err := os.ReadFile(filepath.Join(
		filepath.Dir(current),
		"..",
		"fixtures",
		"agent",
		"storygraph-visual-foundation-result.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		AuthorizationClaims contract.SceneAnalysisDispatchAuthorizationClaims `json:"authorization_claims"`
		AttemptResult       json.RawMessage                                   `json:"attempt_result"`
	}
	if err = json.Unmarshal(fixtureJSON, &fixture); err != nil {
		t.Fatal(err)
	}
	if err = fixture.AuthorizationClaims.ValidateForVisualFoundation(invocation, 1, 100); err != nil {
		t.Fatal(err)
	}
	fixtureResult, err := contract.DecodeVisualFoundationAttemptResult(fixture.AttemptResult)
	if err != nil || fixtureResult.ValidateFor(invocation, 1, authorizationHash) != nil ||
		fixtureResult.ResultHash != accepted.ResultHash {
		t.Fatalf("shared Visual Foundation result fixture drifted: %v", err)
	}

	var unsafe map[string]any
	if err = json.Unmarshal(candidate, &unsafe); err != nil {
		t.Fatal(err)
	}
	unsafe["approved"] = true
	unsafeCandidate := mustJSON(t, unsafe)
	unsafeResult := accepted
	unsafeResult.Candidate = unsafeCandidate
	unsafeHash, hashErr := contract.ProductionCanonicalHash(unsafeCandidate)
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	unsafeResult.OutputHash = &unsafeHash
	unsafeResult.ResultHash, err = unsafeResult.ComputeResultHash()
	if err != nil {
		t.Fatal(err)
	}
	if err = unsafeResult.ValidateFor(invocation, 1, authorizationHash); err == nil {
		t.Fatal("unsafe Visual Foundation Candidate was accepted")
	}

	for _, test := range []struct {
		status, retryClass string
	}{{"rejected", "never"}, {"outcome_unknown", "same_release"}} {
		result := accepted
		result.Status = test.status
		result.Candidate = nil
		result.OutputHash = nil
		result.Error = &contract.SceneAnalysisResultError{
			Code: "model_failed", SafeSummary: "视觉候选未生成。", RetryClass: test.retryClass,
		}
		result.ResultHash, err = result.ComputeResultHash()
		if err != nil || result.ValidateFor(invocation, 1, authorizationHash) != nil {
			t.Fatalf("valid %s result was rejected: %v", test.status, err)
		}
	}

	encoded, err := json.Marshal(accepted)
	if err != nil {
		t.Fatal(err)
	}
	var unknown map[string]any
	if err = json.Unmarshal(encoded, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["unexpected"] = true
	if _, err = contract.DecodeVisualFoundationAttemptResult(mustJSON(t, unknown)); err == nil {
		t.Fatal("unknown Visual Foundation result field was accepted")
	}
	accepted.DispatchAuthorizationHash = strings.Repeat("0", 64)
	accepted.ResultHash, err = accepted.ComputeResultHash()
	if err != nil {
		t.Fatal(err)
	}
	if err = accepted.ValidateFor(invocation, 1, authorizationHash); err == nil {
		t.Fatal("Visual Foundation dispatch identity drift was accepted")
	}
}
