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
