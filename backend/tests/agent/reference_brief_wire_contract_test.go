package agent_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestReferenceBriefInvocationFreezesTargetInputAndRelease(t *testing.T) {
	invocation := validReferenceBriefInvocation(t)
	computed, err := invocation.ComputeInputHash()
	if err != nil || invocation.InputHash != computed || invocation.StageInstanceKey() == "" {
		t.Fatalf("Reference Brief invocation identity is not deterministic: input=%s stage=%s err=%v", invocation.InputHash, invocation.StageInstanceKey(), err)
	}
	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contract.DecodeReferenceBriefInvocation(encoded)
	if err != nil || decoded.InputHash != invocation.InputHash {
		t.Fatalf("Reference Brief invocation does not round trip: %v", err)
	}

	invocation.Payload.StageInput.StageRelease.StageReleaseHash = strings.Repeat("9", 64)
	if err = invocation.Validate(); err == nil {
		t.Fatal("Reference Brief invocation accepted a Stage Release that drifted from its frozen input")
	}
}

func TestReferenceBriefInvocationRejectsUnknownOrDriftingTarget(t *testing.T) {
	invocation := validReferenceBriefInvocation(t)
	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	payload["provider"] = "seedream"
	if _, err = contract.DecodeReferenceBriefInvocation(mustJSON(t, payload)); err == nil {
		t.Fatal("unknown Reference Brief invocation field must fail closed")
	}

	invocation = validReferenceBriefInvocation(t)
	invocation.Payload.Scope.TargetBusinessKey = `["location_board",["asset","asset_identity_state_set","subject:one",""]]`
	if err = invocation.Validate(); err == nil {
		t.Fatal("Reference Brief target scope drift must fail closed")
	}
}

func TestReferenceBriefDispatchAuthorizationBindsAttemptAndExpiry(t *testing.T) {
	invocation := validReferenceBriefInvocation(t)
	claims := referenceBriefDispatchClaims(invocation)
	if err := claims.ValidateForReferenceBrief(invocation, 1, 100); err != nil {
		t.Fatal(err)
	}
	if err := claims.ValidateForReferenceBrief(invocation, 1, 200); err == nil {
		t.Fatal("expired Reference Brief authorization was accepted")
	}
}

func TestReferenceBriefAttemptResultRevalidatesFrozenCandidate(t *testing.T) {
	invocation := validReferenceBriefInvocation(t)
	candidate := referenceBriefCandidateForInput(t, invocation.Payload.StageInput)
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
	authorizationHash := strings.Repeat("8", 64)
	accepted := contract.ReferenceBriefAttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: contract.SceneAnalysisWireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease,
		Control: invocation.Control, ClaimVersion: 1, DispatchAuthorizationHash: authorizationHash,
		Status: "accepted", CandidateType: "reference_brief_candidate", Candidate: candidate,
		InputHash: invocation.InputHash, OutputHash: &outputHash, Diagnostics: diagnostics,
		DiagnosticHash: diagnosticHash, CompletedAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		Executor: contract.ReferenceBriefExecutor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "reference-brief-harness", Model: "codex-cli-default",
		},
	}
	accepted.ResultHash, err = accepted.ComputeResultHash()
	if err != nil || accepted.ValidateFor(invocation, 1, authorizationHash) != nil {
		t.Fatalf("valid Reference Brief result was rejected: %v", err)
	}

	var drifted map[string]any
	if err = json.Unmarshal(candidate, &drifted); err != nil {
		t.Fatal(err)
	}
	drifted["typed_read_set_root"] = strings.Repeat("f", 64)
	accepted.Candidate = mustJSON(t, drifted)
	driftedHash, hashErr := contract.ProductionCanonicalHash(accepted.Candidate)
	if hashErr != nil {
		t.Fatal(hashErr)
	}
	accepted.OutputHash = &driftedHash
	accepted.ResultHash, err = accepted.ComputeResultHash()
	if err != nil {
		t.Fatal(err)
	}
	if err = accepted.ValidateFor(invocation, 1, authorizationHash); err == nil {
		t.Fatal("Reference Brief result accepted a stale Candidate read set")
	}
}

func referenceBriefCandidateForInput(t *testing.T, input contract.ReferenceBriefInput) json.RawMessage {
	t.Helper()
	document := referenceBriefCandidateDocument(t, input.TargetKind)
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var frozen map[string]any
	if err = json.Unmarshal(encoded, &frozen); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"workspace_id", "project_id", "approved_reference_plan_version_ref", "reference_plan_target_ref",
		"target_business_key", "target_kind", "visual_foundation_version_ref", "effective_style_snapshot_ref",
		"effective_policy_snapshot_ref", "dependency_selections", "stage_release", "typed_read_set_root",
		"source_refs", "required_view_roles",
	} {
		document[key] = frozen[key]
	}
	document["positive_instructions"] = frozen["design_focus"]
	document["negative_instructions"] = frozen["forbidden_changes"]
	return mustReferenceBriefJSON(t, document)
}

func validReferenceBriefInvocation(t *testing.T) contract.ReferenceBriefInvocation {
	t.Helper()
	candidate := referenceBriefCandidateDocument(t, "character_appearance")
	input, _, err := contract.DecodeReferenceBriefInput(mustReferenceBriefJSON(t, referenceBriefInputDocument(candidate)))
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := contract.NewReferenceBriefInvocation(
		"00000000-0000-0000-0000-000000000680",
		"00000000-0000-0000-0000-000000000690",
		contract.SceneAnalysisReleaseIdentity{
			SkillReleaseID:   "66666666-6666-4666-8666-666666666666",
			SkillReleaseHash: strings.Repeat("1", 64), StageReleaseHash: input.StageRelease.StageReleaseHash,
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
		contract.ReferenceBriefPayload{
			Variant: contract.ReferenceBriefStageVariant{
				StageKey: "compile_reference_brief", ProfileKey: "default", LaneKey: "primary",
				OutputSchemaVersion: contract.ReferenceBriefCandidateContractID,
			},
			Scope: contract.ReferenceBriefScope{
				WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID, TargetBusinessKey: input.TargetBusinessKey,
			},
			Shard: contract.ReferenceBriefShard{
				ManifestID:   "00000000-0000-0000-0000-000000000640",
				ManifestHash: strings.Repeat("6", 64), ShardKey: "reference_target:" + input.TargetBusinessKey,
				ImpactClosureHash: strings.Repeat("7", 64),
			},
			StageInput: input,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}

func referenceBriefDispatchClaims(invocation contract.ReferenceBriefInvocation) contract.SceneAnalysisDispatchAuthorizationClaims {
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
