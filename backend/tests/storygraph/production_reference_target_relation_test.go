package storygraph_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/google/uuid"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionReferenceTargetRejectsPayloadAndRelationDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot, *storygraph.Node){
		"unknown payload field": func(_ *storygraph.ProductionOwnerSnapshot, target *storygraph.Node) {
			mutateProductionPayload(t, target, func(payload map[string]any) { payload["provider"] = "volcengine" })
		},
		"duplicate coverage scope": func(_ *storygraph.ProductionOwnerSnapshot, target *storygraph.Node) {
			mutateProductionPayload(t, target, func(payload map[string]any) {
				payload["coverage_scope_keys"] = []string{"scene:opening", "scene:opening"}
			})
		},
		"coverage scope drifts from scene": func(_ *storygraph.ProductionOwnerSnapshot, target *storygraph.Node) {
			mutateProductionPayload(t, target, func(payload map[string]any) {
				payload["coverage_scope_keys"] = []string{"scene:elsewhere"}
			})
		},
		"style input omitted": func(_ *storygraph.ProductionOwnerSnapshot, target *storygraph.Node) {
			mutateProductionPayload(t, target, func(payload map[string]any) {
				payload["target_owner_refs"].(map[string]any)["style"] = []any{}
			})
		},
		"base inputs relabeled as scene composition": func(_ *storygraph.ProductionOwnerSnapshot, target *storygraph.Node) {
			mutateProductionPayload(t, target, func(payload map[string]any) { payload["target_kind"] = "scene_composition" })
		},
		"target has no plan parent": func(value *storygraph.ProductionOwnerSnapshot, target *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeContainsReferenceTarget && edge.ToNodeKey == target.StoryNodeKey
			})
		},
		"identity planning edge omitted": func(value *storygraph.ProductionOwnerSnapshot, target *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypePlansReference && edge.ToNodeKey == target.StoryNodeKey && edge.Qualifier.ReferenceRole == "identity"
			})
		},
		"policy constraint edge omitted": func(value *storygraph.ProductionOwnerSnapshot, target *storygraph.Node) {
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeConstrains && edge.ToNodeKey == target.StoryNodeKey && edge.Qualifier.ConstraintRole == "policy"
			})
		},
		"self dependency declared without edge": func(_ *storygraph.ProductionOwnerSnapshot, target *storygraph.Node) {
			mutateProductionPayload(t, target, func(payload map[string]any) {
				payload["depends_on_target_refs"] = []storygraph.OwnerRef{target.OwnerRef}
			})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value, target := productionReferenceTargetFixture(t)
			mutate(&value, target)
			if _, err := storygraph.Canonicalize(value.Graph); err == nil {
				t.Fatal("invalid Production Reference Target was accepted")
			}
		})
	}
}

func TestProductionReferenceTargetRejectsDuplicateBusinessKeyInOnePlan(t *testing.T) {
	value, target := productionReferenceTargetFixture(t)
	duplicate := *target
	duplicate.OwnerRef.FragmentKey = "target:character:linzhou:duplicate-anchor"
	duplicate.OwnerRef.FragmentContentHash = productionHash(duplicate.OwnerRef.FragmentKey)
	duplicate.StoryNodeKey = mustNodeKey(t, duplicate.NodeType, duplicate.OwnerRef)
	mutateProductionPayload(t, &duplicate, func(payload map[string]any) {
		payload["projection_hash"] = duplicate.OwnerRef.FragmentContentHash
	})
	value.Graph.Nodes = append(value.Graph.Nodes, duplicate)
	for _, edge := range append([]storygraph.Edge(nil), value.Graph.Edges...) {
		if edge.ToNodeKey != target.StoryNodeKey || (edge.EdgeType != storygraph.EdgeTypeContainsReferenceTarget && edge.EdgeType != storygraph.EdgeTypePlansReference && edge.EdgeType != storygraph.EdgeTypeConstrains) {
			continue
		}
		qualifier := edge.Qualifier
		if edge.EdgeType == storygraph.EdgeTypeContainsReferenceTarget {
			qualifier.SequenceKey = "target:0002"
		}
		value.Graph.Edges = append(value.Graph.Edges, newEdge(t, edge.EdgeType, edge.FromNodeKey, duplicate.StoryNodeKey, qualifier))
	}
	if _, err := storygraph.Canonicalize(value.Graph); err == nil {
		t.Fatal("duplicate Reference Target business key in one Plan was accepted")
	}
}

