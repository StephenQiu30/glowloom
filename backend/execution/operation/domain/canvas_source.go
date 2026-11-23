package domain

import "github.com/google/uuid"

// CanvasSource freezes the generating node, its document revision, and any row or shot.
// Result media and lifecycle remain operation facts rather than editable node data.
type CanvasSource struct {
	CanvasID     uuid.UUID  `json:"canvas_id"`
	NodeID       uuid.UUID  `json:"node_id"`
	RowID        *uuid.UUID `json:"row_id,omitempty" extensions:"x-nullable"`
	Revision     int64      `json:"revision"`
	ShotID       *uuid.UUID `json:"shot_id,omitempty" extensions:"x-nullable"`
	ShotRevision *int64     `json:"shot_revision,omitempty" extensions:"x-nullable"`
}

// Valid reports whether the frozen source identity has a complete stable scope.
func (s CanvasSource) Valid() bool {
	if s.CanvasID == uuid.Nil || s.NodeID == uuid.Nil || s.Revision <= 0 ||
		(s.RowID != nil && *s.RowID == uuid.Nil) {
		return false
	}
	if s.ShotID == nil {
		return s.ShotRevision == nil
	}
	return s.RowID == nil && *s.ShotID != uuid.Nil && s.ShotRevision != nil && *s.ShotRevision > 0
}
