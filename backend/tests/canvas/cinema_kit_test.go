package canvas_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func cinemaFixture(t *testing.T) (domain.Document, domain.Node, domain.Node) {
	t.Helper()
	source := domain.Node{ID: uuid.New(), NodeType: "cinema_studio", NodeAction: "tool", Title: "相机 1", Config: domain.NodeConfig{CinemaKit: &prompt.CinemaKit{Version: 1, Camera: &prompt.CinemaCamera{Body: "现代数字", Lens: "自动", FocalLength: "35mm", Aperture: "自动"}}}}
	target := composerNode(t)
	edge := domain.Edge{ID: uuid.New(), SourceNodeID: source.ID, TargetNodeID: target.ID, EdgeType: "annotation", Binding: raw(`{"schemaVersion":1,"fromPortId":"camera","toPortId":"camera","purpose":"CINEMA_CAMERA","order":0,"binding":{"kind":"NODE_VALUE"}}`)}
	return domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Nodes: []domain.Node{source, target}, Edges: []domain.Edge{edge}}, source, target
}
func TestCinemaKitFreezeAndUpstreamInvalidation(t *testing.T) {
	doc, source, target := cinemaFixture(t)
	if err := domain.ValidateGraph(doc); err != nil {
		t.Fatal(err)
	}
	input, err := domain.GenerationInput(doc, target)
	if err != nil || input.Composer.CinemaKit == nil || input.Composer.CinemaKit.Camera.FocalLength != "35mm" || len(input.Composer.CinemaKit.Sources) != 1 {
		t.Fatal(input, err)
	}
	compiled, err := input.Composer.Compile()
	if err != nil || !strings.Contains(compiled.Prompt, "焦距：35mm") {
		t.Fatal(compiled, err)
	}
	updated := *source.Config.CinemaKit
	camera := *updated.Camera
	camera.FocalLength = "85mm"
	updated.Camera = &camera
	before, _ := json.Marshal(map[string]any{"cinema_kit": source.Config.CinemaKit})
	value, _ := json.Marshal(map[string]any{"cinema_kit": updated})
	out, err := domain.ApplyOperations(doc, []domain.Operation{{Type: "set", NodeID: source.ID, Path: []string{"config", "cinema_kit"}, Expected: before, ExpectedExists: ptr(true), Value: value}})
	if err != nil || len(out.Inputs) != 1 || out.Inputs[0].NodeID != target.ID || out.Inputs[0].Revision != 2 || out.Inputs[0].Draft.Composer.CinemaKit.Camera.FocalLength != "85mm" {
		t.Fatal(out, err)
	}
	if out.Document.Nodes[1].Config.Draft.Composer.CinemaKit != nil {
		t.Fatal("derived settings overwritten user composer")
	}
}
func TestCinemaKitRejectsWrongTypeAndForgedChoice(t *testing.T) {
	doc, _, _ := cinemaFixture(t)
	doc.Nodes[1].NodeType = "video_generation"
	if err := domain.ValidateGraph(doc); err == nil {
		t.Fatal("camera connected to unsupported video")
	}
	doc, _, _ = cinemaFixture(t)
	doc.Nodes[0].Config.CinemaKit.Camera.Body = "unknown"
	if err := domain.ValidateGraph(doc); err == nil {
		t.Fatal("unknown camera admitted")
	}
	doc, _, _ = cinemaFixture(t)
	duplicate := doc.Edges[0]
	duplicate.ID = uuid.New()
	doc.Edges = append(doc.Edges, duplicate)
	if err := domain.ValidateGraph(doc); err == nil {
		t.Fatal("duplicate slot admitted")
	}
}

func TestCinemaKitEffectiveInputOwnsItsFrozenValues(t *testing.T) {
	doc, _, target := cinemaFixture(t)
	input, err := domain.GenerationInput(doc, target)
	if err != nil {
		t.Fatal(err)
	}
	doc.Nodes[0].Config.CinemaKit.Camera.FocalLength = "135mm"
	if input.Composer.CinemaKit.Camera.FocalLength != "35mm" {
		t.Fatal("frozen values alias mutable canvas config")
	}
}
func TestCinemaKitCompilerPreservesAuthoredDocumentAndRejectsInvalidTypedIntent(t *testing.T) {
	doc, _, target := cinemaFixture(t)
	input, err := domain.GenerationInput(doc, target)
	if err != nil {
		t.Fatal(err)
	}
	authored, _ := json.Marshal(input.Composer.Document)
	first, err := input.Composer.Compile()
	if err != nil {
		t.Fatal(err)
	}
	input.Composer.CinemaKit.Camera.FocalLength = "85mm"
	second, err := input.Composer.Compile()
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(input.Composer.Document)
	if string(authored) != string(after) || first.Prompt == second.Prompt {
		t.Fatal("compiler rewrote authored document or omitted intent")
	}
	for _, kit := range []prompt.CinemaKit{
		{Version: 1, Motion: "迎面跟拍"},
		{Version: 1, Style: "复古对称美学"},
		{Version: 1, Palette: &prompt.CinemaPalette{Name: "自定义", Colors: []string{"#4B90E2", "#FFFFFF"}}},
	} {
		if err := kit.Validate(); err != nil {
			t.Fatal(err)
		}
		if !kit.Supports("video.generate") || kit.Supports("text.generate") || kit.Supports("audio.generate") {
			t.Fatal("wrong compiler capability", kit)
		}
	}
	for _, kit := range []prompt.CinemaKit{{Version: 2, Style: "复古对称美学"}, {Version: 1, Style: "unknown"}, {Version: 1, Motion: "unknown"}, {Version: 1, Palette: &prompt.CinemaPalette{Name: " ", Colors: []string{"#FFFFFF"}}}, {Version: 1, Palette: &prompt.CinemaPalette{Name: "A", Colors: []string{"#ffffff"}}}, {Version: 1, Palette: &prompt.CinemaPalette{Name: "A", Preset: "unknown", Colors: []string{"#FFFFFF"}}}} {
		if kit.Validate() == nil {
			t.Fatal("invalid intent accepted", kit)
		}
	}
}