func TestProductionReferenceTargetAcceptsExactBaseAndAppearanceTargets(t *testing.T) {
	value, _ := productionAssetVersionFixture(t)
	addProductionCharacterAppearance(t, &value)
	stripProductionReferenceResults(&value)
	if _, err := storygraph.Canonicalize(value.Graph); err != nil {
		t.Fatalf("exact base and appearance Reference Targets were rejected: %v", err)
	}

	anchor := productionNodeByFragment(t, &value, storygraph.NodeTypeReferencePlanTarget, "target:character:linzhou:identity-anchor")
	mutateProductionPayload(t, anchor, func(payload map[string]any) { payload["fulfillment"] = "optional" })
	if _, err := storygraph.Canonicalize(value.Graph); err == nil {
		t.Fatal("appearance Target depending on a lower-rank anchor was accepted")
	}
}

func TestProductionReferenceTargetAcceptsAllSixTargetKinds(t *testing.T) {
	value := productionInteractionRelationFixture(t)
	addProductionLocationRelation(t, &value)
	addProductionAllKindReferencePlan(t, &value)
	if _, err := storygraph.Canonicalize(value.Graph); err != nil {
		t.Fatalf("exact six-kind Reference Plan was rejected: %v", err)
	}
}

func productionReferenceTargetFixture(t *testing.T) (storygraph.ProductionOwnerSnapshot, *storygraph.Node) {
	t.Helper()
	value, _ := productionAssetVersionFixture(t)
	stripProductionReferenceResults(&value)
	return value, productionNodeByType(t, &value, storygraph.NodeTypeReferencePlanTarget)
}

func stripProductionReferenceResults(value *storygraph.ProductionOwnerSnapshot) {
	removed := make(map[string]struct{})
	nodes := value.Graph.Nodes[:0]
	for _, node := range value.Graph.Nodes {
		if node.NodeType == storygraph.NodeTypeAssetVersion {
			removed[node.StoryNodeKey] = struct{}{}
			continue
		}
		nodes = append(nodes, node)
	}
	value.Graph.Nodes = nodes
	value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
		_, fromRemoved := removed[edge.FromNodeKey]
		_, toRemoved := removed[edge.ToNodeKey]
		return fromRemoved || toRemoved
	})
}

