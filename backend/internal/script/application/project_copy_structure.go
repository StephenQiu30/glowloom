package application

import (
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// validateCopyStructures closes the typed documents against every materialized row.
// Counts alone cannot prove that historical dialogue/action text was retained.
func validateCopyStructures(h ProjectCopyHistory) error {
	episodes := make(map[uuid.UUID]domain.Episode, len(h.Episodes))
	for _, e := range h.Episodes {
		episodes[e.ID] = e
	}
	type sceneIdentity struct{ structure, key uuid.UUID }
	type itemIdentity struct{ scene, key uuid.UUID }
	scenes := make(map[sceneIdentity]ProjectCopyScene, len(h.Scenes))
	for _, s := range h.Scenes {
		identity := sceneIdentity{s.StructureID, s.SceneKey}
		if _, exists := scenes[identity]; exists {
			return ErrObjectMismatch
		}
		scenes[identity] = s
	}
	dialogue := make(map[itemIdentity]ProjectCopyDialogue, len(h.Dialogue))
	for _, d := range h.Dialogue {
		identity := itemIdentity{d.SceneID, d.LineKey}
		if _, exists := dialogue[identity]; exists {
			return ErrObjectMismatch
		}
		dialogue[identity] = d
	}
	actions := make(map[itemIdentity]ProjectCopyAction, len(h.Actions))
	for _, a := range h.Actions {
		identity := itemIdentity{a.SceneID, a.LineKey}
		if _, exists := actions[identity]; exists {
			return ErrObjectMismatch
		}
		actions[identity] = a
	}
	for _, s := range h.Structures {
		e := episodes[s.EpisodeID]
		if err := s.Document.Validate(e.Start, e.End); err != nil {
			return err
		}
		for _, scene := range s.Document.Scenes {
			identity := sceneIdentity{s.ID, scene.Key}
			row, exists := scenes[identity]
			if !exists || row.SeqNo != scene.SeqNo || row.Heading != scene.Heading || row.LocationText != scene.LocationText || row.TimeOfDay != scene.TimeOfDay || row.Start != scene.Start || row.End != scene.End {
				return ErrObjectMismatch
			}
			delete(scenes, identity)
			for i, item := range scene.Items {
				key := itemIdentity{row.ID, item.Key}
				if item.Type == "line" {
					line, exists := dialogue[key]
					if !exists || line.SeqNo != i+1 || line.Kind != item.Kind || line.Content != item.Content || line.Speaker != item.Speaker || line.Emotion != item.Emotion || line.Start != item.Start || line.End != item.End || line.ContentHash != domain.ContentSHA([]byte(item.Content)) || !sameStructureCharacter(line.CharacterID, item.CharacterID) {
						return ErrObjectMismatch
					}
					delete(dialogue, key)
				} else {
					action, exists := actions[key]
					if !exists || action.SeqNo != i+1 || action.Content != item.Content || action.Start != item.Start || action.End != item.End {
						return ErrObjectMismatch
					}
					delete(actions, key)
				}
			}
		}
	}
	if len(scenes) != 0 || len(dialogue) != 0 || len(actions) != 0 {
		return ErrObjectMismatch
	}
	return nil
}

func sameStructureCharacter(a, b *uuid.UUID) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
