package storygraph_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionIdentityRelationsRejectPayloadAndEdgeDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot){
		"binding omits state refs": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNodeByType(t, value, storygraph.NodeTypeProductionBinding), func(payload map[string]any) {
				delete(payload, "state_refs")
			})
		},
		"binding adds unknown field": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNodeByType(t, value, storygraph.NodeTypeProductionBinding), func(payload map[string]any) {
				payload["latest_state"] = true
			})
		},
		"binding duplicates state ref": func(value *storygraph.ProductionOwnerSnapshot) {
			state := productionNodeByType(t, value, storygraph.NodeTypeAssetState)
			mutateProductionPayload(t, productionNodeByType(t, value, storygraph.NodeTypeProductionBinding), func(payload map[string]any) {
				payload["state_refs"] = []storygraph.OwnerRef{state.OwnerRef, state.OwnerRef}
			})
		},
		"binding loses state materialization": func(value *storygraph.ProductionOwnerSnapshot) {
			binding := productionNodeByType(t, value, storygraph.NodeTypeProductionBinding)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeMaterializes && edge.ToNodeKey == binding.StoryNodeKey && edge.Qualifier.BindingRole == "state"
			})
		},
		"state changes asset kind": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNodeByType(t, value, storygraph.NodeTypeAssetState), func(payload map[string]any) {
				payload["asset_kind"] = "prop"
			})
		},
		"occurrence points at episode as scene": func(value *storygraph.ProductionOwnerSnapshot) {
			episode := productionNodeByType(t, value, storygraph.NodeTypeEpisode)
			mutateProductionPayload(t, productionNodeByType(t, value, storygraph.NodeTypeOccurrence), func(payload map[string]any) {
				payload["scene_ref"] = episode.OwnerRef
			})
		},
		"occurrence loses scene anchor": func(value *storygraph.ProductionOwnerSnapshot) {
			occurrence := productionNodeByType(t, value, storygraph.NodeTypeOccurrence)
			value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
				return edge.EdgeType == storygraph.EdgeTypeAnchorsOccurrence && edge.ToNodeKey == occurrence.StoryNodeKey
			})
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := productionIdentityRelationFixture(t)
			mutate(&value)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("invalid production identity relation was accepted")
			}
		})
	}
}

func productionIdentityRelationFixture(t *testing.T) storygraph.ProductionOwnerSnapshot {
	t.Helper()
	value := productionOwnerSnapshotFixture(t)
	return value
}

func addProductionIdentityRelations(t *testing.T, value *storygraph.ProductionOwnerSnapshot) {
	t.Helper()
	asset := productionNodeByType(t, value, storygraph.NodeTypeAssetIdentity)
	scene := productionNodeByType(t, value, storygraph.NodeTypeScene)
	evidence := productionNodeByType(t, value, storygraph.NodeTypeSourceEvidence)
	evidenceRef := asset.EvidenceRefs[0]

	fragmentRef := func(base storygraph.OwnerRef, fragment string) storygraph.OwnerRef {
		base.FragmentKey = fragment
		base.FragmentContentHash = productionHash(fragment)
		return base
	}
	payload := func(contractID, projectionHash string, fields map[string]any) json.RawMessage {
		fields["payload_contract_id"] = contractID
		fields["projection_hash"] = projectionHash
		encoded, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	node := func(nodeType storygraph.NodeType, owner storygraph.OwnerRef, evidenceRefs []storygraph.EvidenceRef, body json.RawMessage) storygraph.Node {
		return storygraph.Node{StoryNodeKey: mustNodeKey(t, nodeType, owner), NodeType: nodeType, OwnerRef: owner, EvidenceRefs: evidenceRefs, Payload: body}
	}

	specificationRef := fragmentRef(evidence.OwnerRef, "specification:"+uuid.NewString())
	stateRef := fragmentRef(asset.OwnerRef, "state:"+uuid.NewString())
	bindingRef := fragmentRef(evidence.OwnerRef, "binding:"+uuid.NewString())
	occurrenceRef := fragmentRef(scene.OwnerRef, "occurrence:"+uuid.NewString())
	specification := node(storygraph.NodeTypeCharacterSpecification, specificationRef, []storygraph.EvidenceRef{evidenceRef}, payload(
		"storygraph-production/specification-payload-contract", specificationRef.FragmentContentHash,
		map[string]any{"asset_kind": "character", "creator_decision_ref": nil},
	))
	state := node(storygraph.NodeTypeAssetState, stateRef, []storygraph.EvidenceRef{evidenceRef}, payload(
		"storygraph-production/asset-state-payload-contract", stateRef.FragmentContentHash,
		map[string]any{"asset_kind": "character", "state_key": "appearance:default", "story_time_range": nil, "creator_decision_ref": nil},
	))
	binding := node(storygraph.NodeTypeProductionBinding, bindingRef, nil, payload(
		"storygraph-production/production-binding-payload-contract", bindingRef.FragmentContentHash,
		map[string]any{"asset_identity_ref": asset.OwnerRef, "specification_ref": specification.OwnerRef, "state_refs": []storygraph.OwnerRef{state.OwnerRef}},
	))
	occurrence := node(storygraph.NodeTypeOccurrence, occurrenceRef, []storygraph.EvidenceRef{evidenceRef}, payload(
		"storygraph-production/occurrence-payload-contract", occurrenceRef.FragmentContentHash,
		map[string]any{"asset_identity_ref": asset.OwnerRef, "asset_state_ref": state.OwnerRef, "scene_ref": scene.OwnerRef, "beat_ref": nil, "creator_decision_ref": nil},
	))
	value.Graph.Nodes = append(value.Graph.Nodes, specification, state, binding, occurrence)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, specification.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, state.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, occurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeDescribesIdentity, asset.StoryNodeKey, specification.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeHasState, asset.StoryNodeKey, state.StoryNodeKey, storygraph.EdgeQualifier{}),
		newEdge(t, storygraph.EdgeTypeMaterializes, asset.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "asset"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, specification.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "specification"}),
		newEdge(t, storygraph.EdgeTypeMaterializes, state.StoryNodeKey, binding.StoryNodeKey, storygraph.EdgeQualifier{BindingRole: "state"}),
		newEdge(t, storygraph.EdgeTypeAnchorsOccurrence, scene.StoryNodeKey, occurrence.StoryNodeKey, storygraph.EdgeQualifier{AnchorRole: "scene"}),
		newEdge(t, storygraph.EdgeTypeInstantiatesOccurrence, state.StoryNodeKey, occurrence.StoryNodeKey, storygraph.EdgeQualifier{}),
	)
}

func productionNodeByType(t *testing.T, value *storygraph.ProductionOwnerSnapshot, nodeType storygraph.NodeType) *storygraph.Node {
	t.Helper()
	for index := range value.Graph.Nodes {
		if value.Graph.Nodes[index].NodeType == nodeType {
			return &value.Graph.Nodes[index]
		}
	}
	t.Fatalf("missing production node type %s", nodeType)
	return nil
}

func mutateProductionPayload(t *testing.T, node *storygraph.Node, mutate func(map[string]any)) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(node.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	mutate(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	node.Payload = encoded
}
