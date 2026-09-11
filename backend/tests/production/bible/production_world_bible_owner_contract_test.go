package bible_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	"github.com/StephenQiu30/lanverse/backend/internal/production/bible/domain"
)

func TestProductionWorldBibleOwnerUsesSharedCollectionRoot(t *testing.T) {
	workspaceID, projectID := uuid.NewString(), uuid.NewString()
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	version := domain.ProductionWorldBibleVersion{
		ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID,
		Revision: 3, ContentHash: strings.Repeat("a", 64), CreatedAt: now,
	}
	collection, err := domain.BuildProductionWorldBibleCollection(version)
	if err != nil {
		t.Fatal(err)
	}
	wantMember := ownercollection.VersionRef{
		WorkspaceID: workspaceID, ProjectID: projectID,
		OwnerKind: "production/bible", VersionFamily: domain.BibleProductionWorldFamily,
		OwnerLogicalID: projectID, OwnerVersionID: version.ID,
		OwnerRevision: version.Revision, OwnerContentHash: version.ContentHash,
	}
	if collection.ScopeKey != "project:"+projectID || collection.ScopeRevision != version.Revision ||
		collection.MemberCount != 1 || !reflect.DeepEqual(collection.Members, []ownercollection.VersionRef{wantMember}) {
		t.Fatalf("collection=%#v", collection)
	}
	head, err := domain.NewProductionWorldBibleHead(workspaceID, projectID, version.Revision, version, now)
	if err != nil {
		t.Fatal(err)
	}
	if head.CollectionRootHash != collection.CollectionRootHash ||
		!reflect.DeepEqual(head.CurrentVersionRefs, collection.Members) || len(head.HeadContentHash) != 64 {
		t.Fatalf("head=%#v", head)
	}
}
