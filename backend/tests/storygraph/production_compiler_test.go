package storygraph_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestCompileProductionOwnerSnapshotFreezesP0OwnerCollections(t *testing.T) {
	snapshot := productionOwnerSnapshotFixture(t)

	compiled, err := storygraph.CompileProductionOwnerSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Graph.SchemaVersion != storygraph.ProductionSchemaID || len(compiled.OwnerCollections) != 7 ||
		len(compiled.OwnerHeads) != 7 || compiled.OwnerSetHash == "" {
		t.Fatalf("compiled Production StoryGraph = %#v", compiled)
	}

	reordered := snapshot
	reordered.OwnerCollections = reversed(reordered.OwnerCollections)
	reordered.Graph.Nodes = reversed(reordered.Graph.Nodes)
	reordered.Graph.Edges = reversed(reordered.Graph.Edges)
	recompiled, err := storygraph.CompileProductionOwnerSnapshot(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if recompiled.OwnerSetHash != compiled.OwnerSetHash || recompiled.Graph.ContentHash != compiled.Graph.ContentHash {
		t.Fatal("Production StoryGraph compilation depends on input traversal order")
	}
}

func TestCompileProductionOwnerSnapshotRejectsIncompleteOrPollutedReadSet(t *testing.T) {
	tests := map[string]func(*storygraph.ProductionOwnerSnapshot){
		"missing collection": func(value *storygraph.ProductionOwnerSnapshot) {
			value.OwnerCollections = value.OwnerCollections[1:]
		},
		"cross project member": func(value *storygraph.ProductionOwnerSnapshot) {
			value.OwnerCollections[0].Members[0].ProjectID = uuid.NewString()
		},
		"current version": func(value *storygraph.ProductionOwnerSnapshot) {
			value.OwnerCollections[0].Members[0].VersionID = "current"
		},
		"candidate owner": func(value *storygraph.ProductionOwnerSnapshot) {
			value.OwnerCollections[0].Members[0].OwnerKind = "agent"
		},
		"collection root drift": func(value *storygraph.ProductionOwnerSnapshot) {
			value.OwnerCollections[0].CollectionRootHash = strings.Repeat("f", 64)
		},
		"node outside read set": func(value *storygraph.ProductionOwnerSnapshot) {
			value.Graph.Nodes[0].OwnerRef.OwnerVersionID = uuid.NewString()
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := productionOwnerSnapshotFixture(t)
			mutate(&value)
			if _, err := storygraph.CompileProductionOwnerSnapshot(value); err == nil {
				t.Fatal("invalid Production Owner snapshot was accepted")
			}
		})
	}
}