func addProductionLocationRelation(t *testing.T, value *storygraph.ProductionOwnerSnapshot) {
	t.Helper()
	evidence := productionNodeByType(t, value, storygraph.NodeTypeSourceEvidence)
	scene := productionNodeByType(t, value, storygraph.NodeTypeScene)
	character := productionAssetNodeByKind(t, value, storygraph.NodeTypeAssetIdentity, "character")
	owner := character.OwnerRef
	owner.OwnerLogicalID = "location:warehouse"
	owner.OwnerVersionID = uuid.NewString()
	owner.OwnerContentHash = productionHash(owner.OwnerLogicalID)
	owner.FragmentKey, owner.FragmentContentHash = "", ""
	fragment := func(base storygraph.OwnerRef, key string) storygraph.OwnerRef {
		base.FragmentKey = key
		base.FragmentContentHash = productionHash(key)
		return base
	}
	payload := func(contractID, projectionHash string, fields map[string]any) json.RawMessage {
		fields["payload_contract_id"] = contractID
		fields["projection_hash"] = projectionHash
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	node := func(nodeType storygraph.NodeType, ref storygraph.OwnerRef, evidenceRefs []storygraph.EvidenceRef, body json.RawMessage) storygraph.Node {
		return storygraph.Node{StoryNodeKey: mustNodeKey(t, nodeType, ref), NodeType: nodeType, OwnerRef: ref, EvidenceRefs: evidenceRefs, Payload: body}
	}
	specificationRef := fragment(evidence.OwnerRef, "specification:location-warehouse")
	stateRef := fragment(owner, "state:location-default")
	bindingRef := fragment(evidence.OwnerRef, "binding:location-warehouse")
	occurrenceRef := fragment(scene.OwnerRef, "occurrence:location-warehouse")
	identity := node(storygraph.NodeTypeAssetIdentity, owner, []storygraph.EvidenceRef{character.EvidenceRefs[0]}, payload(
		"storygraph-production/asset-identity-payload-contract", owner.OwnerContentHash,
		map[string]any{"asset_kind": "location", "creator_decision_ref": nil},
	))
	specification := node(storygraph.NodeTypeLocationSpecification, specificationRef, []storygraph.EvidenceRef{character.EvidenceRefs[0]}, payload(
		"storygraph-production/specification-payload-contract", specificationRef.FragmentContentHash,
		map[string]any{"asset_kind": "location", "creator_decision_ref": nil},
	))
	state := node(storygraph.NodeTypeAssetState, stateRef, []storygraph.EvidenceRef{character.EvidenceRefs[0]}, payload(
		"storygraph-production/asset-state-payload-contract", stateRef.FragmentContentHash,
		map[string]any{"asset_kind": "location", "state_key": "state:location-default", "story_time_range": nil, "creator_decision_ref": nil},
	))
	binding := node(storygraph.NodeTypeProductionBinding, bindingRef, nil, payload(
		"storygraph-production/production-binding-payload-contract", bindingRef.FragmentContentHash,
		map[string]any{"asset_identity_ref": identity.OwnerRef, "specification_ref": specification.OwnerRef, "state_refs": []storygraph.OwnerRef{state.OwnerRef}},
	))
	occurrence := node(storygraph.NodeTypeOccurrence, occurrenceRef, []storygraph.EvidenceRef{character.EvidenceRefs[0]}, payload(
		"storygraph-production/occurrence-payload-contract", occurrenceRef.FragmentContentHash,
		map[string]any{"asset_identity_ref": identity.OwnerRef, "asset_state_ref": state.OwnerRef, "scene_ref": scene.OwnerRef, "beat_ref": nil, "creator_decision_ref": nil},
	))
	value.Graph.Nodes = append(value.Graph.Nodes, identity, specification, state, binding, occurrence)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, identity.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, specification.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, state.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, occurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDescribesIdentity, identity.StoryNodeKey, specification.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeHasState, identity.StoryNodeKey, state.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeMaterializes, identity.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "asset"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, specification.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "specification"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, state.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"}),
		newEdge(t, storygraph.EdgeTypeAnchorsOccurrence, scene.StoryNodeKey, occurrence.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scene"}),
		newEdge(t, storygraph.EdgeTypeInstantiatesOccurrence, state.StoryNodeKey, occurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
	)
}

type productionReferenceTargetTestInput struct {
	kind           string
	fragment       string
	fulfillment    string
	identities     []*storygraph.Node
	specifications []*storygraph.Node
	states         []*storygraph.Node
	scenes         []*storygraph.Node
	occurrences    []*storygraph.Node
	interactions   []*storygraph.Node
	dependencies   []*storygraph.Node
}

func addProductionAllKindReferencePlan(t *testing.T, value *storygraph.ProductionOwnerSnapshot) {
	t.Helper()
	character := productionAssetNodeByKind(t, value, storygraph.NodeTypeAssetIdentity, "character")
	characterSpecification := productionAssetNodeByKind(t, value, storygraph.NodeTypeCharacterSpecification, "character")
	characterState := productionAssetNodeByKind(t, value, storygraph.NodeTypeAssetState, "character")
	characterOccurrence := productionOccurrenceByAssetKind(t, value, "character")
	location := productionAssetNodeByKind(t, value, storygraph.NodeTypeAssetIdentity, "location")
	locationSpecification := productionAssetNodeByKind(t, value, storygraph.NodeTypeLocationSpecification, "location")
	locationState := productionAssetNodeByKind(t, value, storygraph.NodeTypeAssetState, "location")
	locationOccurrence := productionOccurrenceByAssetKind(t, value, "location")
	prop := productionAssetNodeByKind(t, value, storygraph.NodeTypeAssetIdentity, "prop")
	propSpecification := productionAssetNodeByKind(t, value, storygraph.NodeTypePropSpecification, "prop")
	propState := productionAssetNodeByKind(t, value, storygraph.NodeTypeAssetState, "prop")
	propOccurrence := productionOccurrenceByAssetKind(t, value, "prop")
	scene := productionNodeByType(t, value, storygraph.NodeTypeScene)
	interaction := productionInteractionNode(t, value)

	owner := func(ownerKind, family, logicalID string) storygraph.OwnerRef {
		return storygraph.OwnerRef{
			WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID, OwnerKind: ownerKind,
			VersionFamily: family, OwnerLogicalID: logicalID, OwnerVersionID: uuid.NewString(),
			OwnerRevision: 1, OwnerContentHash: productionHash(logicalID),
		}
	}
	payload := func(contractID, projectionHash string, fields map[string]any) json.RawMessage {
		fields["payload_contract_id"] = contractID
		fields["projection_hash"] = projectionHash
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	node := func(nodeType storygraph.NodeType, ref storygraph.OwnerRef, body json.RawMessage) storygraph.Node {
		return storygraph.Node{StoryNodeKey: mustNodeKey(t, nodeType, ref), NodeType: nodeType, OwnerRef: ref, EvidenceRefs: []storygraph.EvidenceRef{}, Payload: body}
	}
	policyRef := owner("preset", "preset_effective_set", "policy:all-kind")
	styleRef := owner("preset", "preset_effective_set", "style:all-kind")
	planRef := owner("production/reference", "reference_plan_set", "reference-plan:all-kind")
	policy := node(storygraph.NodeTypePolicySnapshot, policyRef, payload("storygraph-production/policy_snapshot-ref-payload-contract", policyRef.OwnerContentHash, map[string]any{}))
	style := node(storygraph.NodeTypeEffectiveStyleSnapshot, styleRef, payload("storygraph-production/effective_style_snapshot-ref-payload-contract", styleRef.OwnerContentHash, map[string]any{}))
	plan := node(storygraph.NodeTypeApprovedReferencePlanVersion, planRef, payload("storygraph-production/approved_reference_plan_version-ref-payload-contract", planRef.OwnerContentHash, map[string]any{}))
	value.Graph.Nodes = append(value.Graph.Nodes, policy, style, plan)
	constraints := map[string]any{"world_rule_refs": []storygraph.OwnerRef{}, "policy_snapshot_ref": policyRef, "effective_style_snapshot_ref": styleRef}

	inputs := []productionReferenceTargetTestInput{
		{kind: "character_identity_anchor", fragment: "target:character-anchor", fulfillment: "required", identities: []*storygraph.Node{character}, specifications: []*storygraph.Node{characterSpecification}, states: []*storygraph.Node{characterState}, scenes: []*storygraph.Node{scene}, occurrences: []*storygraph.Node{characterOccurrence}},
		{kind: "location_board", fragment: "target:location-board", fulfillment: "required", identities: []*storygraph.Node{location}, specifications: []*storygraph.Node{locationSpecification}, states: []*storygraph.Node{locationState}, scenes: []*storygraph.Node{scene}, occurrences: []*storygraph.Node{locationOccurrence}},
		{kind: "prop_sheet", fragment: "target:prop-sheet", fulfillment: "required", identities: []*storygraph.Node{prop}, specifications: []*storygraph.Node{propSpecification}, states: []*storygraph.Node{propState}, scenes: []*storygraph.Node{scene}, occurrences: []*storygraph.Node{propOccurrence}},
	}
	for index := range inputs {
		inputs[index].dependencies = []*storygraph.Node{}
		inputs[index].interactions = []*storygraph.Node{}
	}
	targets := make([]storygraph.Node, 0, 5)
	for _, input := range inputs {
		targets = append(targets, addProductionReferenceTargetNode(t, value, planRef, styleRef, constraints, input))
	}
	basePointers := []*storygraph.Node{&targets[0], &targets[1], &targets[2]}
	allIdentities := []*storygraph.Node{character, location, prop}
	allSpecifications := []*storygraph.Node{characterSpecification, locationSpecification, propSpecification}
	allStates := []*storygraph.Node{characterState, locationState, propState}
	allOccurrences := []*storygraph.Node{characterOccurrence, locationOccurrence, propOccurrence}
	targets = append(targets,
		addProductionReferenceTargetNode(t, value, planRef, styleRef, constraints, productionReferenceTargetTestInput{
			kind: "scene_composition", fragment: "target:scene-composition", fulfillment: "required",
			identities: allIdentities, specifications: allSpecifications, states: allStates, scenes: []*storygraph.Node{scene},
			occurrences: allOccurrences, interactions: []*storygraph.Node{interaction}, dependencies: basePointers,
		}),
		addProductionReferenceTargetNode(t, value, planRef, styleRef, constraints, productionReferenceTargetTestInput{
			kind: "interaction_composition", fragment: "target:interaction-composition", fulfillment: "required",
			identities: []*storygraph.Node{character, prop}, specifications: []*storygraph.Node{characterSpecification, propSpecification},
			states: []*storygraph.Node{characterState, propState}, scenes: []*storygraph.Node{scene},
			occurrences: []*storygraph.Node{characterOccurrence, propOccurrence}, interactions: []*storygraph.Node{interaction}, dependencies: []*storygraph.Node{&targets[0], &targets[2]},
		}),
	)
	value.Graph.Nodes = append(value.Graph.Nodes, targets...)
	for index := range targets {
		input := inputs[0]
		if index < len(inputs) {
			input = inputs[index]
		} else if index == 3 {
			input = productionReferenceTargetTestInput{identities: allIdentities, specifications: allSpecifications, states: allStates, scenes: []*storygraph.Node{scene}, occurrences: allOccurrences, interactions: []*storygraph.Node{interaction}, dependencies: basePointers}
		} else {
			input = productionReferenceTargetTestInput{identities: []*storygraph.Node{character, prop}, specifications: []*storygraph.Node{characterSpecification, propSpecification}, states: []*storygraph.Node{characterState, propState}, scenes: []*storygraph.Node{scene}, occurrences: []*storygraph.Node{characterOccurrence, propOccurrence}, interactions: []*storygraph.Node{interaction}, dependencies: []*storygraph.Node{&targets[0], &targets[2]}}
		}
		addProductionReferenceTargetEdges(t, value, plan, policy, style, &targets[index], input, index+1)
	}
}

func addProductionReferenceTargetNode(
	t *testing.T,
	value *storygraph.ProductionOwnerSnapshot,
	planRef, styleRef storygraph.OwnerRef,
	constraints map[string]any,
	input productionReferenceTargetTestInput,
) storygraph.Node {
	t.Helper()
	ref := planRef
	ref.FragmentKey = input.fragment
	ref.FragmentContentHash = productionHash(input.fragment)
	ownerRefs := func(nodes []*storygraph.Node) []storygraph.OwnerRef {
		refs := make([]storygraph.OwnerRef, 0, len(nodes))
		for _, node := range nodes {
			refs = append(refs, node.OwnerRef)
		}
		sortProductionOwnerRefs(refs)
		return refs
	}
	fields := map[string]any{
		"payload_contract_id": "storygraph-production/reference-plan-target-payload-contract",
		"projection_hash":     ref.FragmentContentHash, "target_kind": input.kind, "fulfillment": input.fulfillment,
		"target_owner_refs": map[string]any{
			"identity": ownerRefs(input.identities), "specification": ownerRefs(input.specifications), "state": ownerRefs(input.states),
			"style": []storygraph.OwnerRef{styleRef}, "scene": ownerRefs(input.scenes), "occurrence": ownerRefs(input.occurrences), "interaction": ownerRefs(input.interactions),
		},
		"coverage_scope_keys": []string{input.scenes[0].OwnerRef.OwnerLogicalID}, "depends_on_target_refs": ownerRefs(input.dependencies), "constraints": constraints,
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return storygraph.Node{StoryNodeKey: mustNodeKey(t, storygraph.NodeTypeReferencePlanTarget, ref), NodeType: storygraph.NodeTypeReferencePlanTarget, OwnerRef: ref, EvidenceRefs: []storygraph.EvidenceRef{}, Payload: raw}
}

func addProductionReferenceTargetEdges(
	t *testing.T,
	value *storygraph.ProductionOwnerSnapshot,
	plan, policy, style storygraph.Node,
	target *storygraph.Node,
	input productionReferenceTargetTestInput,
	order int,
) {
	t.Helper()
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeContainsReferenceTarget, plan.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{SequenceKey: "target:" + string(rune('0'+order))}),
		newEdge(t, storygraph.EdgeTypeConstrains, policy.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "policy"}),
		newEdge(t, storygraph.EdgeTypeConstrains, style.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ConstraintRole: "style"}),
	)
	groups := []struct {
		nodes []*storygraph.Node
		role  string
	}{
		{input.identities, "identity"}, {input.specifications, "specification"}, {input.states, "state"},
		{[]*storygraph.Node{&style}, "style"}, {input.scenes, "scene"}, {input.occurrences, "occurrence"}, {input.interactions, "interaction"},
	}
	for _, group := range groups {
		for _, source := range group.nodes {
			value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypePlansReference, source.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{ReferenceRole: group.role}))
		}
	}
	for _, dependency := range input.dependencies {
		value.Graph.Edges = append(value.Graph.Edges, newEdge(t, storygraph.EdgeTypeDependsOnReferenceTarget, dependency.StoryNodeKey, target.StoryNodeKey, storygraph.EdgeQualifier{}))
	}
}

