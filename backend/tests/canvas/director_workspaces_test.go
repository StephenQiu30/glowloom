package canvas_test

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func directorWorkspaceNode(t *testing.T) domain.Node {
	t.Helper()
	node := directorNode()
	raw, err := json.Marshal(node.Config.Director)
	if err != nil {
		t.Fatal(err)
	}
	var scene map[string]any
	if err := json.Unmarshal(raw, &scene); err != nil {
		t.Fatal(err)
	}
	id, title := scene["id"], scene["title"]
	for _, key := range []string{"id", "title", "version", "mode"} {
		delete(scene, key)
	}
	encoded, err := json.Marshal(map[string]any{"id": id, "title": title, "version": 2, "mode": "static", "workspaces": map[string]any{"static": scene, "dynamic": scene}})
	if err != nil {
		t.Fatal(err)
	}
	node.Config.Director = &domain.DirectorConfig{}
	if err := json.Unmarshal(encoded, node.Config.Director); err != nil {
		t.Fatal(err)
	}
	return node
}

func TestDirectorIndependentWorkspacesRoundTrip(t *testing.T) {
	node := directorWorkspaceNode(t)
	saved, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}})
	if err != nil {
		t.Fatalf("two workspace document rejected: %v", err)
	}
	raw, err := json.Marshal(saved.Nodes[0].Config.Director)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["workspaces"] == nil {
		t.Fatal("workspaces discarded during roundtrip")
	}
}

func TestDirectorRejectsInvalidInactiveWorkspace(t *testing.T) {
	for name, change := range map[string]func(*domain.DirectorConfig){
		"missing workspace": func(c *domain.DirectorConfig) { c.Workspaces.Dynamic = nil },
		"mixed legacy":      func(c *domain.DirectorConfig) { c.Objects = []domain.DirectorObject{} },
		"inactive camera":   func(c *domain.DirectorConfig) { c.Workspaces.Dynamic.Shots[0].CameraID = uuid.New() },
		"inactive asset":    func(c *domain.DirectorConfig) { c.Workspaces.Dynamic.Objects[0].SourceNodeID = ptr(uuid.New()) },
	} {
		t.Run(name, func(t *testing.T) {
			node := directorWorkspaceNode(t)
			change(node.Config.Director)
			if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
				t.Fatal("invalid inactive workspace accepted")
			}
		})
	}
}

