package agent_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestReferencePlanInvocationFreezesProjectInputAndIdentity(t *testing.T) {
	invocation := validReferencePlanInvocation(t)
	computed, err := invocation.ComputeInputHash()
	if err != nil || invocation.InputHash != computed ||
		invocation.InputHash != "f4d42820086b7afb8ff972039c8ca983c4acd7c655085e31ad6ec012585fe496" ||
		invocation.StageInstanceKey() != "548fbb5b68210ae231e2a83d5c9f9e5cf127d0fb80c01a83b8b4d135d6268227" {
		t.Fatalf(
			"Reference Plan invocation identity is not deterministic: input=%s stage=%s err=%v",
			invocation.InputHash,
			invocation.StageInstanceKey(),
			err,
		)
	}
	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contract.DecodeReferencePlanInvocation(encoded)
	if err != nil || decoded.InputHash != invocation.InputHash {
		t.Fatalf("Reference Plan invocation does not round trip: %v", err)
	}
}

func TestReferencePlanInvocationRejectsUnknownOrDriftingInput(t *testing.T) {
	invocation := validReferencePlanInvocation(t)
	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	payload["unexpected"] = true
	if _, err = contract.DecodeReferencePlanInvocation(mustJSON(t, payload)); err == nil {
		t.Fatal("unknown Reference Plan invocation field must fail closed")
	}

	invocation = validReferencePlanInvocation(t)
	invocation.Payload.Scope.ProjectID = "00000000-0000-0000-0000-000000000099"
	if err = invocation.Validate(); err == nil {
		t.Fatal("Reference Plan Project scope drift must fail closed")
	}
}

func TestReferencePlanDispatchAuthorizationBindsAttemptAndExpiry(t *testing.T) {
	invocation := validReferencePlanInvocation(t)
	claims := referencePlanDispatchClaims(invocation)
	if err := claims.ValidateForReferencePlan(invocation, 1, 100); err != nil {
		t.Fatal(err)
	}
	if err := claims.ValidateForReferencePlan(invocation, 1, 200); err == nil {
		t.Fatal("expired Reference Plan authorization was accepted")
	}
}

func TestReferencePlanAttemptResultValidatesCandidateAndTerminalStates(t *testing.T) {
	invocation := validReferencePlanInvocation(t)
	candidate := mustJSON(t, referencePlanCandidate(invocation.Payload.StageInput))
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
	authorizationHash := referencePlanHash("reference-plan-dispatch")
	accepted := contract.ReferencePlanAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: contract.SceneAnalysisWireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease,
		Control: invocation.Control, ClaimVersion: 1, DispatchAuthorizationHash: authorizationHash,
		Status: "accepted", CandidateType: "reference_plan_candidate", Candidate: candidate,
		InputHash: invocation.InputHash, OutputHash: &outputHash, Diagnostics: diagnostics,
		DiagnosticHash: diagnosticHash, CompletedAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		Executor: contract.ReferencePlanExecutor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "reference-plan-harness", Model: "codex-cli-default",
		},
	}
	accepted.ResultHash, err = accepted.ComputeResultHash()
	if err != nil || accepted.ValidateFor(invocation, 1, authorizationHash) != nil {
		t.Fatalf("valid Reference Plan result was rejected: %v", err)
	}
	if outputHash != "9176e76d8c146a3e92899322b88e4a92e87002c3f17f94527f8cc174371d3ba1" ||
		accepted.ResultHash != "98fb434c47ad0f3a8da518d736ef04b2616a064d19c0152d9df550101774dde8" {
		t.Fatalf(
			"Reference Plan output or result hash is not deterministic: output=%s result=%s",
			outputHash,
			accepted.ResultHash,
		)
	}

	var unsafe map[string]any
	if err = json.Unmarshal(candidate, &unsafe); err != nil {
		t.Fatal(err)
	}
	unsafe["provider"] = "seedream"
	unsafeResult := accepted
	unsafeResult.Candidate = mustJSON(t, unsafe)
	unsafeHash, hashErr := contract.ProductionCanonicalHash(unsafeResult.Candidate)
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	unsafeResult.OutputHash = &unsafeHash
	unsafeResult.ResultHash, err = unsafeResult.ComputeResultHash()
	if err != nil {
		t.Fatal(err)
	}
	if err = unsafeResult.ValidateFor(invocation, 1, authorizationHash); err == nil {
		t.Fatal("unsafe Reference Plan Candidate was accepted")
	}

	for _, test := range []struct {
		status, retryClass string
	}{{"rejected", "never"}, {"outcome_unknown", "same_release"}} {
		result := accepted
		result.Status = test.status
		result.Candidate = nil
		result.OutputHash = nil
		result.Error = &contract.SceneAnalysisResultError{
			Code: "model_failed", SafeSummary: "参考规划候选未生成。", RetryClass: test.retryClass,
		}
		result.ResultHash, err = result.ComputeResultHash()
		if err != nil || result.ValidateFor(invocation, 1, authorizationHash) != nil {
			t.Fatalf("valid %s Reference Plan result was rejected: %v", test.status, err)
		}
	}

	accepted.DispatchAuthorizationHash = strings.Repeat("0", 64)
	accepted.ResultHash, err = accepted.ComputeResultHash()
	if err != nil {
		t.Fatal(err)
	}
	if err = accepted.ValidateFor(invocation, 1, authorizationHash); err == nil {
		t.Fatal("Reference Plan dispatch identity drift was accepted")
	}
}

