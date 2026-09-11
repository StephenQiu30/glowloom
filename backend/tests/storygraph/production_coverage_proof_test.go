package storygraph_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestBuildProductionCoverageProofBindsP0ReceiptsCollectionsAndSchema(t *testing.T) {
	snapshot := productionOwnerSnapshotFixture(t)
	proof := productionCoverageProofFixture(t, snapshot)
	snapshot.Coverage = proof

	compiled, err := storygraph.CompileProductionOwnerSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := storygraph.BuildProductionSchemaRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if compiled.SchemaID != storygraph.ProductionSchemaID || compiled.SchemaRank != storygraph.ProductionSchemaRank ||
		compiled.SchemaManifestHash != registry.SchemaHash || compiled.NodeKeyDerivationID != storygraph.StoryNodeKeyDerivationID ||
		compiled.EdgeKeyDerivationID != storygraph.StoryEdgeKeyDerivationID ||
		compiled.Coverage.CoverageScopeManifestHash == "" || len(compiled.Coverage.OwnerApplyReceiptRefs) != 6 ||
		len(compiled.Coverage.CollectionRootHashes) != 7 {
		t.Fatalf("compiled Production identity = %#v", compiled)
	}

	reordered, err := storygraph.BuildProductionCoverageProof(storygraph.ProductionCoverageProofInput{
		WorkspaceID: snapshot.WorkspaceID, ProjectID: snapshot.ProjectID,
		CoverageScopeSets:     proof.CoverageScopeSets,
		OwnerApplyReceiptRefs: reversed(proof.OwnerApplyReceiptRefs),
		OwnerCollections:      reversed(snapshot.OwnerCollections),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reordered.CoverageScopeManifestHash != proof.CoverageScopeManifestHash ||
		!slices.Equal(reordered.CollectionRootHashes, proof.CollectionRootHashes) {
		t.Fatal("P0 coverage proof depends on input traversal order")
	}
}

func TestProductionCompilationInputRoundTripRecomputesSchemaCoverageAndGraphHashes(t *testing.T) {
	snapshot := productionOwnerSnapshotFixture(t)
	compiled, err := storygraph.CompileProductionOwnerSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	input := storygraph.ProductionCompilationInput{
		SchemaID: compiled.SchemaID, SchemaRank: compiled.SchemaRank,
		SchemaManifestHash: compiled.SchemaManifestHash, Coverage: compiled.Coverage,
		CoveragePhase:             compiled.Coverage.CoveragePhase,
		CoverageScopeManifestHash: compiled.Coverage.CoverageScopeManifestHash,
		NodeKeyDerivationID:       compiled.NodeKeyDerivationID, EdgeKeyDerivationID: compiled.EdgeKeyDerivationID,
		OwnerCollections: compiled.OwnerCollections,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := storygraph.DecodeProductionCompilationInput(encoded)
	if err != nil {
		t.Fatal(err)
	}
	version := storygraph.Version{
		WorkspaceID: snapshot.WorkspaceID, ProjectID: snapshot.ProjectID,
		SourceRevisionID: snapshot.SourceRevisionID, SourceRevisionHash: snapshot.SourceRevisionHash,
		OwnerHeads: compiled.OwnerHeads, OwnerSetHash: compiled.OwnerSetHash,
		SchemaVersion: storygraph.ProductionSchemaID, Nodes: compiled.Graph.Nodes, Edges: compiled.Graph.Edges,
		TopologyHash: compiled.Graph.TopologyHash, ContentHash: compiled.Graph.ContentHash,
		ProductionInput: &decoded,
	}
	if err = storygraph.ValidateProductionVersion(version); err != nil {
		t.Fatal(err)
	}

	drifted := version
	driftedInput := decoded
	driftedInput.SchemaManifestHash = strings.Repeat("f", 64)
	drifted.ProductionInput = &driftedInput
	if err = storygraph.ValidateProductionVersion(drifted); err == nil {
		t.Fatal("drifted persisted Production compilation input was accepted")
	}

	var object map[string]any
	if err = json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	object["compatibility_schema_hash"] = compiled.SchemaManifestHash
	unknown, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = storygraph.DecodeProductionCompilationInput(unknown); err == nil {
		t.Fatal("unknown Production compilation input field was accepted")
	}
}

func TestBuildProductionCoverageProofRejectsReceiptOrScopeDrift(t *testing.T) {
	snapshot := productionOwnerSnapshotFixture(t)
	proof := productionCoverageProofFixture(t, snapshot)
	tests := map[string]func(*storygraph.ProductionCoverageProofInput){
		"missing receipt": func(value *storygraph.ProductionCoverageProofInput) {
			value.OwnerApplyReceiptRefs = value.OwnerApplyReceiptRefs[1:]
		},
		"collection root mismatch": func(value *storygraph.ProductionCoverageProofInput) {
			value.OwnerApplyReceiptRefs[0].CommittedCollectionRootHash = productionHash("wrong-root")
		},
		"gate coverage gap": func(value *storygraph.ProductionCoverageProofInput) {
			for index := range value.OwnerApplyReceiptRefs {
				if value.OwnerApplyReceiptRefs[index].DecisionCheckpointID == "gate_2_bible_continuity" {
					value.OwnerApplyReceiptRefs[index].CoveredScopeKeys = []string{"scene:another"}
				}
			}
		},
		"committed ref outside collection": func(value *storygraph.ProductionCoverageProofInput) {
			value.OwnerApplyReceiptRefs[0].CommittedOwnerVersionRef.OwnerVersionID = "current"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := storygraph.ProductionCoverageProofInput{
				WorkspaceID: snapshot.WorkspaceID, ProjectID: snapshot.ProjectID,
				CoverageScopeSets:     proof.CoverageScopeSets,
				OwnerApplyReceiptRefs: append([]storygraph.ProductionOwnerApplyReceiptRef(nil), proof.OwnerApplyReceiptRefs...),
				OwnerCollections:      append([]storygraph.OwnerCollectionRef(nil), snapshot.OwnerCollections...),
			}
			for index := range input.OwnerApplyReceiptRefs {
				if input.OwnerApplyReceiptRefs[index].CommittedOwnerVersionRef != nil {
					copy := *input.OwnerApplyReceiptRefs[index].CommittedOwnerVersionRef
					input.OwnerApplyReceiptRefs[index].CommittedOwnerVersionRef = &copy
				}
			}
			mutate(&input)
			if _, err := storygraph.BuildProductionCoverageProof(input); err == nil {
				t.Fatal("invalid Production coverage proof was accepted")
			}
		})
	}
}

func productionCoverageProofFixture(t *testing.T, snapshot storygraph.ProductionOwnerSnapshot) storygraph.ProductionCoverageProof {
	t.Helper()
	projectScope := "project:" + snapshot.ProjectID
	receipts := make([]storygraph.ProductionOwnerApplyReceiptRef, 0, 6)
	for _, collection := range snapshot.OwnerCollections {
		checkpoint := "gate_2_bible_continuity"
		coveredScopes := []string{"scene:opening"}
		switch collection.VersionFamily {
		case "script_source_set":
			checkpoint, coveredScopes = "source_revision_accepted", []string{}
		case "project_episode_set":
			checkpoint, coveredScopes = "project_episode_lifecycle_confirmed", []string{}
		case "bible_structure_identity_set":
			checkpoint = "gate_1_structure_identity"
		case "planning_structure_rebase_set":
			continue
		}
		member := collection.Members[0]
		committed := ownercollection.VersionRef{
			WorkspaceID: member.WorkspaceID, ProjectID: member.ProjectID, OwnerKind: member.OwnerKind,
			VersionFamily: member.VersionFamily, OwnerLogicalID: member.LogicalID,
			OwnerVersionID: member.VersionID, OwnerRevision: member.Revision, OwnerContentHash: member.ContentHash,
		}
		receipts = append(receipts, storygraph.ProductionOwnerApplyReceiptRef{
			DecisionCheckpointID: checkpoint, ReceiptScopeKey: projectScope, CoveredScopeKeys: coveredScopes,
			OwnerKind: collection.OwnerKind, VersionFamily: collection.VersionFamily,
			CommittedCollectionRootHash: collection.CollectionRootHash, CommittedOwnerVersionRef: &committed,
			ReceiptContentHash: productionHash(checkpoint + collection.VersionFamily),
		})
	}
	proof, err := storygraph.BuildProductionCoverageProof(storygraph.ProductionCoverageProofInput{
		WorkspaceID: snapshot.WorkspaceID, ProjectID: snapshot.ProjectID,
		CoverageScopeSets:     storygraph.ProductionCoverageScopeSets{P0ScopeKeys: []string{"scene:opening"}},
		OwnerApplyReceiptRefs: receipts, OwnerCollections: snapshot.OwnerCollections,
	})
	if err != nil {
		t.Fatal(err)
	}
	return proof
}
