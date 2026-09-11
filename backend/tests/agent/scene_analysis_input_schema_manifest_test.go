package agent_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestSceneAnalysisInputSchemasMatchTheAgentModels(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "fixtures", "agent", "storygraph-scene-analysis-input-schemas.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, canonical, err := contract.DecodeSceneAnalysisInputSchemaManifest(raw)
	if err != nil {
		t.Fatalf("decode Scene Analysis Input Schema manifest: %v", err)
	}
	if !reflect.DeepEqual(manifest.Schemas, contract.SceneAnalysisInputSchemas()) {
		t.Fatalf("Backend Input Schema registry drifted: %#v", manifest.Schemas)
	}
	if !bytes.Equal(append(canonical, '\n'), raw) {
		t.Fatal("Input Schema manifest is not canonical")
	}

	var unknown map[string]any
	if err = json.Unmarshal(raw, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["activation"] = "current"
	if _, _, err = contract.DecodeSceneAnalysisInputSchemaManifest(mustJSON(t, unknown)); err == nil {
		t.Fatal("runtime activation field was accepted by the Input Schema manifest")
	}
	delete(unknown, "activation")
	unknown["schema_set_hash"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, _, err = contract.DecodeSceneAnalysisInputSchemaManifest(mustJSON(t, unknown)); err == nil {
		t.Fatal("Input Schema manifest hash drift was accepted")
	}
}