func TestDirectorWorkspaceReferencesSurviveDeleteAndCopy(t *testing.T) {
	node, image := directorWorkspaceNode(t), resourceNode("image")
	for _, scene := range []*domain.DirectorSceneData{node.Config.Director.Workspaces.Static, node.Config.Director.Workspaces.Dynamic} {
		scene.Objects[0].Kind = "billboard"
		scene.Objects[0].BuiltinActor = ""
		scene.Objects[0].SourceNodeID = &image.ID
		scene.Objects[0].AssetID = image.RefID
	}
	source, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{image, node}}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := domain.Apply(source, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{image.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, scene := range domain.DirectorScenes(*updated.Nodes[0].Config.Director) {
		if scene.Objects[0].SourceNodeID != nil || scene.Objects[0].AssetID == nil || *scene.Objects[0].AssetID != *image.RefID {
			t.Fatal("inactive image reference lost")
		}
	}
	if source.Nodes[1].Config.Director.Workspaces.Dynamic.Objects[0].SourceNodeID == nil {
		t.Fatal("source mutated")
	}
	source.ID, source.ProjectID, source.Name, source.Revision, source.Scope = uuid.New(), uuid.New(), "导演台工程", 1, json.RawMessage(`{}`)
	source.Viewport = domain.Viewport{Zoom: 1}
	targetAsset := uuid.New()
	copied, err := domain.CopyProjectDocument(source, uuid.New(), uuid.New(), map[uuid.UUID]uuid.UUID{*image.RefID: targetAsset})
	if err != nil {
		t.Fatal(err)
	}
	var config *domain.DirectorConfig
	for _, candidate := range copied.Document.Nodes {
		if candidate.Config.Director != nil {
			config = candidate.Config.Director
		}
	}
	if config == nil || config.Workspaces == nil {
		t.Fatal("copied workspaces missing")
	}
	a, b := config.Workspaces.Static, config.Workspaces.Dynamic
	if a.Objects[0].ID != b.Objects[0].ID || a.Objects[0].ID == node.Config.Director.Workspaces.Static.Objects[0].ID {
		t.Fatal("cross-workspace identity mapping inconsistent")
	}
	if *b.Objects[0].AssetID != targetAsset || *b.Objects[0].SourceNodeID == image.ID {
		t.Fatal("inactive references not remapped")
	}
}

func TestDirectorWorkspacesPostgresRoundTrip(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	writer := canvasfixture.NewEditService(canvasStore(runtime))
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "双工作区导演台"})
	if err != nil {
		t.Fatal(err)
	}
	node := directorWorkspaceNode(t)
	node.Config.Director.Workspaces.Dynamic.Cameras[0].Keyframes = []domain.DirectorKeyframe{{ID: uuid.New(), Time: 1, Transform: node.Config.Director.Workspaces.Dynamic.Cameras[0].Transform, PositionKeyed: ptr(false), RotationKeyed: ptr(false), ScaleKeyed: ptr(false), FOV: ptr(45.0), Target: []float64{2, 1, 0}, Roll: ptr(.5)}}
	node.Config.Director.Workspaces.Static.Objects[0].FrozenBones = map[string]domain.DirectorTransform{"pelvis": {Position: []float64{0, 1, 0}, Rotation: []float64{0, .2, 0}, Scale: []float64{1, 1, 1}}}
	node.Config.Director.Workspaces.Static.Objects[0].FrozenNodes = map[string]domain.DirectorFrozenNode{"root": {Transform: domain.DirectorTransform{Position: []float64{5, 0, 0}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}}}
	if _, err := writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node})}); err != nil {
		t.Fatal(err)
	}
	reopened, err := application.NewService(canvasStore(runtime)).Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Nodes[0].Config.Director
	if got.Version != 2 || got.Workspaces.Static.Objects[0].FrozenNodes["root"].Transform.Position[0] != 5 || got.Workspaces.Static.Objects[0].FrozenBones["pelvis"].Rotation[1] != .2 || *got.Workspaces.Dynamic.Cameras[0].Keyframes[0].FOV != 45 || *got.Workspaces.Dynamic.Cameras[0].Keyframes[0].PositionKeyed {
		t.Fatal("workspace channels changed after persisted reload")
	}
}

func TestDirectorFrozenNodesRoundTrip(t *testing.T) {
	node := directorWorkspaceNode(t)
	object := &node.Config.Director.Workspaces.Static.Objects[0]
	object.FrozenNodes = map[string]domain.DirectorFrozenNode{
		"root":   {Transform: domain.DirectorTransform{Position: []float64{5, 0, 0}, Rotation: []float64{0, 0, 0}, Scale: []float64{-1, 1, 1}}},
		"root/0": {Transform: domain.DirectorTransform{Position: []float64{2, 0, 0}, Rotation: []float64{0, .2, 0}, Scale: []float64{1, 1, 1}}, MorphWeights: []float64{.25, .75}},
	}
	object.FrozenBones = map[string]domain.DirectorTransform{"Hip": object.FrozenNodes["root/0"].Transform}
	saved, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(saved.Nodes[0].Config.Director)
	if err != nil {
		t.Fatal(err)
	}
	var restored domain.DirectorConfig
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	got := restored.Workspaces.Static.Objects[0]
	if !reflect.DeepEqual(got.FrozenNodes, object.FrozenNodes) || !reflect.DeepEqual(got.FrozenBones, object.FrozenBones) {
		t.Fatal("node or compatible bone snapshots changed during roundtrip")
	}
}

func TestDirectorRejectsInvalidFrozenNodesInInactiveWorkspace(t *testing.T) {
	valid := domain.DirectorFrozenNode{Transform: domain.DirectorTransform{Position: []float64{5, 0, 0}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}}
	for name, nodes := range map[string]map[string]domain.DirectorFrozenNode{
		"leading zero path":       {"root/01": valid},
		"parent path":             {"root/../1": valid},
		"missing transform":       {"root": {}},
		"nonfinite":               {"root": {Transform: domain.DirectorTransform{Position: []float64{math.NaN(), 0, 0}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}}},
		"oversized morph targets": {"root": {Transform: valid.Transform, MorphWeights: make([]float64, 257)}},
	} {
		t.Run(name, func(t *testing.T) {
			node := directorWorkspaceNode(t)
			node.Config.Director.Workspaces.Dynamic.Objects[0].FrozenNodes = nodes
			if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
				t.Fatal("invalid inactive node snapshot accepted")
			}
		})
	}
}
