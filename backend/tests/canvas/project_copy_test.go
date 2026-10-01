package canvas_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func TestProjectCopyDocumentRemapsContentAndPreservesSource(t *testing.T) {
	group, image, director := resourceNode("group"), resourceNode("image"), directorNode()
	image.ParentID = &group.ID
	image.X, image.Y = 300, 400
	image.LastOperationID = ptr(uuid.New())
	director.Config.Director.Objects[0].Kind = "billboard"
	director.Config.Director.Objects[0].BuiltinActor = ""
	director.Config.Director.Objects[0].SourceNodeID = &image.ID
	director.Config.Director.Objects[0].AssetID = image.RefID
	screenshot := directorScreenshot()
	screenshot.AssetID = *image.RefID
	director.Config.Director.Shots[0].Screenshots = []domain.DirectorScreenshot{screenshot}
	director.Config.Director.Cover = &domain.DirectorCover{AssetID: *image.RefID, ShotID: director.Config.Director.Shots[0].ID}
	director.Config.Director.Panorama = &domain.DirectorPanorama{AssetID: *image.RefID}
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "完整工程", Scope: json.RawMessage(`{}`), Revision: 9, Viewport: domain.Viewport{X: 10, Y: 20, Zoom: 0.5}, Nodes: []domain.Node{group, image, director}, Edges: []domain.Edge{{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: image.ID, TargetNodeID: director.ID}}}
	before, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	target, job, asset := uuid.New(), uuid.New(), uuid.New()
	mapping := map[uuid.UUID]uuid.UUID{*image.RefID: asset}
	copied, err := domain.CopyProjectDocument(doc, target, job, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Document.ID == doc.ID || copied.Document.ProjectID != target || copied.Document.Revision != 1 || copied.Document.Viewport != doc.Viewport || copied.ClearedOperationBindings != 1 || len(copied.Document.Nodes) != 3 || len(copied.Document.Edges) != 1 {
		t.Fatalf("incomplete document copy: %+v", copied)
	}
	nodes := copied.Document.Nodes
	if nodes[1].LastOperationID != nil || *nodes[1].ParentID != nodes[0].ID || *nodes[1].RefID != asset || nodes[1].X != 300 || nodes[1].Y != 400 || copied.Document.Edges[0].SourceNodeID != nodes[1].ID || copied.Document.Edges[0].TargetNodeID != nodes[2].ID {
		t.Fatal("node hierarchy/media/edges were not remapped")
	}
	c := nodes[2].Config.Director
	if c.ID == director.Config.Director.ID || c.Shots[0].ID == director.Config.Director.Shots[0].ID || c.Cameras[0].ID == director.Config.Director.Cameras[0].ID || c.Shots[0].CameraID != c.Cameras[0].ID || c.ActiveShotID != c.Shots[0].ID || *c.Objects[0].SourceNodeID != nodes[1].ID || *c.Objects[0].AssetID != asset || c.Panorama.AssetID != asset || c.Cover.ShotID != c.Shots[0].ID || c.Cover.AssetID != asset || c.Shots[0].Screenshots[0].AssetID != asset || c.Shots[0].Screenshots[0].ID == screenshot.ID {
		t.Fatal("director identities or image references were not remapped")
	}
	if c.Shots[0].Screenshots[0].Name != screenshot.Name || !c.Shots[0].Screenshots[0].CreatedAt.Equal(screenshot.CreatedAt) {
		t.Fatal("gallery presentation metadata changed")
	}
	again, err := domain.CopyProjectDocument(doc, target, job, mapping)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(copied)
	if err != nil {
		t.Fatal(err)
	}
	againJSON, err := json.Marshal(again)
	if err != nil || !bytes.Equal(firstJSON, againJSON) {
		t.Fatal("same copy job changed target identities")
	}
	after, err := json.Marshal(doc)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("copy mutated source document")
	}
	if _, err := domain.CopyProjectDocument(doc, target, job, nil); !errors.Is(err, domain.ErrInvalidProjectCopy) {
		t.Fatalf("missing media silently removed: %v", err)
	}
}

func TestProjectCopyRejectsUnsupportedBusinessContent(t *testing.T) {
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "完整工程", Scope: json.RawMessage(`{}`), Revision: 1, Viewport: domain.Viewport{Zoom: 1}, Nodes: []domain.Node{}, Edges: []domain.Edge{}}
	if copied, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), nil); err != nil || len(copied.Document.Nodes) != 0 {
		t.Fatalf("empty actual project canvas should copy: %+v %v", copied, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*domain.Document)
	}{
		{"business scope", func(d *domain.Document) { d.Scope = json.RawMessage(`{"episode_id":"` + uuid.NewString() + `"}`) }},
		{"business node", func(d *domain.Document) {
			d.Nodes = []domain.Node{{ID: uuid.New(), Title: "业务镜头", NodeType: "shot", NodeAction: "generate", RefType: "shot", RefID: ptr(uuid.New())}}
		}},
		{"business edge", func(d *domain.Document) {
			d.Nodes = []domain.Node{resourceNode("text"), resourceNode("text")}
			d.Edges = []domain.Edge{{ID: uuid.New(), EdgeType: "promote", SourceNodeID: d.Nodes[0].ID, TargetNodeID: d.Nodes[1].ID}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := doc
			tc.change(&source)
			if _, err := domain.CopyProjectDocument(source, uuid.New(), uuid.New(), nil); !errors.Is(err, domain.ErrUnsupportedProjectCopy) {
				t.Fatalf("unsupported content silently dropped: %v", err)
			}
		})
	}
}

