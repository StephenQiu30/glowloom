package canvas_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func TestYjsOptionalConfigProjectionPreservesPresenceThroughSaveAndClear(t *testing.T) {
	for _, kind := range []string{"camera", "palette", "style", "motion", "container"} {
		t.Run(kind, func(t *testing.T) {
			node := resourceNode("video")
			switch kind {
			case "camera":
				node = resourceNode("image")
			case "container":
				node = domain.Node{ID: uuid.New(), NodeType: "group", NodeAction: "resource", Title: "Group", Config: domain.NodeConfig{Collapsed: ptr(false)}}
			}
			doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "Optional config", SchemaVersion: 1, Revision: 1, Nodes: []domain.Node{node}, Edges: []domain.Edge{}}
			for _, phase := range []string{"add", "replace", "clear"} {
				projected := canvas.Snapshot(doc)
				body, err := json.Marshal(projected)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(body, &projected); err != nil {
					t.Fatal(err)
				}
				config := &projected.Nodes[0].Config
				if phase == "clear" {
					config.CinemaKit, config.Container = nil, nil
				} else {
					switch kind {
					case "camera":
						focal := "24mm"
						if phase == "replace" {
							focal = "35mm"
						}
						config.CinemaKit = &prompt.CinemaKit{Version: 1, Camera: &prompt.CinemaCamera{Body: "自动", Lens: "自动", FocalLength: focal, Aperture: "自动"}}
					case "palette":
						color := "#FFFFFF"
						if phase == "replace" {
							color = "#112233"
						}
						config.CinemaKit = &prompt.CinemaKit{Version: 1, Palette: &prompt.CinemaPalette{Name: "自定义", Colors: []string{color}}}
					case "style":
						style := "复古对称美学"
						if phase == "replace" {
							style = "3D动画风"
						}
						config.CinemaKit = &prompt.CinemaKit{Version: 1, Style: style}
					case "motion":
						motion := "迎面跟拍"
						if phase == "replace" {
							motion = "侧面跟拍"
						}
						config.CinemaKit = &prompt.CinemaKit{Version: 1, Motion: motion}
					case "container":
						background := "none"
						if phase == "replace" {
							background = "purple"
						}
						config.Container = &domain.ContainerConfig{Version: 1, Kind: "stack", Background: background}
					}
				}
				ops, err := canvas.YjsProjectionOperations(doc, projected)
				if err != nil {
					t.Fatalf("%s: %v", phase, err)
				}
				if len(ops) != 1 || ops[0].ExpectedExists == nil || *ops[0].ExpectedExists != (phase != "add") {
					t.Fatalf("%s: incorrect presence precondition: %+v", phase, ops)
				}
				applied, err := domain.ApplyOperations(doc, ops)
				if err != nil {
					t.Fatalf("%s: %v", phase, err)
				}
				actual := canvas.Snapshot(applied.Document)
				if !reflect.DeepEqual(actual.Nodes[0].Config, *config) {
					t.Fatalf("%s: saved config differs from the candidate", phase)
				}
				doc = applied.Document
			}
		})
	}
}
