package agent_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

func TestProductionWorldRepairDirectiveIsBoundToStageBaseAndClosure(t *testing.T) {
	directive := productionWorldRepairDirective(t, contract.ProductionWorldRepairReviseEntity, "derive_production_entities")
	for _, stage := range []string{"derive_production_entities", "bind_scene_occurrences", "reconcile_interaction_continuity"} {
		value := directive
		value.BaseCandidate = productionWorldRepairBase(t, stage)
		if err := value.ValidateFor(stage); err != nil {
			t.Fatalf("stage %s: %v", stage, err)
		}
	}

	mutations := []func(*contract.ProductionWorldRepairDirective){
		func(value *contract.ProductionWorldRepairDirective) {
			value.ChangeSpec.TargetKeys[0] = "character:outside"
		},
		func(value *contract.ProductionWorldRepairDirective) { value.Closure.SceneScopeKeys = nil },
		func(value *contract.ProductionWorldRepairDirective) {
			value.BaseCandidate.CandidateContentHash = strings.Repeat("f", 64)
		},
		func(value *contract.ProductionWorldRepairDirective) { value.ReasonCode = "rewrite_everything" },
		func(value *contract.ProductionWorldRepairDirective) { value.EvidenceRefs = nil },
	}
	for index, mutate := range mutations {
		changed := productionWorldRepairDirective(t, contract.ProductionWorldRepairReviseEntity, "derive_production_entities")
		mutate(&changed)
		if err := changed.ValidateFor("derive_production_entities"); err == nil {
			t.Fatalf("accepted repair directive mutation %d: %#v", index, changed)
		}
	}
}

func TestProductionWorldRepairDirectiveRejectsStageBeforeTypedRoot(t *testing.T) {
	directive := productionWorldRepairDirective(t, contract.ProductionWorldRepairRebindOccurrence, "bind_scene_occurrences")
	directive.BaseCandidate = productionWorldRepairBase(t, "derive_production_entities")
	if err := directive.ValidateFor("derive_production_entities"); err == nil {
		t.Fatal("Scene occurrence repair was injected before its typed root")
	}
}

func TestProductionWorldRepairCandidatePreservesContentOutsideAuthorizedClosure(t *testing.T) {
	tests := []struct {
		name       string
		stage      string
		operation  string
		authorized func(map[string]any)
		forbidden  func(map[string]any)
	}{
		{
			name: "Production Entity", stage: "derive_production_entities",
			operation: contract.ProductionWorldRepairReviseEntity,
			authorized: func(value map[string]any) {
				value["entities"].([]any)[0].(map[string]any)["description"] = "修正后"
			},
			forbidden: func(value map[string]any) {
				value["entities"].([]any)[1].(map[string]any)["description"] = "越界修改"
			},
		},
		{
			name: "Scene Occurrence", stage: "bind_scene_occurrences",
			operation: contract.ProductionWorldRepairRebindOccurrence,
			authorized: func(value map[string]any) {
				value["scenes"].([]any)[0].(map[string]any)["occurrences"].([]any)[0].(map[string]any)["state_key"] = "state_character_corrected"
			},
			forbidden: func(value map[string]any) {
				value["scenes"].([]any)[0].(map[string]any)["dialogues"] = []any{map[string]any{"dialogue_key": "dialogue_added"}}
			},
		},
		{
			name: "Interaction", stage: "reconcile_interaction_continuity",
			operation: contract.ProductionWorldRepairReviseInteraction,
			authorized: func(value map[string]any) {
				value["interactions"].([]any)[0].(map[string]any)["predicate"] = "use"
				value["continuity"].([]any)[0].(map[string]any)["delta"] = "修正"
				value["continuity_ledger"].([]any)[0].(map[string]any)["state_key"] = "state_prop_corrected"
			},
			forbidden: func(value map[string]any) {
				value["scene_story_times"].([]any)[0].(map[string]any)["story_time_key"] = "storytime:changed"
			},
		},
		{
			name: "Continuity", stage: "reconcile_interaction_continuity",
			operation: contract.ProductionWorldRepairReviseContinuity,
			authorized: func(value map[string]any) {
				value["continuity"].([]any)[0].(map[string]any)["delta"] = "修正"
			},
			forbidden: func(value map[string]any) {
				value["interactions"].([]any)[0].(map[string]any)["predicate"] = "use"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := productionWorldRepairCandidate(test.stage)
			directive := productionWorldRepairDirective(t, test.operation, test.stage)
			directive.BaseCandidate = productionWorldRepairBaseFromCandidate(t, test.stage, base)
			if err := contract.ValidateProductionWorldRepairCandidate(
				directive, test.stage, mustJSON(t, base),
			); err == nil {
				t.Fatal("no-op repair was accepted")
			}

			authorized := cloneProductionWorldRepairCandidate(t, base)
			test.authorized(authorized)
			if err := contract.ValidateProductionWorldRepairCandidate(
				directive, test.stage, mustJSON(t, authorized),
			); err != nil {
				t.Fatalf("authorized repair rejected: %v", err)
			}

			forbidden := cloneProductionWorldRepairCandidate(t, base)
			test.forbidden(forbidden)
			if err := contract.ValidateProductionWorldRepairCandidate(
				directive, test.stage, mustJSON(t, forbidden),
			); err == nil {
				t.Fatal("repair escaped its authorized closure")
			}
		})
	}
}

