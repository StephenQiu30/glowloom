package planning_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	"github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
)

func TestProductionWorldPlanningOwnerUsesSharedCollectionRoot(t *testing.T) {
	workspaceID, projectID, episodeID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	createdBy := uuid.NewString()
	now := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	scene, err := domain.NewProductionWorldPlanningFact(
		uuid.NewString(), workspaceID, projectID, episodeID, "scene", "scene:opening", 2,
		[]byte(`{"scene_scope_key":"scene:opening"}`), createdBy, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	beat, err := domain.NewProductionWorldPlanningFact(
		uuid.NewString(), workspaceID, projectID, episodeID, "narrative_beat", "beat:arrival", 1,
		[]byte(`{"sequence_key":1}`), createdBy, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	facts := []domain.ProductionWorldPlanningFact{scene, beat}

	collection, err := domain.BuildProductionWorldPlanningCollection(
		workspaceID, projectID, episodeID, 3, facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantMembers := []ownercollection.VersionRef{
		{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "production/planning", VersionFamily: domain.PlanningSceneCollectionFamily,
			OwnerLogicalID: beat.BusinessKey, OwnerVersionID: beat.ID,
			OwnerRevision: int64(beat.Revision), OwnerContentHash: beat.ContentHash,
		},
		{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "production/planning", VersionFamily: domain.PlanningSceneCollectionFamily,
			OwnerLogicalID: scene.BusinessKey, OwnerVersionID: scene.ID,
			OwnerRevision: int64(scene.Revision), OwnerContentHash: scene.ContentHash,
		},
	}
	if collection.ScopeKind != "episode" || collection.ScopeKey != "episode:"+episodeID ||
		collection.ScopeRevision != 3 || !reflect.DeepEqual(collection.Members, wantMembers) {
		t.Fatalf("collection=%#v", collection)
	}

	head, err := domain.NewProductionWorldPlanningEpisodeHead(
		workspaceID, projectID, episodeID, 3, facts, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if head.CollectionRootHash != collection.CollectionRootHash ||
		head.ScopeContentHash != collection.ScopeContentHash ||
		head.MembersHash != collection.MembersHash ||
		head.MemberCount != int(collection.MemberCount) ||
		!reflect.DeepEqual(head.CurrentVersionRefs, collection.Members) || len(head.HeadContentHash) != 64 {
		t.Fatalf("head=%#v", head)
	}
}
