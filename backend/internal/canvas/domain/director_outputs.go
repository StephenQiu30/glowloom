package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// DirectorScreenshot retains a gallery entry by durable media identity.
// CreatedAt is user supplied presentation metadata, not execution evidence.
type DirectorScreenshot struct {
	ID        uuid.UUID `json:"id"`
	AssetID   uuid.UUID `json:"asset_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// DirectorCover selects a private image and its existing scene shot.
type DirectorCover struct {
	AssetID uuid.UUID `json:"asset_id"`
	ShotID  uuid.UUID `json:"shot_id"`
}

var directorTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]([01]\d|2[0-3]):[0-5]\d)$`)

// UnmarshalJSON rejects unknown output facts, invalid UTF-8 and non-RFC3339
// syntax before encoding/json or time.Parse can normalize those inputs.
func (s *DirectorScreenshot) UnmarshalJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("%w: director screenshot encoding", ErrInvalidCommand)
	}
	var fields struct {
		ID        uuid.UUID `json:"id"`
		AssetID   uuid.UUID `json:"asset_id"`
		Name      string    `json:"name"`
		CreatedAt string    `json:"created_at"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fields); err != nil {
		return fmt.Errorf("decode director screenshot: %w", err)
	}
	if !directorTimestamp.MatchString(fields.CreatedAt) {
		return fmt.Errorf("%w: director screenshot timestamp", ErrInvalidCommand)
	}
	createdAt, err := time.Parse(time.RFC3339, fields.CreatedAt)
	if err != nil {
		return fmt.Errorf("parse director screenshot timestamp: %w", err)
	}
	*s = DirectorScreenshot{ID: fields.ID, AssetID: fields.AssetID, Name: fields.Name, CreatedAt: createdAt}
	return nil
}

func validDirectorOutputs(config DirectorConfig, shots map[uuid.UUID]bool) bool {
	if config.Cover != nil && (config.Cover.AssetID == uuid.Nil || !shots[config.Cover.ShotID]) {
		return false
	}
	seen := make(map[uuid.UUID]bool)
	for _, shot := range config.Shots {
		if len(shot.Screenshots) > 64 || len(seen)+len(shot.Screenshots) > 512 {
			return false
		}
		for _, screenshot := range shot.Screenshots {
			if screenshot.ID == uuid.Nil || seen[screenshot.ID] || screenshot.AssetID == uuid.Nil || !utf8.ValidString(screenshot.Name) || strings.TrimSpace(screenshot.Name) == "" || utf8.RuneCountInString(screenshot.Name) > 128 || strings.ContainsRune(screenshot.Name, '\x00') || screenshot.CreatedAt.IsZero() {
				return false
			}
			// MarshalJSON enforces the RFC3339 year and timezone bounds even for
			// application callers that did not enter through the JSON decoder.
			if _, err := screenshot.CreatedAt.MarshalJSON(); err != nil {
				return false
			}
			seen[screenshot.ID] = true
		}
	}
	return true
}
