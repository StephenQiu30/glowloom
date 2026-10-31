package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func subjectNode(t *testing.T, body string) domain.Node {
	t.Helper()
	node := domain.Node{ID: uuid.New(), Title: "主体", NodeType: "subject", NodeAction: "resource"}
	if err := json.Unmarshal([]byte(body), &node.Config); err != nil {
		t.Fatal(err)
	}
	return node
}

func TestSubjectNodeClosedConfig(t *testing.T) {
	pin := `{"subject":{"version":1,"subject_id":"` + uuid.NewString() + `","version_id":"` + uuid.NewString() + `"}}`
	for _, body := range []string{`{"subject":{"version":1}}`, pin} {
		node := subjectNode(t, body)
		doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}})
		if err != nil {
			t.Fatalf("valid subject %s: %v", body, err)
		}
		encoded, _ := json.Marshal(doc.Nodes[0].Config)
		if string(encoded) != body {
			t.Fatalf("lost pin: %s", encoded)
		}
		if domain.IsDraft(doc.Nodes[0]) {
			t.Fatal("subject became generation input")
		}
	}
	for _, body := range []string{`{}`, `{"subject":{"version":2}}`, `{"subject":{"version":1,"subject_id":null,"version_id":null}}`, `{"subject":{"version":1,"subject_id":"` + uuid.NewString() + `"}}`, `{"subject":{"version":1,"version_id":"` + uuid.NewString() + `"}}`, `{"subject":{"version":1,"subject_id":"00000000-0000-0000-0000-000000000000","version_id":"` + uuid.NewString() + `"}}`, `{"subject":{"version":1,"revision":1}}`, `{"subject":{"version":1},"text":"fake"}`} {
		node := subjectNode(t, body)
		if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
			t.Fatalf("invalid subject accepted: %s", body)
		}
	}
	node := subjectNode(t, pin)
	node.NodeType = "image"
	if err := domain.ValidateGraph(domain.Document{Nodes: []domain.Node{node}}); err == nil {
		t.Fatal("subject config accepted on image")
	}
	node.NodeType = "subject"
	node.NodeAction = "tool"
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
		t.Fatal("subject tool accepted")
	}
}

func TestSubjectNodeProjectCopyRetainsFixedVersion(t *testing.T) {
	node := subjectNode(t, `{"subject":{"version":1}}`)
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "主体复制", Scope: json.RawMessage(`{}`), Revision: 1, Viewport: domain.Viewport{Zoom: 1}, Nodes: []domain.Node{node}}
	copied, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), nil)
	if err != nil || len(copied.Document.Nodes) != 1 || copied.Document.Nodes[0].ID == node.ID || copied.Document.Nodes[0].Config.Subject == nil {
		t.Fatal("empty subject copy", copied, err)
	}
	id, version := uuid.New(), uuid.New()
	doc.Nodes[0].Config.Subject = &domain.SubjectConfig{Version: 1, SubjectID: &id, VersionID: &version}
	copied, err = domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), nil)
	if err != nil || *copied.Document.Nodes[0].Config.Subject.SubjectID != id || *copied.Document.Nodes[0].Config.Subject.VersionID != version {
		t.Fatal("fixed identity remapped", copied, err)
	}
}
