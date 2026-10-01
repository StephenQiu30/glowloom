package canvas_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func timelineNode() domain.Node {
	track := uuid.New()
	return domain.Node{ID: uuid.New(), Title: "时间线", NodeType: "timeline", NodeAction: "tool", Config: domain.NodeConfig{Timeline: &domain.TimelineConfig{
		Version: 1, AspectRatio: "16:9", FPS: 30,
		Tracks:        []domain.TimelineTrack{{ID: track, Kind: "text", Label: "字幕", Visible: true}},
		Clips:         []domain.TimelineClip{{ID: uuid.New(), TrackID: track, Kind: "text", Title: "开场", DurationMS: 1000, Volume: 1, Text: "第一幕"}},
		SubtitleStyle: domain.SubtitleStyle{FontSize: 36, Color: "#FFFFFF", Position: "bottom"},
	}}}
}

func TestTimelineRejectsOverlapsAndUnknownReferences(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*domain.TimelineConfig)
	}{
		{"overlap", func(c *domain.TimelineConfig) {
			next := c.Clips[0]
			next.ID = uuid.New()
			next.StartMS = 999
			c.Clips = append(c.Clips, next)
		}},
		{"track mismatch", func(c *domain.TimelineConfig) { c.Tracks[0].Kind = "video" }},
		{"foreign node", func(c *domain.TimelineConfig) {
			c.Tracks[0].Kind = "image"
			c.Clips[0].Kind = "image"
			c.Clips[0].NodeID = ptr(uuid.New())
		}},
		{"negative trim", func(c *domain.TimelineConfig) { c.Clips[0].SourceStartMS = -1 }},
		{"invalid color", func(c *domain.TimelineConfig) { c.SubtitleStyle.Color = "url(https://invalid)" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := timelineNode()
			tc.change(node.Config.Timeline)
			_, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}})
			if !errors.Is(err, domain.ErrInvalidCommand) {
				t.Fatalf("invalid timeline accepted: %v", err)
			}
		})
	}
	valid := timelineNode()
	next := valid.Config.Timeline.Clips[0]
	next.ID = uuid.New()
	next.StartMS = 1000
	valid.Config.Timeline.Clips = append(valid.Config.Timeline.Clips, next)
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{valid}}}); err != nil {
		t.Fatal(err)
	}
}

func TestTimelineDeletionKeepsStableMediaIdentity(t *testing.T) {
	image := resourceNode("image")
	node := timelineNode()
	node.Config.Timeline.Tracks[0].Kind = "image"
	clip := &node.Config.Timeline.Clips[0]
	clip.Kind = "image"
	clip.NodeID = &image.ID
	doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node, image}}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := domain.Apply(doc, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{image.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	kept := after.Nodes[0].Config.Timeline.Clips[0]
	if kept.NodeID != nil || kept.AssetID == nil || *kept.AssetID != *image.RefID {
		t.Fatal("deleted node lost stable clip asset")
	}
	if doc.Nodes[0].Config.Timeline.Clips[0].NodeID == nil || doc.Nodes[0].Config.Timeline.Clips[0].AssetID != nil {
		t.Fatal("delete mutated input")
	}
}

func TestTimelineCropRejectsInvalidPixelsAndUnsupportedKinds(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		crop domain.TimelineCrop
	}{
		{"negative position", "image", domain.TimelineCrop{X: -1, Width: 100, Height: 80}},
		{"zero width", "image", domain.TimelineCrop{Height: 80}},
		{"oversized area", "image", domain.TimelineCrop{X: 32760, Width: 100, Height: 80}},
		{"odd video width", "video", domain.TimelineCrop{Width: 101, Height: 80}},
		{"audio crop", "audio", domain.TimelineCrop{Width: 100, Height: 80}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := timelineNode()
			node.Config.Timeline.Tracks[0].Kind = tc.kind
			clip := &node.Config.Timeline.Clips[0]
			clip.Kind = tc.kind
			clip.AssetID = ptr(uuid.New())
			clip.Crop = &tc.crop
			if node.Config.Timeline.Validate() == nil {
				t.Fatal("invalid source crop accepted")
			}
		})
	}
	valid := timelineNode()
	valid.Config.Timeline.Tracks[0].Kind = "image"
	valid.Config.Timeline.Clips[0].Kind = "image"
	valid.Config.Timeline.Clips[0].AssetID = ptr(uuid.New())
	valid.Config.Timeline.Clips[0].Crop = &domain.TimelineCrop{X: 1, Y: 1, Width: 101, Height: 81}
	if err := valid.Config.Timeline.Validate(); err != nil {
		t.Fatal(err)
	}
}
