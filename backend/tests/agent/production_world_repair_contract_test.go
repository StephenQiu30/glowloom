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

func productionWorldRepairDirective(
	t *testing.T,
	operation string,
	stage string,
) contract.ProductionWorldRepairDirective {
	t.Helper()
	target := "character:linzhou"
	reason := "production_entity_incorrect"
	if operation == contract.ProductionWorldRepairRebindOccurrence {
		target, reason = "occurrence_scene_0001_character", "scene_occurrence_incorrect"
	}
	return contract.ProductionWorldRepairDirective{
		ReviewDecisionID: uuid.NewString(), DecisionPayloadHash: strings.Repeat("a", 64),
		IssueRefs: []string{},
		EvidenceRefs: []contract.StructureIdentityRepairEvidence{{
			SourceVersionID: uuid.NewString(), SourceStart: 0, SourceEnd: 2, TextHash: strings.Repeat("b", 64),
		}},
		ChangeSpec: contract.ProductionWorldRepairChange{Operation: operation, TargetKeys: []string{target}},
		Closure: contract.ProductionWorldRepairClosure{
			SceneScopeKeys: []string{"scene:0001"}, EntityKeys: []string{"character:linzhou"},
			StateKeys:       []string{"state_character_linzhou_initial"},
			OccurrenceKeys:  []string{"occurrence_scene_0001_character"},
			InteractionKeys: []string{}, ContinuityKeys: []string{}, LedgerKeys: []string{"ledger_character_scene_0001"},
		},
		ReasonCode: reason, BaseCandidate: productionWorldRepairBase(t, stage),
	}
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
