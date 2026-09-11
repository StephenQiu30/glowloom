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

func TestStructureIdentityOwnerUsesSharedCollectionAndReceiptContracts(t *testing.T) {
	workspaceID, projectID := uuid.NewString(), uuid.NewString()
	now := time.Date(2026, time.September, 11, 10, 0, 0, 0, time.UTC)
	version := domain.StructureIdentitySetVersion{
		SchemaVersion: domain.StructureIdentitySetSchemaVersion,
		ID:            uuid.NewString(), WorkspaceID: workspaceID, ProjectID: projectID,
		Version: 3, ContentHash: strings.Repeat("a", 64), CreatedAt: now,
	}
	collection, err := domain.BuildStructureIdentityCollection(version)
	if err != nil {
		t.Fatal(err)
	}
	wantMember := ownercollection.VersionRef{
		WorkspaceID: workspaceID, ProjectID: projectID,
		OwnerKind: "production/bible", VersionFamily: domain.StructureIdentityCollectionFamily,
		OwnerLogicalID: projectID, OwnerVersionID: version.ID,
		OwnerRevision: int64(version.Version), OwnerContentHash: version.ContentHash,
	}
	if collection.ScopeKey != "project:"+projectID || collection.ScopeRevision != int64(version.Version) ||
		collection.MemberCount != 1 || !reflect.DeepEqual(collection.Members, []ownercollection.VersionRef{wantMember}) {
		t.Fatalf("collection=%#v", collection)
	}
	head, err := domain.NewStructureIdentityScopeHead(collection, version.ID, 3, now)
	if err != nil {
		t.Fatal(err)
	}
	if head.CurrentVersionID != version.ID || head.CollectionRootHash != collection.CollectionRootHash || len(head.HeadContentHash) != 64 {
		t.Fatalf("head=%#v", head)
	}
	scopes := []string{"scene:a", "scene:b"}
	receipt, err := domain.NewStructureIdentityCollectionReceipt(
		uuid.NewString(), uuid.NewString(), "gate-1-structure", uuid.NewString(), collection, scopes, now, uuid.NewString(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CheckpointKey != domain.StructureIdentityCheckpointKey ||
		receipt.CollectionFamily != domain.StructureIdentityCollectionFamily ||
		!reflect.DeepEqual(receipt.Members, collection.Members) ||
		!reflect.DeepEqual(receipt.CommittedOwnerVersionRefs, collection.Members) ||
		!reflect.DeepEqual(receipt.CoveredScopeKeys, scopes) || len(receipt.ReceiptContentHash) != 64 {
		t.Fatalf("receipt=%#v", receipt)
	}
}

func TestStructureIdentityReceiptRejectsNonCanonicalSceneScopes(t *testing.T) {
	version := domain.StructureIdentitySetVersion{
		SchemaVersion: domain.StructureIdentitySetSchemaVersion,
		ID:            uuid.NewString(), WorkspaceID: uuid.NewString(), ProjectID: uuid.NewString(),
		Version: 1, ContentHash: strings.Repeat("b", 64), CreatedAt: time.Now().UTC(),
	}
	collection, err := domain.BuildStructureIdentityCollection(version)
	if err != nil {
		t.Fatal(err)
	}
	_, err = domain.NewStructureIdentityCollectionReceipt(
		uuid.NewString(), uuid.NewString(), "gate-1-structure", uuid.NewString(), collection,
		[]string{"scene:b", "scene:a"}, time.Now().UTC(), uuid.NewString(),
	)
	if err == nil {
		t.Fatal("non-canonical covered scope keys were accepted")
	}
}
