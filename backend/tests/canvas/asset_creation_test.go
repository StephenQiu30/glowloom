package canvas_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestCanvasAssetSystemTemplatesCreateEditableNonExecutingGraphs(t *testing.T) {
	catalog, err := domain.SystemAssetTemplates("")
	if err != nil || len(catalog) != 2 {
		t.Fatal(catalog, err)
	}
	for _, template := range catalog {
		base := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: template.Name, Kind: template.Kind, Revision: 1, SchemaVersion: 1, OperationVersion: 1, Scope: json.RawMessage(`{}`), Viewport: domain.Viewport{Zoom: 1}, Nodes: []domain.Node{}, Edges: []domain.Edge{}}
		result, err := domain.ApplySystemAssetTemplate(base, template.Key)
		if err != nil || len(result.Document.Nodes) != template.NodeCount || len(result.Document.Edges) != template.NodeCount/2 || !template.IsTemplate || !template.RequiresModel {
			t.Fatal(template, result, err)
		}
		for _, node := range result.Document.Nodes {
			if node.LastOperationID != nil || node.RefID != nil {
				t.Fatal("template contains execution or user media", node)
			}
			if node.NodeType == "image" && (node.Config.Editor == nil || node.Config.Editor.Parameters.Mode != "GENERATE" || node.Config.Editor.Parameters.ModelSelection != nil) {
				t.Fatal("missing editable image workflow", node)
			}
			if node.NodeType == "text" && (node.Config.Text == nil || *node.Config.Text == "") {
				t.Fatal("empty template placeholder", node)
			}
		}
		again, err := domain.ApplySystemAssetTemplate(base, template.Key)
		if err != nil || again.Document.Nodes[0].ID != result.Document.Nodes[0].ID {
			t.Fatal("same new-document seed unstable", err)
		}
		base.ID = uuid.New()
		other, err := domain.ApplySystemAssetTemplate(base, template.Key)
		if err != nil || other.Document.Nodes[0].ID == result.Document.Nodes[0].ID {
			t.Fatal("template identity reused in another object", err)
		}
	}
}

func TestCanvasAssetCreationNormalizesAndRejectsMismatchedTemplate(t *testing.T) {
	input, err := application.NormalizeAssetCreation(application.AssetCreationInput{})
	if err != nil || input.Kind != "canvas" || input.Name != "未命名画布" {
		t.Fatal(input, err)
	}
	input, err = application.NormalizeAssetCreation(application.AssetCreationInput{Kind: "short_drama", TemplateKey: "storyboard-three-shots-v1"})
	if err != nil || input.Name != "三镜头分镜" {
		t.Fatal(input, err)
	}
	for _, bad := range []application.AssetCreationInput{{Kind: "video"}, {TemplateKey: "unknown"}, {Kind: "short_drama", TemplateKey: "text-to-image-v1"}, {Name: "  "}, {Name: "名\x00称"}} {
		if _, err := application.NormalizeAssetCreation(bad); !errors.Is(err, domain.ErrInvalidCommand) {
			t.Fatal("invalid seed admitted", bad, err)
		}
	}
}