func TestProjectCopyRemapsBatchTimelineAndAnimationIdentities(t *testing.T) {
	image, batch, timeline, generation, director := resourceNode("image"), batchNode(nil), timelineNode(), generationNode(), directorNode()
	batch.Config.BatchTable.Rows[0].InputNodeIDs[0] = &image.ID
	timeline.Config.Timeline.Tracks[0].Kind = "image"
	timeline.Config.Timeline.Clips[0].Kind = "image"
	timeline.Config.Timeline.Clips[0].NodeID = &image.ID
	timeline.Config.Timeline.Clips[0].AssetID = image.RefID
	generation.Config.Generation.Inputs = []domain.GenerationReference{{Role: "reference", MediaAssetID: *image.RefID}}
	generation.Config.Generation.Prompt = "用户文本包含原身份 " + image.ID.String()
	profile := uuid.New()
	generation.Config.Generation.ModelProfileID = &profile
	object := &director.Config.Director.Objects[0]
	object.MotionClips = []domain.DirectorMotionClip{{ID: uuid.New(), Name: "走路", SourceAnimation: "Walk", Start: 0, Duration: 2, PlaybackRate: 1}}
	object.ActiveMotionClipID = &object.MotionClips[0].ID
	object.Keyframes = []domain.DirectorKeyframe{{ID: uuid.New(), Time: 1, Transform: object.Transform}}
	object.BoneTracks = []domain.DirectorBoneTrack{{Bone: "head", Keyframes: []domain.DirectorBoneFrame{{ID: uuid.New(), Time: 1, Rotation: []float64{0, 0, 0, 1}}}}}
	camera := &director.Config.Director.Cameras[0]
	camera.FollowObjectID = &object.ID
	camera.LookAtObjectID = &object.ID
	camera.Keyframes = []domain.DirectorKeyframe{{ID: uuid.New(), Time: 1, Transform: camera.Transform}}
	director.Config.Director.Lights = []domain.DirectorLight{{ID: uuid.New(), Name: "主光", Type: "ambient", Transform: object.Transform, Color: "#ffffff", Intensity: 1}}
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "全部工具", Scope: json.RawMessage(`{}`), Revision: 1, Viewport: domain.Viewport{Zoom: 1}, Nodes: []domain.Node{image, batch, timeline, generation, director}, Edges: []domain.Edge{}}
	copied, err := domain.CopyProjectDocument(doc, uuid.New(), uuid.New(), map[uuid.UUID]uuid.UUID{*image.RefID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	n := copied.Document.Nodes
	if *n[1].Config.BatchTable.Rows[0].InputNodeIDs[0] != n[0].ID || n[1].Config.BatchTable.Rows[0].ID == batch.Config.BatchTable.Rows[0].ID || n[1].Config.BatchTable.ReferenceColumns[0].ID == batch.Config.BatchTable.ReferenceColumns[0].ID {
		t.Fatal("batch input or row/column identities not independent")
	}
	tl := n[2].Config.Timeline
	if tl.Tracks[0].ID == timeline.Config.Timeline.Tracks[0].ID || tl.Clips[0].ID == timeline.Config.Timeline.Clips[0].ID || tl.Clips[0].TrackID != tl.Tracks[0].ID || *tl.Clips[0].NodeID != n[0].ID || *tl.Clips[0].AssetID != *n[0].RefID {
		t.Fatal("timeline references not independent")
	}
	gen := n[3].Config.Generation
	if gen.Inputs[0].MediaAssetID != *n[0].RefID || *gen.ModelProfileID != profile || gen.Prompt != generation.Config.Generation.Prompt {
		t.Fatal("generation media not mapped, or user text/catalog identity changed")
	}
	c := n[4].Config.Director
	gotObject, gotCamera := c.Objects[0], c.Cameras[0]
	if gotObject.ID == object.ID || gotObject.MotionClips[0].ID == object.MotionClips[0].ID || *gotObject.ActiveMotionClipID != gotObject.MotionClips[0].ID || gotObject.Keyframes[0].ID == object.Keyframes[0].ID || gotObject.BoneTracks[0].Keyframes[0].ID == object.BoneTracks[0].Keyframes[0].ID || *gotCamera.FollowObjectID != gotObject.ID || *gotCamera.LookAtObjectID != gotObject.ID || gotCamera.Keyframes[0].ID == camera.Keyframes[0].ID || c.Lights[0].ID == director.Config.Director.Lights[0].ID {
		t.Fatal("animation/follow/light identities not remapped")
	}
	if gotObject.BoneTracks[0].Bone != "head" || gotObject.MotionClips[0].SourceAnimation != "Walk" {
		t.Fatal("animation semantic names were changed")
	}
}
