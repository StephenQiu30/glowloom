package domain

import (
	"github.com/google/uuid"
)

// SystemAssetTemplate is an immutable system recipe, never a user's canvas.
type SystemAssetTemplate struct {
	Key           string `json:"key"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	IsTemplate    bool   `json:"is_template"`
	NodeCount     int    `json:"node_count"`
	RequiresModel bool   `json:"requires_model"`
}

// Retired template identities remain valid for replaying durable receipts.
var retiredAssetTemplates = map[string]SystemAssetTemplate{
	"text-to-image-v1":          {Key: "text-to-image-v1", Kind: "canvas", Name: "文生图工作流"},
	"storyboard-three-shots-v1": {Key: "storyboard-three-shots-v1", Kind: "short_drama", Name: "三镜头分镜"},
}

// IsRetiredSystemAssetTemplate reports whether a recipe can only replay receipts.
func IsRetiredSystemAssetTemplate(key string) bool {
	_, retired := retiredAssetTemplates[key]
	return retired
}

// SystemAssetTemplates returns fresh metadata; versioned recipe keys are immutable.
func SystemAssetTemplates(kind string) ([]SystemAssetTemplate, error) {
	if kind != "" && kind != "canvas" && kind != "short_drama" {
		return nil, ErrInvalidCommand
	}
	catalog := []SystemAssetTemplate{
		{Key: "director-stage-v1", Kind: "canvas", Name: "3D导演台", Description: "从示例白模、机位和镜头开始预演构图与动作。", IsTemplate: true, NodeCount: 1},
	}
	result := make([]SystemAssetTemplate, 0, len(catalog))
	for _, item := range catalog {
		if kind == "" || item.Kind == kind {
			result = append(result, item)
		}
	}
	return result, nil
}

// SystemAssetTemplateByKey resolves a server-owned recipe without caller content.
func SystemAssetTemplateByKey(key string) (SystemAssetTemplate, error) {
	catalog, _ := SystemAssetTemplates("")
	for _, item := range catalog {
		if item.Key == key {
			return item, nil
		}
	}
	if retired, ok := retiredAssetTemplates[key]; ok {
		return retired, nil
	}
	return SystemAssetTemplate{}, ErrInvalidCommand
}

// ApplySystemAssetTemplate seeds only a new empty document with editable intent.
// IDs belong to the new document; no template entity or media identity is reused.
func ApplySystemAssetTemplate(base Document, key string) (OperationOutcome, error) {
	template, err := SystemAssetTemplateByKey(key)
	if err != nil || IsRetiredSystemAssetTemplate(key) || base.ID == uuid.Nil || base.ProjectID == uuid.Nil || base.Revision != 1 || len(base.Nodes) != 0 || len(base.Edges) != 0 {
		return OperationOutcome{}, ErrInvalidCommand
	}
	kind, err := ParseDocumentKind(base.Kind)
	if err != nil || kind != template.Kind {
		return OperationOutcome{}, ErrInvalidCommand
	}
	node := directorStageNode(base.ID, template)
	return ApplyOperations(base, []Operation{{Type: "create_node", Node: &node}})
}

func directorStageNode(documentID uuid.UUID, template SystemAssetTemplate) Node {
	id := func(role string) uuid.UUID {
		return uuid.NewSHA1(documentID, []byte("template/"+template.Key+"/"+role))
	}
	cameraID, shotID := id("camera"), id("shot")
	width, height := 480.0, 402.0
	pose := func(x, y, z float64) DirectorTransform {
		return DirectorTransform{Position: []float64{x, y, z}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}
	}
	scene := DirectorConfig{
		ID: id("scene"), Version: 1, Title: "构图预演", Mode: "static",
		Background: "#171717", EnvironmentIntensity: 1, GridVisible: true, AspectRatio: "16:9",
		Objects: []DirectorObject{{
			ID: id("box"), Name: "白模占位", Kind: "primitive", Primitive: "box",
			Transform: pose(0, 0.5, 0), Color: "#d4d4d8", Visible: true, CastShadow: true,
			Keyframes: []DirectorKeyframe{},
		}},
		Cameras: []DirectorCamera{{
			ID: cameraID, Name: "机位", Transform: pose(5, 3, 7), Target: []float64{0, 1, 0},
			LookAtMode: "coordinates", FocalLength: 35, FOV: 50, Aperture: 2.8,
			FocusDistance: 8, Near: 0.1, Far: 1000, Keyframes: []DirectorKeyframe{},
		}},
		Lights: []DirectorLight{},
		Shots: []DirectorShot{{
			ID: shotID, Name: "构图预演", CameraID: cameraID, Duration: 15, FPS: 30,
			ShotSize: "medium", CameraMove: "static",
		}},
		ActiveShotID: shotID,
	}
	return Node{
		ID: id("node"), Title: template.Name, NodeType: NodeTypeDirector, NodeAction: NodeActionTool,
		Width: &width, Height: &height, Config: NodeConfig{Director: &scene},
	}
}
