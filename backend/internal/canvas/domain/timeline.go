package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"

	"github.com/google/uuid"
)

// TimelineConfig persists editable tracks and clips; render progress is separate.
type TimelineConfig struct {
	Version       int             `json:"version"`
	AspectRatio   string          `json:"aspect_ratio"`
	FPS           int             `json:"fps"`
	Tracks        []TimelineTrack `json:"tracks"`
	Clips         []TimelineClip  `json:"clips"`
	SubtitleStyle SubtitleStyle   `json:"subtitle_style"`
	Snapping      bool            `json:"snapping"`
}

// TimelineTrack is one nonoverlapping lane of clips of the same kind.
type TimelineTrack struct {
	ID      uuid.UUID `json:"id"`
	Kind    string    `json:"kind"`
	Label   string    `json:"label"`
	Locked  bool      `json:"locked"`
	Visible bool      `json:"visible"`
	Muted   bool      `json:"muted"`
}

// TimelineClip addresses project-owned media by identity, never by object URL.
type TimelineClip struct {
	ID               uuid.UUID     `json:"id"`
	TrackID          uuid.UUID     `json:"track_id"`
	Kind             string        `json:"kind"`
	NodeID           *uuid.UUID    `json:"node_id" extensions:"x-nullable"`
	AssetID          *uuid.UUID    `json:"asset_id" extensions:"x-nullable"`
	Title            string        `json:"title"`
	StartMS          int64         `json:"start_ms"`
	DurationMS       int64         `json:"duration_ms"`
	SourceStartMS    int64         `json:"source_start_ms"`
	SourceDurationMS *int64        `json:"source_duration_ms" extensions:"x-nullable"`
	Volume           float64       `json:"volume"`
	FadeInMS         int64         `json:"fade_in_ms"`
	FadeOutMS        int64         `json:"fade_out_ms"`
	Text             string        `json:"text"`
	Crop             *TimelineCrop `json:"crop,omitempty" extensions:"x-nullable"`
}

// TimelineCrop selects integer source pixels before the final export scaling.
type TimelineCrop struct {
	X      int32 `json:"x"`
	Y      int32 `json:"y"`
	Width  int32 `json:"width"`
	Height int32 `json:"height"`
}

// SubtitleStyle contains closed visual properties supported by the renderer.
type SubtitleStyle struct {
	FontSize int    `json:"font_size"`
	Color    string `json:"color"`
	Position string `json:"position"`
}

const timelineLimitMS int64 = 24 * 60 * 60 * 1000

var subtitleColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Validate checks the closed edit contract before freezing or rendering a timeline.
func (c TimelineConfig) Validate() error {
	if !validTimeline(c) {
		return fmt.Errorf("validate timeline configuration: %w", ErrInvalidCommand)
	}
	return nil
}

