package canvas_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	promptpg "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestCinemaKitPostgresYjsSaveEditReadAndDetach(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	store := canvasStore(runtime)
	writer := canvasfixture.NewEditService(store)
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "CinemaKit real persistence"})
	if err != nil {
		t.Fatal(err)
	}
	fixture, source, target := cinemaFixture(t)
	target.InputRevision = 0
	submit := func(ops ...domain.Operation) {
		t.Helper()
		receipt, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: ops})
		if err != nil || receipt.Status != "accepted" {
			t.Fatal(receipt, err)
		}
	}
	read := func() domain.Document {
		t.Helper()
		got, err := application.NewService(canvasStore(runtime)).Get(t.Context(), actor, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	find := func(d domain.Document, id uuid.UUID) domain.Node {
		t.Helper()
		for _, n := range d.Nodes {
			if n.ID == id {
				return n
			}
		}
		t.Fatal("node missing")
		return domain.Node{}
	}
	submit(domain.Operation{Type: "create_node", Node: &source}, domain.Operation{Type: "create_node", Node: &target}, domain.Operation{Type: "connect", Edge: &fixture.Edges[0]})
	first := read()
	firstTarget := find(first, target.ID)
	inputReader := promptpg.NewDraftStore(runtime, promptwire.DraftAccess)
	readInput := func(d domain.Document) (prompt.Draft, error) {
		n := find(d, target.ID)
		return inputReader.ReadInput(t.Context(), actor, project, prompt.InputSource{Kind: "canvas_input", CanvasID: doc.ID, CanvasRevision: d.Revision, NodeID: target.ID, InputRevision: n.InputRevision}, false)
	}
	input, err := readInput(first)
	if err != nil || input.CinemaKit == nil || input.CinemaKit.Camera.FocalLength != "35mm" {
		t.Fatal(input, err)
	}
	before, _ := json.Marshal(map[string]any{"cinema_kit": source.Config.CinemaKit})
	next := *source.Config.CinemaKit
	camera := *next.Camera
	camera.FocalLength = "85mm"
	next.Camera = &camera
	value, _ := json.Marshal(map[string]any{"cinema_kit": next})
	submit(domain.Operation{Type: "set", NodeID: source.ID, Path: []string{"config", "cinema_kit"}, ExpectedExists: ptr(true), Expected: before, Value: value})
	updated := read()
	if find(updated, source.ID).Config.CinemaKit.Camera.FocalLength != "85mm" || find(updated, target.ID).InputRevision != firstTarget.InputRevision+1 {
		t.Fatal("setting edit or downstream revision lost", updated)
	}
	effective, err := readInput(updated)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := effective.Compile()
	if err != nil || !strings.Contains(compiled.Prompt, "焦距：85mm") {
		t.Fatal(compiled, err)
	}
	if _, err = readInput(first); err == nil {
		t.Fatal("stale input accepted")
	}
	var frozen string
	if err := owner.Raw(`SELECT draft::text FROM canvas.node_input WHERE node_id=? AND input_revision=?`, target.ID, firstTarget.InputRevision).Scan(&frozen).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(frozen, `"focal_length": "35mm"`) {
		t.Fatal("original immutable snapshot changed", frozen)
	}
	submit(domain.Operation{Type: "set", NodeID: source.ID, Path: []string{"position"}, Value: raw(`{"x":88,"y":3}`)})
	moved := read()
	if find(moved, target.ID).InputRevision != find(updated, target.ID).InputRevision {
		t.Fatal("move invalidated settings")
	}
	submit(domain.Operation{Type: "disconnect", NodeID: fixture.Edges[0].ID})
	detached := read()
	cleared, err := readInput(detached)
	if err != nil || cleared.CinemaKit != nil || find(detached, target.ID).InputRevision != find(moved, target.ID).InputRevision+1 {
		t.Fatal("detach did not invalidate", cleared, err)
	}
}
