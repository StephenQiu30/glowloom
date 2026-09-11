package storygraph_test

import (
	"encoding/json"
	"strings"
	"testing"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestProductionNodeContractRejectsOwnerPayloadAndEvidenceDrift(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot){
		"owner family": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Nodes[0].OwnerRef.VersionFamily = "bible_production_world_set"
		},
		"excluded node": func(value *storygraph.ProductionOwnerSnapshot) {
			node := &value.Graph.Nodes[0]
			node.NodeType = storygraph.NodeTypeGenerationTarget
			node.OwnerRef.OwnerKind = "generation"
			node.OwnerRef.VersionFamily = "script_source_set"
			node.StoryNodeKey, _ = storygraph.DeriveStoryNodeKey(node.NodeType, node.OwnerRef)
		},
		"payload contract": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Nodes[0].Payload = json.RawMessage(`{"payload_contract_id":"storygraph-production/episode-ref-payload-contract","projection_hash":"` + value.Graph.Nodes[0].OwnerRef.OwnerContentHash + `"}`)
		},
		"projection hash": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Nodes[0].Payload = json.RawMessage(`{"payload_contract_id":"storygraph-production/source_revision-ref-payload-contract","projection_hash":"` + strings.Repeat("f", 64) + `"}`)
		},
		"evidence missing": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Nodes[3].EvidenceRefs = nil
		},
		"evidence forbidden": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Nodes[0].EvidenceRefs = append([]storygraph.EvidenceRef(nil), value.Graph.Nodes[1].EvidenceRefs...)
		},
		"copied label": func(value *storygraph.ProductionOwnerSnapshot) {
			productionNodeByType(t, value, storygraph.NodeTypeScene).Label = "开场"
		},
		"copied business position": func(value *storygraph.ProductionOwnerSnapshot) {
			productionNodeByType(t, value, storygraph.NodeTypeOccurrence).BusinessPosition = json.RawMessage(`{"sequence_key":"0001"}`)
		},
		"ref payload copies owner content": func(value *storygraph.ProductionOwnerSnapshot) {
			mutateProductionPayload(t, productionNodeByType(t, value, storygraph.NodeTypeEpisode), func(payload map[string]any) {
				payload["summary"] = "不应复制到内容图"
			})
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := productionOwnerSnapshotFixture(t)
			mutate(&value)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("invalid production node contract was accepted")
			}
		})
	}
}

func TestProductionAuditableBibleFactPayloadMatchesManifest(t *testing.T) {
	for name, nodeType := range map[string]storygraph.NodeType{
		"world rule":  storygraph.NodeTypeWorldRule,
		"story arc":   storygraph.NodeTypeStoryArc,
		"plot thread": storygraph.NodeTypePlotThread,
	} {
		t.Run(name, func(t *testing.T) {
			value := productionOwnerSnapshotFixture(t)
			node := addProductionAuditableBibleFact(t, &value, nodeType)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err != nil {
				t.Fatalf("exact auditable Bible fact was rejected: %v", err)
			}

			mutateProductionPayload(t, node, func(payload map[string]any) {
				payload["statement"] = "不得复制 Bible 正文"
			})
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("auditable Bible fact accepted a field excluded by its manifest contract")
			}
		})
	}
}

func TestProductionAuditableBibleFactRejectsInvalidCreatorDecision(t *testing.T) {
	value := productionOwnerSnapshotFixture(t)
	node := addProductionAuditableBibleFact(t, &value, storygraph.NodeTypeWorldRule)
	node.EvidenceRefs = nil
	mutateProductionPayload(t, node, func(payload map[string]any) {
		payload["creator_decision_ref"] = map[string]any{
			"workspace_id": node.OwnerRef.WorkspaceID, "project_id": node.OwnerRef.ProjectID,
			"audit_owner_kind": node.OwnerRef.OwnerKind, "audit_id": "decision:world-rule",
			"audit_revision": 1, "audit_content_hash": productionHash("decision:world-rule"),
		}
	})
	value.Graph.Edges = removeProductionEdge(value.Graph.Edges, func(edge storygraph.Edge) bool {
		return edge.EdgeType == storygraph.EdgeTypeDerivedFrom && edge.ToNodeKey == node.StoryNodeKey
	})
	if _, err := storygraph.CompileProductionOwnerSnapshot(value); err != nil {
		t.Fatalf("exact creator decision Bible fact was rejected: %v", err)
	}

	mutateProductionPayload(t, node, func(payload map[string]any) {
		payload["creator_decision_ref"] = map[string]any{"audit_id": "not-a-content-addressed-ref"}
	})
	if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
		t.Fatal("auditable Bible fact accepted an invalid creator decision ref")
	}
}

func addProductionAuditableBibleFact(t *testing.T, value *storygraph.ProductionOwnerSnapshot, nodeType storygraph.NodeType) *storygraph.Node {
	t.Helper()
	evidence := productionNodeByType(t, value, storygraph.NodeTypeSourceEvidence)
	owner := evidence.OwnerRef
	owner.FragmentKey = "auditable-bible-fact:" + string(nodeType)
	owner.FragmentContentHash = productionHash(owner.FragmentKey)
	payload, err := json.Marshal(map[string]any{
		"payload_contract_id":  "storygraph-production/auditable-bible-fact-payload-contract",
		"projection_hash":      owner.FragmentContentHash,
		"creator_decision_ref": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	node := storygraph.Node{
		StoryNodeKey: mustNodeKey(t, nodeType, owner), NodeType: nodeType, OwnerRef: owner,
		EvidenceRefs: append([]storygraph.EvidenceRef(nil), productionNodeByType(t, value, storygraph.NodeTypeAssetIdentity).EvidenceRefs...),
		Payload:      payload,
	}
	value.Graph.Nodes = append(value.Graph.Nodes, node)
	value.Graph.Edges = append(value.Graph.Edges,
		newEdge(t, storygraph.EdgeTypeDerivedFrom, evidence.StoryNodeKey, node.StoryNodeKey, storygraph.EdgeQualifier{}),
	)
	return &value.Graph.Nodes[len(value.Graph.Nodes)-1]
}

func TestProductionContinuityPayloadOnlyAcceptsDeclaredDiscriminants(t *testing.T) {
	for _, claimType := range []string{"interaction", "continuity"} {
		contractID, err := storygraph.ProductionPayloadContract(storygraph.NodeTypeContinuityClaim, claimType)
		if err != nil {
			t.Fatalf("claim type %s: %v", claimType, err)
		}
		want := map[string]string{
			"interaction": "storygraph-production/continuity-interaction-payload-contract",
			"continuity":  "storygraph-production/continuity-state-payload-contract",
		}[claimType]
		if contractID != want {
			t.Fatalf("claim type %s contract = %q, want %q", claimType, contractID, want)
		}
	}
	if _, err := storygraph.ProductionPayloadContract(storygraph.NodeTypeContinuityClaim, "uncertain"); err == nil {
		t.Fatal("undeclared continuity discriminant was accepted")
	}
	if _, err := storygraph.ProductionPayloadContract(storygraph.NodeTypeGenerationTarget, ""); err == nil {
		t.Fatal("excluded generation node received a production payload contract")
	}
}
