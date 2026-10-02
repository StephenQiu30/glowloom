package domain

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxStructureBytes bounds a complete manual structure independently from sources.
const MaxStructureBytes = 1 << 20

// DecodeStructure checks the closed schema before accepting editable scene facts.
func DecodeStructure(data []byte, episodeStart, episodeEnd int) (StructureDocument, error) {
	if len(data) > MaxStructureBytes || ValidateJSON(data) != nil {
		return StructureDocument{}, ErrInvalidStructure
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result StructureDocument
	if err := decoder.Decode(&result); err != nil {
		return StructureDocument{}, ErrInvalidStructure
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return StructureDocument{}, ErrInvalidStructure
	}
	if err := result.Validate(episodeStart, episodeEnd); err != nil {
		return StructureDocument{}, err
	}
	return result, nil
}

func validStructureText(text string) bool {
	return utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}

// Validate enforces exact source coordinates, scene/item order and stable-key uniqueness.
// Editable content may differ from its source span; the original remains immutable.
func (d StructureDocument) Validate(episodeStart, episodeEnd int) error {
	if episodeStart < 0 || episodeEnd <= episodeStart || episodeEnd > MaxScalarCount || len(d.Scenes)+len(d.Unassigned) == 0 {
		return ErrInvalidStructure
	}
	bytes, err := json.Marshal(d)
	if err != nil || len(bytes) > MaxStructureBytes {
		return ErrInvalidStructure
	}
	keys := make(map[uuid.UUID]bool)
	assigned := make([]ScalarSpan, 0)
	priorSceneEnd := episodeStart
	for i, scene := range d.Scenes {
		if scene.Key == uuid.Nil || keys[scene.Key] || scene.SeqNo != i+1 || scene.Start < priorSceneEnd || scene.End <= scene.Start || scene.End > episodeEnd || !validStructureText(scene.Heading) || !validStructureText(scene.LocationText) || !validStructureText(scene.TimeOfDay) {
			return ErrInvalidStructure
		}
		keys[scene.Key] = true
		priorSceneEnd = scene.End
		priorItemEnd := scene.Start
		for _, item := range scene.Items {
			if item.Key == uuid.Nil || keys[item.Key] || item.Start < priorItemEnd || item.End <= item.Start || item.End > scene.End || !validStructureText(item.Content) || item.Content == "" || !validStructureText(item.Speaker) || !validStructureText(item.Emotion) || item.CharacterID != nil && *item.CharacterID == uuid.Nil {
				return ErrInvalidStructure
			}
			if item.Type == "action" {
				if item.Kind != "" || item.Speaker != "" || item.Emotion != "" || item.CharacterID != nil {
					return ErrInvalidStructure
				}
			} else if item.Type != "line" || !slices.Contains([]string{"dialogue", "voiceover", "inner"}, item.Kind) {
				return ErrInvalidStructure
			}
			keys[item.Key] = true
			priorItemEnd = item.End
			assigned = append(assigned, ScalarSpan{item.Start, item.End})
		}
	}
	for _, line := range d.Unassigned {
		if line.Key == uuid.Nil || keys[line.Key] || line.Start < episodeStart || line.End <= line.Start || line.End > episodeEnd || line.Content == "" || !validStructureText(line.Content) {
			return ErrInvalidStructure
		}
		keys[line.Key] = true
		assigned = append(assigned, ScalarSpan{line.Start, line.End})
	}
	slices.SortFunc(assigned, func(a, b ScalarSpan) int { return a.Start - b.Start })
	for i := 1; i < len(assigned); i++ {
		if assigned[i].Start < assigned[i-1].End {
			return ErrInvalidStructure
		}
	}
	return nil
}
