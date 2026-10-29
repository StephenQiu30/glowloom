package canvas_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestDirectorMotionSourceOffsetRoundTrip(t *testing.T) {
	node := directorWorkspaceNode(t)
	object := &node.Config.Director.Workspaces.Dynamic.Objects[0]
	object.MotionClips = []domain.DirectorMotionClip{{ID: uuid.New(), Name: "走路", SourceAnimation: "walk", Start: 1.5, Duration: 2, PlaybackRate: 2, SourceOffset: 1, Loop: true}}
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
	got := restored.Workspaces.Dynamic.Objects[0].MotionClips[0]
	if got.SourceOffset != 1 || got.Start != 1.5 || got.PlaybackRate != 2 || !got.Loop {
		t.Fatal("source offset or timing changed")
	}
	for _, offset := range []float64{-1, math.NaN(), 3601} {
		object.MotionClips[0].SourceOffset = offset
		if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err == nil {
			t.Fatal("invalid source offset accepted")
		}
	}
	object.MotionClips[0].SourceOffset = 0
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}}); err != nil {
		t.Fatalf("legacy zero offset rejected: %v", err)
	}
}
