package canvas_test

import (
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func directorNode() domain.Node {
	camera, shot := uuid.New(), uuid.New()
	transform := domain.DirectorTransform{Position: []float64{0, 0, 0}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}
	return domain.Node{ID: uuid.New(), Title: "导演台", NodeType: "director", NodeAction: "tool", Config: domain.NodeConfig{Director: &domain.DirectorConfig{
		ID: uuid.New(), Version: 1, Title: "场景", Background: "#ffffff", EnvironmentIntensity: 1, ActiveShotID: shot,
		Objects: []domain.DirectorObject{{ID: uuid.New(), Name: "演员", Kind: "actor", BuiltinActor: "mannequin", Transform: transform, Color: "#aaaaaa", Pose: "stand", Visible: true, Keyframes: []domain.DirectorKeyframe{}}},
		Cameras: []domain.DirectorCamera{{ID: camera, Name: "摄影机", Transform: transform, Target: []float64{0, 1, 0}, FocalLength: 35, FOV: 54, Aperture: 2.8, FocusDistance: 5, Near: 0.1, Far: 100, Keyframes: []domain.DirectorKeyframe{}}},
		Lights:  []domain.DirectorLight{}, Shots: []domain.DirectorShot{{ID: shot, Name: "镜头", CameraID: camera, Duration: 5, FPS: 24, ShotSize: "medium", CameraMove: "static"}},
	}}}
}

func TestDirectorRetainsSceneIdentityAndRejectsBrokenGeometry(t *testing.T) {
	node := directorNode()
	doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}})
	if err != nil || len(doc.Nodes) != 1 || doc.Nodes[0].Config.Director.Cameras[0].ID != node.Config.Director.Cameras[0].ID {
		t.Fatalf("valid scene did not persist: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*domain.DirectorConfig)
	}{
		{"missing camera", func(c *domain.DirectorConfig) { c.Shots[0].CameraID = uuid.New() }},
		{"missing objects array", func(c *domain.DirectorConfig) { c.Objects = nil }},
		{"missing lights array", func(c *domain.DirectorConfig) { c.Lights = nil }},
		{"missing camera frames", func(c *domain.DirectorConfig) { c.Cameras[0].Keyframes = nil }},
		{"missing object frames", func(c *domain.DirectorConfig) { c.Objects[0].Keyframes = nil }},
		{"missing active shot", func(c *domain.DirectorConfig) { c.ActiveShotID = uuid.New() }},
		{"duplicate objects", func(c *domain.DirectorConfig) { c.Objects = append(c.Objects, c.Objects[0]) }},
		{"invalid clip range", func(c *domain.DirectorConfig) { c.Cameras[0].Near = c.Cameras[0].Far }},
		{"short vector", func(c *domain.DirectorConfig) { c.Objects[0].Transform.Position = []float64{0, 0} }},
		{"nonfinite vector", func(c *domain.DirectorConfig) { c.Objects[0].Transform.Position[0] = math.NaN() }},
		{"negative scale", func(c *domain.DirectorConfig) { c.Objects[0].Transform.Scale[0] = -1 }},
		{"missing model", func(c *domain.DirectorConfig) { c.Objects[0].Kind = "model"; c.Objects[0].BuiltinActor = "" }},
		{"foreign follow", func(c *domain.DirectorConfig) { c.Cameras[0].FollowObjectID = ptr(uuid.New()) }},
		{"foreign source", func(c *domain.DirectorConfig) { c.Objects[0].SourceNodeID = ptr(uuid.New()) }},
		{"invalid quaternion", func(c *domain.DirectorConfig) {
			c.Objects[0].BoneOverrides = map[string][]float64{"head": {0, 0, 0, 0}}
		}},
		{"unknown bone", func(c *domain.DirectorConfig) {
			c.Objects[0].BoneOverrides = map[string][]float64{"unknown": {0, 0, 0, 1}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := directorNode()
			tc.change(n.Config.Director)
			_, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}})
			if !errors.Is(err, domain.ErrInvalidCommand) {
				t.Fatalf("invalid scene accepted: %v", err)
			}
		})
	}
}

func TestDirectorDeletingImageNodePreservesAssetAndDoesNotMutateInput(t *testing.T) {
	image, director := resourceNode("image"), directorNode()
	director.Config.Director.Objects[0].Kind = "billboard"
	director.Config.Director.Objects[0].BuiltinActor = ""
	director.Config.Director.Objects[0].SourceNodeID = &image.ID
	director.Config.Director.Objects[0].AssetID = image.RefID
	doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{image, director}}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := domain.Apply(doc, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{image.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	object := updated.Nodes[0].Config.Director.Objects[0]
	if object.SourceNodeID != nil || object.AssetID == nil || *object.AssetID != *image.RefID || doc.Nodes[1].Config.Director.Objects[0].SourceNodeID == nil {
		t.Fatal("delete lost asset identity or mutated input")
	}
}
