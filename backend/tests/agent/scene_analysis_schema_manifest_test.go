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

func TestSceneAnalysisCandidateSchemasMatchTheAgentModels(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "fixtures", "agent", "storygraph-scene-analysis-candidate-schemas.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, canonical, err := contract.DecodeSceneAnalysisCandidateSchemaManifest(raw)
	if err != nil {
		t.Fatalf("decode Scene Analysis Candidate Schema manifest: %v", err)
	}
	if !reflect.DeepEqual(manifest.Schemas, contract.SceneAnalysisCandidateSchemas()) {
		t.Fatalf("Backend Candidate Schema registry drifted: %#v", manifest.Schemas)
	}
	if !bytes.Equal(append(canonical, '\n'), raw) {
		t.Fatal("Candidate Schema manifest is not canonical")
	}

	var unknown map[string]any
	if err = json.Unmarshal(raw, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["provider"] = "latest"
	if _, _, err = contract.DecodeSceneAnalysisCandidateSchemaManifest(mustJSON(t, unknown)); err == nil {
		t.Fatal("unknown Candidate Schema manifest field was accepted")
	}
	delete(unknown, "provider")
	unknown["schema_set_hash"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, _, err = contract.DecodeSceneAnalysisCandidateSchemaManifest(mustJSON(t, unknown)); err == nil {
		t.Fatal("Candidate Schema manifest hash drift was accepted")
	}
}
