package platform_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/ownercollection"
)

func TestOwnerCollectionBuildProducesExactCanonicalContract(t *testing.T) {
	t.Parallel()

	workspaceID := "11111111-1111-4111-8111-111111111111"
	projectID := "22222222-2222-4222-8222-222222222222"
	members := []ownercollection.VersionRef{
		{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "production/planning", VersionFamily: "planning_scene_set",
			OwnerLogicalID: "scene:b", OwnerVersionID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			OwnerRevision: 2, OwnerContentHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		{
			WorkspaceID: workspaceID, ProjectID: projectID,
			OwnerKind: "production/planning", VersionFamily: "planning_scene_set",
			OwnerLogicalID: "scene:a", OwnerVersionID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			OwnerRevision: 1, OwnerContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}

	got, err := ownercollection.Build(ownercollection.Scope{
		WorkspaceID: workspaceID, ProjectID: projectID,
		OwnerKind: "production/planning", VersionFamily: "planning_scene_set",
		ScopeKind: "episode", ScopeKey: "episode:33333333-3333-4333-8333-333333333333",
		ScopeRevision: 7,
	}, members)
	if err != nil {
		t.Fatalf("build Owner Collection: %v", err)
	}
	if got.Members[0].OwnerLogicalID != "scene:a" || got.MemberCount != 2 {
		t.Fatalf("members were not canonically ordered: %#v", got.Members)
	}
	if got.ScopeContentHash != "146266b861640f9820752e454cad6ce4608242e982f87efb02caea2bae54b397" {
		t.Fatalf("unexpected scope content hash: %s", got.ScopeContentHash)
	}
	if got.MembersHash != "9f6e3072e3325bf91d04e260fc1e0b9191e3f4499439e3f02622951403029a02" {
		t.Fatalf("unexpected members hash: %s", got.MembersHash)
	}
	if got.CollectionRootHash != "6a92ae581e6dae35b05b636022c7abab9a31842f5ace05a02962084d62a4181c" {
		t.Fatalf("unexpected collection root hash: %s", got.CollectionRootHash)
	}

	encoded, err := json.Marshal(got.Members[0])
	if err != nil {
		t.Fatalf("encode member: %v", err)
	}
	assertExactJSONKeys(t, encoded, []string{
		"workspace_id", "project_id", "owner_kind", "version_family",
		"owner_logical_id", "owner_version_id", "owner_revision", "owner_content_hash",
	})
	encoded, err = json.Marshal(got)
	if err != nil {
		t.Fatalf("encode collection: %v", err)
	}
	assertExactJSONKeys(t, encoded, []string{
		"workspace_id", "project_id", "owner_kind", "version_family", "scope_kind", "scope_key",
		"scope_revision", "scope_content_hash", "members", "member_count", "members_hash", "collection_root_hash",
	})
}

func assertExactJSONKeys(t *testing.T, encoded []byte, keys []string) {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	if len(decoded) != len(keys) {
		t.Fatalf("wire field count drifted: %s", encoded)
	}
	for _, key := range keys {
		if _, exists := decoded[key]; !exists {
			t.Fatalf("wire field %q is missing: %s", key, encoded)
		}
	}
}

func TestOwnerCollectionBuildRejectsNonCanonicalOrCrossScopeMembers(t *testing.T) {
	t.Parallel()

	base := ownercollection.VersionRef{
		WorkspaceID: "11111111-1111-4111-8111-111111111111",
		ProjectID:   "22222222-2222-4222-8222-222222222222",
		OwnerKind:   "production/bible", VersionFamily: "bible_structure_identity_set",
		OwnerLogicalID: "identity-set", OwnerVersionID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		OwnerRevision: 1, OwnerContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	scope := ownercollection.Scope{
		WorkspaceID: base.WorkspaceID, ProjectID: base.ProjectID,
		OwnerKind: base.OwnerKind, VersionFamily: base.VersionFamily,
		ScopeKind: "project", ScopeKey: "project:" + base.ProjectID, ScopeRevision: 1,
	}

	for name, mutate := range map[string]func(*ownercollection.VersionRef){
		"cross project": func(value *ownercollection.VersionRef) { value.ProjectID = "33333333-3333-4333-8333-333333333333" },
		"wrong family":  func(value *ownercollection.VersionRef) { value.VersionFamily = "bible_production_world_set" },
		"non NFC":       func(value *ownercollection.VersionRef) { value.OwnerLogicalID = "e\u0301" },
		"unsafe revision": func(value *ownercollection.VersionRef) {
			value.OwnerRevision = 9007199254740992
		},
	} {
		t.Run(name, func(t *testing.T) {
			member := base
			mutate(&member)
			if _, err := ownercollection.Build(scope, []ownercollection.VersionRef{member}); err == nil {
				t.Fatal("expected invalid Owner Collection member to fail")
			}
		})
	}
	if _, err := ownercollection.Build(scope, []ownercollection.VersionRef{base, base}); err == nil {
		t.Fatal("expected duplicate Owner Collection members to fail")
	}
}

func TestOwnerCollectionBuildKeepsAnEmptyCollectionAsAnArray(t *testing.T) {
	t.Parallel()

	got, err := ownercollection.Build(ownercollection.Scope{
		WorkspaceID: "11111111-1111-4111-8111-111111111111",
		ProjectID:   "22222222-2222-4222-8222-222222222222",
		OwnerKind:   "production/planning", VersionFamily: "planning_structure_rebase_set",
		ScopeKind: "project", ScopeKey: "project:22222222-2222-4222-8222-222222222222", ScopeRevision: 1,
	}, []ownercollection.VersionRef{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got.Members)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[]" || got.MemberCount != 0 {
		t.Fatalf("empty Owner Collection is not an explicit array: %s", encoded)
	}
}