func validReferencePlanInvocation(t *testing.T) contract.ReferencePlanInvocation {
	t.Helper()
	input := referencePlanInput(t)
	invocation, err := contract.NewReferencePlanInvocation(
		"00000000-0000-0000-0000-000000000580",
		"00000000-0000-0000-0000-000000000590",
		contract.SceneAnalysisReleaseIdentity{
			SkillReleaseID:   "66666666-6666-4666-8666-666666666666",
			SkillReleaseHash: strings.Repeat("1", 64), StageReleaseHash: strings.Repeat("2", 64),
			BundleContentHash: contract.StoryGraphSkillBundleHash,
			AgentImageDigest:  "sha256:" + strings.Repeat("4", 64),
		},
		contract.SceneAnalysisControlProof{
			ControlRecordID: "77777777-7777-4777-8777-777777777777", ControlRevision: 1,
			Status: "approved", ControlHash: strings.Repeat("5", 64), ReleaseFence: 0,
		},
		contract.SceneAnalysisExecutionBudget{
			MaxAttempts: 3, MaxModelCalls: 1, MaxExecutionSeconds: 120, MaxOutputBytes: 131072,
		},
		contract.ReferencePlanPayload{
			Variant: contract.ReferencePlanStageVariant{
				StageKey: "plan_reference_assets", ProfileKey: "default", LaneKey: "primary",
				OutputSchemaVersion: contract.ReferencePlanCandidateContractID,
			},
			Scope: contract.ReferencePlanScope{WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID},
			Shard: contract.ReferencePlanShard{
				ManifestID:   "00000000-0000-0000-0000-000000000540",
				ManifestHash: referencePlanHash("reference-plan-shard"), ShardKey: "project:" + input.ProjectID,
				ImpactClosureHash: referencePlanHash("reference-plan-impact"),
			},
			StageInput: input,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}

func referencePlanDispatchClaims(
	invocation contract.ReferencePlanInvocation,
) contract.SceneAnalysisDispatchAuthorizationClaims {
	return contract.SceneAnalysisDispatchAuthorizationClaims{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		InputHash: invocation.InputHash, SkillReleaseID: invocation.StageRelease.SkillReleaseID,
		SkillReleaseHash:  invocation.StageRelease.SkillReleaseHash,
		StageReleaseHash:  invocation.StageRelease.StageReleaseHash,
		BundleContentHash: invocation.StageRelease.BundleContentHash,
		ControlHash:       invocation.Control.ControlHash, ReleaseFence: invocation.Control.ReleaseFence,
		ClaimVersion: 1, AgentImageDigest: invocation.StageRelease.AgentImageDigest, ExpiresAt: 200,
	}
}
