package domain

import (
	"encoding/json"
	"math"
	"slices"
	"unicode/utf8"

	"github.com/google/uuid"
)

// DirectorConfig stores bounded scene inputs; rendered media belongs to media assets.
type DirectorConfig struct {
	ID                   uuid.UUID               `json:"id"`
	Version              int                     `json:"version"`
	Title                string                  `json:"title"`
	Background           string                  `json:"background"`
	EnvironmentIntensity float64                 `json:"environment_intensity"`
	GridVisible          bool                    `json:"grid_visible"`
	GridSnap             *bool                   `json:"grid_snap,omitempty"`
	Ground               *DirectorGround         `json:"ground,omitempty"`
	StageTransform       *DirectorStageTransform `json:"stage_transform,omitempty"`
	LabelsVisible        *bool                   `json:"labels_visible,omitempty"`
	AspectRatio          string                  `json:"aspect_ratio,omitempty"`
	Panorama             *DirectorPanorama       `json:"panorama,omitempty"`
	PanoramaRotation     *float64                `json:"panorama_rotation,omitempty"`
	PanoramaRadius       *float64                `json:"panorama_radius,omitempty"`
	Objects              []DirectorObject        `json:"objects"`
	Cameras              []DirectorCamera        `json:"cameras"`
	Lights               []DirectorLight         `json:"lights"`
	Shots                []DirectorShot          `json:"shots"`
	ActiveShotID         uuid.UUID               `json:"active_shot_id"`
}

// DirectorTransform is an explicit position, rotation and positive scale.
type DirectorTransform struct {
	Position []float64 `json:"position"`
	Rotation []float64 `json:"rotation"`
	Scale    []float64 `json:"scale"`
}

// DirectorGround describes the editing ground plane.
type DirectorGround struct {
	Visible bool    `json:"visible"`
	Opacity float64 `json:"opacity"`
	Height  float64 `json:"height"`
}

// DirectorStageTransform affects the entire stage without changing object identities.
type DirectorStageTransform struct {
	Scale    float64   `json:"scale"`
	Position []float64 `json:"position"`
	Rotation []float64 `json:"rotation"`
}

// DirectorPanorama references one authorized image.
type DirectorPanorama struct {
	AssetID  uuid.UUID `json:"asset_id"`
	Name     string    `json:"name,omitempty"`
	Rotation float64   `json:"rotation"`
}

// DirectorKeyframe stores one transform and interpolation choice.
type DirectorKeyframe struct {
	ID        uuid.UUID         `json:"id"`
	Time      float64           `json:"time"`
	Transform DirectorTransform `json:"transform"`
	Easing    string            `json:"easing,omitempty"`
}

// DirectorRig maps the closed humanoid bone names to imported model nodes.
type DirectorRig struct {
	BoneMap        map[string]string `json:"bone_map"`
	AnimationNames []string          `json:"animation_names"`
}

// DirectorMotionClip schedules an imported model animation.
type DirectorMotionClip struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	SourceAnimation string    `json:"source_animation"`
	Start           float64   `json:"start"`
	Duration        float64   `json:"duration"`
	PlaybackRate    float64   `json:"playback_rate"`
	Loop            bool      `json:"loop"`
}

// DirectorBoneFrame stores a unit quaternion for a named humanoid bone.
type DirectorBoneFrame struct {
	ID       uuid.UUID `json:"id"`
	Time     float64   `json:"time"`
	Rotation []float64 `json:"rotation"`
	Easing   string    `json:"easing,omitempty"`
}

// DirectorBoneTrack contains independent bounded keyframes for one bone.
type DirectorBoneTrack struct {
	Bone      string              `json:"bone"`
	Keyframes []DirectorBoneFrame `json:"keyframes"`
}

// DirectorObject contains no executable source URLs or runtime task state.
type DirectorObject struct {
	ID                 uuid.UUID            `json:"id"`
	Name               string               `json:"name"`
	Kind               string               `json:"kind"`
	Primitive          string               `json:"primitive,omitempty"`
	Transform          DirectorTransform    `json:"transform"`
	Color              string               `json:"color"`
	UniformScale       *float64             `json:"uniform_scale,omitempty"`
	Visible            bool                 `json:"visible"`
	CastShadow         bool                 `json:"cast_shadow"`
	ReceiveShadow      bool                 `json:"receive_shadow"`
	Pose               string               `json:"pose,omitempty"`
	Rig                *DirectorRig         `json:"rig,omitempty"`
	MotionClips        []DirectorMotionClip `json:"motion_clips,omitempty"`
	ActiveMotionClipID *uuid.UUID           `json:"active_motion_clip_id,omitempty"`
	BoneOverrides      map[string][]float64 `json:"bone_overrides,omitempty" swaggertype:"object"`
	BoneTracks         []DirectorBoneTrack  `json:"bone_tracks,omitempty"`
	SourceNodeID       *uuid.UUID           `json:"source_node_id,omitempty"`
	AssetID            *uuid.UUID           `json:"asset_id,omitempty"`
	BuiltinActor       string               `json:"builtin_actor,omitempty"`
	Keyframes          []DirectorKeyframe   `json:"keyframes"`
}

