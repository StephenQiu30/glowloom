package preset_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	preset "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
)

func TestProjectPresetSelectionFreezesExactReleaseAndMode(t *testing.T) {
	release, _, err := preset.NewRelease(validPresetReleaseInput())
	if err != nil {
		t.Fatal(err)
	}
	selection, encoded, err := preset.NewProjectSelection(uuid.NewString(), preset.ProjectSelectionInput{
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), Revision: 1,
		PresetRelease: preset.ProjectSelectionRelease{
			Key: release.Key, Release: release.Release, ContentHash: release.ContentHash,
		},
		ApplicationMode: "faithful", SelectedBy: uuid.NewString(),
		SelectedAt: time.Date(2026, time.September, 12, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("new Project Preset selection: %v", err)
	}
	if selection.ContractID != preset.ProjectSelectionContractID || selection.ContentHash == "" ||
		selection.PresetRelease.ContentHash != release.ContentHash || selection.ApplicationMode != "faithful" {
		t.Fatalf("incomplete Project Preset selection: %#v", selection)
	}
	decoded, canonical, err := preset.DecodeProjectSelection(encoded)
	if err != nil {
		t.Fatalf("decode Project Preset selection: %v", err)
	}
	if !reflect.DeepEqual(decoded, selection) || string(canonical) != string(encoded) {
		t.Fatal("Project Preset selection did not round-trip canonically")
	}
}

func TestProjectPresetSelectionRequiresLinearParentAndSemanticRelease(t *testing.T) {
	release, _, err := preset.NewRelease(validPresetReleaseInput())
	if err != nil {
		t.Fatal(err)
	}
	base := preset.ProjectSelectionInput{
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), Revision: 1,
		PresetRelease:   preset.ProjectSelectionRelease{Key: release.Key, Release: release.Release, ContentHash: release.ContentHash},
		ApplicationMode: "faithful", SelectedBy: uuid.NewString(), SelectedAt: time.Now().UTC(),
	}
	invalid := []preset.ProjectSelectionInput{
		func() preset.ProjectSelectionInput { value := base; value.Revision = 2; return value }(),
		func() preset.ProjectSelectionInput {
			value := base
			parent := uuid.NewString()
			value.ParentSelectionID = &parent
			return value
		}(),
		func() preset.ProjectSelectionInput {
			value := base
			value.PresetRelease.Release = "current"
			return value
		}(),
		func() preset.ProjectSelectionInput {
			value := base
			value.PresetRelease.ContentHash = "short"
			return value
		}(),
		func() preset.ProjectSelectionInput { value := base; value.ApplicationMode = "custom"; return value }(),
	}
	for index, input := range invalid {
		if _, _, err = preset.NewProjectSelection(uuid.NewString(), input); err == nil {
			t.Fatalf("invalid Project Preset selection %d was accepted", index)
		}
	}
	parentID, parentHash := uuid.NewString(), release.ContentHash
	base.Revision, base.ParentSelectionID, base.ParentContentHash = 2, &parentID, &parentHash
	if _, _, err = preset.NewProjectSelection(uuid.NewString(), base); err != nil {
		t.Fatalf("valid successor Project Preset selection: %v", err)
	}
}

func TestProjectPresetSelectionRejectsUnknownFieldsAndHashDrift(t *testing.T) {
	release, _, err := preset.NewRelease(validPresetReleaseInput())
	if err != nil {
		t.Fatal(err)
	}
	_, encoded, err := preset.NewProjectSelection(uuid.NewString(), preset.ProjectSelectionInput{
		WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(), Revision: 1,
		PresetRelease:   preset.ProjectSelectionRelease{Key: release.Key, Release: release.Release, ContentHash: release.ContentHash},
		ApplicationMode: "world_adaptation", SelectedBy: uuid.NewString(),
		SelectedAt: time.Date(2026, time.September, 12, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	payload["visual_style"] = "free-form"
	mutated, _ := json.Marshal(payload)
	if _, _, err = preset.DecodeProjectSelection(mutated); err == nil {
		t.Fatal("unknown Project Preset selection field was accepted")
	}
	delete(payload, "visual_style")
	payload["content_hash"] = release.ContentHash
	mutated, _ = json.Marshal(payload)
	if _, _, err = preset.DecodeProjectSelection(mutated); err == nil {
		t.Fatal("Project Preset selection content hash drift was accepted")
	}
}
