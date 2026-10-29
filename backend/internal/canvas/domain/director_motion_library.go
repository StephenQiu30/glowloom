package domain

import (
	"math"
	"strings"

	"github.com/google/uuid"
)

// DirectorMotionTrack stores linear samples for one bone transform channel.
type DirectorMotionTrack struct {
	Bone     string    `json:"bone"`
	Property string    `json:"property"`
	Times    []float64 `json:"times"`
	Values   []float64 `json:"values"`
}

// DirectorMotionSource stores an imported action or captured pose with its source rest transforms.
type DirectorMotionSource struct {
	ID       uuid.UUID                    `json:"id"`
	Name     string                       `json:"name"`
	Kind     string                       `json:"kind"`
	Duration float64                      `json:"duration"`
	Tracks   []DirectorMotionTrack        `json:"tracks"`
	Rest     map[string]DirectorTransform `json:"rest"`
}

func validDirectorMotionLibrary(o DirectorObject) bool {
	if len(o.MotionLibrary) > 16 || len(o.MotionLibrary) > 0 && !directorChoice(o.Kind, "actor", "model") {
		return false
	}
	ids := map[uuid.UUID]bool{}
	for _, source := range o.MotionLibrary {
		if source.ID == uuid.Nil || ids[source.ID] || !validTitle(source.Name) || !directorChoice(source.Kind, "import", "pose") || !directorRange(source.Duration, .1, 60) || len(source.Tracks) == 0 || len(source.Tracks) > 256 || len(source.Rest) > 256 {
			return false
		}
		ids[source.ID] = true
		channels := map[string]bool{}
		points := 0
		for _, track := range source.Tracks {
			key := track.Bone + ":" + track.Property
			rest, ok := source.Rest[track.Bone]
			if !directorString(track.Bone) || channels[key] || !ok || !directorChoice(track.Property, "position", "quaternion", "scale") || !directorVector(rest.Position, -10000, 10000) || !directorVector(rest.Rotation, -10000, 10000) || !directorVector(rest.Scale, -10000, 10000) || len(track.Times) == 0 || len(track.Times) > 2048 {
				return false
			}
			for _, v := range rest.Scale {
				if math.Abs(v) < .000001 {
					return false
				}
			}
			width := 3
			if track.Property == "quaternion" {
				width = 4
			}
			if len(track.Values) != len(track.Times)*width {
				return false
			}
			channels[key] = true
			points += len(track.Times)
			for i, t := range track.Times {
				values := track.Values[i*width : (i+1)*width]
				if !directorRange(t, 0, source.Duration+.0001) || i > 0 && t <= track.Times[i-1] {
					return false
				}
				if width == 4 {
					if !directorQuaternion(values) {
						return false
					}
				} else if !directorVector(values, -10000, 10000) {
					return false
				}
			}
		}
		if points > 8192 {
			return false
		}
	}
	for _, clip := range o.MotionClips {
		if strings.HasPrefix(clip.SourceAnimation, "user:") {
			id, err := uuid.Parse(strings.TrimPrefix(clip.SourceAnimation, "user:"))
			if err != nil || !ids[id] {
				return false
			}
		}
		if !directorRange(clip.BlendIn, 0, clip.Duration) {
			return false
		}
		if clip.BlendIn == 0 {
			continue
		}
		previous := 0
		for _, candidate := range o.MotionClips {
			if candidate.ID == clip.ID {
				continue
			}
			if math.Abs(candidate.Start+candidate.Duration-clip.Start) < .000001 && candidate.Duration >= clip.BlendIn {
				previous++
			}
			if candidate.Start < clip.Start+clip.BlendIn && candidate.Start+candidate.Duration > clip.Start+.000001 {
				return false
			}
		}
		if previous != 1 {
			return false
		}
	}
	return true
}