// DirectorCamera stores render optics and stable object follow identities.
type DirectorCamera struct {
	ID             uuid.UUID          `json:"id"`
	Name           string             `json:"name"`
	Transform      DirectorTransform  `json:"transform"`
	Target         []float64          `json:"target"`
	FollowObjectID *uuid.UUID         `json:"follow_object_id,omitempty"`
	FollowAnchor   []float64          `json:"follow_anchor,omitempty"`
	LookAtMode     string             `json:"look_at_mode,omitempty"`
	LookAtObjectID *uuid.UUID         `json:"look_at_object_id,omitempty"`
	FocalLength    float64            `json:"focal_length"`
	FOV            float64            `json:"fov"`
	Aperture       float64            `json:"aperture"`
	FocusDistance  float64            `json:"focus_distance"`
	Near           float64            `json:"near"`
	Far            float64            `json:"far"`
	Keyframes      []DirectorKeyframe `json:"keyframes"`
}

// DirectorLight records a supported light type and its physical parameters.
type DirectorLight struct {
	ID         uuid.UUID         `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Transform  DirectorTransform `json:"transform"`
	Color      string            `json:"color"`
	Intensity  float64           `json:"intensity"`
	Angle      *float64          `json:"angle,omitempty"`
	Penumbra   *float64          `json:"penumbra,omitempty"`
	CastShadow bool              `json:"cast_shadow"`
}

// DirectorShot identifies a camera and the editable filmmaking intent.
type DirectorShot struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	CameraID   uuid.UUID `json:"camera_id"`
	Duration   float64   `json:"duration"`
	FPS        int       `json:"fps"`
	ShotSize   string    `json:"shot_size"`
	CameraMove string    `json:"camera_move"`
	Prompt     string    `json:"prompt"`
}

var directorBones = []string{"root", "hips", "spine", "chest", "neck", "head", "leftShoulder", "leftUpperArm", "leftLowerArm", "leftHand", "rightShoulder", "rightUpperArm", "rightLowerArm", "rightHand", "leftUpperLeg", "leftLowerLeg", "leftFoot", "rightUpperLeg", "rightLowerLeg", "rightFoot", "leftThumb1", "leftThumb2", "leftThumb3", "leftIndex1", "leftIndex2", "leftIndex3", "leftMiddle1", "leftMiddle2", "leftMiddle3", "leftRing1", "leftRing2", "leftRing3", "leftPinky1", "leftPinky2", "leftPinky3", "rightThumb1", "rightThumb2", "rightThumb3", "rightIndex1", "rightIndex2", "rightIndex3", "rightMiddle1", "rightMiddle2", "rightMiddle3", "rightRing1", "rightRing2", "rightRing3", "rightPinky1", "rightPinky2", "rightPinky3"}

func directorRange(v, minimum, maximum float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= minimum && v <= maximum
}
func directorOptionalRange(v *float64, minimum, maximum float64) bool {
	return v == nil || directorRange(*v, minimum, maximum)
}
func directorVector(v []float64, minimum, maximum float64) bool {
	return len(v) == 3 && directorRange(v[0], minimum, maximum) && directorRange(v[1], minimum, maximum) && directorRange(v[2], minimum, maximum)
}
func directorTransform(v DirectorTransform) bool {
	return directorVector(v.Position, -10000, 10000) && directorVector(v.Rotation, -10000, 10000) && directorVector(v.Scale, 0.01, 100)
}
func directorString(s string) bool                    { return utf8.ValidString(s) && utf8.RuneCountInString(s) <= 128 }
func directorChoice(s string, options ...string) bool { return slices.Contains(options, s) }
func directorEasing(s string) bool                    { return directorChoice(s, "", "step", "linear", "smooth") }
func directorQuaternion(v []float64) bool {
	if len(v) != 4 {
		return false
	}
	length := 0.0
	for _, number := range v {
		if !directorRange(number, -1, 1) {
			return false
		}
		length += number * number
	}
	return math.Abs(math.Sqrt(length)-1) < 0.01
}
func directorFrames(frames []DirectorKeyframe) bool {
	if frames == nil || len(frames) > 256 {
		return false
	}
	seen := make(map[uuid.UUID]bool, len(frames))
	for _, frame := range frames {
		if frame.ID == uuid.Nil || seen[frame.ID] || !directorRange(frame.Time, 0, 3600) || !directorTransform(frame.Transform) || !directorEasing(frame.Easing) {
			return false
		}
		seen[frame.ID] = true
	}
	return true
}
func directorIDs[T any](items []T, id func(T) uuid.UUID) (map[uuid.UUID]bool, bool) {
	seen := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		value := id(item)
		if value == uuid.Nil || seen[value] {
			return nil, false
		}
		seen[value] = true
	}
	return seen, true
}
func validDirector(c DirectorConfig) bool {
	if c.ID == uuid.Nil || c.Version != 1 || !validTitle(c.Title) || !subtitleColor.MatchString(c.Background) || !directorRange(c.EnvironmentIntensity, 0, 10) || c.Objects == nil || c.Lights == nil || len(c.Objects) > 128 || len(c.Cameras) < 1 || len(c.Cameras) > 16 || len(c.Lights) > 32 || len(c.Shots) < 1 || len(c.Shots) > 128 || !directorChoice(c.AspectRatio, "", "adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16") || !directorOptionalRange(c.PanoramaRotation, -360, 360) || !directorOptionalRange(c.PanoramaRadius, 1, 200) {
		return false
	}
	if c.Ground != nil && (!directorRange(c.Ground.Opacity, 0, 1) || !directorRange(c.Ground.Height, -2, 2)) {
		return false
	}
	if c.StageTransform != nil && (!directorRange(c.StageTransform.Scale, 0.1, 10) || !directorVector(c.StageTransform.Position, -10000, 10000) || !directorVector(c.StageTransform.Rotation, -10000, 10000)) {
		return false
	}
	if c.Panorama != nil && (c.Panorama.AssetID == uuid.Nil || !directorString(c.Panorama.Name) || !directorRange(c.Panorama.Rotation, -360, 360)) {
		return false
	}
	objects, ok := directorIDs(c.Objects, func(o DirectorObject) uuid.UUID { return o.ID })
	if !ok {
		return false
	}
	cameras, ok := directorIDs(c.Cameras, func(o DirectorCamera) uuid.UUID { return o.ID })
	if !ok {
		return false
	}
	shots, ok := directorIDs(c.Shots, func(o DirectorShot) uuid.UUID { return o.ID })
	if !ok || !shots[c.ActiveShotID] {
		return false
	}
	if _, ok = directorIDs(c.Lights, func(o DirectorLight) uuid.UUID { return o.ID }); !ok {
		return false
	}
	for _, object := range c.Objects {
		if !validDirectorObject(object) {
			return false
		}
	}
	for _, camera := range c.Cameras {
		if !validTitle(camera.Name) || !directorTransform(camera.Transform) || !directorVector(camera.Target, -10000, 10000) || len(camera.FollowAnchor) > 0 && !directorVector(camera.FollowAnchor, -10000, 10000) || !directorChoice(camera.LookAtMode, "", "coordinates", "rotation", "object") || camera.FollowObjectID != nil && !objects[*camera.FollowObjectID] || camera.LookAtObjectID != nil && !objects[*camera.LookAtObjectID] || !directorRange(camera.FocalLength, 1, 1000) || !directorRange(camera.FOV, 1, 179) || !directorRange(camera.Aperture, 0.5, 64) || !directorRange(camera.FocusDistance, 0.01, 10000) || !directorRange(camera.Near, 0.001, 100) || !directorRange(camera.Far, 0.01, 10000) || camera.Near >= camera.Far || !directorFrames(camera.Keyframes) {
			return false
		}
	}
	for _, light := range c.Lights {
		if !validTitle(light.Name) || !directorChoice(light.Type, "directional", "point", "spot", "ambient") || !directorTransform(light.Transform) || !subtitleColor.MatchString(light.Color) || !directorRange(light.Intensity, 0, 100) || !directorOptionalRange(light.Angle, 0.01, math.Pi/2) || !directorOptionalRange(light.Penumbra, 0, 1) {
			return false
		}
	}
	for _, shot := range c.Shots {
		if !validTitle(shot.Name) || !cameras[shot.CameraID] || !directorRange(shot.Duration, 0.1, 3600) || !slices.Contains([]int{24, 25, 30}, shot.FPS) || !directorChoice(shot.ShotSize, "extreme_wide", "wide", "full", "medium", "close_up", "extreme_close_up") || !directorChoice(shot.CameraMove, "static", "push_in", "pull_out", "pan_left", "pan_right", "tilt_up", "tilt_down", "orbit_left", "orbit_right", "handheld") || !validPrompt(shot.Prompt) {
			return false
		}
	}
	raw, err := json.Marshal(c)
	return err == nil && len(raw) <= 512<<10
}

func validDirectorObject(o DirectorObject) bool {
	if !validTitle(o.Name) || !directorChoice(o.Kind, "primitive", "model", "actor", "billboard") || !directorTransform(o.Transform) || !subtitleColor.MatchString(o.Color) || !directorOptionalRange(o.UniformScale, 0.1, 10) || !directorFrames(o.Keyframes) || (o.AssetID != nil && *o.AssetID == uuid.Nil) || (o.SourceNodeID != nil && *o.SourceNodeID == uuid.Nil) || !directorChoice(o.Primitive, "", "box", "sphere", "cylinder", "plane", "character") || !directorChoice(o.BuiltinActor, "", "mannequin") || !directorChoice(o.Pose, "", "neutral", "stand", "t_pose", "walk", "run", "sit", "squat", "kneel_single", "kneel_double", "hands_hips", "lean", "bow", "think", "fight", "kick", "throw", "push", "wave", "reach", "arms_crossed", "phone") {
		return false
	}
	if o.Kind == "primitive" && o.Primitive == "" || o.Kind == "model" && o.AssetID == nil || o.Kind == "actor" && o.AssetID == nil && o.BuiltinActor == "" || o.Kind == "billboard" && o.AssetID == nil && o.SourceNodeID == nil || len(o.MotionClips) > 128 || len(o.BoneOverrides) > len(directorBones) || len(o.BoneTracks) > len(directorBones) {
		return false
	}
	if o.Rig != nil {
		if len(o.Rig.BoneMap) > len(directorBones) || len(o.Rig.AnimationNames) > 128 {
			return false
		}
		for bone, name := range o.Rig.BoneMap {
			if !slices.Contains(directorBones, bone) || !directorString(name) {
				return false
			}
		}
		for _, name := range o.Rig.AnimationNames {
			if !directorString(name) {
				return false
			}
		}
	}
	for bone, rotation := range o.BoneOverrides {
		if !slices.Contains(directorBones, bone) || !directorQuaternion(rotation) {
			return false
		}
	}
	seenBones := make(map[string]bool, len(o.BoneTracks))
	for _, track := range o.BoneTracks {
		if !slices.Contains(directorBones, track.Bone) || seenBones[track.Bone] || len(track.Keyframes) > 256 {
			return false
		}
		seenBones[track.Bone] = true
		seen := make(map[uuid.UUID]bool, len(track.Keyframes))
		for _, frame := range track.Keyframes {
			if frame.ID == uuid.Nil || seen[frame.ID] || !directorRange(frame.Time, 0, 3600) || !directorQuaternion(frame.Rotation) || !directorEasing(frame.Easing) {
				return false
			}
			seen[frame.ID] = true
		}
	}
	clips, ok := directorIDs(o.MotionClips, func(c DirectorMotionClip) uuid.UUID { return c.ID })
	if !ok || o.ActiveMotionClipID != nil && !clips[*o.ActiveMotionClipID] {
		return false
	}
	for _, clip := range o.MotionClips {
		if !validTitle(clip.Name) || !directorString(clip.SourceAnimation) || !directorRange(clip.Start, 0, 3600) || !directorRange(clip.Duration, 0.1, 3600) || clip.Start+clip.Duration > 3600 || !directorRange(clip.PlaybackRate, 0.1, 4) {
			return false
		}
	}
	return true
}

func validDirectorReferences(c DirectorConfig, nodes map[uuid.UUID]Node) bool {
	for _, object := range c.Objects {
		if object.SourceNodeID == nil {
			continue
		}
		node, ok := nodes[*object.SourceNodeID]
		kind := "model"
		if object.Kind == "billboard" {
			kind = "image"
		}
		if !ok || node.NodeType != kind || node.RefType != "media_asset" || node.RefID == nil || object.AssetID != nil && *object.AssetID != *node.RefID {
			return false
		}
	}
	return true
}

func clearDirectorReferences(nodes []Node, removed map[uuid.UUID]bool) {
	assets := make(map[uuid.UUID]*uuid.UUID)
	for _, node := range nodes {
		if removed[node.ID] {
			assets[node.ID] = node.RefID
		}
	}
	for i, node := range nodes {
		if node.Config.Director == nil {
			continue
		}
		config := *node.Config.Director
		config.Objects = slices.Clone(config.Objects)
		for j, object := range config.Objects {
			if object.SourceNodeID != nil && removed[*object.SourceNodeID] {
				config.Objects[j].AssetID = assets[*object.SourceNodeID]
				config.Objects[j].SourceNodeID = nil
			}
		}
		nodes[i].Config.Director = &config
	}
}
