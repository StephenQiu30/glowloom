package script_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
	scriptdomain "github.com/StephenQiu30/lanverse/backend/internal/production/script/domain"
)

func TestBuildSourceCollectionRefUsesTheSharedOwnerContract(t *testing.T) {
	t.Parallel()

	workspaceID := "11111111-1111-4111-8111-111111111111"
	projectID := "22222222-2222-4222-8222-222222222222"
	documentID := "33333333-3333-4333-8333-333333333333"
	revisionID := "44444444-4444-4444-8444-444444444444"
	indexID := "55555555-5555-4555-8555-555555555555"
	source := scriptdomain.SourceVersionIdentity{
		OwnerKind: "production/script", LogicalID: documentID, VersionID: revisionID, Revision: 3,
		ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CreatedAt:   time.Date(2026, time.September, 11, 9, 0, 0, 0, time.UTC),
	}
	index := scriptdomain.SourceSpanIndex{
		ID: indexID, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: revisionID,
		SourceHash: source.ContentHash, ContentHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}

	got, err := scriptdomain.BuildSourceCollectionRef(workspaceID, projectID, 7, source, index)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ownercollection.Build(ownercollection.Scope{
		WorkspaceID: workspaceID, ProjectID: projectID,
		OwnerKind: "production/script", VersionFamily: "script_source_set",
		ScopeKind: "project", ScopeKey: "project:" + projectID, ScopeRevision: 7,
	}, []ownercollection.VersionRef{
		{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "production/script", VersionFamily: "script_source_set",
			OwnerLogicalID: documentID, OwnerVersionID: revisionID, OwnerRevision: 3,
			OwnerContentHash: source.ContentHash,
		},
		{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "production/script", VersionFamily: "script_source_set",
			OwnerLogicalID: documentID + ":span-index", OwnerVersionID: indexID, OwnerRevision: 3,
			OwnerContentHash: index.ContentHash,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Script Source Collection does not use the shared contract:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestBuildSourceCollectionRefRejectsAnUnrelatedSpanIndex(t *testing.T) {
	t.Parallel()

	source := scriptdomain.SourceVersionIdentity{
		OwnerKind: "production/script", LogicalID: "33333333-3333-4333-8333-333333333333",
		VersionID: "44444444-4444-4444-8444-444444444444", Revision: 1,
		ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CreatedAt:   time.Date(2026, time.September, 11, 9, 0, 0, 0, time.UTC),
	}
	index := scriptdomain.SourceSpanIndex{
		ID:          "55555555-5555-4555-8555-555555555555",
		WorkspaceID: "11111111-1111-4111-8111-111111111111", ProjectID: "22222222-2222-4222-8222-222222222222",
		DocumentRevisionID: "66666666-6666-4666-8666-666666666666",
		SourceHash:         source.ContentHash,
		ContentHash:        "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if _, err := scriptdomain.BuildSourceCollectionRef(index.WorkspaceID, index.ProjectID, 1, source, index); err == nil {
		t.Fatal("an unrelated SourceSpanIndexVersion was accepted")
	}
}