func productionWorldRepairDirective(
	t *testing.T,
	operation string,
	stage string,
) contract.ProductionWorldRepairDirective {
	t.Helper()
	target, reason := "character:linzhou", "production_entity_incorrect"
	switch operation {
	case contract.ProductionWorldRepairRebindOccurrence:
		target, reason = "occurrence_scene_0001_character", "scene_occurrence_incorrect"
	case contract.ProductionWorldRepairReviseInteraction:
		target, reason = "interaction_one", "interaction_incorrect"
	case contract.ProductionWorldRepairReviseContinuity:
		target, reason = "continuity_one", "continuity_incorrect"
	}
	closure := contract.ProductionWorldRepairClosure{
		SceneScopeKeys: []string{"scene:0001"}, EntityKeys: []string{"character:linzhou"},
		StateKeys:       []string{"state_character_linzhou_initial"},
		OccurrenceKeys:  []string{"occurrence_scene_0001_character"},
		InteractionKeys: []string{"interaction_one"}, ContinuityKeys: []string{"continuity_one"},
		LedgerKeys: []string{"ledger_character_scene_0001"},
	}
	return contract.ProductionWorldRepairDirective{
		ReviewDecisionID: uuid.NewString(), DecisionPayloadHash: strings.Repeat("a", 64),
		IssueRefs: []string{},
		EvidenceRefs: []contract.StructureIdentityRepairEvidence{{
			SourceVersionID: uuid.NewString(), SourceStart: 0, SourceEnd: 2, TextHash: strings.Repeat("b", 64),
		}},
		ChangeSpec: contract.ProductionWorldRepairChange{Operation: operation, TargetKeys: []string{target}},
		Closure:    closure,
		ReasonCode: reason, BaseCandidate: productionWorldRepairBase(t, stage),
	}
}

func productionWorldRepairCandidate(stage string) map[string]any {
	switch stage {
	case "derive_production_entities":
		return map[string]any{
			"source_hash": "frozen",
			"entities": []any{
				map[string]any{
					"identity_key": "character:linzhou", "kind": "character",
					"specification_key": "specification_character_linzhou", "description": "原值",
					"states": []any{map[string]any{"state_key": "state_character_linzhou_initial", "value": "原值"}},
				},
				map[string]any{
					"identity_key": "prop:key", "kind": "prop", "specification_key": "specification_prop_key",
					"description": "原值", "states": []any{map[string]any{"state_key": "state_prop_key_initial", "value": "原值"}},
				},
			},
			"world_claims": []any{}, "design_gaps": []any{}, "review_issues": []any{},
		}
	case "bind_scene_occurrences":
		return map[string]any{
			"source_hash": "frozen",
			"scenes": []any{map[string]any{
				"scene_scope_key": "scene:0001", "dialogues": []any{}, "beats": []any{},
				"occurrences": []any{map[string]any{
					"occurrence_key": "occurrence_scene_0001_character", "state_key": "state_character_linzhou_initial",
				}},
			}},
			"review_issues": []any{},
		}
	default:
		return map[string]any{
			"source_hash":       "frozen",
			"scene_story_times": []any{map[string]any{"scene_scope_key": "scene:0001", "story_time_key": "storytime:one"}},
			"interactions": []any{
				map[string]any{"interaction_key": "interaction_one", "predicate": "hold"},
				map[string]any{"interaction_key": "interaction_two", "predicate": "carry"},
			},
			"continuity": []any{
				map[string]any{"continuity_key": "continuity_one", "delta": "原值"},
				map[string]any{"continuity_key": "continuity_two", "delta": "原值"},
			},
			"continuity_ledger": []any{
				map[string]any{"ledger_key": "ledger_character_scene_0001", "state_key": "state_character_linzhou_initial"},
				map[string]any{"ledger_key": "ledger_other", "state_key": "state_other"},
			},
			"review_issues": []any{},
		}
	}
}

func productionWorldRepairBaseFromCandidate(
	t *testing.T,
	stage string,
	candidate map[string]any,
) contract.ProductionWorldRepairBaseCandidate {
	t.Helper()
	raw := mustJSON(t, candidate)
	hash, err := contract.CanonicalHash(raw)
	if err != nil {
		t.Fatal(err)
	}
	value := productionWorldRepairBase(t, stage)
	value.Candidate, value.CandidateContentHash = raw, hash
	return value
}

func cloneProductionWorldRepairCandidate(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	var cloned map[string]any
	if err := json.Unmarshal(mustJSON(t, value), &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func productionWorldRepairBase(t *testing.T, stage string) contract.ProductionWorldRepairBaseCandidate {
	t.Helper()
	typeByStage := map[string]string{
		"derive_production_entities":       "production_entity_fragment_candidate",
		"bind_scene_occurrences":           "scene_binding_fragment_candidate",
		"reconcile_interaction_continuity": "continuity_fragment_candidate",
	}
	body, err := json.Marshal(map[string]any{"stage": stage, "value": "frozen"})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := contract.CanonicalHash(body)
	if err != nil {
		t.Fatal(err)
	}
	return contract.ProductionWorldRepairBaseCandidate{
		Identity: contract.SceneAnalysisCandidateRevisionIdentity{
			StageKey: stage, ShardKey: "script:full", CandidateRevisionID: uuid.NewString(),
			CandidateRevisionHash: strings.Repeat("c", 64), SourceInvocationID: uuid.NewString(),
			SourceResultHash: strings.Repeat("d", 64),
		},
		CandidateType: typeByStage[stage], CandidateContentHash: hash, Candidate: body,
	}
}
