package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestSubjectLocalClosedSources(t *testing.T) {
	asset := uuid.NewString()
	valid := `{"subject":{"version":1,"local":{"description":"局部说明","primary":{"asset_id":"` + asset + `","asset_revision":1,"kind":"image"},"references":[]}}}`
	node := subjectNode(t, valid)
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err != nil {
		t.Fatal("local source", err)
	}
	for _, body := range []string{
		`{"subject":{"version":1,"local":{"description":"","primary":{"asset_id":"` + asset + `","asset_revision":0,"kind":"image"},"references":[]}}}`,
		`{"subject":{"version":1,"local":{"description":"","primary":{"asset_id":"` + asset + `","asset_revision":1,"kind":"audio"},"references":[]}}}`,
		`{"subject":{"version":1,"local":{"description":"","primary":{"asset_id":"` + asset + `","asset_revision":1,"kind":"image","url":"https://example.com"},"references":[]}}}`,
		`{"subject":{"version":1,"local":{"description":"","primary":{"asset_id":"` + asset + `","asset_revision":1,"kind":"image","binding_id":"` + uuid.NewString() + `"},"references":[]}}}`,
	} {
		if err := domain.ValidateGraph(domain.Document{Nodes: []domain.Node{subjectNode(t, body)}}); err == nil {
			t.Fatalf("invalid local accepted %s", body)
		}
	}
	raw, _ := json.Marshal(node.Config)
	if string(raw) != valid {
		t.Fatal("local metadata changed", string(raw))
	}
}

func TestSubjectLocalCopyRemapsOnlyProjectMedia(t *testing.T) {
	primary, aux, mapped, sid, vid, binding := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	node := domain.Node{ID: uuid.New(), Title: "局部主体", NodeType: "subject", NodeAction: "resource", InputRevision: 7, Config: domain.NodeConfig{Subject: &domain.SubjectConfig{Version: 1, SubjectID: &sid, VersionID: &vid, Local: &domain.SubjectLocal{Description: "局部", Primary: &domain.SubjectMedia{AssetID: primary, AssetRevision: 9, Kind: "image"}, References: []domain.SubjectMedia{{AssetID: aux, AssetRevision: 3, Kind: "image", BindingID: &binding}}}}}}
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "主体副本", Scope: json.RawMessage(`{}`), Revision: 3, Viewport: domain.Viewport{Zoom: 1}, Nodes: []domain.Node{node}, Edges: []domain.Edge{}}
	target, job := uuid.New(), uuid.New()
	if _, err := domain.CopyProjectDocument(doc, target, job, nil); err == nil {
		t.Fatal("missing project mapping accepted")
	}
	result, err := domain.CopyProjectDocument(doc, target, job, map[uuid.UUID]uuid.UUID{primary: mapped})
	if err != nil {
		t.Fatal(err)
	}
	got := result.Document.Nodes[0]
	if got.ID == node.ID || got.InputRevision != 1 || got.Config.Subject.Local.Primary.AssetID != mapped || got.Config.Subject.Local.Primary.AssetRevision != 1 || *got.Config.Subject.SubjectID != sid || *got.Config.Subject.VersionID != vid || got.Config.Subject.Local.References[0].AssetID != aux || got.Config.Subject.Local.References[0].AssetRevision != 3 || *got.Config.Subject.Local.References[0].BindingID != binding {
		t.Fatal("copy identity mapping", got)
	}
	if node.Config.Subject.Local.Primary.AssetID != primary || node.InputRevision != 7 {
		t.Fatal("copy mutated source")
	}
}
