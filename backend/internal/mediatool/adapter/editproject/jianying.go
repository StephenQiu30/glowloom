package editproject

import (
	"encoding/json"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	"github.com/google/uuid"
)

// This independent serializer follows an authorized synthetic native draft. It
// contains no application implementation, remote URL, credentials or jypack wrapper.
type jianyingDraft struct {
	ID             string                      `json:"id"`
	Name           string                      `json:"name"`
	Version        int                         `json:"version"`
	NewVersion     string                      `json:"new_version"`
	FPS            int                         `json:"fps"`
	Duration       int64                       `json:"duration"`
	Canvas         map[string]any              `json:"canvas_config"`
	Config         map[string]any              `json:"config"`
	Materials      map[string][]map[string]any `json:"materials"`
	Tracks         []map[string]any            `json:"tracks"`
	Keyframes      map[string][]any            `json:"keyframes"`
	Platform       map[string]any              `json:"platform"`
	LastPlatform   map[string]any              `json:"last_modified_platform"`
	ColorSpace     int                         `json:"color_space"`
	Source         string                      `json:"source"`
	Path           string                      `json:"path"`
	CreateTime     int                         `json:"create_time"`
	UpdateTime     int                         `json:"update_time"`
	Cover          any                         `json:"cover"`
	ExtraInfo      any                         `json:"extra_info"`
	GroupContainer any                         `json:"group_container"`
	MutableConfig  any                         `json:"mutable_config"`
	Relationships  []any                       `json:"relationships"`
	KeyframeGraphs []any                       `json:"keyframe_graph_list"`
	FreeRender     bool                        `json:"free_render_index_mode_on"`
	TrackRender    bool                        `json:"render_index_track_mode_on"`
	DropFrame      bool                        `json:"is_drop_frame_timecode"`
}

