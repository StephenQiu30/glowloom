package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

type productionWireFixture struct {
	CanonicalUnicodeRoot json.RawMessage `json:"canonical_unicode_root"`
	CanonicalUnicodeJSON string          `json:"canonical_unicode_json"`
	CanonicalUnicodeHash string          `json:"canonical_unicode_hash"`
	ValidInvocation      json.RawMessage `json:"valid_invocation"`
	ExpectedInputHash    string          `json:"expected_input_hash"`
	ExpectedStageKey     string          `json:"expected_stage_instance_key"`
}

func TestProductionWireUsesSemanticIdentityAndCanonicalUnicode(t *testing.T) {
	if contract.SceneAnalysisWireSchemaVersion != "storygraph-stage-wire-production" {
		t.Fatalf("production Wire identity = %q", contract.SceneAnalysisWireSchemaVersion)
	}
	fixture := loadProductionWireFixture(t)
	var rawInvocation contract.SceneAnalysisInvocation
	if err := json.Unmarshal(fixture.ValidInvocation, &rawInvocation); err != nil {
		t.Fatal(err)
	}
	computedInputHash, err := rawInvocation.ComputeInputHash()
	if err != nil {
		t.Fatal(err)
	}
	if computedInputHash != fixture.ExpectedInputHash || rawInvocation.InputHash != computedInputHash {
		t.Fatalf("production input hash drifted: got=%s fixture=%s invocation=%s", computedInputHash, fixture.ExpectedInputHash, rawInvocation.InputHash)
	}
	if rawInvocation.StageInstanceKey() != fixture.ExpectedStageKey {
		t.Fatalf("production stage instance key drifted: got=%s want=%s", rawInvocation.StageInstanceKey(), fixture.ExpectedStageKey)
	}
	invocation, err := contract.DecodeSceneAnalysisInvocation(fixture.ValidInvocation)
	if err != nil {
		t.Fatal(err)
	}
	if invocation.WireSchemaVersion != "storygraph-stage-wire-production" ||
		invocation.Payload.Variant.LaneKey != "primary" ||
		invocation.Payload.Variant.OutputSchemaVersion != "script-span-candidate-production" {
		t.Fatalf("production Wire semantic identity = %#v", invocation)
	}
	canonical, err := contract.ProductionCanonicalJSON(fixture.CanonicalUnicodeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != fixture.CanonicalUnicodeJSON {
		t.Fatalf("canonical JSON drifted: got=%q want=%q", canonical, fixture.CanonicalUnicodeJSON)
	}
	hash, err := contract.ProductionCanonicalHash(fixture.CanonicalUnicodeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if hash != fixture.CanonicalUnicodeHash {
		t.Fatalf("canonical hash drifted: got=%s want=%s", hash, fixture.CanonicalUnicodeHash)
	}
}

func loadProductionWireFixture(t *testing.T) productionWireFixture {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve production Wire fixture path")
	}
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "fixtures", "agent", "storygraph-scene-analysis-wire.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture productionWireFixture
	if err = json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}
