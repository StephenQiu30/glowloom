package canvas_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestDirectorBodyProfileRoundTrip(t *testing.T) {
	node := directorWorkspaceNode(t)
	object := &node.Config.Director.Workspaces.Static.Objects[0]
	object.Kind, object.Primitive, object.BuiltinActor = "actor", "", "male_child"
	object.AssetID = nil
	object.BodyProfile = map[string]float64{"leg": .55, "headWidth": 1.27}
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
	if got.BodyProfile["leg"] != .55 || got.BodyProfile["headWidth"] != 1.27 || got.BuiltinActor != "male_child" {
		t.Fatal("body overrides or preset lost")
	}
}

func TestDirectorRejectsInvalidBodyProfile(t *testing.T) {
	for name, profile := range map[string]map[string]float64{
		"zero": {"leg": 0}, "large": {"leg": 1.61}, "nonfinite": {"leg": math.NaN()}, "unknown": {"unknownArm": 1},
	} {
		t.Run(name, func(t *testing.T) {
			node := directorWorkspaceNode(t)
			object := &node.Config.Director.Workspaces.Dynamic.Objects[0]
			object.Kind, object.Primitive, object.BuiltinActor = "actor", "", "female_standard"
			object.AssetID = nil
			object.BodyProfile = profile
			if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
				t.Fatal("invalid body profile accepted")
			}
		})
	}
	node := directorWorkspaceNode(t)
	node.Config.Director.Workspaces.Static.Objects[0].BodyProfile = map[string]float64{"leg": 1}
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
		t.Fatal("non-bundled body profile accepted")
	}
}
