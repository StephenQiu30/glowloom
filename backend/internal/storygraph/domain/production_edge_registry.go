package domain

import (
	"encoding/json"
	"fmt"
	"slices"
)

func productionEdgeMatrixDefinitions() []ProductionEdgeMatrixDefinition {
	values := make([]ProductionEdgeMatrixDefinition, 0, 65)
	values = append(values, productionMatrixRows("edge-type-matrix-contract", []string{"edge_type", "allowed_source_to_target", "qualifier_and_cardinality"}, [][]string{
		{"contains", "episode → scene；scene → dialogue|narrative_beat", "Parent 必须逐字节等于 Target Owner 已发布的 parent ref，sequence_key: SequenceKey 逐字节等于 Target 在该 Parent 下的 Owner order key；每个子节点恰有 1 个结构父级"},
		{"derived_from", "source_revision → source_evidence；source_evidence → asset_identity|SPECIFICATION|asset_state|world_rule|story_arc|plot_thread|episode|scene|dialogue|narrative_beat|occurrence", "无；每个 Evidence 恰 1 条 Source Revision 入边，其他目标入边集必须精确等于 Node Envelope Evidence 映射；Identity/Specification/State/WorldRule/Arc/Thread/Occurrence 有显式创作者决定时可为 0，Episode/Scene/Dialogue/Beat 必须为 1..n"},
		{"describes_identity", "asset_identity → SPECIFICATION", "无；每个 Specification 恰有 1 条，Asset kind 与 Specification Type 匹配"},
		{"has_state", "asset_identity → asset_state", "无；每个 State 恰有 1 条，kind 匹配"},
		{"precedes", "episode|scene|narrative_beat|shot 各自同类型相邻节点", "只连接同 Parent/同 Scene scope 按 Owner SequenceKey 排序后的相邻项；sequence_key: SequenceKey 逐字节等于 Target（后一项）的 Owner order key，非首项恰有 1 条入边，非末项恰有 1 条出边"},
		{"anchors_occurrence", "scene|narrative_beat → occurrence", "anchor_role=scene|beat；入边精确等于 Occurrence payload 的 scene_ref + beat_ref，Scene 恰 1、Beat 0..1"},
		{"instantiates_occurrence", "asset_state → occurrence", "无；入边精确等于 Occurrence payload 的 asset_state_ref，恰 1"},
		{"supports", "source_evidence → ANY_CLAIM", "无；入边集精确等于 Claim Node Envelope Evidence 映射；显式创作者决定可为 0"},
		{"claim_participant", "asset_identity|occurrence → ANY_CLAIM", "participant_role=subject|object|participant|actor|prop|counterparty|holder_before|holder_after；分支基数见下表"},
		{"claim_anchor", "STRUCTURE_ANCHOR → ANY_CLAIM", "anchor_role=episode|scene|beat|character_occurrence|prop_occurrence|scope_start|scope_end；分支基数见下表"},
		{"claim_state", "asset_state → continuity_claim", "state_role=before|after|prop_before|prop_after；分支基数见下表"},
		{"supersedes", "同 Node Type、同 Claim family/discriminant 与同 claim_series_key 的 Older Claim → Newer Claim", "无；每个 Newer Claim 的入边精确等于 payload supersedes_claim_ref（0..1），两个 Claim 的 owner_logical_id/story_node_key 必须不同、valid scope 兼容，且 Newer claim_revision = Older claim_revision + 1；同一 logical id 的 Owner Version 替换只表现为 Node content changed，不生成自环；continuity 与 interaction 绝不得互相 supersede"},
		{"constrains", "world_rule|policy_snapshot|effective_style_snapshot → CONSTRAINT_TARGET", "constraint_role=world|policy|style；source 与 role 一一对应，边集等于 target payload 的精确 constraint refs"},
		{"materializes", "见下方 Materialization Matrix", "binding_role=asset|specification|state|identity_anchor|artifact；不接受其他组合"},
		{"contains_reference_target", "approved_reference_plan_version → reference_plan_target", "sequence_key: SequenceKey 逐字节等于 Target 在 Plan 内的 Owner order key；Target 恰有 1 个 Plan 父级"},
		{"depends_on_reference_target", "reference_plan_target → reference_plan_target", "无；边集等于 Target payload 依赖且 Plan 内无环"},
		{"plans_reference", "见下方 Reference Planning Matrix", "reference_role=identity|specification|state|style|scene|occurrence|interaction"},
		{"fulfills_reference_target", "reference_plan_target → REFERENCE_RESULT", "无；target/result 类型与 phase-aware 基数见下表"},
		{"binds_reference_input", "见下方 Reference Binding Matrix", "reference_role=scene|occurrence|interaction|character_asset|location_asset|prop_asset"},
		{"binds_reference_output", "artifact → REFERENCE_BINDING", "无；每个 Binding 恰有 1 个 selected READY Composition Artifact"},
		{"realizes", "narrative_beat → shot", "无；每个 Shot 入边 1..n，精确等于 Shot payload source refs"},
		{"informs", "occurrence|scene_reference_binding_version|interaction_reference_binding_version → shot_production_binding_version", "informs_role=occurrence|scene_reference|interaction_reference；source 与 role 一一对应且同 Scene scope，三类入边分别精确等于 Binding payload 的 occurrence_refs[]|scene_reference_binding_refs[]|interaction_reference_binding_refs[]"},
		{"binds_input", "shot|occurrence|asset_version|REFERENCE_BINDING|effective_style_snapshot → shot_production_binding_version", "binding_role=shot|occurrence|asset_version|scene_reference|interaction_reference|style；source 与 role 一一对应，边集等于 target payload"},
	})...)
	values = append(values, productionMatrixRows("claim-cardinality-matrix-contract", []string{"claim_branch", "participant_cardinality", "anchor_cardinality", "state_cardinality"}, [][]string{
		{"BIBLE_CLAIM|causal_claim", "subject 恰 1，object 为 0..1，participant 为 0..n；精确等于 payload", "episode|scene|beat 合计 1..n；精确等于 payload", "0"},
		{"continuity_claim(claim_type=continuity)", "asset_identity → claim，subject 恰 1", "scope_start 恰 1 + scope_end 恰 1", "before 恰 1 + after 恰 1"},
		{"continuity_claim(claim_type=interaction)", "Character Occurrence actor 1..n；Prop Occurrence prop 恰 1；give|receive 的 Character Occurrence counterparty 恰 1，其他为 0；非空 holder 以 AssetIdentity 投影对应 role 恰 1", "Scene scene 恰 1，Beat beat 0..1，Character Occurrence character_occurrence 1..n，Prop Occurrence prop_occurrence 恰 1", "prop_before 恰 1 + prop_after 恰 1"},
	})...)
	values = append(values, productionMatrixRows("materialization-matrix-contract", []string{"source", "target", "binding_role", "target_cardinality"}, [][]string{
		{"asset_identity", "production_binding", "asset", "恰 1"},
		{"SPECIFICATION", "production_binding", "specification", "恰 1，kind 匹配"},
		{"asset_state", "production_binding", "state", "1..n，精确等于 Binding payload"},
		{"asset_identity", "asset_version", "asset", "恰 1"},
		{"SPECIFICATION", "asset_version", "specification", "恰 1，kind/purpose 匹配"},
		{"asset_state", "asset_version", "state", "恰 1"},
		{"asset_version(purpose=character_identity_anchor)", "asset_version(purpose=character_appearance)", "identity_anchor", "Character appearance 恰 1，其他 purpose 为 0"},
		{"artifact", "asset_version", "artifact", "恰 1 selected READY Artifact"},
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
		{"character_identity_anchor", "(target_kind, identity[0]) 的 logical ref key", "第一遍：p1_scope_keys[] 完整 Occurrence 并集中每个 Character Identity 恰 1；Scope 精确为该 Character 实际出现的 p1 Scene 集", "恰 1 个 Character", "恰 1 个描述该 Character 的 CharacterSpecification", "恰 1 个属于该 Character 且由 Gate 3 审核选定的身份锚点 AssetState", "恰 1，等于 constraints.effective_style_snapshot_ref", "1..n，与 coverage_scope_keys[] 逐一对应", "0..n，均为该 Character/锚点 State 在 scene[] 中的 Occurrence", "0", "0"},
		{"character_appearance", "(target_kind, identity[0], specification[0], state[0]) 的 logical ref key", "第二遍：先读该 Character 唯一 Anchor Target 的 State，再为每个实际出现且不等于该 State 的 Character Identity/Specification/State 三元组生成恰 1；Scope 精确为该三元组出现的 p1 Scene 集", "恰 1 个 Character", "恰 1 个描述该 Character 的 CharacterSpecification", "恰 1 个属于该 Character 的 AssetState", "恰 1，等于 constraints.effective_style_snapshot_ref", "1..n，与 coverage_scope_keys[] 逐一对应", "0..n，均为该 Character/State 在 scene[] 中的 Occurrence", "0", "恰 1 个同 Character/Specification/Style 且覆盖当前 Scope 的 character_identity_anchor Target，且依赖 fulfillment rank 不低于本 Target"},
		{"location_board", "(target_kind, identity[0], specification[0], state[0]) 的 logical ref key", "p1_scope_keys[] 内每个实际出现的 Location Identity/Specification/State 三元组恰 1；Scope 精确为该三元组出现的 p1 Scene 集", "恰 1 个 Location", "恰 1 个描述该 Location 的 LocationSpecification", "恰 1 个属于该 Location 的 AssetState", "恰 1，等于 constraints.effective_style_snapshot_ref", "1..n，与 coverage_scope_keys[] 逐一对应", "0..n，均为该 Location/State 在 scene[] 中的 Occurrence", "0", "0"},
		{"prop_sheet", "(target_kind, identity[0], specification[0], state[0]) 的 logical ref key", "p1_scope_keys[] 内每个实际出现的 Prop Identity/Specification/State 三元组恰 1；Scope 精确为该三元组出现的 p1 Scene 集", "恰 1 个 Prop", "恰 1 个描述该 Prop 的 PropSpecification", "恰 1 个属于该 Prop 的 AssetState", "恰 1，等于 constraints.effective_style_snapshot_ref", "1..n，与 coverage_scope_keys[] 逐一对应", "0..n，均为该 Prop/State 在 scene[] 中的 Occurrence", "0", "0"},
		{"scene_composition", "(target_kind, scene[0]) 的 logical ref key", "每个 p1_scope_key 恰 1；不制作也必须显式保存 fulfillment=not_generated", "1..n，精确等于 Scene production closure 的 Identity 去重集", "1..n，精确等于该 closure 的 Specification 去重集", "1..n，精确等于该 closure 的 AssetState 去重集", "恰 1，等于 constraints.effective_style_snapshot_ref", "恰 1，且对应唯一 coverage_scope_keys[0]", "精确等于该 Scene 的完整 Occurrence 集", "精确等于锚定该 Scene 的 InteractionClaim 集", "not_generated 为 0；否则 1..n，恰为 Scene production closure 每个三元组对应的基础 Target 去重集，每个依赖 fulfillment rank 不低于本 Target"},
		{"interaction_composition", "(target_kind, interaction[0]) 的 logical ref key", "p1_scope_keys[] 内每个已确认 InteractionClaim 恰 1；不制作也必须显式保存 fulfillment=not_generated", "2..n，精确等于 actor/counterparty/prop 的 Identity 去重集", "2..n，精确等于这些参与者的 Specification 去重集", "2..n，精确等于参与 Occurrence 的 AssetState 去重集", "恰 1，等于 constraints.effective_style_snapshot_ref", "恰 1，等于 InteractionClaim 的 Scene，且对应唯一 coverage_scope_keys[0]", "2..n，精确等于 InteractionClaim 的 actor/prop/counterparty Occurrence 去重集", "恰 1 个 claim_type=interaction 的 ContinuityClaim", "not_generated 为 0；否则 2..n，恰为参与三元组对应的基础 Target 去重集，每个依赖 fulfillment rank 不低于本 Target"},
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
		{"基础 Target：target.coverage_scope_keys ∩ p1_scope_keys ≠ ∅", "恰 1", "0..1", "0"},
		{"组合 Target：唯一 Scope 属于 p2_scope_keys", "恰 1", "0..1", "0"},
		{"不满足上述激活条件", "0；即使未来 required 也尚未到履约阶段", "0", "0"},
	})...)
	values = append(values, productionMatrixRows("reference-binding-matrix-contract", []string{"target_binding", "source", "reference_role", "target_cardinality"}, [][]string{
		{"scene_reference_binding_version", "scene", "scene", "恰 1"},
		{"同上", "occurrence", "occurrence", "0..n，精确等于 Binding payload"},
		{"同上", "continuity_claim(claim_type=interaction)", "interaction", "0..n，精确等于 Binding payload"},
		{"同上", "asset_version(purpose=character_identity_anchor|character_appearance)", "character_asset", "0..n，精确等于 Scene Occurrence 需求"},
		{"同上", "asset_version(purpose=location_board)", "location_asset", "恰 1"},
		{"同上", "asset_version(purpose=prop_sheet)", "prop_asset", "0..n，精确等于 Scene Occurrence 需求"},
		{"interaction_reference_binding_version", "continuity_claim(claim_type=interaction)", "interaction", "恰 1"},
		{"同上", "asset_version(purpose=character_identity_anchor|character_appearance)", "character_asset", "1..n，精确等于 actor/counterparty"},
		{"同上", "asset_version(purpose=prop_sheet)", "prop_asset", "恰 1"},
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
