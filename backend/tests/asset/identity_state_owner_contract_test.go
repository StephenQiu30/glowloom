package asset_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/asset/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

func TestAssetIdentityStateOwnerUsesSharedCollectionRoot(t *testing.T) {
	workspaceID, projectID, actorID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Date(2026, time.September, 11, 21, 0, 0, 0, time.UTC)
	asset, err := domain.NewAsset(domain.AssetInput{
		ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID,
		Kind: domain.AssetKindCharacter, IdentityKey: "character:linzhou",
		CreatedBy: actorID, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := domain.NewAssetState(domain.AssetStateInput{
		ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, AssetID: asset.ID,
		StateKey: "state_character_linzhou_initial", Label: "初始", Revision: 1,
		Snapshot: []byte(`{"appearance":"plain"}`), CreatedBy: actorID, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	injured, err := domain.NewAssetState(domain.AssetStateInput{
		ID: uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID, AssetID: asset.ID,
		StateKey: "state_character_linzhou_injured", Label: "受伤", Revision: 2,
		Snapshot: []byte(`{"appearance":"injured"}`), CreatedBy: actorID, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assets, states := []domain.Asset{asset}, []domain.AssetState{initial, injured}

	collection, err := domain.BuildIdentityStateCollection(workspaceID, projectID, 4, assets, states)
	if err != nil {
		t.Fatal(err)
	}
	wantMembers := []ownercollection.VersionRef{
		{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "asset", VersionFamily: domain.AssetIdentityStateCollectionFamily,
			OwnerLogicalID: asset.IdentityKey, OwnerVersionID: asset.ID,
			OwnerRevision: int64(asset.Revision), OwnerContentHash: asset.ContentHash,
		},
		{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "asset", VersionFamily: domain.AssetIdentityStateCollectionFamily,
			OwnerLogicalID: initial.StateKey, OwnerVersionID: initial.ID,
			OwnerRevision: int64(initial.Revision), OwnerContentHash: initial.ContentHash,
		},
		{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "asset", VersionFamily: domain.AssetIdentityStateCollectionFamily,
			OwnerLogicalID: injured.StateKey, OwnerVersionID: injured.ID,
			OwnerRevision: int64(injured.Revision), OwnerContentHash: injured.ContentHash,
		},
	}
	if collection.ScopeKind != "project" || collection.ScopeKey != "project:"+projectID ||
		collection.ScopeRevision != 4 || collection.MemberCount != 3 ||
		!reflect.DeepEqual(collection.Members, wantMembers) {
		t.Fatalf("collection=%#v", collection)
	}

	head, err := domain.NewIdentityStateCollectionHead(workspaceID, projectID, 4, assets, states, now)
	if err != nil {
		t.Fatal(err)
	}
	if head.CollectionRootHash != collection.CollectionRootHash ||
		head.MemberCount != int(collection.MemberCount) ||
		!reflect.DeepEqual(head.CurrentVersionRefs, collection.Members) || len(head.Members) != 2 ||
		len(head.HeadContentHash) != 64 {
		t.Fatalf("head=%#v", head)
	}
}
