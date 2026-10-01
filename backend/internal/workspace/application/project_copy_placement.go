package application

import (
	"math"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// CopyPlacementExpectation is an optional complete CAS of the source's personal classification.
// Absence freezes the classification observed in the admission transaction.
type CopyPlacementExpectation struct {
	ExpectedPlacementRevision int64      `json:"expected_placement_revision"`
	FolderID                  *uuid.UUID `json:"folder_id"`
	ExpectedFolderRevision    int64      `json:"expected_folder_revision"`
}

// Validate rejects incomplete identity and revision combinations before persistence.
func (e CopyPlacementExpectation) Validate() error {
	if e.ExpectedPlacementRevision < 0 || e.ExpectedPlacementRevision > math.MaxInt32 || e.ExpectedFolderRevision < 0 || e.ExpectedFolderRevision >= math.MaxInt32 || e.FolderID == nil && e.ExpectedFolderRevision != 0 || e.FolderID != nil && (*e.FolderID == uuid.Nil || e.ExpectedPlacementRevision < 1 || e.ExpectedFolderRevision < 1) {
		return domain.ErrInvalidProjectCopy
	}
	return nil
}
