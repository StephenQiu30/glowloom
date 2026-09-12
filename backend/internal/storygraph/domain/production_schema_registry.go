package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

// ProductionSchemaRegistry is the complete, content-addressed production
// registry. It is rebuilt from declarations so hashes cannot drift from the
// contracts they identify.
type ProductionSchemaRegistry struct {
	Manifest                  ProductionSchemaManifest
	CanonicalManifest         []byte
	SchemaHash                string
	PayloadContracts          []ProductionPayloadContractDefinition
	CanonicalPayloadContracts map[string][]byte
	PayloadContractHashes     map[string]string
}

// Only immutable declaration identities are retained, never Owner facts or
// mutable registry maps/slices. A new binary validates its own declarations.
type productionSchemaIdentity struct {
	SchemaID, SchemaHash, NodeKeyDerivationID, EdgeKeyDerivationID string
}

var currentProductionSchemaIdentity = sync.OnceValues(func() (productionSchemaIdentity, error) {
	registry, err := BuildProductionSchemaRegistry()
	if err != nil {
		return productionSchemaIdentity{}, err
	}
	return productionSchemaIdentity{
		SchemaID: registry.Manifest.SchemaID, SchemaHash: registry.SchemaHash,
		NodeKeyDerivationID: registry.Manifest.NodeKeyDerivationID,
		EdgeKeyDerivationID: registry.Manifest.EdgeKeyDerivationID,
	}, nil
})

// BuildProductionSchemaRegistry builds and validates the complete frozen
// production registry. Meta-contract decoding alone is not publication
// authority; callers must use this complete registry identity.
func BuildProductionSchemaRegistry() (ProductionSchemaRegistry, error) {
	payloads, err := productionPayloadDefinitions()
	if err != nil {
		return ProductionSchemaRegistry{}, err
	}
	registry := ProductionSchemaRegistry{
		PayloadContracts:          payloads,
		CanonicalPayloadContracts: make(map[string][]byte, len(payloads)),
		PayloadContractHashes:     make(map[string]string, len(payloads)),
	}
	for _, payload := range payloads {
		raw, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return ProductionSchemaRegistry{}, marshalErr
		}
		canonical, canonicalErr := platformcanonical.JSON(raw)
		if canonicalErr != nil {
			return ProductionSchemaRegistry{}, canonicalErr
		}
		hash, hashErr := platformcanonical.Hash(canonical)
		if hashErr != nil {
			return ProductionSchemaRegistry{}, hashErr
		}
		registry.CanonicalPayloadContracts[payload.PayloadContractID] = canonical
		registry.PayloadContractHashes[payload.PayloadContractID] = hash
	}

	manifest, err := buildProductionSchemaManifest(payloads, registry.PayloadContractHashes)
	if err != nil {
		return ProductionSchemaRegistry{}, err
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return ProductionSchemaRegistry{}, err
	}
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return ProductionSchemaRegistry{}, err
	}
	decoded, hash, err := DecodeProductionSchemaManifestMetaContract(canonical)
	if err != nil {
		return ProductionSchemaRegistry{}, fmt.Errorf("build Production Schema Manifest: %w", err)
	}
	registry.Manifest, registry.CanonicalManifest, registry.SchemaHash = decoded, canonical, hash
	return registry, nil
}

