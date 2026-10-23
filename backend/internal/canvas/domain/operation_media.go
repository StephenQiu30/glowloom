package domain

import (
	"bytes"
	"encoding/json"

	"github.com/google/uuid"
)

// MediaRef identifies immutable uploaded content; preview URLs are not identity.
type MediaRef struct {
	Empty   bool      `json:"empty,omitempty"`
	AssetID uuid.UUID `json:"asset_id"`
}

// UnmarshalJSON rejects unversioned additions instead of dropping input.
func (ref *MediaRef) UnmarshalJSON(body []byte) error {
	type wire MediaRef
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*ref = MediaRef(decoded)
	return nil
}

func replaceMedia(document *Document, op Operation) (FieldChange, error) {
	index := nodeIndex(document, op.NodeID)
	if index < 0 {
		return FieldChange{}, operationError("target_not_found")
	}
	node := &document.Nodes[index]
	if !editable(*node) || (editorNodeType(*node) != "image" && editorNodeType(*node) != "video" && editorNodeType(*node) != "audio") {
		return FieldChange{}, operationError("protected_target")
	}
	empty := node.RefID == nil
	if empty && node.Config.Editor == nil {
		return FieldChange{}, operationError("protected_target")
	}
	if empty != op.ExpectedMediaRef.Empty || !empty && *node.RefID != op.ExpectedMediaRef.AssetID {
		return FieldChange{}, operationError("media_conflict")
	}
	before := effectJSON(op.ExpectedMediaRef)
	asset := op.NewMediaRef.AssetID
	if op.NewMediaRef.Empty {
		if node.Config.Editor == nil {
			return FieldChange{}, operationError("protected_target")
		}
		node.RefID = nil
		node.RefType = ""
	} else {
		node.RefID = &asset
		node.RefType = "media_asset"
	}
	return FieldChange{NodeID: node.ID, Path: []string{"media_ref"}, Before: before, After: effectJSON(op.NewMediaRef), BeforeExists: true, AfterExists: true}, nil
}
