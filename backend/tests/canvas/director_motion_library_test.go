package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestDirectorMotionLibraryRoundTripAndValidation(t *testing.T) {
	node := directorWorkspaceNode(t)
	o := &node.Config.Director.Workspaces.Dynamic.Objects[0]
	o.Kind = "actor"
	o.Primitive = ""
	o.BuiltinActor = "male_standard"
	o.AssetID = nil
	source := domain.DirectorMotionSource{ID: uuid.New(), Name: "姿势", Kind: "pose", Duration: 1, Rest: map[string]domain.DirectorTransform{"Hip": {Position: []float64{0, 0, 0}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}}, Tracks: []domain.DirectorMotionTrack{{Bone: "Hip", Property: "quaternion", Times: []float64{0, 1}, Values: []float64{0, 0, 0, 1, 0, 0, 0, 1}}}}
	o.MotionLibrary = []domain.DirectorMotionSource{source}
	a := domain.DirectorMotionClip{ID: uuid.New(), Name: "姿势1", SourceAnimation: "user:" + source.ID.String(), Start: 0, Duration: 2, PlaybackRate: 1}
	b := a
	b.ID = uuid.New()
	b.Start = 2
	b.BlendIn = .5
	o.MotionClips = []domain.DirectorMotionClip{a, b}
	saved, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(saved.Nodes[0].Config.Director)
	if err != nil {
		t.Fatal(err)
	}
	var got domain.DirectorConfig
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Workspaces.Dynamic.Objects[0].MotionLibrary[0].ID != source.ID || got.Workspaces.Dynamic.Objects[0].MotionClips[1].BlendIn != .5 {
		t.Fatal("library or blend lost")
	}
	o.MotionClips[1].Start = 3
	if _, err = domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
		t.Fatal("non-adjacent blend accepted")
	}
	o.MotionClips[1].BlendIn = 0
	o.MotionLibrary = nil
	if _, err = domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
		t.Fatal("missing user source accepted")
	}
}
func TestDirectorBuiltinModelWhitelist(t *testing.T) {
	node := directorWorkspaceNode(t)
	o := &node.Config.Director.Workspaces.Static.Objects[0]
	o.Kind = "model"
	o.Primitive = ""
	o.BuiltinActor = ""
	o.AssetID = nil
	o.BuiltinModel = "kenney-chair"
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err != nil {
		t.Fatal(err)
	}
	o.BuiltinModel = "https://invalid.example/model.glb"
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
		t.Fatal("unlisted model accepted")
	}
	o.BuiltinModel = "kenney-chair"
	id := uuid.New()
	o.AssetID = &id
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
		t.Fatal("mixed sources accepted")
	}
}