func validTimeline(c TimelineConfig) bool {
	if c.Version != 1 || !slices.Contains([]string{"16:9", "9:16", "1:1"}, c.AspectRatio) || !slices.Contains([]int{24, 25, 30, 60}, c.FPS) || len(c.Tracks) > 32 || len(c.Clips) > 1000 ||
		c.SubtitleStyle.FontSize < 12 || c.SubtitleStyle.FontSize > 96 || !subtitleColor.MatchString(c.SubtitleStyle.Color) || !slices.Contains([]string{"top", "center", "bottom"}, c.SubtitleStyle.Position) {
		return false
	}
	tracks := make(map[uuid.UUID]TimelineTrack, len(c.Tracks))
	for _, track := range c.Tracks {
		if track.ID == uuid.Nil || tracks[track.ID].ID != uuid.Nil || !timelineKind(track.Kind) || !validTitle(track.Label) {
			return false
		}
		tracks[track.ID] = track
	}
	byTrack := make(map[uuid.UUID][]TimelineClip, len(c.Tracks))
	ids := make(map[uuid.UUID]bool, len(c.Clips))
	for _, clip := range c.Clips {
		track, ok := tracks[clip.TrackID]
		if clip.ID == uuid.Nil || ids[clip.ID] || !ok || track.Kind != clip.Kind || !validTitle(clip.Title) || clip.StartMS < 0 || clip.DurationMS < 100 || clip.DurationMS > timelineLimitMS || clip.StartMS > timelineLimitMS-clip.DurationMS ||
			clip.SourceStartMS < 0 || clip.SourceStartMS > timelineLimitMS || (clip.SourceDurationMS != nil && (*clip.SourceDurationMS < 100 || *clip.SourceDurationMS > timelineLimitMS || clip.SourceStartMS > *clip.SourceDurationMS || ((clip.Kind == "video" || clip.Kind == "audio") && clip.DurationMS > *clip.SourceDurationMS-clip.SourceStartMS))) ||
			math.IsNaN(clip.Volume) || math.IsInf(clip.Volume, 0) || clip.Volume < 0 || clip.Volume > 2 || clip.FadeInMS < 0 || clip.FadeOutMS < 0 || clip.FadeInMS > clip.DurationMS || clip.FadeOutMS > clip.DurationMS || !validPrompt(clip.Text) || (clip.NodeID != nil && *clip.NodeID == uuid.Nil) || (clip.AssetID != nil && *clip.AssetID == uuid.Nil) {
			return false
		}
		if clip.Kind == "text" || clip.Kind == "subtitle" {
			if clip.NodeID != nil || clip.AssetID != nil {
				return false
			}
		} else if clip.NodeID == nil && clip.AssetID == nil {
			return false
		}
		if clip.Crop != nil && !validTimelineCrop(*clip.Crop, clip.Kind) {
			return false
		}
		ids[clip.ID] = true
		byTrack[clip.TrackID] = append(byTrack[clip.TrackID], clip)
	}
	for _, clips := range byTrack {
		slices.SortFunc(clips, func(a, b TimelineClip) int {
			if a.StartMS < b.StartMS {
				return -1
			}
			if a.StartMS > b.StartMS {
				return 1
			}
			return 0
		})
		for i := 1; i < len(clips); i++ {
			if clips[i].StartMS < clips[i-1].StartMS+clips[i-1].DurationMS {
				return false
			}
		}
	}
	raw, err := json.Marshal(c)
	return err == nil && len(raw) <= 512<<10
}

func validTimelineCrop(c TimelineCrop, kind string) bool {
	return (kind == "image" || kind == "video") && c.X >= 0 && c.Y >= 0 && c.Width >= 2 && c.Height >= 2 && c.Width <= 32768 && c.Height <= 32768 && c.X <= 32768-c.Width && c.Y <= 32768-c.Height && (kind != "video" || c.Width%2 == 0 && c.Height%2 == 0)
}

func timelineKind(kind string) bool {
	return slices.Contains([]string{"video", "audio", "image", "text", "subtitle"}, kind)
}

func validTimelineReferences(c TimelineConfig, nodes map[uuid.UUID]Node) bool {
	for _, clip := range c.Clips {
		if clip.NodeID == nil {
			continue
		}
		node, ok := nodes[*clip.NodeID]
		if !ok || node.NodeType != clip.Kind || node.RefType != "media_asset" || node.RefID == nil || (clip.AssetID != nil && *clip.AssetID != *node.RefID) {
			return false
		}
	}
	return true
}

func clearTimelineReferences(nodes []Node, removed map[uuid.UUID]bool) {
	assets := make(map[uuid.UUID]*uuid.UUID)
	for _, node := range nodes {
		if removed[node.ID] {
			assets[node.ID] = node.RefID
		}
	}
	for i, node := range nodes {
		if node.Config.Timeline == nil {
			continue
		}
		config := *node.Config.Timeline
		config.Clips = slices.Clone(config.Clips)
		for j, clip := range config.Clips {
			if clip.NodeID != nil && removed[*clip.NodeID] {
				config.Clips[j].AssetID = assets[*clip.NodeID]
				config.Clips[j].NodeID = nil
			}
		}
		nodes[i].Config.Timeline = &config
	}
}
