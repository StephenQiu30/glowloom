package agent_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestStoryGraphV2SpanAndSceneFactContracts(t *testing.T) {
	text := "第一场 夜 内\n林舟握住门把。\n第二场 日 外\n林舟离开。"
	sourceHash := hashTextV2(text)
	sourceID := uuid.NewString()
	payload := contract.V2StagePayload{
		Variant: contract.StageVariantKeyV2{
			StageKey: "propose_script_spans", ProfileKey: "default", LaneKey: "primary",
			OutputSchemaVersion: "script-span-candidate-v1",
		},
		Scope: contract.V2InvocationScope{WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString()},
		SourceRefs: []contract.OwnerVersionIdentityV1{{
			OwnerKind: "production/script-source", LogicalID: "script:demo",
			VersionID: sourceID, Revision: 1, ContentHash: sourceHash,
			CreatedAt: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		}},
		Shard: contract.V2InvocationShard{
			ManifestID: uuid.NewString(), ManifestHash: strings.Repeat("2", 64),
			ShardKey: "script:full", CodepointStart: 0, CodepointEnd: len([]rune(text)),
		},
		StageInput: mustJSON(t, contract.ProposeScriptSpansInput{
			SourceVersionID: sourceID, SourceHash: sourceHash,
			NormalizedText: text, CodepointCount: len([]rune(text)), NewlineNormalization: "lf",
		}),
	}
	invocation, err := contract.NewV2StageInvocation(
		uuid.NewString(),
		uuid.NewString(),
		contract.StageReleaseIdentityV2{
			ReleaseID: uuid.NewString(), DefinitionHash: strings.Repeat("3", 64),
			BundleHash:       contract.StoryGraphV2SkillBundleHash,
			AgentImageDigest: "sha256:" + strings.Repeat("4", 64),
		},
		contract.StageControlProofV2{
			RecordID: uuid.NewString(), Revision: 1, Status: "approved",
			ContentHash: strings.Repeat("5", 64),
		},
		contract.V2ExecutionBudget{
			MaxModelCalls: 1, MaxExecutionSeconds: 120, MaxOutputBytes: 131072,
		},
		payload,
	)
	if err != nil {
		t.Fatalf("build v2 invocation: %v", err)
	}
	if invocation.WireSchemaVersion != "storygraph-stage-wire-v2" ||
		invocation.InputHash == "" || len(invocation.StageInstanceKey()) != 64 {
		t.Fatalf("unexpected v2 invocation identity: %#v", invocation)
	}
	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contract.DecodeV2StageInvocation(encoded)
	if err != nil || decoded.InputHash != invocation.InputHash {
		t.Fatalf("strict v2 round trip failed: decoded=%#v err=%v", decoded, err)
	}

	spanCandidate := mustJSON(t, map[string]any{
		"source_version_id": sourceID,
		"source_hash":       sourceHash,
		"codepoint_count":   len([]rune(text)),
		"spans": []any{
			map[string]any{
				"temporary_span_id": "span_0001", "kind": "scene",
				"codepoint_start": 0, "codepoint_end": 16, "heading": "第一场 夜 内",
				"evidence": map[string]any{
					"source_start": 0, "source_end": 7,
					"text_hash": hashTextV2("第一场 夜 内"), "exact_anchor": "第一场 夜 内",
				},
			},
			map[string]any{
				"temporary_span_id": "span_0002", "kind": "scene",
				"codepoint_start": 16, "codepoint_end": len([]rune(text)), "heading": "第二场 日 外",
				"evidence": map[string]any{
					"source_start": 16, "source_end": 23,
					"text_hash": hashTextV2("第二场 日 外"), "exact_anchor": "第二场 日 外",
				},
			},
		},
		"review_issues": []any{},
	})
	if err = contract.ValidateV2ScriptSpanCandidate(spanCandidate, text); err != nil {
		t.Fatalf("valid span candidate rejected: %v", err)
	}
	var drifted map[string]any
	if err = json.Unmarshal(spanCandidate, &drifted); err != nil {
		t.Fatal(err)
	}
	spans := drifted["spans"].([]any)
	spans[1].(map[string]any)["codepoint_start"] = float64(17)
	if err = contract.ValidateV2ScriptSpanCandidate(mustJSON(t, drifted), text); err == nil {
		t.Fatal("span coverage gap was accepted")
	}

	sceneFact := mustJSON(t, map[string]any{
		"source_version_id":            sourceID,
		"source_hash":                  sourceHash,
		"span_candidate_revision_id":   uuid.NewString(),
		"span_candidate_revision_hash": strings.Repeat("2", 64),
		"scenes": []any{
			map[string]any{
				"temporary_scene_id": "scene_0001", "span_id": "span_0001",
				"source_start": 0, "source_end": 16, "location_text": "室内", "time_text": "夜",
				"actions": []any{}, "dialogues": []any{}, "raw_character_mentions": []any{},
				"raw_prop_mentions": []any{},
			},
			map[string]any{
				"temporary_scene_id": "scene_0002", "span_id": "span_0002",
				"source_start": 16, "source_end": len([]rune(text)), "location_text": "室外", "time_text": "日",
				"actions": []any{}, "dialogues": []any{}, "raw_character_mentions": []any{},
				"raw_prop_mentions": []any{},
			},
		},
		"review_issues": []any{},
	})
	if err = contract.ValidateV2SceneFactCandidate(sceneFact, text, spanCandidate); err != nil {
		t.Fatalf("valid scene fact rejected: %v", err)
	}
	var styled map[string]any
	if err = json.Unmarshal(sceneFact, &styled); err != nil {
		t.Fatal(err)
	}
	styled["visual_style"] = "赛博朋克"
	if err = contract.ValidateV2SceneFactCandidate(mustJSON(t, styled), text, spanCandidate); err == nil {
		t.Fatal("style-bearing SceneFact candidate was accepted")
	}
}

