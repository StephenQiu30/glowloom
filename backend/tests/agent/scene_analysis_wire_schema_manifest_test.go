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

func TestSceneAnalysisWireSchemasMatchTheAgentModels(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "fixtures", "agent", "storygraph-scene-analysis-wire-schemas.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, canonical, err := contract.DecodeSceneAnalysisWireSchemaManifest(raw)
	if err != nil {
		t.Fatalf("decode Scene Analysis Wire Schema manifest: %v", err)
	}
	if !reflect.DeepEqual(manifest.Schemas, contract.SceneAnalysisWireSchemas()) {
		t.Fatalf("Backend Wire Schema registry drifted: %#v", manifest.Schemas)
	}
	if !bytes.Equal(append(canonical, '\n'), raw) {
		t.Fatal("Wire Schema manifest is not canonical")
	}

	var unknown map[string]any
	if err = json.Unmarshal(raw, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["release"] = "latest"
	if _, _, err = contract.DecodeSceneAnalysisWireSchemaManifest(mustJSON(t, unknown)); err == nil {
		t.Fatal("unknown Wire Schema manifest field was accepted")
	}
	delete(unknown, "release")
	unknown["wire_schema_hash"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, _, err = contract.DecodeSceneAnalysisWireSchemaManifest(mustJSON(t, unknown)); err == nil {
		t.Fatal("Wire Schema manifest hash drift was accepted")
	}
}
