package agent_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestVisualFoundationSchemaManifestMatchesAgentModels(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "fixtures", "agent", "storygraph-visual-foundation-schemas.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, canonical, err := contract.DecodeVisualFoundationSchemaManifest(raw)
	if err != nil {
		t.Fatalf("decode Visual Foundation Schema manifest: %v", err)
	}
	if len(manifest.Schemas) != 2 || !bytes.Equal(append(canonical, '\n'), raw) {
		t.Fatal("Visual Foundation Schema manifest is incomplete or non-canonical")
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	value["runtime"] = "latest"
	if _, _, err = contract.DecodeVisualFoundationSchemaManifest(mustJSON(t, value)); err == nil {
		t.Fatal("unknown Visual Foundation Schema manifest field was accepted")
	}
	delete(value, "runtime")
	value["schema_set_hash"] = visualFoundationHash("drift")
	if _, _, err = contract.DecodeVisualFoundationSchemaManifest(mustJSON(t, value)); err == nil {
		t.Fatal("Visual Foundation Schema manifest hash drift was accepted")
	}
}

func TestVisualFoundationCandidateBindsPresetWorldAndDesignGaps(t *testing.T) {
	inputJSON := visualFoundationInputJSON(t)
	input, canonicalInput, err := contract.DecodeVisualFoundationInput(inputJSON)
	if err != nil || len(canonicalInput) == 0 {
		t.Fatalf("decode Visual Foundation input: %v", err)
	}
	candidateJSON := visualFoundationCandidateJSON(t, input)
	candidate, canonicalCandidate, err := contract.DecodeVisualFoundationCandidate(candidateJSON)
	if err != nil || len(canonicalCandidate) == 0 {
		t.Fatalf("decode Visual Foundation Candidate: %v", err)
	}
	if err = candidate.ValidateFor(input); err != nil {
		t.Fatalf("validate Visual Foundation Candidate: %v", err)
	}
}

func TestVisualFoundationCandidateRejectsUnsafeOrIncompleteOutput(t *testing.T) {
	input, _, err := contract.DecodeVisualFoundationInput(visualFoundationInputJSON(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"approved mutation":           func(value map[string]any) { value["approved"] = true },
		"missing conflict collection": func(value map[string]any) { delete(value, "world_conflicts") },
		"missing forbidden change": func(value map[string]any) {
			policy := value["style_policy"].(map[string]any)
			policy["forbidden_changes"] = []any{"story_fact"}
		},
		"missing design gap": func(value map[string]any) { value["creative_fill_proposals"] = []any{} },
		"invented source fact": func(value map[string]any) {
			adaptations := value["world_adaptations"].([]any)
			adaptations[0].(map[string]any)["source_fact_key"] = "fact_invented"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(visualFoundationCandidateJSON(t, input), &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			candidate, _, decodeErr := contract.DecodeVisualFoundationCandidate(mustJSON(t, value))
			if decodeErr == nil && candidate.ValidateFor(input) == nil {
				t.Fatal("unsafe Visual Foundation Candidate was accepted")
			}
		})
	}
}

func TestFaithfulVisualFoundationRejectsWorldAdaptation(t *testing.T) {
	var value map[string]any
	if err := json.Unmarshal(visualFoundationInputJSON(t), &value); err != nil {
		t.Fatal(err)
	}
	value["application_mode"] = "faithful"
	input, _, err := contract.DecodeVisualFoundationInput(mustJSON(t, value))
	if err != nil {
		t.Fatal(err)
	}
	candidate, _, err := contract.DecodeVisualFoundationCandidate(visualFoundationCandidateJSON(t, input))
	if err != nil {
		t.Fatal(err)
	}
	if err = candidate.ValidateFor(input); err == nil {
		t.Fatal("faithful Visual Foundation accepted a world adaptation")
	}
}

func TestVisualFoundationInputRejectsNonSemanticPresetRelease(t *testing.T) {
	var value map[string]any
	if err := json.Unmarshal(visualFoundationInputJSON(t), &value); err != nil {
		t.Fatal(err)
	}
	preset := value["preset_release"].(map[string]any)
	preset["release"] = "2026x09x12"
	if _, _, err := contract.DecodeVisualFoundationInput(mustJSON(t, value)); err == nil {
		t.Fatal("non-semantic Preset release was accepted")
	}
}

func visualFoundationInputJSON(t *testing.T) []byte {
	t.Helper()
	overrides := []contract.TypedVisualOverride{
		{
			OverrideKey: "override_architecture_detail", ScopeKind: "project",
			ScopeKey: "project:00000000-0000-0000-0000-000000000001", DesignDomain: "architecture",
			Value: "保留原场景动线，以木构细节替换未指定表皮。", CreatorDecisionHash: visualFoundationHash("decision"),
		},
	}
	attachments := []contract.VisualReferenceAttachment{
		{
			AttachmentID: "00000000-0000-0000-0000-000000000030",
			ObjectKey:    "visual-references/project-1/courtyard.webp", ContentHash: visualFoundationHash("attachment"),
			MediaType: "image/webp", RightsBasis: "owned", RightsRefHash: visualFoundationHash("rights"),
		},
	}
	overrideJSON, err := json.Marshal(overrides)
	if err != nil {
		t.Fatal(err)
	}
	overrideHash, err := contract.ProductionCanonicalHash(overrideJSON)
	if err != nil {
		t.Fatal(err)
	}
	attachmentJSON, err := json.Marshal(attachments)
	if err != nil {
		t.Fatal(err)
	}
	attachmentHash, err := contract.ProductionCanonicalHash(attachmentJSON)
	if err != nil {
		t.Fatal(err)
	}
	return mustJSON(t, contract.VisualFoundationInput{
		WorkspaceID:                 "00000000-0000-0000-0000-000000000010",
		ProjectID:                   "00000000-0000-0000-0000-000000000001",
		ProductionWorldOwnerSetHash: visualFoundationHash("owner-set"),
		ConfirmedWorldRoots: []contract.ConfirmedWorldRoot{
			{OwnerFamily: "asset_identity_state_set", ScopeKey: "project:00000000-0000-0000-0000-000000000001", CollectionRootHash: visualFoundationHash("asset-root")},
			{OwnerFamily: "bible_production_world_set", ScopeKey: "project:00000000-0000-0000-0000-000000000001", CollectionRootHash: visualFoundationHash("bible-root")},
			{OwnerFamily: "planning_scene_set", ScopeKey: "episode:00000000-0000-0000-0000-000000000020", CollectionRootHash: visualFoundationHash("planning-root")},
		},
		PresetRelease: contract.VisualFoundationPresetReleaseIdentity{
			Key: "xianxia-animation", Release: "2026.09.12", ContentHash: visualFoundationHash("preset"),
		},
		ApplicationMode:    "world_adaptation",
		FidelityInvariants: []string{"character_identity", "holder_relation", "scene_continuity", "story_fact"},
		AdaptationRules: []contract.PresetAdaptationRuleSnapshot{
			{
				RuleKey: "translate-technology-language", SourceFactKind: "technology_or_magic",
				DesignDomain: "technology_or_magic", Directive: "translate_expression_without_changing_function_or_outcome",
				PreservedInvariantKeys: []string{"character_identity", "holder_relation", "scene_continuity", "story_fact"},
				ImpactScopeKinds:       []string{"asset", "interaction", "scene"},
			},
		},
		VisualGrammar: contract.VisualGrammarSnapshot{
			Palette: "muted_jade_and_ink", MaterialRendering: "ink_wash_with_grounded_materials",
			Lighting: "motivated_soft_cinematic", Camera: "grounded_cinematic",
			NegativeConstraints: []string{"protected_ip_imitation", "story_fact_rewrite"},
		},
		TypedOverrides: overrides, TypedOverridesHash: overrideHash,
		ReferenceAttachments: attachments, ReferenceAttachmentsHash: attachmentHash,
		ConfirmedWorldFacts: []contract.ConfirmedWorldFact{
			{
				FactKey: "fact_phone_message", FactKind: "technology_or_magic", ContentHash: visualFoundationHash("fact"),
				AffectedScopeKeys: []string{"scene:00000000-0000-0000-0000-000000000002"},
			},
		},
		DesignGaps: []contract.VisualDesignGap{
			{
				GapKey: "gap_uniform_material", DesignDomain: "wardrobe", SourceConstraintHash: visualFoundationHash("gap"),
				AffectedScopeKeys: []string{"scene:00000000-0000-0000-0000-000000000002"},
			},
		},
	})
}

func visualFoundationCandidateJSON(t *testing.T, input contract.VisualFoundationInput) []byte {
	t.Helper()
	return mustJSON(t, contract.VisualFoundationCandidate{
		WorkspaceID: input.WorkspaceID, ProjectID: input.ProjectID,
		ProductionWorldOwnerSetHash: input.ProductionWorldOwnerSetHash,
		PresetReleaseContentHash:    input.PresetRelease.ContentHash, ApplicationMode: input.ApplicationMode,
		TypedOverridesHash: input.TypedOverridesHash, ReferenceAttachmentsHash: input.ReferenceAttachmentsHash,
		FidelityInvariants: append([]string(nil), input.FidelityInvariants...),
		StylePolicy: contract.VisualFoundationPolicy{
			PaletteRules:  []string{"muted_jade_and_ink"},
			MaterialRules: []string{"grounded_material_identity", "ink_wash_surface_language"},
			LightingRules: []string{"motivated_soft_cinematic"}, CameraRules: []string{"grounded_cinematic"},
			ForbiddenChanges: append([]string(nil), input.FidelityInvariants...),
		},
		WorldAdaptations: []contract.WorldAdaptationProposal{
			{
				MappingKey: "mapping_phone_message", SourceFactKey: "fact_phone_message",
				PresetRuleKey:          "translate-technology-language",
				DesignValue:            "手机传信改编为传音符，但发送者、接收者、信息内容与剧情结果不变。",
				PreservedInvariantKeys: append([]string(nil), input.FidelityInvariants...),
				AffectedScopeKeys:      []string{"scene:00000000-0000-0000-0000-000000000002"},
				DecisionStatus:         "needs_creator_decision",
			},
		},
		WorldConflicts: []contract.VisualWorldConflict{},
		CreativeFillProposals: []contract.CreativeFillProposal{
			{
				GapKey: "gap_uniform_material", DesignDomain: "wardrobe",
				Proposal:          "使用低反光织物并保留角色身份轮廓，具体纹样等待创作者确认。",
				AffectedScopeKeys: []string{"scene:00000000-0000-0000-0000-000000000002"},
				DecisionStatus:    "needs_creator_decision",
			},
		},
	})
}

func visualFoundationHash(seed string) string {
	value := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(value[:])
}