func buildProductionSchemaManifest(payloads []ProductionPayloadContractDefinition, payloadHashes map[string]string) (ProductionSchemaManifest, error) {
	nodes := make([]ProductionNodeManifestDefinition, 0, len(productionNodeDefinitions))
	unions := make([]ProductionPayloadUnionDefinition, 0, len(productionNodeDefinitions)+1)
	contractConsumers := make(map[string][]string)
	for nodeType, definition := range productionNodeDefinitions {
		contracts := make([]string, 0, len(definition.payloadContracts))
		for _, contractID := range definition.payloadContracts {
			contracts = append(contracts, contractID)
		}
		slices.Sort(contracts)
		contracts = slices.Compact(contracts)
		families := append([]string(nil), definition.versionFamilies...)
		slices.Sort(families)
		var discriminant *string
		if definition.discriminant != "" {
			discriminant = stringPointer(definition.discriminant)
		}
		nodes = append(nodes, ProductionNodeManifestDefinition{
			NodeType: string(nodeType), OwnerKind: definition.ownerKind, AllowedVersionFamilies: families,
			PayloadContractIDs: contracts, PayloadDiscriminantField: discriminant, EvidencePolicyID: definition.evidencePolicy,
		})
		for discriminantValue, contractID := range definition.payloadContracts {
			hash, exists := payloadHashes[contractID]
			if !exists {
				return ProductionSchemaManifest{}, fmt.Errorf("payload contract %s is not registered", contractID)
			}
			union := ProductionPayloadUnionDefinition{NodeType: string(nodeType), PayloadContractID: contractID, PayloadContractHash: hash}
			if definition.discriminant != "" {
				union.DiscriminantField, union.DiscriminantValue = stringPointer(definition.discriminant), stringPointer(discriminantValue)
			}
			unions = append(unions, union)
			contractConsumers[contractID] = append(contractConsumers[contractID], string(nodeType))
		}
	}
	for contractID, consumers := range contractConsumers {
		slices.Sort(consumers)
		contractConsumers[contractID] = slices.Compact(consumers)
	}
	if len(payloads) != len(payloadHashes) {
		return ProductionSchemaManifest{}, fmt.Errorf("production payload registry cardinality has drifted")
	}
	for _, payload := range payloads {
		if !slices.Equal(payload.AllowedNodeTypes, contractConsumers[payload.PayloadContractID]) {
			return ProductionSchemaManifest{}, fmt.Errorf("payload contract %s consumer set has drifted", payload.PayloadContractID)
		}
	}
	slices.SortFunc(nodes, func(left, right ProductionNodeManifestDefinition) int {
		return compareString(left.NodeType, right.NodeType)
	})
	slices.SortFunc(unions, func(left, right ProductionPayloadUnionDefinition) int {
		return compareString(payloadUnionKey(left), payloadUnionKey(right))
	})

	manifest := ProductionSchemaManifest{
		ManifestContractID: ProductionSchemaManifestContractID, SchemaID: ProductionSchemaID,
		CanonicalJSONContractID: ProductionCanonicalJSONContractID, NodeKeyDerivationID: StoryNodeKeyDerivationID,
		EdgeKeyDerivationID: StoryEdgeKeyDerivationID, NodeDefinitions: nodes, PayloadUnionDefinitions: unions,
		EdgeAliasDefinitions: productionEdgeAliasDefinitions(), EdgeMatrixDefinitions: productionEdgeMatrixDefinitions(),
		OwnerCollectionDefinitions: productionOwnerCollectionDefinitions(), CoverageRules: productionCoverageRuleDefinitions(),
		CheckpointOwnerFamilyDefinitions: productionCheckpointDefinitions(), ExclusionDefinitions: productionExclusionDefinitions(),
	}
	return manifest, nil
}

func payloadUnionKey(value ProductionPayloadUnionDefinition) string {
	return value.NodeType + "\x00" + pointerText(value.DiscriminantField) + "\x00" + pointerText(value.DiscriminantValue) + "\x00" + value.PayloadContractID
}

func productionEdgeAliasDefinitions() []ProductionEdgeAliasDefinition {
	values := []ProductionEdgeAliasDefinition{
		{Alias: "ANY_CLAIM", ExpandedNodeTypes: []string{"causal_claim", "continuity_claim", "foreshadowing_claim", "payoff_claim", "relationship_claim"}},
		{Alias: "BIBLE_CLAIM", ExpandedNodeTypes: []string{"foreshadowing_claim", "payoff_claim", "relationship_claim"}},
		{Alias: "CONSTRAINT_TARGET", ExpandedNodeTypes: []string{"asset_version", "interaction_reference_binding_version", "reference_plan_target", "scene_reference_binding_version", "shot_production_binding_version"}},
		{Alias: "REFERENCE_BINDING", ExpandedNodeTypes: []string{"interaction_reference_binding_version", "scene_reference_binding_version"}},
		{Alias: "REFERENCE_RESULT", ExpandedNodeTypes: []string{"asset_version", "interaction_reference_binding_version", "scene_reference_binding_version"}},
		{Alias: "SPECIFICATION", ExpandedNodeTypes: []string{"character_specification", "location_specification", "prop_specification"}},
		{Alias: "STRUCTURE_ANCHOR", ExpandedNodeTypes: []string{"episode", "narrative_beat", "occurrence", "scene"}},
	}
	return values
}

