package domain

import (
	"encoding/json"
	"strings"
)

func timelineGate(object map[string]any, op Operation) error {
	if len(op.Path) < 3 || op.Path[0] != "config" || op.Path[1] != "timeline" {
		return nil
	}
	parent, _, err := fieldContainer(object, []string{"config", "timeline", "tracks"})
	if err != nil {
		return err
	}
	tracks, _ := parent["tracks"].([]any)
	clips, _ := parent["clips"].([]any)
	locked := func(id string) bool {
		index := collectionIndex(tracks, id)
		if index < 0 {
			return false
		}
		return tracks[index].(map[string]any)["locked"] == true
	}
	if op.Path[2] == "tracks" && op.Type == "remove" && locked(op.ItemID) {
		for _, item := range clips {
			if item.(map[string]any)["track_id"] == op.ItemID {
				return operationError("track_locked")
			}
		}
	}
	if op.Path[2] != "clips" {
		return nil
	}
	id := op.ItemID
	if len(op.Path) > 3 {
		id = strings.TrimPrefix(op.Path[3], "@")
	}
	if index := collectionIndex(clips, id); index >= 0 {
		clip := clips[index].(map[string]any)
		track, _ := clip["track_id"].(string)
		if locked(track) {
			return operationError("track_locked")
		}
	}
	if op.Value != nil {
		var value map[string]any
		if json.Unmarshal(op.Value, &value) == nil {
			if track, ok := value["track_id"].(string); ok && locked(track) {
				return operationError("track_locked")
			}
		}
	}
	return nil
}
