package generation_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
)

func TestGenerationTargetCanonicalizesReferenceAssetAndRejectsMixedPayloads(t *testing.T) {
	createdAt := time.Date(2026, 8, 29, 9, 30, 0, 0, time.UTC)
	input := domain.GenerationTargetInput{
		ID:          "10000000-0000-4000-8000-000000000001",
		WorkspaceID: "20000000-0000-4000-8000-000000000002",
		ProjectID:   "30000000-0000-4000-8000-000000000003",
		Kind:        domain.GenerationTargetReferenceAsset,
		SourceOwnerRef: domain.FrozenOwnerReference{
			Owner: "storyboard", Resource: "approved_storyboard_intents",
			ID: "40000000-0000-4000-8000-000000000004", Revision: 1,
			ContentHash: generationHash("1"),
		},
		PolicySnapshotRef: domain.FrozenOwnerReference{
			Owner: "preset", Resource: "effective_style_snapshot",
			ID: "30000000-0000-4000-8000-000000000003", Revision: 3,
			ContentHash: generationHash("2"),
		},
		ReferenceAsset: &domain.ReferenceAssetTarget{
			AssetID: "50000000-0000-4000-8000-000000000005", AssetKind: "character",
			SpecificationVersionRef: domain.FrozenOwnerReference{
				Owner: "production", Resource: "production_bible_specification_version",
				ID: "60000000-0000-4000-8000-000000000006", Revision: 1,
				ContentHash: generationHash("3"),
			},
			AssetStateRef: domain.FrozenOwnerReference{
				Owner: "asset", Resource: "asset_state",
				ID: "70000000-0000-4000-8000-000000000007", Revision: 1,
				ContentHash: generationHash("4"),
			},
			OutputKind: "reference_sheet", RequiredViewRoles: []string{"back", "front", "profile"},
			PromptVersion:  "character-reference-sheet",
			PositivePrompt: "same character, front profile and back views",
			NegativePrompt: "different identities, missing view, text, watermark",
			Width:          1536, Height: 1024, NumberResults: 4, OutputFormat: "PNG",
		},
		Revision: 1, CreatedBy: "80000000-0000-4000-8000-000000000008", CreatedAt: createdAt,
	}
	target, err := domain.NewGenerationTarget(input)
	if err != nil {
		t.Fatal(err)
	}
	if target.TargetHash == "" || len(target.TargetHash) != 64 || target.ReferenceAsset == nil ||
		target.ShotFrame != nil || target.ReferenceAsset.RequiredViewRoles[0] != "front" ||
		target.ReferenceAsset.RequiredViewRoles[1] != "profile" ||
		target.ReferenceAsset.RequiredViewRoles[2] != "back" {
		t.Fatalf("reference asset target was not canonicalized: %#v", target)
	}
	reordered := input
	reordered.ReferenceAsset = &domain.ReferenceAssetTarget{}
	*reordered.ReferenceAsset = *input.ReferenceAsset
	reordered.ReferenceAsset.RequiredViewRoles = []string{"profile", "back", "front"}
	again, err := domain.NewGenerationTarget(reordered)
	if err != nil || again.TargetHash != target.TargetHash {
		t.Fatalf("equivalent target changed hash: first=%s second=%s err=%v", target.TargetHash, again.TargetHash, err)
	}
	if err = domain.ValidateGenerationTarget(target); err != nil {
		t.Fatalf("valid target failed reconstruction: %v", err)
	}
	drifted := target
	drifted.ReferenceAsset = &domain.ReferenceAssetTarget{}
	*drifted.ReferenceAsset = *target.ReferenceAsset
	drifted.ReferenceAsset.RequiredViewRoles = []string{"back", "front", "profile"}
	if err = domain.ValidateGenerationTarget(drifted); err == nil {
		t.Fatal("non-canonical persisted target was accepted")
	}

	mixed := input
	mixed.ShotFrame = &domain.ShotFrameTarget{
		ShotRef: domain.FrozenOwnerReference{
			Owner: "storyboard", Resource: "shot", ID: "90000000-0000-4000-8000-000000000009",
			Revision: 1, ContentHash: generationHash("5"),
		},
	}
	if _, err = domain.NewGenerationTarget(mixed); err == nil {
		t.Fatal("mixed reference_asset and shot_frame payload was accepted")
	}
	missing := input
	missing.ReferenceAsset = nil
	if _, err = domain.NewGenerationTarget(missing); err == nil {
		t.Fatal("target without its kind payload was accepted")
	}
	wrongViews := input
	wrongViews.ReferenceAsset = &domain.ReferenceAssetTarget{}
	*wrongViews.ReferenceAsset = *input.ReferenceAsset
	wrongViews.ReferenceAsset.RequiredViewRoles = []string{"front", "profile"}
	if _, err = domain.NewGenerationTarget(wrongViews); err == nil {
		t.Fatal("character reference sheet without front/profile/back was accepted")
	}
}

func generationHash(character string) string {
	value := ""
	for len(value) < 64 {
		value += character
	}
	return value[:64]
}
