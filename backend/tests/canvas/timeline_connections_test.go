package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/google/uuid"
)

func timelineRelationFixture() (domain.Document, domain.Edge) {
	source := resourceNode("image")
	target := timelineNode()
	target.Config.Timeline.Tracks[0].Kind = "image"
	clip := &target.Config.Timeline.Clips[0]
	clip.Kind, clip.Text, clip.NodeID, clip.AssetID = "image", "", &source.ID, source.RefID
	edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: source.ID, TargetNodeID: target.ID}
	return domain.Document{Nodes: []domain.Node{source, target}, Edges: []domain.Edge{edge}}, edge
}

func TestTimelineRelationRejectsBareEdgesButPreservesLegacyClips(t *testing.T) {
	doc, _ := timelineRelationFixture()
	if err := domain.ValidateGraph(doc); err != nil {
		t.Fatal(err)
	}
	doc.Nodes[1].Config.Timeline.Clips = nil
	if domain.ValidateGraph(doc) == nil {
		t.Fatal("bare annotation accepted as timeline input")
	}
	doc, _ = timelineRelationFixture()
	doc.Edges = nil
	if err := domain.ValidateGraph(doc); err != nil {
		t.Fatal("legacy clip without annotation rejected", err)
	}
}

func TestTimelineRelationDisconnectRequiresAtomicBindingDetach(t *testing.T) {
	doc, edge := timelineRelationFixture()
	if _, err := domain.Apply(doc, []domain.Command{{Type: "Disconnect", IDs: []uuid.UUID{edge.ID}}}); err == nil {
		t.Fatal("disconnected edge retained live input binding")
	}
	before, _ := json.Marshal(doc)
	config := *doc.Nodes[1].Config.Timeline
	config.Clips = append([]domain.TimelineClip(nil), config.Clips...)
	config.Clips[0].NodeID = nil
	next, err := domain.Apply(doc, []domain.Command{{Type: "Disconnect", IDs: []uuid.UUID{edge.ID}}, {Type: "UpdateNodeConfig", ID: doc.Nodes[1].ID, Config: &domain.NodeConfig{Timeline: &config}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Edges) != 0 || next.Nodes[1].Config.Timeline.Clips[0].NodeID != nil || next.Nodes[1].Config.Timeline.Clips[0].AssetID == nil {
		t.Fatal("atomic detach lost clip media")
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatal("mutated input graph")
	}
}

func TestTimelineRelationSourceDeletionRetainsClipAsset(t *testing.T) {
	doc, _ := timelineRelationFixture()
	next, err := domain.Apply(doc, []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{doc.Nodes[0].ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Edges) != 0 || next.Nodes[0].Config.Timeline.Clips[0].NodeID != nil || next.Nodes[0].Config.Timeline.Clips[0].AssetID == nil {
		t.Fatal("deleted source left dangling timeline input")
	}
}

func TestTimelineRelationRejectsOversizedSingleCandidate(t *testing.T) {
	target := timelineNode()
	target.Config.Timeline.Tracks[0].Kind = "image"
	target.Config.Timeline.Clips = nil
	nodes := []domain.Node{target}
	edges := []domain.Edge{}
	for range 51 {
		source := resourceNode("image")
		nodes = append(nodes, source)
		target.Config.Timeline.Clips = append(target.Config.Timeline.Clips, domain.TimelineClip{ID: uuid.New(), TrackID: target.Config.Timeline.Tracks[0].ID, Kind: "image", NodeID: &source.ID, AssetID: source.RefID, Title: "image", StartMS: int64(len(edges)) * 5000, DurationMS: 5000, Volume: 1})
		edges = append(edges, domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: source.ID, TargetNodeID: target.ID})
	}
	if _, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: nodes}, {Type: "Connect", Edges: edges}}); err == nil {
		t.Fatal("51-source timeline candidate accepted")
	}
}

func TestTimelineRelationChecksAuthoritativeMediaDuration(t *testing.T) {
	duration := int32(7200)
	sourceDuration := int64(7200)
	clip := domain.TimelineClip{Kind: "video", DurationMS: 7200, SourceDurationMS: &sourceDuration}
	if err := domain.ValidateTimelineSourceDuration(clip, &duration); err != nil {
		t.Fatal(err)
	}
	for _, actual := range []*int32{nil, ptr(int32(0)), ptr(int32(4000)), ptr(int32(8000))} {
		if domain.ValidateTimelineSourceDuration(clip, actual) == nil {
			t.Fatal("unconfirmed or stale source duration accepted")
		}
	}
	clip.SourceStartMS = 7000
	clip.DurationMS = 500
	if domain.ValidateTimelineSourceDuration(clip, &duration) == nil {
		t.Fatal("clip beyond source accepted")
	}
}
