package domain

import (
	"encoding/json"
	"fmt"
	"slices"
)

func productionEdgeMatrixDefinitions() []ProductionEdgeMatrixDefinition {
	values := make([]ProductionEdgeMatrixDefinition, 0, 65)
	values = append(values, productionMatrixRows("edge-type-matrix-contract", []string{"edge_type", "allowed_source_to_target", "qualifier_and_cardinality"}, [][]string{
		{"contains", "episode → scene; scene → dialogue|narrative_beat", "Parent equals Target Owner parent ref; sequence_key equals Owner order key; each child exactly one parent"},
		{"derived_from", "source_revision → source_evidence; source_evidence → asset_identity|SPECIFICATION|asset_state|world_rule|story_arc|plot_thread|episode|scene|dialogue|narrative_beat|occurrence", "Evidence exactly one Source Revision; other inputs equal Envelope Evidence; creator decision permits zero where declared; structural nodes require 1..n"},
		{"describes_identity", "asset_identity → SPECIFICATION", "each Specification exactly one; asset kind matches"},
		{"has_state", "asset_identity → asset_state", "each State exactly one; asset kind matches"},
		{"precedes", "episode|scene|narrative_beat|shot adjacent same-type nodes", "same parent or Scene scope; sequence_key equals following Owner order key; only adjacent nodes"},
		{"anchors_occurrence", "scene|narrative_beat → occurrence", "anchor_role=scene|beat; equals scene_ref plus beat_ref; Scene exactly one and Beat 0..1"},
		{"instantiates_occurrence", "asset_state → occurrence", "equals asset_state_ref; exactly one"},
		{"supports", "source_evidence → ANY_CLAIM", "equals Claim Envelope Evidence; creator decision permits zero"},
		{"claim_participant", "asset_identity|occurrence → ANY_CLAIM", "participant_role=subject|object|participant|actor|prop|counterparty|holder_before|holder_after; branch cardinality matrix"},
		{"claim_anchor", "STRUCTURE_ANCHOR → ANY_CLAIM", "anchor_role=episode|scene|beat|character_occurrence|prop_occurrence|scope_start|scope_end; branch cardinality matrix"},
		{"claim_state", "asset_state → continuity_claim", "state_role=before|after|prop_before|prop_after; branch cardinality matrix"},
		{"supersedes", "same Node Type, Claim family/discriminant, and claim_series_key: Older Claim → Newer Claim", "equals supersedes_claim_ref 0..1; revision increments by one; no self-edge or cross-discriminant supersession"},
		{"constrains", "world_rule|policy_snapshot|effective_style_snapshot → CONSTRAINT_TARGET", "constraint_role=world|policy|style; equals exact constraint refs"},
		{"materializes", "Materialization Matrix", "binding_role=asset|specification|state|identity_anchor|artifact; no other combinations"},
		{"contains_reference_target", "approved_reference_plan_version → reference_plan_target", "sequence_key equals Plan Owner order key; each Target exactly one Plan parent"},
		{"depends_on_reference_target", "reference_plan_target → reference_plan_target", "equals payload dependencies and is acyclic inside one Plan"},
		{"plans_reference", "Reference Planning Matrix", "reference_role=identity|specification|state|style|scene|occurrence|interaction"},
		{"fulfills_reference_target", "reference_plan_target → REFERENCE_RESULT", "target/result type and phase-aware cardinality matrices"},
		{"binds_reference_input", "Reference Binding Matrix", "reference_role=scene|occurrence|interaction|character_asset|location_asset|prop_asset"},
		{"binds_reference_output", "artifact → REFERENCE_BINDING", "each Binding exactly one selected READY Composition Artifact"},
		{"realizes", "narrative_beat → shot", "each Shot input 1..n and equals source_beat_refs"},
		{"informs", "occurrence|scene_reference_binding_version|interaction_reference_binding_version → shot_production_binding_version", "informs_role=occurrence|scene_reference|interaction_reference; same Scene and equals payload arrays"},
		{"binds_input", "shot|occurrence|asset_version|REFERENCE_BINDING|effective_style_snapshot → shot_production_binding_version", "binding_role=shot|occurrence|asset_version|scene_reference|interaction_reference|style; equals payload"},
	})...)
	values = append(values, productionMatrixRows("claim-cardinality-matrix-contract", []string{"claim_branch", "participant_cardinality", "anchor_cardinality", "state_cardinality"}, [][]string{
		{"BIBLE_CLAIM|causal_claim", "subject exactly 1; object 0..1; participant 0..n; equals payload", "episode|scene|beat total 1..n; equals payload", "0"},
		{"continuity_claim(claim_type=continuity)", "asset_identity subject exactly 1", "scope_start exactly 1 plus scope_end exactly 1", "before exactly 1 plus after exactly 1"},
		{"continuity_claim(claim_type=interaction)", "Character Occurrence actor 1..n; Prop Occurrence prop exactly 1; give|receive counterparty exactly 1 otherwise 0; non-null holders exactly 1", "Scene exactly 1; Beat 0..1; Character Occurrence 1..n; Prop Occurrence exactly 1", "prop_before exactly 1 plus prop_after exactly 1"},
	})...)
	values = append(values, productionMatrixRows("materialization-matrix-contract", []string{"source", "target", "binding_role", "target_cardinality"}, [][]string{
		{"asset_identity", "production_binding", "asset", "exactly 1"},
		{"SPECIFICATION", "production_binding", "specification", "exactly 1; kind matches"},
		{"asset_state", "production_binding", "state", "1..n; equals Binding payload"},
		{"asset_identity", "asset_version", "asset", "exactly 1"},
		{"SPECIFICATION", "asset_version", "specification", "exactly 1; kind and purpose match"},
		{"asset_state", "asset_version", "state", "exactly 1"},
		{"asset_version(purpose=character_identity_anchor)", "asset_version(purpose=character_appearance)", "identity_anchor", "Character appearance exactly 1; other purpose 0"},
		{"artifact", "asset_version", "artifact", "exactly 1 selected READY Artifact"},
	})...)
	values = append(values, productionMatrixRows("reference-planning-matrix-contract", []string{"source", "reference_role", "target_payload_field"}, [][]string{
		{"asset_identity", "identity", "target_owner_refs.identity[]"},
		{"SPECIFICATION", "specification", "target_owner_refs.specification[]"},
		{"asset_state", "state", "target_owner_refs.state[]"},
		{"effective_style_snapshot", "style", "target_owner_refs.style[]"},
		{"scene", "scene", "target_owner_refs.scene[]"},
		{"occurrence", "occurrence", "target_owner_refs.occurrence[]"},
		{"continuity_claim(claim_type=interaction)", "interaction", "target_owner_refs.interaction[]"},
	})...)
	values = append(values, productionMatrixRows("reference-target-input-compatibility-matrix-contract", []string{"target_kind", "target_uniqueness_key", "expected_target_coverage", "identity_refs", "specification_refs", "state_refs", "style_refs", "scene_refs", "occurrence_refs", "interaction_refs", "target_dependencies"}, [][]string{
		{"character_identity_anchor", "(target_kind, identity[0]) logical ref key", "first pass: one per Character Identity in complete p1 Occurrence union; scope is its actual p1 Scene set", "exactly 1 Character", "exactly 1 CharacterSpecification", "exactly 1 Gate 3 selected anchor AssetState", "exactly 1 effective style", "1..n matching coverage_scope_keys", "0..n matching Character and anchor State", "0", "0"},
		{"character_appearance", "(target_kind, identity[0], specification[0], state[0]) logical ref key", "second pass: one per actual non-anchor Character identity/specification/state triple; exact p1 Scene scope", "exactly 1 Character", "exactly 1 CharacterSpecification", "exactly 1 Character AssetState", "exactly 1 effective style", "1..n matching coverage_scope_keys", "0..n matching Character and State", "0", "exactly 1 covering identity-anchor Target with fulfillment rank not lower"},
		{"location_board", "(target_kind, identity[0], specification[0], state[0]) logical ref key", "one per actual Location identity/specification/state triple in p1; exact Scene scope", "exactly 1 Location", "exactly 1 LocationSpecification", "exactly 1 Location AssetState", "exactly 1 effective style", "1..n matching coverage_scope_keys", "0..n matching Location and State", "0", "0"},
		{"prop_sheet", "(target_kind, identity[0], specification[0], state[0]) logical ref key", "one per actual Prop identity/specification/state triple in p1; exact Scene scope", "exactly 1 Prop", "exactly 1 PropSpecification", "exactly 1 Prop AssetState", "exactly 1 effective style", "1..n matching coverage_scope_keys", "0..n matching Prop and State", "0", "0"},
		{"scene_composition", "(target_kind, scene[0]) logical ref key", "one per p1_scope_key; not_generated must be explicit", "1..n exact Scene production closure Identity set", "1..n exact closure Specification set", "1..n exact closure AssetState set", "exactly 1 effective style", "exactly 1 matching coverage_scope_keys[0]", "complete Scene Occurrence set", "complete anchored InteractionClaim set", "not_generated 0; otherwise 1..n exact base Targets with adequate fulfillment rank"},
		{"interaction_composition", "(target_kind, interaction[0]) logical ref key", "one per confirmed InteractionClaim in p1; not_generated must be explicit", "2..n exact actor/counterparty/prop Identity set", "2..n exact participant Specification set", "2..n exact participating AssetState set", "exactly 1 effective style", "exactly 1 Interaction Scene matching coverage_scope_keys[0]", "2..n exact actor/prop/counterparty Occurrences", "exactly 1 interaction ContinuityClaim", "not_generated 0; otherwise 2..n exact participant base Targets with adequate fulfillment rank"},
	})...)
	values = append(values, productionMatrixRows("reference-target-result-matrix-contract", []string{"target_kind", "unique_allowed_result"}, [][]string{
		{"character_identity_anchor", "asset_version(purpose=character_identity_anchor)"},
		{"character_appearance", "asset_version(purpose=character_appearance)"},
		{"location_board", "asset_version(purpose=location_board)"},
		{"prop_sheet", "asset_version(purpose=prop_sheet)"},
		{"scene_composition", "scene_reference_binding_version"},
		{"interaction_composition", "interaction_reference_binding_version"},
	})...)
	values = append(values, productionMatrixRows("reference-target-activation-matrix-contract", []string{"target_activation_condition", "required_cardinality", "optional_cardinality", "not_generated_cardinality"}, [][]string{
		{"base Target: coverage_scope_keys intersects p1_scope_keys", "exactly 1", "0..1", "0"},
		{"composition Target: unique Scope belongs to p2_scope_keys", "exactly 1", "0..1", "0"},
		{"neither activation condition", "0 even when future required", "0", "0"},
	})...)
	values = append(values, productionMatrixRows("reference-binding-matrix-contract", []string{"target_binding", "source", "reference_role", "target_cardinality"}, [][]string{
		{"scene_reference_binding_version", "scene", "scene", "exactly 1"},
		{"scene_reference_binding_version", "occurrence", "occurrence", "0..n; equals Binding payload"},
		{"scene_reference_binding_version", "continuity_claim(claim_type=interaction)", "interaction", "0..n; equals Binding payload"},
		{"scene_reference_binding_version", "asset_version(purpose=character_identity_anchor|character_appearance)", "character_asset", "0..n; equals Scene Occurrence needs"},
		{"scene_reference_binding_version", "asset_version(purpose=location_board)", "location_asset", "exactly 1"},
		{"scene_reference_binding_version", "asset_version(purpose=prop_sheet)", "prop_asset", "0..n; equals Scene Occurrence needs"},
		{"interaction_reference_binding_version", "continuity_claim(claim_type=interaction)", "interaction", "exactly 1"},
		{"interaction_reference_binding_version", "asset_version(purpose=character_identity_anchor|character_appearance)", "character_asset", "1..n; equals actor and counterparty"},
		{"interaction_reference_binding_version", "asset_version(purpose=prop_sheet)", "prop_asset", "exactly 1"},
	})...)
	slices.SortFunc(values, func(left, right ProductionEdgeMatrixDefinition) int {
		return compareString(fmt.Sprintf("%s\x00%020d", left.MatrixID, left.RowIndex), fmt.Sprintf("%s\x00%020d", right.MatrixID, right.RowIndex))
	})
	return values
}

func productionMatrixRows(matrixID string, fields []string, rows [][]string) []ProductionEdgeMatrixDefinition {
	values := make([]ProductionEdgeMatrixDefinition, 0, len(rows))
	for index, cells := range rows {
		if len(cells) != len(fields) {
			panic("Production Schema Matrix row width mismatch")
		}
		row := make(map[string]string, len(fields))
		for fieldIndex, field := range fields {
			row[field] = cells[fieldIndex]
		}
		raw, err := json.Marshal(row)
		if err != nil {
			panic(err)
		}
		rowIndex := int64(index + 1)
		prefix := fmt.Sprintf("storygraph-production/%s/row-%d", matrixID, rowIndex)
		values = append(values, ProductionEdgeMatrixDefinition{
			MatrixID: matrixID, RowIndex: rowIndex, RowPayload: raw,
			ProjectionRuleID: prefix + "-projection-contract", CardinalityRuleID: prefix + "-cardinality-contract",
		})
	}
	return values
}
