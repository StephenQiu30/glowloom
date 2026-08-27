package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

type storyGraphWireFixture struct {
	WireSchemaVersion        string                             `json:"wire_schema_version"`
	ValidInvocation          json.RawMessage                    `json:"valid_invocation"`
	ExpectedInputHash        string                             `json:"expected_input_hash"`
	ExpectedStageInstanceKey string                             `json:"expected_stage_instance_key"`
	ValidSuccessResult       json.RawMessage                    `json:"valid_success_result"`
	ExpectedResultHash       string                             `json:"expected_result_hash"`
	ValidUnknownResult       json.RawMessage                    `json:"valid_unknown_result"`
	ExecutionGrantClaims     contract.StageExecutionGrantClaims `json:"execution_grant_claims"`
}

func TestStoryGraphStageWireMatchesCanonicalFixture(t *testing.T) {
	fixture := loadStoryGraphWireFixture(t)
	invocation, err := contract.DecodeStageInvocation(fixture.ValidInvocation)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.WireSchemaVersion != contract.StoryGraphWireSchemaVersion {
		t.Fatalf("wire schema version = %q", fixture.WireSchemaVersion)
	}
	inputHash, err := invocation.ComputeInputHash()
	if err != nil || inputHash != fixture.ExpectedInputHash || invocation.InputHash != inputHash {
		t.Fatalf("input hash = %q, fixture = %q, invocation = %q, error = %v", inputHash, fixture.ExpectedInputHash, invocation.InputHash, err)
	}
	stageKey, err := invocation.StageInstanceKey()
	if err != nil || stageKey != fixture.ExpectedStageInstanceKey {
		t.Fatalf("stage instance key = %q, want %q, error = %v", stageKey, fixture.ExpectedStageInstanceKey, err)
	}
	changedID := invocation
	changedID.InvocationID = "20000000-0000-0000-0000-000000000099"
	unchangedHash, err := changedID.ComputeInputHash()
	if err != nil || unchangedHash != inputHash {
		t.Fatalf("invocation id changed input hash to %q: %v", unchangedHash, err)
	}
	changedPolicy := invocation
	changedPolicy.ExecutionPolicy.MaxModelCalls++
	changedHash, err := changedPolicy.ComputeInputHash()
	if err != nil || changedHash == inputHash {
		t.Fatalf("policy mutation did not change input hash: %q, %v", changedHash, err)
	}
}

func TestStoryGraphStageResultAndGrantClaimsAreStrict(t *testing.T) {
	fixture := loadStoryGraphWireFixture(t)
	invocation, err := contract.DecodeStageInvocation(fixture.ValidInvocation)
	if err != nil {
		t.Fatal(err)
	}
	success, err := contract.DecodeStageResult(fixture.ValidSuccessResult)
	if err != nil {
		t.Fatal(err)
	}
	resultHash, err := success.ComputeResultHash()
	if err != nil || resultHash != fixture.ExpectedResultHash || success.ResultHash == nil || *success.ResultHash != resultHash {
		t.Fatalf("result hash = %q, want %q, error = %v", resultHash, fixture.ExpectedResultHash, err)
	}
	if err = success.ValidateFor(invocation); err != nil {
		t.Fatal(err)
	}
	unknown, err := contract.DecodeStageResult(fixture.ValidUnknownResult)
	if err != nil || unknown.ValidateFor(invocation) != nil {
		t.Fatalf("valid unknown result rejected: decode=%v validate=%v", err, unknown.ValidateFor(invocation))
	}
	if err = fixture.ExecutionGrantClaims.ValidateFor(invocation, 1799999999); err != nil {
		t.Fatal(err)
	}
	changed := fixture.ExecutionGrantClaims
	changed.FencingToken = 0
	if err = changed.ValidateFor(invocation, 1799999999); err == nil {
		t.Fatal("execution grant claims accepted a zero fencing token")
	}

	var raw map[string]any
	if err = json.Unmarshal(fixture.ValidInvocation, &raw); err != nil {
		t.Fatal(err)
	}
	raw["unexpected"] = true
	withExtra, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = contract.DecodeStageInvocation(withExtra); err == nil {
		t.Fatal("stage invocation accepted an unknown field")
	}
}

func TestSourceEvidenceStageInputIsStrictAndBoundToItsSourceShard(t *testing.T) {
	fixture := loadStoryGraphWireFixture(t)
	var raw map[string]any
	if err := json.Unmarshal(fixture.ValidInvocation, &raw); err != nil {
		t.Fatal(err)
	}
	payload := raw["payload"].(map[string]any)
	stageInput := payload["stage_input"].(map[string]any)
	stageInput["unexpected"] = true
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = contract.DecodeStageInvocation(encoded); err == nil {
		t.Fatal("Source Evidence stage input accepted an unknown field")
	}

	if err = json.Unmarshal(fixture.ValidInvocation, &raw); err != nil {
		t.Fatal(err)
	}
	payload = raw["payload"].(map[string]any)
	stageInput = payload["stage_input"].(map[string]any)
	stageInput["logical_end"] = float64(10)
	encoded, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = contract.DecodeStageInvocation(encoded); err == nil {
		t.Fatal("Source Evidence stage input accepted a range that drifted from its shard")
	}
}

func loadStoryGraphWireFixture(t *testing.T) storyGraphWireFixture {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "fixtures", "agent", "storygraph-stage-wire-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture storyGraphWireFixture
	if err = json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}
