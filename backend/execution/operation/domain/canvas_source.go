package domain

import "github.com/google/uuid"

// CanvasSource freezes only the generating node identity and document revision.
// Result media and lifecycle remain operation facts rather than editable node data.
type CanvasSource struct {
	CanvasID uuid.UUID  `json:"canvas_id"`
	NodeID   uuid.UUID  `json:"node_id"`
	RowID    *uuid.UUID `json:"row_id,omitempty" extensions:"x-nullable"`
	Revision int64      `json:"revision"`
}

// Valid reports whether the frozen source identity has a complete stable scope.
func (s CanvasSource) Valid() bool {
	return s.CanvasID != uuid.Nil && s.NodeID != uuid.Nil && s.Revision > 0 && (s.RowID == nil || *s.RowID != uuid.Nil)
}
