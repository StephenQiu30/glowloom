package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/google/uuid"
)

func boundSubjectFixture(t *testing.T) domain.Document {
	t.Helper()
	source := resourceNode("image")
	target := subjectNode(t, `{"subject":{"version":1,"local":{"description":"来源绑定","primary":{"asset_id":"`+source.RefID.String()+`","asset_revision":1,"kind":"image","source_node_id":"`+source.ID.String()+`"},"references":[]}}}`)
	return domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "主体来源", Scope: json.RawMessage(`{}`), Revision: 1, Viewport: domain.Viewport{Zoom: 1}, Nodes: []domain.Node{source, target}, Edges: []domain.Edge{{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: source.ID, TargetNodeID: target.ID}}}
}
func TestSubjectInputRelationRetainsSourceAndAsset(t *testing.T) {
	doc := boundSubjectFixture(t)
	if err := domain.ValidateGraph(doc); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*domain.Document){
		func(d *domain.Document) { d.Edges = nil },
		func(d *domain.Document) { d.Nodes = d.Nodes[1:] },
		func(d *domain.Document) { d.Nodes[0].RefID = ptr(uuid.New()) },
		func(d *domain.Document) { d.Nodes[0].NodeType = "video" },
		func(d *domain.Document) {
			d.Nodes[0].Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Files: []domain.EditorFileReference{{ReferenceKey: "content", FileID: *d.Nodes[0].RefID, Version: "2", Purpose: "CONTENT"}}}
		},
	} {
		candidate := boundSubjectFixture(t)
		change(&candidate)
		if domain.ValidateGraph(candidate) == nil {
			t.Fatal("stale or unbound subject source accepted")
		}
	}
}
func TestSubjectInputRelationRemovalAndCopy(t *testing.T) {
	doc := boundSubjectFixture(t)
	result, err := domain.Apply(doc, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{doc.Nodes[0].ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 || len(result.Edges) != 0 {
		t.Fatal("source deletion incomplete")
	}
	body, _ := json.Marshal(result.Nodes[0].Config.Subject.Local.Primary)
	var media map[string]any
	_ = json.Unmarshal(body, &media)
	if _, ok := media["source_node_id"]; ok || media["asset_id"] != doc.Nodes[0].RefID.String() {
		t.Fatal("deletion lost pinned asset or retained source", string(body))
	}
	copied, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), map[uuid.UUID]uuid.UUID{*doc.Nodes[0].RefID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(copied.Document.Nodes[1].Config.Subject.Local.Primary)
	_ = json.Unmarshal(encoded, &media)
	if media["source_node_id"] != copied.Document.Nodes[0].ID.String() || media["asset_id"] != copied.Document.Nodes[0].RefID.String() {
		t.Fatal("copy did not remap source and asset", string(encoded))
	}
	if domain.ValidateGraph(doc) != nil {
		t.Fatal("original graph mutated")
	}
}

func TestSubjectInputRelationDetachIsAtomic(t *testing.T) {
	doc := boundSubjectFixture(t)
	if _, err := domain.Apply(doc, []domain.Command{{Type: "Disconnect", IDs: []uuid.UUID{doc.Edges[0].ID}}}); err == nil {
		t.Fatal("bare disconnect left a bound source")
	}
	target := doc.Nodes[1]
	config := *target.Config.Subject
	local := *config.Local
	primary := *local.Primary
	primary.SourceNodeID = nil
	local.Primary = &primary
	config.Local = &local
	next := target.Config
	next.Subject = &config
	if _, err := domain.Apply(doc, []domain.Command{{Type: "UpdateNodeConfig", ID: target.ID, Config: &next}}); err == nil {
		t.Fatal("detached assignment left a bare domain edge")
	}
	result, err := domain.Apply(doc, []domain.Command{{Type: "UpdateNodeConfig", ID: target.ID, Config: &next}, {Type: "Disconnect", IDs: []uuid.UUID{doc.Edges[0].ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edges) != 0 || result.Nodes[1].Config.Subject.Local.Primary.AssetID != primary.AssetID {
		t.Fatal("detach lost pin")
	}
}

func TestSubjectConnectedDurationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		kind     string
		duration *int32
		valid    bool
	}{
		{"image", nil, true}, {"audio", ptr(int32(2000)), true}, {"audio", ptr(int32(5000)), true}, {"audio", ptr(int32(1999)), false}, {"audio", ptr(int32(5001)), false}, {"audio", nil, false},
		{"video", ptr(int32(15499)), true}, {"video", ptr(int32(15500)), false}, {"video", ptr(int32(0)), false}, {"video", nil, false},
	} {
		if got := domain.ValidateSubjectSourceDuration(tc.kind, tc.duration) == nil; got != tc.valid {
			t.Errorf("%s duration %v: valid=%v want=%v", tc.kind, tc.duration, got, tc.valid)
		}
	}
}
