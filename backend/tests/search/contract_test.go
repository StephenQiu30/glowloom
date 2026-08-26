package search_test

import (
	"strings"
	"testing"

	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

const (
	searchWorkspaceID = "61000000-0000-0000-0000-000000000001"
	searchProjectID   = "61000000-0000-0000-0000-000000000002"
	searchVersionID   = "61000000-0000-0000-0000-000000000003"
	searchEventID     = "61000000-0000-0000-0000-000000000004"
	searchHash        = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestSearchSnapshotRequiresTenantOwnerVersionAndTraceability(t *testing.T) {
	snapshot := search.Snapshot{
		Kind: search.KindStoryGraph, WorkspaceID: searchWorkspaceID, ProjectID: searchProjectID,
		VersionID: searchVersionID, Revision: 2, ContentHash: searchHash,
		Documents: []search.Document{{
			ID:   "storygraph:" + searchProjectID + ":sgn_" + strings.Repeat("b", 64),
			Kind: search.KindStoryGraph, WorkspaceID: searchWorkspaceID, ProjectID: searchProjectID,
			OwnerKind: "production/planning", OwnerLogicalID: "scene-1", OwnerVersionID: searchVersionID,
			OwnerRevision: 2, OwnerContentHash: searchHash, ProjectionVersionID: searchVersionID,
			StoryNodeKey: "sgn_" + strings.Repeat("b", 64), NodeType: "scene", Label: "雨夜追逐",
			SearchText: "雨夜追逐 主角在码头发现线索", Evidence: []search.Evidence{{
				DocumentRevisionID: searchVersionID, Start: 12, End: 26, TextHash: searchHash,
			}},
		}},
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}

	invalid := snapshot
	invalid.Documents = append([]search.Document(nil), snapshot.Documents...)
	invalid.Documents[0].WorkspaceID = "62000000-0000-0000-0000-000000000001"
	if err := invalid.Validate(); err == nil {
		t.Fatal("cross-workspace search document was accepted")
	}

	invalid = snapshot
	invalid.Documents = append([]search.Document(nil), snapshot.Documents...)
	invalid.Documents[0].StoryNodeKey = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("StoryGraph document without a deep-linkable node key was accepted")
	}
}

func TestSearchProjectionSourceIsExplicit(t *testing.T) {
	source := search.ProjectionSource{Kind: search.SourceEvent, ID: searchEventID}
	if err := source.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (search.ProjectionSource{Kind: search.SourceReindex, ID: searchEventID}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (search.ProjectionSource{Kind: "implicit", ID: searchEventID}).Validate(); err == nil {
		t.Fatal("implicit projection source was accepted")
	}
}
