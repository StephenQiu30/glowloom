package http

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectCopyPlacementRequest requires a complete observed classification, including explicit root.
type ProjectCopyPlacementRequest struct {
	ExpectedPlacementRevision *int64     `json:"expected_placement_revision" binding:"required"`
	FolderID                  *uuid.UUID `json:"folder_id" extensions:"x-nullable"`
	ExpectedFolderRevision    *int64     `json:"expected_folder_revision" binding:"required"`
}

// UnmarshalJSON closes the optional classification block and distinguishes root from a missing field.
func (r *ProjectCopyPlacementRequest) UnmarshalJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return domain.ErrInvalidProjectCopy
	}
	type fields ProjectCopyPlacementRequest
	var value fields
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&value) != nil || d.Decode(new(any)) != io.EOF {
		return domain.ErrInvalidProjectCopy
	}
	var present map[string]json.RawMessage
	if json.Unmarshal(raw, &present) != nil || present == nil {
		return domain.ErrInvalidProjectCopy
	}
	if _, ok := present["folder_id"]; !ok || value.ExpectedPlacementRevision == nil || value.ExpectedFolderRevision == nil {
		return domain.ErrInvalidProjectCopy
	}
	*r = ProjectCopyPlacementRequest(value)
	if r.expectation().Validate() != nil {
		return domain.ErrInvalidProjectCopy
	}
	return nil
}
func (r ProjectCopyPlacementRequest) expectation() application.CopyPlacementExpectation {
	return application.CopyPlacementExpectation{ExpectedPlacementRevision: *r.ExpectedPlacementRevision, FolderID: r.FolderID, ExpectedFolderRevision: *r.ExpectedFolderRevision}
}
