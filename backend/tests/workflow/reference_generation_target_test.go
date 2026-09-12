package workflow_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	agentapp "github.com/StephenQiu30/lanverse/backend/internal/agent/application"
	openaiadapter "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/openai"
	generationapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

func assertPersistedReferenceImageCompilation(t *testing.T, target generationapp.ReferenceGenerationTarget, brief agentapp.AcceptedReferenceBrief, profile domain.ProviderModelProfileVersion) {
	t.Helper()
	compiled, err := openaiadapter.NewFactory(nil, nil, nil).CompileReferenceImages(target, brief, profile)
	if err != nil || len(compiled.Requests) != target.OutputContract.CandidateBundleCount*len(target.OutputContract.Slots) {
		t.Fatalf("compile persisted Target/Brief/Profile: %v", err)
	}
	replay, err := openaiadapter.NewFactory(nil, nil, nil).CompileReferenceImages(target, brief, profile)
	if err != nil || !reflect.DeepEqual(replay, compiled) {
		t.Fatalf("persisted compilation replay: %v", err)
	}
	raw, err := json.Marshal(compiled)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"prompt", "source_design_slots", "ciphertext", "api_key", "https://"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("request content leaked into manifest")
		}
	}
	preimage := compiled
	preimage.ManifestHash = ""
	raw, err = json.Marshal(preimage)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := canonical.Hash(raw)
	if err != nil || hash != compiled.ManifestHash {
		t.Fatal("manifest identity drifted")
	}
}

func assertReferenceGenerationTargetContract(t *testing.T, target generationapp.ReferenceGenerationTarget) {
	t.Helper()
	raw, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := generationapp.DecodeReferenceGenerationTarget(raw)
	if err != nil || !reflect.DeepEqual(decoded, target) {
		t.Fatalf("Target roundtrip: %v", err)
	}
	for name, mutate := range map[string]func(*generationapp.ReferenceGenerationTarget){
		"identity": func(value *generationapp.ReferenceGenerationTarget) { value.ID = uuid.NewString() },
		"source":   func(value *generationapp.ReferenceGenerationTarget) { value.SourcePayload = json.RawMessage(`{}`) },
		"read set": func(value *generationapp.ReferenceGenerationTarget) {
			value.TargetReadSetRoot = strings.Repeat("f", 64)
		},
		"generation round": func(value *generationapp.ReferenceGenerationTarget) { value.GenerationRound++ },
	} {
		changed := target
		mutate(&changed)
		changedRaw, marshalErr := json.Marshal(changed)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, err = generationapp.DecodeReferenceGenerationTarget(changedRaw); err == nil {
			t.Fatalf("Target accepted %s drift", name)
		}
	}
	audit := target
	audit.CreatedBy, audit.CreatedAt = uuid.NewString(), target.CreatedAt.Add(time.Minute)
	auditRaw, err := json.Marshal(audit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = generationapp.DecodeReferenceGenerationTarget(auditRaw); err != nil {
		t.Fatalf("audit fields changed Target content identity: %v", err)
	}
	var document map[string]json.RawMessage
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	document["provider"] = json.RawMessage(`"forbidden"`)
	extra, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = generationapp.DecodeReferenceGenerationTarget(extra); err == nil {
		t.Fatal("Target accepted Provider field")
	}
	// Even a newly self-hashed document cannot add fields inside the source union.
	changed := target
	var source map[string]json.RawMessage
	if err = json.Unmarshal(changed.SourcePayload, &source); err != nil {
		t.Fatal(err)
	}
	source["latest_asset"] = json.RawMessage(`true`)
	changed.SourcePayload, err = json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	preimage := changed
	preimage.ContentHash, preimage.CreatedBy, preimage.CreatedAt = "", "", time.Time{}
	preimageRaw, err := json.Marshal(preimage)
	if err != nil {
		t.Fatal(err)
	}
	changed.ContentHash, err = canonical.Hash(preimageRaw)
	if err != nil {
		t.Fatal(err)
	}
	changedRaw, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = generationapp.DecodeReferenceGenerationTarget(changedRaw); err == nil {
		t.Fatal("Target accepted unknown source field with recomputed hash")
	}
}
