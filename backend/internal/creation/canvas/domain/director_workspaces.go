package domain

import (
	"encoding/json"
	"reflect"

	"github.com/google/uuid"
)

// DirectorSceneData is one independently editable scene. Legacy version one
// fields remain on DirectorConfig only for lossless persisted-document reads.
type DirectorSceneData struct {
	Exposure             *float64                `json:"exposure,omitempty"`
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
	Cover                *DirectorCover          `json:"cover,omitempty"`
}

// DirectorWorkspaces keeps static posing separate from animated rehearsal.
type DirectorWorkspaces struct {
	Static  *DirectorSceneData `json:"static"`
	Dynamic *DirectorSceneData `json:"dynamic"`
}

func directorSceneData(c DirectorConfig) *DirectorSceneData {
	return &DirectorSceneData{Exposure: c.Exposure, Background: c.Background, EnvironmentIntensity: c.EnvironmentIntensity, GridVisible: c.GridVisible, GridSnap: c.GridSnap, Ground: c.Ground, StageTransform: c.StageTransform, LabelsVisible: c.LabelsVisible, AspectRatio: c.AspectRatio, Panorama: c.Panorama, PanoramaRotation: c.PanoramaRotation, PanoramaRadius: c.PanoramaRadius, Objects: c.Objects, Cameras: c.Cameras, Lights: c.Lights, Shots: c.Shots, ActiveShotID: c.ActiveShotID, Cover: c.Cover}
}
func directorSceneConfig(c DirectorConfig, scene *DirectorSceneData, mode string) DirectorConfig {
	return DirectorConfig{Exposure: scene.Exposure, ID: c.ID, Title: c.Title, Version: 1, Mode: mode, Background: scene.Background, EnvironmentIntensity: scene.EnvironmentIntensity, GridVisible: scene.GridVisible, GridSnap: scene.GridSnap, Ground: scene.Ground, StageTransform: scene.StageTransform, LabelsVisible: scene.LabelsVisible, AspectRatio: scene.AspectRatio, Panorama: scene.Panorama, PanoramaRotation: scene.PanoramaRotation, PanoramaRadius: scene.PanoramaRadius, Objects: scene.Objects, Cameras: scene.Cameras, Lights: scene.Lights, Shots: scene.Shots, ActiveShotID: scene.ActiveShotID, Cover: scene.Cover}
}

// DirectorScenes visits both workspaces, including inactive asset references.
// Returned scenes are read-only views; callers that edit must clone the document.
func DirectorScenes(c DirectorConfig) []DirectorConfig {
	if c.Workspaces == nil {
		return []DirectorConfig{c}
	}
	scenes := make([]DirectorConfig, 0, 2)
	if c.Workspaces.Static != nil {
		scenes = append(scenes, directorSceneConfig(c, c.Workspaces.Static, "static"))
	}
	if c.Workspaces.Dynamic != nil {
		scenes = append(scenes, directorSceneConfig(c, c.Workspaces.Dynamic, "dynamic"))
	}
	return scenes
}
func updateDirectorScenes(c *DirectorConfig, change func(*DirectorConfig)) {
	if c.Workspaces == nil {
		change(c)
		return
	}
	workspaces := *c.Workspaces
	if workspaces.Static != nil {
		scene := directorSceneConfig(*c, workspaces.Static, "static")
		change(&scene)
		workspaces.Static = directorSceneData(scene)
	}
	if workspaces.Dynamic != nil {
		scene := directorSceneConfig(*c, workspaces.Dynamic, "dynamic")
		change(&scene)
		workspaces.Dynamic = directorSceneData(scene)
	}
	c.Workspaces = &workspaces
}
func validDirectorWorkspaces(c DirectorConfig) bool {
	if c.ID == uuid.Nil || !validTitle(c.Title) || !directorChoice(c.Mode, "static", "dynamic") || c.Workspaces == nil || c.Workspaces.Static == nil || c.Workspaces.Dynamic == nil {
		return false
	}
	legacy := c
	legacy.ID = uuid.Nil
	legacy.Title = ""
	legacy.Version = 0
	legacy.Mode = ""
	legacy.Workspaces = nil
	if !reflect.ValueOf(legacy).IsZero() {
		return false
	}
	for _, scene := range DirectorScenes(c) {
		if !validDirector(scene) {
			return false
		}
	}
	raw, err := json.Marshal(c)
	return err == nil && len(raw) <= 512<<10
}

// MarshalJSON never emits obsolete flat scene fields alongside version two.
func (c DirectorConfig) MarshalJSON() ([]byte, error) {
	if c.Version == 2 {
		return json.Marshal(struct {
			ID         uuid.UUID           `json:"id"`
			Version    int                 `json:"version"`
			Title      string              `json:"title"`
			Mode       string              `json:"mode"`
			Workspaces *DirectorWorkspaces `json:"workspaces"`
		}{c.ID, c.Version, c.Title, c.Mode, c.Workspaces})
	}
	type legacy DirectorConfig
	return json.Marshal(legacy(c))
}
