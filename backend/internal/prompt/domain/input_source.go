package domain

import (
	"github.com/google/uuid"
)

// InputSource selects a saved version, never an inline alternate generation intent.
type InputSource struct {
	Kind           string    `json:"kind"`
	DraftID        uuid.UUID `json:"draft_id,omitempty"`
	Revision       int64     `json:"revision,omitempty"`
	CanvasID       uuid.UUID `json:"canvas_id,omitempty"`
	NodeID         uuid.UUID `json:"node_id,omitempty"`
	CanvasRevision int64     `json:"canvas_revision,omitempty"`
	InputRevision  int64     `json:"input_revision,omitempty"`
}

// Valid checks mutually exclusive owner fields and positive revisions.
func (s InputSource) Valid() bool {
	if s.Kind == "prompt_draft" {
		return s.DraftID != uuid.Nil && s.Revision > 0 && s.CanvasID == uuid.Nil && s.NodeID == uuid.Nil && s.CanvasRevision == 0 && s.InputRevision == 0
	}
	return s.Kind == "canvas_input" && s.DraftID == uuid.Nil && s.Revision == 0 && s.CanvasID != uuid.Nil && s.NodeID != uuid.Nil && s.CanvasRevision > 0 && s.InputRevision > 0
}

// OwnerKey scopes the persistent submission slot independently of revisions.
func (s InputSource) OwnerKey() string {
	if s.Kind == "prompt_draft" {
		return "draft/" + s.DraftID.String()
	}
	return "canvas/" + s.CanvasID.String() + "/" + s.NodeID.String()
}