func jianyingDocument(f domain.FrozenExport) ([]byte, error) {
	width, height, err := f.VideoDimensions()
	if err != nil {
		return nil, err
	}
	name := "时间线"
	if f.Settings != nil {
		name = f.Settings.FileName
	}
	d := jianyingDraft{ID: uuid.NewSHA1(*f.ExportID, []byte("native-draft")).String(), Name: name, ColorSpace: -1, Source: "default", Version: 360000, NewVersion: f.OutputKind.FormatVersion(), FPS: f.Timeline.FPS,
		Canvas: map[string]any{"width": width, "height": height, "ratio": "original", "background": nil}, Config: map[string]any{"maintrack_adsorb": f.Timeline.Snapping, "video_mute": false, "material_save_mode": 0}, Materials: map[string][]map[string]any{}, Tracks: []map[string]any{}, Keyframes: map[string][]any{}, Platform: map[string]any{"app_id": 0, "app_source": "", "app_version": "10.5.0", "os": "web", "os_version": "", "device_id": "", "hard_disk_id": "", "mac_address": ""}, Relationships: []any{}, KeyframeGraphs: []any{}, TrackRender: true}
	d.LastPlatform = d.Platform
	for _, k := range []string{"videos", "audios", "speeds", "canvases", "beats", "placeholder_infos", "sound_channel_mappings", "vocal_separations", "audio_balances", "audio_effects", "audio_fades", "audio_track_indexes", "chromas", "color_curves", "digital_humans", "drafts", "effects", "filters", "green_screens", "handwrites", "hsl", "images", "log_color_wheels", "loudnesses", "manual_deformations", "masks", "material_animations", "multi_language_refs", "placeholders", "plugin_effects", "primary_color_wheels", "realtime_denoises", "shapes", "smart_crops", "smart_relights", "stickers", "tail_leaders", "text_templates", "texts", "time_marks", "transitions", "video_effects", "video_trackings"} {
		d.Materials[k] = []map[string]any{}
	}
	for _, k := range []string{"adjusts", "audios", "effects", "filters", "handwrites", "stickers", "texts", "videos"} {
		d.Keyframes[k] = []any{}
	}
	inputs := map[uuid.UUID]domain.FrozenSource{}
	for _, in := range f.Inputs {
		inputs[in.AssetID] = in
	}
	id := func(clip uuid.UUID, kind string) string {
		return uuid.NewSHA1(clip, []byte("native-draft/"+kind)).String()
	}
	tr := func(start, duration int64) map[string]int64 {
		return map[string]int64{"start": start * 1000, "duration": duration * 1000}
	}
	for ti, track := range f.Timeline.Tracks {
		segments := []map[string]any{}
		for _, c := range sortedClips(f.Timeline.Clips) {
			if c.TrackID != track.ID {
				continue
			}
			in := inputs[*c.AssetID]
			mid := id(c.ID, "material")
			material := map[string]any{"id": mid, "path": mediaPath(f.OutputKind, sourceName(in)), "type": "music", "name": c.Title, "duration": int64(0)}
			if in.DurationMS != nil {
				material["duration"] = int64(*in.DurationMS) * 1000
			}
			bucket := "audios"
			if c.Kind != "audio" {
				bucket = "videos"
				material["type"] = "video"
				if c.Kind == "image" {
					material["type"] = "photo"
					material["duration"] = int64(10800000000)
				}
				material["width"] = *in.Width
				material["height"] = *in.Height
				material["has_audio"] = in.AudioChannels != nil && *in.AudioChannels > 0
				material["crop"] = map[string]float64{"upper_left_x": 0, "upper_left_y": 0, "upper_right_x": 1, "upper_right_y": 0, "lower_left_x": 0, "lower_left_y": 1, "lower_right_x": 1, "lower_right_y": 1}
				material["crop_ratio"] = "free"
				material["crop_scale"] = 1
				material["material_name"] = c.Title
			}
			d.Materials[bucket] = append(d.Materials[bucket], material)
			refs := []string{id(c.ID, "speed"), id(c.ID, "placeholder"), id(c.ID, "canvas-or-beats"), id(c.ID, "channel"), id(c.ID, "vocal")}
			d.Materials["speeds"] = append(d.Materials["speeds"], map[string]any{"id": refs[0], "type": "speed", "mode": 0, "speed": 1, "curve_speed": nil})
			d.Materials["placeholder_infos"] = append(d.Materials["placeholder_infos"], map[string]any{"id": refs[1], "type": "placeholder_info", "meta_type": "none", "error_path": "", "error_text": "", "res_path": "", "res_text": ""})
			d.Materials["sound_channel_mappings"] = append(d.Materials["sound_channel_mappings"], map[string]any{"id": refs[3], "type": "", "audio_channel_mapping": 0, "is_config_open": false})
			d.Materials["vocal_separations"] = append(d.Materials["vocal_separations"], map[string]any{"id": refs[4], "type": "vocal_separation", "choice": 0, "production_path": "", "removed_sounds": []any{}, "time_range": nil})
			volume := c.Volume
			if track.Muted {
				volume = 0
			}
			s := map[string]any{"id": c.ID.String(), "material_id": mid, "source_timerange": tr(c.SourceStartMS, c.DurationMS), "target_timerange": tr(c.StartMS, c.DurationMS), "render_timerange": tr(0, 0), "volume": volume, "last_nonzero_volume": c.Volume, "speed": 1, "visible": track.Visible, "reverse": false, "track_render_index": ti, "render_index": 0, "state": 0, "track_attribute": 0, "extra_material_refs": refs, "common_keyframes": []any{}, "keyframe_refs": []any{}, "clip": nil, "uniform_scale": nil, "hdr_settings": nil, "enable_adjust": c.Kind != "audio", "enable_lut": c.Kind != "audio", "enable_video_mask": true, "enable_color_curves": true, "enable_color_wheels": true, "template_scene": "default"}
			if c.Kind != "audio" {
				d.Materials["canvases"] = append(d.Materials["canvases"], map[string]any{"id": refs[2], "type": "canvas_color", "color": "", "blur": 0, "image": "", "image_id": "", "image_name": "", "album_image": "", "source_platform": 0, "team_id": ""})
				s["clip"] = map[string]any{"alpha": 1, "rotation": 0, "scale": map[string]int{"x": 1, "y": 1}, "transform": map[string]int{"x": 0, "y": 0}, "flip": map[string]bool{"horizontal": false, "vertical": false}}
				s["uniform_scale"] = map[string]any{"on": true, "value": 1}
				s["hdr_settings"] = map[string]any{"intensity": 1, "mode": 1, "nits": 1000}
			} else {
				d.Materials["beats"] = append(d.Materials["beats"], map[string]any{"id": refs[2], "type": "beats", "enable_ai_beats": false, "gear": 404, "gear_count": 0, "mode": 404, "user_beats": []any{}, "user_delete_ai_beats": nil, "ai_beats": map[string]any{"beat_speed_infos": []any{}, "beats_path": "", "beats_url": "", "melody_path": "", "melody_url": "", "melody_percents": []float64{.6}}})
			}
			segments = append(segments, s)
			d.Duration = max(d.Duration, (c.StartMS+c.DurationMS)*1000)
		}
		kind := "video"
		if track.Kind == "audio" {
			kind = "audio"
		}
		d.Tracks = append(d.Tracks, map[string]any{"id": track.ID.String(), "type": kind, "name": track.Label, "is_default_name": track.Label == "", "attribute": 0, "flag": 0, "segments": segments})
	}
	return json.Marshal(d)
}