func productionOwnerSnapshotFixture(t *testing.T) storygraph.ProductionOwnerSnapshot {
	t.Helper()
	workspaceID, projectID := uuid.NewString(), uuid.NewString()
	now := time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC)
	member := func(ownerKind, family, logicalID string) storygraph.OwnerVersionIdentity {
		return storygraph.OwnerVersionIdentity{
			WorkspaceID: workspaceID, ProjectID: projectID, OwnerKind: ownerKind,
			VersionFamily: family, LogicalID: logicalID, VersionID: uuid.NewString(), Revision: 1,
			ContentHash: productionHash(logicalID), CreatedAt: now,
		}
	}
	source := member("production/script", "script_source_set", "script")
	spanIndex := member("production/script", "script_source_set", "span-index")
	episode := member("production/project", "project_episode_set", "episode")
	structure := member("production/bible", "bible_structure_identity_set", "structure")
	bible := member("production/bible", "bible_production_world_set", "world")
	asset := member("asset", "asset_identity_state_set", "character:linzhou")
	planning := member("production/planning", "planning_scene_set", "scene:opening")
	collections := []storygraph.OwnerCollectionRef{
		productionCollection(t, workspaceID, projectID, "production/script", "script_source_set", "project", "project:"+projectID, []storygraph.OwnerVersionIdentity{source, spanIndex}),
		productionCollection(t, workspaceID, projectID, "production/project", "project_episode_set", "project", "project:"+projectID, []storygraph.OwnerVersionIdentity{episode}),
		productionCollection(t, workspaceID, projectID, "production/bible", "bible_structure_identity_set", "project", "project:"+projectID, []storygraph.OwnerVersionIdentity{structure}),
		productionCollection(t, workspaceID, projectID, "production/bible", "bible_production_world_set", "project", "project:"+projectID, []storygraph.OwnerVersionIdentity{bible}),
		productionCollection(t, workspaceID, projectID, "production/planning", "planning_scene_set", "episode", "episode:"+episode.LogicalID, []storygraph.OwnerVersionIdentity{planning}),
		productionCollection(t, workspaceID, projectID, "production/planning", "planning_structure_rebase_set", "project", "project:"+projectID, []storygraph.OwnerVersionIdentity{}),
		productionCollection(t, workspaceID, projectID, "asset", "asset_identity_state_set", "project", "project:"+projectID, []storygraph.OwnerVersionIdentity{asset}),
	}
	ownerRef := func(value storygraph.OwnerVersionIdentity, fragment string) storygraph.OwnerRef {
		fragmentHash := ""
		if fragment != "" {
			fragmentHash = productionHash(fragment)
		}
		return storygraph.OwnerRef{WorkspaceID: value.WorkspaceID, ProjectID: value.ProjectID,
			OwnerKind: value.OwnerKind, VersionFamily: value.VersionFamily, OwnerLogicalID: value.LogicalID,
			FragmentKey: fragment, FragmentContentHash: fragmentHash,
			OwnerVersionID: value.VersionID, OwnerRevision: value.Revision, OwnerContentHash: value.ContentHash}
	}
	sourceRef, episodeRef := ownerRef(source, ""), ownerRef(episode, "")
	assetRef, planningRef := ownerRef(asset, ""), ownerRef(planning, "")
	evidence := storygraph.EvidenceRef{DocumentRevisionID: source.VersionID, AbsoluteStart: 0, AbsoluteEnd: 4, TextHash: productionHash("evidence")}
	bibleRef := ownerRef(bible, fmt.Sprintf("source-evidence:%s:%012d:%012d:%s", evidence.DocumentRevisionID, evidence.AbsoluteStart, evidence.AbsoluteEnd, evidence.TextHash))
	bibleRef.FragmentContentHash = evidence.TextHash
	sourceKey := mustNodeKey(t, storygraph.NodeTypeSourceRevision, sourceRef)
	episodeKey := mustNodeKey(t, storygraph.NodeTypeEpisode, episodeRef)
	evidenceKey := mustNodeKey(t, storygraph.NodeTypeSourceEvidence, bibleRef)
	assetKey := mustNodeKey(t, storygraph.NodeTypeAssetIdentity, assetRef)
	sceneKey := mustNodeKey(t, storygraph.NodeTypeScene, planningRef)
	payload := func(contractID, projectionHash string, fields map[string]any) json.RawMessage {
		value := map[string]any{"payload_contract_id": contractID, "projection_hash": projectionHash}
		for key, field := range fields {
			value[key] = field
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	result := storygraph.ProductionOwnerSnapshot{
		Origin: storygraph.OwnerSnapshotOriginConfirmed, WorkspaceID: workspaceID, ProjectID: projectID,
		SourceRevisionID: source.VersionID, SourceRevisionHash: source.ContentHash,
		Coverage: storygraph.ProductionCoverageProof{
			Phase: "p0", StructureIdentityReceiptID: uuid.NewString(), StructureIdentityReceiptHash: productionHash("gate-one"),
			ProductionWorldReceiptID: uuid.NewString(), ProductionWorldReceiptHash: productionHash("gate-two"),
		},
		OwnerCollections: collections,
		Graph: storygraph.Snapshot{SchemaVersion: storygraph.ProductionSchemaID, Nodes: []storygraph.Node{
			{StoryNodeKey: sourceKey, NodeType: storygraph.NodeTypeSourceRevision, OwnerRef: sourceRef, Payload: payload("storygraph-production/source_revision-ref-payload-contract", sourceRef.OwnerContentHash, nil)},
			{StoryNodeKey: episodeKey, NodeType: storygraph.NodeTypeEpisode, OwnerRef: episodeRef, EvidenceRefs: []storygraph.EvidenceRef{evidence}, Payload: payload("storygraph-production/episode-ref-payload-contract", episodeRef.OwnerContentHash, nil)},
			{StoryNodeKey: evidenceKey, NodeType: storygraph.NodeTypeSourceEvidence, OwnerRef: bibleRef, Payload: payload("storygraph-production/source_evidence-ref-payload-contract", bibleRef.FragmentContentHash, nil)},
			{StoryNodeKey: assetKey, NodeType: storygraph.NodeTypeAssetIdentity, OwnerRef: assetRef, EvidenceRefs: []storygraph.EvidenceRef{evidence}, Payload: payload("storygraph-production/asset-identity-payload-contract", assetRef.OwnerContentHash, map[string]any{"asset_kind": "character", "creator_decision_ref": nil})},
			{StoryNodeKey: sceneKey, NodeType: storygraph.NodeTypeScene, OwnerRef: planningRef, EvidenceRefs: []storygraph.EvidenceRef{evidence}, Payload: payload("storygraph-production/scene-ref-payload-contract", planningRef.OwnerContentHash, nil)},
		}, Edges: []storygraph.Edge{
			newEdge(t, storygraph.EdgeTypeDerivedFrom, sourceKey, evidenceKey, storygraph.EdgeQualifier{}),
			newEdge(t, storygraph.EdgeTypeDerivedFrom, evidenceKey, episodeKey, storygraph.EdgeQualifier{}),
			newEdge(t, storygraph.EdgeTypeDerivedFrom, evidenceKey, assetKey, storygraph.EdgeQualifier{}),
			newEdge(t, storygraph.EdgeTypeDerivedFrom, evidenceKey, sceneKey, storygraph.EdgeQualifier{}),
			newEdge(t, storygraph.EdgeTypeContains, episodeKey, sceneKey, storygraph.EdgeQualifier{SequenceKey: "scene:0001"}),
		}},
	}
	addProductionIdentityRelations(t, &result)
	return result
}

func productionHash(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func productionCollection(t *testing.T, workspaceID, projectID, ownerKind, family, scopeKind, scopeKey string, members []storygraph.OwnerVersionIdentity) storygraph.OwnerCollectionRef {
	t.Helper()
	value, err := storygraph.BuildOwnerCollectionRef(storygraph.OwnerCollectionRef{
		WorkspaceID: workspaceID, ProjectID: projectID, OwnerKind: ownerKind, VersionFamily: family,
		ScopeKind: scopeKind, ScopeKey: scopeKey, ScopeRevision: 1, Members: members,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