func productionAssetNodeByKind(t *testing.T, value *storygraph.ProductionOwnerSnapshot, nodeType storygraph.NodeType, kind string) *storygraph.Node {
	t.Helper()
	for index := range value.Graph.Nodes {
		node := &value.Graph.Nodes[index]
		if node.NodeType != nodeType {
			continue
		}
		var payload struct {
			AssetKind string `json:"asset_kind"`
		}
		if json.Unmarshal(node.Payload, &payload) == nil && payload.AssetKind == kind {
			return node
		}
	}
	t.Fatalf("missing production %s %s", kind, nodeType)
	return nil
}

func sortProductionOwnerRefs(refs []storygraph.OwnerRef) {
	sort.Slice(refs, func(left, right int) bool {
		leftRef, rightRef := refs[left], refs[right]
		leftKey := leftRef.OwnerKind + "\x00" + leftRef.VersionFamily + "\x00" + leftRef.OwnerLogicalID + "\x00" + leftRef.FragmentKey + "\x00" + leftRef.OwnerVersionID
		rightKey := rightRef.OwnerKind + "\x00" + rightRef.VersionFamily + "\x00" + rightRef.OwnerLogicalID + "\x00" + rightRef.FragmentKey + "\x00" + rightRef.OwnerVersionID
		return leftKey < rightKey
	})
}
