package storygraph_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
	storygraph "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

func TestVisualFoundationWorldReadSetUsesCurrentConfirmedProductionRoots(t *testing.T) {
	version := visualFoundationProductionVersion(t)
	reader := &storyGraphQueryReader{
		current:             version.ID,
		currentOwnerSetHash: version.OwnerSetHash,
		versions:            map[string]storygraph.Version{version.ID: version},
	}

	result, err := storygraphapp.NewQueryService(reader).VisualFoundationWorld(
		context.Background(),
		storygraphapp.Actor{UserID: uuid.NewString(), TokenVersion: 1},
		version.ProjectID,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantFamilies := []string{
		"asset_identity_state_set",
		"bible_production_world_set",
		"planning_scene_set",
	}
	gotFamilies := make([]string, len(result.ConfirmedWorldRoots))
	for index, root := range result.ConfirmedWorldRoots {
		gotFamilies[index] = root.OwnerFamily
	}
	if result.WorkspaceID != version.WorkspaceID || result.ProjectID != version.ProjectID ||
		result.StoryGraphVersionID != version.ID || result.StoryGraphContentHash != version.ContentHash ||
		result.ProductionWorldOwnerSetHash != version.OwnerSetHash ||
		!slices.Equal(gotFamilies, wantFamilies) {
		t.Fatalf("Visual Foundation world read set = %#v", result)
	}
}

func TestVisualFoundationWorldReadSetRejectsStaleOrNonProductionGraph(t *testing.T) {
	version := visualFoundationProductionVersion(t)
	reader := &storyGraphQueryReader{
		current:             version.ID,
		currentOwnerSetHash: productionHash("new-owner-set"),
		versions:            map[string]storygraph.Version{version.ID: version},
	}
	service := storygraphapp.NewQueryService(reader)
	_, err := service.VisualFoundationWorld(context.Background(), storygraphapp.Actor{}, version.ProjectID)
	expectStoryGraphQueryCode(t, err, "stale_production_world")

	legacy := queryVersionForVisualFoundationRejection(t, version)
	legacy.OwnerSetHash = productionHash("legacy-owner-set")
	reader.currentOwnerSetHash = legacy.OwnerSetHash
	reader.versions[legacy.ID] = legacy
	reader.current = legacy.ID
	_, err = service.VisualFoundationWorld(context.Background(), storygraphapp.Actor{}, legacy.ProjectID)
	expectStoryGraphQueryCode(t, err, "production_world_unavailable")
}

func visualFoundationProductionVersion(t *testing.T) storygraph.Version {
	t.Helper()
	compiled, err := storygraph.CompileProductionOwnerSnapshot(productionOwnerSnapshotFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	version := storygraph.Version{
		ID: uuid.NewString(), WorkspaceID: compiled.WorkspaceID, ProjectID: compiled.ProjectID,
		VersionNo: 1, SourceRevisionID: compiled.SourceRevisionID, SourceRevisionHash: compiled.SourceRevisionHash,
		OwnerHeads: compiled.OwnerHeads, OwnerSetHash: compiled.OwnerSetHash, SchemaVersion: compiled.SchemaID,
		Nodes: compiled.Graph.Nodes, Edges: compiled.Graph.Edges, TopologyHash: compiled.Graph.TopologyHash,
		ContentHash: compiled.Graph.ContentHash, Status: "published", PublishedAt: now,
		CreatedBy: uuid.NewString(), CreatedAt: now,
		ProductionInput: &storygraph.ProductionCompilationInput{
			SchemaID: compiled.SchemaID, SchemaRank: compiled.SchemaRank,
			SchemaManifestHash: compiled.SchemaManifestHash, Coverage: compiled.Coverage,
			CoveragePhase:             compiled.Coverage.CoveragePhase,
			CoverageScopeManifestHash: compiled.Coverage.CoverageScopeManifestHash,
			NodeKeyDerivationID:       compiled.NodeKeyDerivationID,
			EdgeKeyDerivationID:       compiled.EdgeKeyDerivationID,
			OwnerCollections:          compiled.OwnerCollections,
		},
	}
	if err = storygraph.ValidateProductionVersion(version); err != nil {
		t.Fatalf("build production version fixture: %v", err)
	}
	return version
}

func queryVersionForVisualFoundationRejection(t *testing.T, production storygraph.Version) storygraph.Version {
	t.Helper()
	legacy, _ := queryVersion(t, uuid.NewString(), "旧图")
	legacy.WorkspaceID = production.WorkspaceID
	legacy.ProjectID = production.ProjectID
	return legacy
}