func productionOwnerCollectionDefinitions() []ProductionOwnerCollectionDefinition {
	type declaration struct{ phase, owner, family, scope, cardinality, empty, members string }
	rows := []declaration{
		{"p0", "production/script", "script_source_set", "project", "project:<project_id>；每 Project 恰 1 Collection", "forbidden", "精确 1 个已接受 Source Revision + 1 个匹配 SourceSpanIndexVersion；后者作校验输入但不成为 Node"},
		{"p0", "production/project", "project_episode_set", "project", "project:<project_id>；每 Project 恰 1 Collection", "forbidden", "完整 active Episode 顺序，1..n Episode"},
		{"p0", "production/bible", "bible_structure_identity_set", "project", "project:<project_id>；每 Project 恰 1 Collection", "forbidden", "精确 1 个 Gate 1 StructureIdentitySetVersion 完整性根；只作 Proof，不成为 Node"},
		{"p0", "production/bible", "bible_production_world_set", "project", "project:<project_id>；每 Project 恰 1 Collection", "forbidden", "精确 1 个 Gate 2 ProductionBibleVersion 且内嵌精确 StructureIdentitySetVersion Ref；Evidence/Specification/Binding/Rule/跨集 Claim/Arc/Thread 各 0..n 且与根一致"},
		{"p0", "production/planning", "planning_scene_set", "episode", "episode:<episode_owner_logical_id>；每 active Episode 恰 1 Collection", "forbidden", "Scene 1..n；Dialogue/Beat/Occurrence/Continuity/Causal Claim 各 0..n 且集合完整"},
		{"p0", "production/planning", "planning_structure_rebase_set", "project", "project:<project_id>；每 Project 恰 1 Collection", "scope_rebase_conditional", "正常编译为 0；Scope Rebase 时恰 1 个已发布不可变 Structure Rebase/Supersession Version，只作 Proof，不成为 Node"},
		{"p0", "asset", "asset_identity_state_set", "project", "project:<project_id>；每 Project 恰 1 Collection", "forbidden", "生产世界内 AssetIdentity 1..n，每个身份 AssetState 1..n；未发布候选为 0"},
		{"p1", "preset", "preset_effective_set", "project", "project:<project_id>；每 Project 恰 1 Collection", "forbidden", "精确 1 个 EffectivePolicySnapshot + 1 个 EffectiveStyleSnapshot"},
		{"p1", "production/reference", "reference_plan_set", "reference_plan", "reference-plan:<plan_owner_logical_id>；p1 非空的 Project 恰 1 个 active Plan 且恰 1 Collection", "forbidden", "精确 1 个 ApprovedReferencePlanVersion，Target 1..n，与 Plan fragment 根完全一致"},
		{"p1", "asset", "asset_base_reference_set", "reference_plan", "与 reference_plan_set 同 Scope；p1 恰 1 Collection", "receipt_bound", "已履约基础 Target 的 AssetVersion 和每个版本精确 1 个 selected READY Artifact；基数见下文 phase-aware 表"},
		{"p2", "production/reference", "reference_binding_set", "scene", "对每个 p2_scope_key 恰 1 Collection，scope_key 即该 Scene Scope Key", "receipt_bound", "已履约组合 Target 的 Scene/Interaction Binding；基数见下文 phase-aware 表"},
		{"p2", "asset", "asset_composition_artifact_set", "scene", "与 reference_binding_set 同 Scope；每 p2_scope_key 恰 1 Collection", "receipt_bound", "每个正式 Scene/Interaction Binding 精确 1 个 selected READY Composition Artifact；不包含未选 Candidate Artifact"},
		{"p3", "production/storyboard", "storyboard_formal_set", "scene", "对每个 p3_scope_key 恰 1 Collection，scope_key 即该 Scene Scope Key", "forbidden", "完整正式 Shot 集；每个 Shot 精确 1 个 ShotProductionBindingVersion，Draft/Intent Candidate 为 0"},
	}
	values := make([]ProductionOwnerCollectionDefinition, 0, len(rows))
	for _, row := range rows {
		rank := map[string]int64{"p0": 0, "p1": 1, "p2": 2, "p3": 3}[row.phase]
		prefix := "storygraph-production/owner-collection/" + row.family
		values = append(values, ProductionOwnerCollectionDefinition{
			MinimumCoverage: row.phase, PhaseRank: rank, OwnerKind: row.owner, VersionFamily: row.family, ScopeKind: row.scope,
			ScopeKeyAndCollectionCardinality: row.cardinality, MemberCompleteness: row.members,
			ScopeKeyRuleID: prefix + "-scope-key-contract", MemberContractID: prefix + "-members-contract",
			CardinalityRuleID: prefix + "-cardinality-contract", EmptyCollectionPolicy: row.empty,
		})
	}
	slices.SortFunc(values, func(left, right ProductionOwnerCollectionDefinition) int {
		leftKey := fmt.Sprintf("%d\x00%s\x00%s\x00%s", left.PhaseRank, left.OwnerKind, left.VersionFamily, left.ScopeKind)
		rightKey := fmt.Sprintf("%d\x00%s\x00%s\x00%s", right.PhaseRank, right.OwnerKind, right.VersionFamily, right.ScopeKind)
		return compareString(leftKey, rightKey)
	})
	return values
}

