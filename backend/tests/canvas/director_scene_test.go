package canvas_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestDirectorSceneControlsPersistAndRemainClosed(t *testing.T) {
	n := directorNode()
	n.Config.Director.Mode = "dynamic"
	n.Config.Director.Objects[0].Locked = ptr(true)
	n.Config.Director.Cameras[0].Locked = ptr(true)
	n.Config.Director.Ground = &domain.DirectorGround{Visible: true, Opacity: 0.8, Color: "#242424"}
	n.Config.Director.Shots[0].FPS = 60
	saved, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}})
	if err != nil || saved.Nodes[0].Config.Director.Mode != "dynamic" || !*saved.Nodes[0].Config.Director.Objects[0].Locked {
		t.Fatalf("scene controls did not persist: %v", err)
	}
	for _, mode := range []string{"static", "dynamic", ""} {
		n.Config.Director.Mode = mode
		if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}}); err != nil {
			t.Fatalf("accepted mode %q rejected: %v", mode, err)
		}
	}
	for _, change := range []func(*domain.DirectorConfig){
		func(c *domain.DirectorConfig) { c.Mode = "script" },
		func(c *domain.DirectorConfig) { c.Ground.Color = "url(https://external.invalid)" },
		func(c *domain.DirectorConfig) { c.Shots[0].FPS = 120 },
	} {
		n := directorNode()
		n.Config.Director.Ground = &domain.DirectorGround{Visible: true, Opacity: 1, Color: "#242424"}
		change(n.Config.Director)
		if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}}); !errors.Is(err, domain.ErrInvalidCommand) {
			t.Fatalf("invalid controls accepted: %v", err)
		}
	}
}
