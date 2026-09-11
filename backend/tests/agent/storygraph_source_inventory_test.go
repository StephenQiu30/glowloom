package agent_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestStoryGraphSourceInventoryPinsReviewedExternalReferences(t *testing.T) {
	inventory, canonical, err := contract.PinnedStoryGraphSourceInventory()
	if err != nil || len(inventory.Sources) != 3 || len(canonical) == 0 {
		t.Fatalf("load pinned StoryGraph Source Inventory: inventory=%#v err=%v", inventory, err)
	}
	wantSources := map[string]struct {
		sourceHash, licenseHash   string
		sourceBytes, licenseBytes int64
	}{
		"agent-skills-specification": {
			"b9079c0c10b7930e8c6a20ff2bc10cda2a3343c55185120e3f1116a1a529b220",
			"993da3f8d330d69108e521636555f7783ef81d46b904394c59ec5ff651331fe5", 7166, 11344,
		},
		"anthropic-skill-creator": {
			"dcd4803e61e913e6fc27294184cd3a71f09f5e924ff20c8a9a20173e7b3c2bcf",
			"bc6b3af2f331cbc7fb0da1344efb2cbe5877a31498b4d70dbc7000f3405a1362", 33168, 11345,
		},
		"libtv-public-skill": {
			"7e8ffa94928cf50af201c8fb36d094af74664e274346f4e3ee49ef097585866a",
			"f9209da70e868f531222a9c2e75e195d67d8cde93ee1ec0cdb47002ffe0e0bf0", 11469, 1067,
		},
	}
	wantIDs := []string{"agent-skills-specification", "anthropic-skill-creator", "libtv-public-skill"}
	gotIDs := make([]string, len(inventory.Sources))
	for index, source := range inventory.Sources {
		gotIDs[index] = source.SourceID
		want, exists := wantSources[source.SourceID]
		if source.AbsorptionMode != "reference_only" || len(source.ProductionFileMappings) != 0 ||
			source.ReviewerID != "project-owner" || source.SourceCommit != source.SourceVersion ||
			len(source.ExpectedCapabilityKeys) == 0 || !exists || source.SourceFileSHA256 != want.sourceHash ||
			source.License.SHA256 != want.licenseHash || source.SourceByteLength != want.sourceBytes ||
			source.License.ByteLength != want.licenseBytes {
			t.Fatalf("external source escaped its reviewed boundary: %#v", source)
		}
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("Source Inventory order = %v want %v", gotIDs, wantIDs)
	}

	decoded, roundTrip, err := contract.DecodeStoryGraphSourceInventory(canonical)
	if err != nil || decoded.SourceInventoryHash != inventory.SourceInventoryHash || !slices.Equal(roundTrip, canonical) {
		t.Fatalf("Source Inventory did not round-trip: decoded=%#v err=%v", decoded, err)
	}
}

func TestStoryGraphSourceInventoryRejectsMutableOrUnreviewedSources(t *testing.T) {
	_, canonical, err := contract.PinnedStoryGraphSourceInventory()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"unknown field": func(value map[string]any) { value["active"] = true },
		"mutable revision": func(value map[string]any) {
			value["sources"].([]any)[0].(map[string]any)["source_commit"] = "current"
		},
		"unmapped copy": func(value map[string]any) {
			value["sources"].([]any)[0].(map[string]any)["absorption_mode"] = "adopt"
		},
		"source drift": func(value map[string]any) {
			value["sources"].([]any)[0].(map[string]any)["source_file_sha256"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(canonical, &candidate); err != nil {
				t.Fatal(err)
			}
			mutate(candidate)
			if _, _, err := contract.DecodeStoryGraphSourceInventory(mustJSON(t, candidate)); err == nil {
				t.Fatal("invalid external Source Inventory was accepted")
			}
		})
	}
}