func productionCoverageRuleDefinitions() []ProductionCoverageRuleDefinition {
	p0 := []string{"asset_identity_state_set", "bible_production_world_set", "bible_structure_identity_set", "planning_scene_set", "planning_structure_rebase_set", "project_episode_set", "script_source_set"}
	p1 := append(append([]string(nil), p0...), "asset_base_reference_set", "preset_effective_set", "reference_plan_set")
	p2 := append(append([]string(nil), p1...), "asset_composition_artifact_set", "reference_binding_set")
	p3 := append(append([]string(nil), p2...), "storyboard_formal_set")
	for _, familySet := range [][]string{p0, p1, p2, p3} {
		slices.Sort(familySet)
	}
	return []ProductionCoverageRuleDefinition{
		coverageRule("p0", 0, []string{}, p0, []string{"gate_1_structure_identity", "gate_2_bible_continuity", "project_episode_lifecycle_confirmed", "source_revision_accepted"}),
		coverageRule("p1", 1, []string{"p0"}, p1, []string{"gate_3_visual_foundation_scope", "gate_4_base_reference_selection"}),
		coverageRule("p2", 2, []string{"p0", "p1"}, p2, []string{"gate_4_composition_selection"}),
		coverageRule("p3", 3, []string{"p0", "p1", "p2"}, p3, []string{"gate_5_storyboard"}),
	}
}

func coverageRule(phase string, rank int64, prerequisites, families, checkpoints []string) ProductionCoverageRuleDefinition {
	return ProductionCoverageRuleDefinition{CoveragePhase: phase, PhaseRank: rank, ScopeSetField: phase + "_scope_keys", ActivationCondition: "scope_set_non_empty",
		PrerequisitePhases: prerequisites, RequiredVersionFamilies: families, DecisionCheckpointIDs: checkpoints,
		CoverageRuleID: "storygraph-production/coverage/" + phase + "-contract"}
}

func productionCheckpointDefinitions() []ProductionCheckpointOwnerFamilyDefinition {
	values := []ProductionCheckpointOwnerFamilyDefinition{
		{"project_episode_lifecycle_confirmed", "production/project", "project_episode_set"},
		{"source_revision_accepted", "production/script", "script_source_set"},
		{"gate_1_structure_identity", "production/bible", "bible_structure_identity_set"},
		{"gate_2_bible_continuity", "production/bible", "bible_production_world_set"},
		{"gate_2_bible_continuity", "production/planning", "planning_scene_set"},
		{"gate_2_bible_continuity", "production/planning", "planning_structure_rebase_set"},
		{"gate_2_bible_continuity", "asset", "asset_identity_state_set"},
		{"gate_3_visual_foundation_scope", "preset", "preset_effective_set"},
		{"gate_3_visual_foundation_scope", "production/reference", "reference_plan_set"},
		{"gate_4_base_reference_selection", "asset", "asset_base_reference_set"},
		{"gate_4_composition_selection", "production/reference", "reference_binding_set"},
		{"gate_4_composition_selection", "asset", "asset_composition_artifact_set"},
		{"gate_5_storyboard", "production/storyboard", "storyboard_formal_set"},
	}
	slices.SortFunc(values, func(left, right ProductionCheckpointOwnerFamilyDefinition) int {
		return compareString(left.DecisionCheckpointID+"\x00"+left.OwnerKind+"\x00"+left.VersionFamily, right.DecisionCheckpointID+"\x00"+right.OwnerKind+"\x00"+right.VersionFamily)
	})
	return values
}

func productionExclusionDefinitions() []ProductionExclusionDefinition {
	values := []ProductionExclusionDefinition{
		{"node_type", "project"}, {"node_type", "interaction_claim"}, {"node_type", "shot_continuity_claim"}, {"node_type", "generation_target"}, {"node_type", "shot_image_binding_version"}, {"node_type", "shot_video_binding_version"},
		{"compiler_input_kind", "candidate"}, {"compiler_input_kind", "human_task"}, {"compiler_input_kind", "review_decision"}, {"compiler_input_kind", "candidate_selection"}, {"compiler_input_kind", "unselected_artifact"},
		{"runtime_kind", "authoring_revision_graph"}, {"runtime_kind", "workflow_definition"}, {"runtime_kind", "temporal_history"}, {"runtime_kind", "provider_profile"}, {"runtime_kind", "provider_job"}, {"runtime_kind", "provider_call"}, {"runtime_kind", "workflow_run"}, {"runtime_kind", "workflow_invocation"},
		{"query_view_kind", "scene_production_packet"}, {"query_view_kind", "continuity_ledger_view"}, {"query_view_kind", "visual_production_package_view"},
	}
	slices.SortFunc(values, func(left, right ProductionExclusionDefinition) int {
		return compareString(left.ExclusionKind+"\x00"+left.Literal, right.ExclusionKind+"\x00"+right.Literal)
	})
	return values
}