type storyGraphV2WireFixture struct {
	ValidInvocation          json.RawMessage `json:"valid_invocation"`
	ValidScriptSpanCandidate json.RawMessage `json:"valid_script_span_candidate"`
	ExpectedInputHash        string          `json:"expected_input_hash"`
	ExpectedStageInstanceKey string          `json:"expected_stage_instance_key"`
	RejectMutations          []struct {
		Name      string `json:"name"`
		Operation string `json:"operation"`
		Path      string `json:"path"`
		Value     any    `json:"value"`
	} `json:"reject_mutations"`
}

func TestStoryGraphV2WireMatchesSharedFixtureAndRejectsMutations(t *testing.T) {
	fixture := loadStoryGraphV2WireFixture(t)
	invocation, err := contract.DecodeV2StageInvocation(fixture.ValidInvocation)
	if err != nil {
		t.Fatal(err)
	}
	inputHash, err := invocation.ComputeInputHash()
	if err != nil || inputHash != fixture.ExpectedInputHash || invocation.InputHash != inputHash {
		t.Fatalf("v2 input hash drifted: got=%s fixture=%s err=%v", inputHash, fixture.ExpectedInputHash, err)
	}
	if invocation.StageInstanceKey() != fixture.ExpectedStageInstanceKey {
		t.Fatalf("v2 stage instance key drifted: got=%s fixture=%s", invocation.StageInstanceKey(), fixture.ExpectedStageInstanceKey)
	}
	changedIdentity := invocation
	changedIdentity.InvocationID = uuid.NewString()
	changedIdentity.AttemptID = uuid.NewString()
	if unchangedHash, hashErr := changedIdentity.ComputeInputHash(); hashErr != nil || unchangedHash != inputHash {
		t.Fatalf("transport identity changed semantic input hash: hash=%s err=%v", unchangedHash, hashErr)
	}

	for _, mutation := range fixture.RejectMutations {
		var raw map[string]any
		if err = json.Unmarshal(fixture.ValidInvocation, &raw); err != nil {
			t.Fatal(err)
		}
		switch mutation.Operation {
		case "remove":
			delete(raw, mutation.Path)
		case "add_stage_input":
			raw["payload"].(map[string]any)["stage_input"].(map[string]any)[mutation.Path] = mutation.Value
		case "replace_scope":
			raw["payload"].(map[string]any)["scope"].(map[string]any)[mutation.Path] = mutation.Value
		default:
			raw[mutation.Path] = mutation.Value
		}
		if _, err = contract.DecodeV2StageInvocation(mustJSON(t, raw)); err == nil {
			t.Fatalf("v2 fixture mutation %q was accepted", mutation.Name)
		}
	}

	diagnostics := []contract.V2Diagnostic{}
	diagnosticJSON, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticHash, err := contract.CanonicalHash(diagnosticJSON)
	if err != nil {
		t.Fatal(err)
	}
	outputHash, err := contract.CanonicalHash(fixture.ValidScriptSpanCandidate)
	if err != nil {
		t.Fatal(err)
	}
	result := contract.V2AttemptResult{
		InvocationID: invocation.InvocationID, AttemptID: invocation.AttemptID,
		Kind: "storygraph_stage", WireSchemaVersion: contract.StoryGraphV2WireSchemaVersion,
		Variant: invocation.Payload.Variant, StageRelease: invocation.StageRelease,
		Control: invocation.Control, Status: "accepted", CandidateType: "script_span_candidate_v2",
		Candidate: fixture.ValidScriptSpanCandidate, InputHash: invocation.InputHash,
		OutputHash: &outputHash, Diagnostics: diagnostics, DiagnosticHash: diagnosticHash,
		CompletedAt: time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC),
		Executor: contract.V2Executor{
			RuntimeClass: "text", RuntimeImageDigest: invocation.StageRelease.AgentImageDigest,
			HarnessVersion: "storygraph-stage-harness-v2", Model: "codex-cli-default",
		},
	}
	encodedResult, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	decodedResult, err := contract.DecodeV2AttemptResult(encodedResult)
	if err != nil || decodedResult.ValidateFor(invocation) != nil {
		t.Fatalf("valid v2 AttemptResult rejected: decode=%v validate=%v", err, decodedResult.ValidateFor(invocation))
	}
	result.Status = "succeeded"
	if err = result.ValidateFor(invocation); err == nil {
		t.Fatal("legacy v1 result status was accepted on the v2 path")
	}
}

func loadStoryGraphV2WireFixture(t *testing.T) storyGraphV2WireFixture {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve v2 fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "fixtures", "agent", "storygraph-stage-wire-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture storyGraphV2WireFixture
	if err = json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func hashTextV2(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
