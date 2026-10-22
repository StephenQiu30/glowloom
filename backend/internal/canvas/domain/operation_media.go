package domain

import (
	"bytes"
	"encoding/json"

	"github.com/google/uuid"
)

// MediaRef identifies immutable uploaded content; preview URLs are not identity.
type MediaRef struct {
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
	if !editable(*node) || node.RefType != "media_asset" || node.RefID == nil || (node.NodeType != "image" && node.NodeType != "video" && node.NodeType != "audio") {
		return FieldChange{}, operationError("protected_target")
	}
	if *node.RefID != op.ExpectedMediaRef.AssetID {
		return FieldChange{}, operationError("media_conflict")
	}
	before := effectJSON(op.ExpectedMediaRef)
	asset := op.NewMediaRef.AssetID
	node.RefID = &asset
	return FieldChange{NodeID: node.ID, Path: []string{"media_ref"}, Before: before, After: effectJSON(op.NewMediaRef), BeforeExists: true, AfterExists: true}, nil
}
