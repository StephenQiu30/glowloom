package domain

import "github.com/google/uuid"

// ImportStatus records the durable publication outcome of a script import job.
type ImportStatus string

// Import job states retain partial, cancelled and complete publication outcomes.
const (
	ImportStatusQueued          ImportStatus = "queued"
	ImportStatusRunning         ImportStatus = "running"
	ImportStatusPartial         ImportStatus = "partial"
	ImportStatusSucceeded       ImportStatus = "succeeded"
	ImportStatusFailed          ImportStatus = "failed"
	ImportStatusCancelRequested ImportStatus = "cancel_requested"
	ImportStatusCancelled       ImportStatus = "cancelled"
)

// ImportStage names the processing or recovery step of a script import job.
type ImportStage string

// Import stages expose the current processing or reconciliation step.
const (
	ImportStageQueued                 ImportStage = "queued"
	ImportStageExtracting             ImportStage = "extracting"
	ImportStageNormalizing            ImportStage = "normalizing"
	ImportStageStoring                ImportStage = "storing"
	ImportStageCommitting             ImportStage = "committing"
	ImportStageCompleted              ImportStage = "completed"
	ImportStageFailed                 ImportStage = "failed"
	ImportStageCancelling             ImportStage = "cancelling"
	ImportStageAwaitingReconciliation ImportStage = "awaiting_reconciliation"
)

// ImportFileStatus records the outcome of one frozen original document.
type ImportFileStatus string

// Import file states record the outcome of each frozen original.
const (
	ImportFileStatusQueued    ImportFileStatus = "queued"
	ImportFileStatusSucceeded ImportFileStatus = "succeeded"
	ImportFileStatusFailed    ImportFileStatus = "failed"
)

// ImportIOState records ownership and proven cessation of remote object I/O.
type ImportIOState string

// Import I/O states distinguish active, ended and unconfirmed remote operations.
const (
	ImportIOStateIdle    ImportIOState = "idle"
	ImportIOStateRunning ImportIOState = "running"
	ImportIOStateEnded   ImportIOState = "ended"
	ImportIOStateUnknown ImportIOState = "unknown"
)

// ImportAction is an admitted command for script import execution or recovery.
type ImportAction string

// Import actions are the accepted admission, execution and explicit recovery commands.
const (
	ImportActionCreate    ImportAction = "create"
	ImportActionStart     ImportAction = "start"
	ImportActionCancel    ImportAction = "cancel"
	ImportActionRetry     ImportAction = "retry"
	ImportActionReconcile ImportAction = "reconcile"
)

// ValidateImportFiles bounds the ordered frozen originals independently of chapters.
func ValidateImportFiles(files []uuid.UUID, rights bool) error {
	if !rights || len(files) < 1 || len(files) > 200 {
		return ErrInvalidSource
	}
	seen := make(map[uuid.UUID]bool, len(files))
	for _, id := range files {
		if id == uuid.Nil || seen[id] {
			return ErrInvalidSource
		}
		seen[id] = true
	}
	return nil
}

// ImportOutcome reports all failed originals rather than hiding partial publication.
func ImportOutcome(succeeded, failed int) ImportStatus {
	if succeeded == 0 {
		return ImportStatusFailed
	}
	if failed != 0 {
		return ImportStatusPartial
	}
	return ImportStatusSucceeded
}

// CanRetryImport requires explicit failed items and proven cessation of all I/O.
func CanRetryImport(status ImportStatus, activeIO, uncertainIO, uncertainObjects bool, failures int) bool {
	return (status == ImportStatusPartial || status == ImportStatusFailed) && !activeIO && !uncertainIO && !uncertainObjects && failures > 0
}

// OrderImportSources inserts newly successful files at their original frozen
// relative positions while retaining every unrelated current source in order.
func OrderImportSources(current, frozen []uuid.UUID) ([]uuid.UUID, error) {
	member := make(map[uuid.UUID]bool, len(frozen))
	present := make(map[uuid.UUID]bool, len(current))
	for _, id := range frozen {
		if id == uuid.Nil || member[id] {
			return nil, ErrInvalidSource
		}
		member[id] = true
	}
	for _, id := range current {
		if id == uuid.Nil || present[id] {
			return nil, ErrInvalidSource
		}
		present[id] = true
	}
	result := make([]uuid.UUID, 0, len(current))
	inserted := false
	for _, id := range current {
		if !member[id] {
			result = append(result, id)
			continue
		}
		if inserted {
			continue
		}
		inserted = true
		for _, fileID := range frozen {
			if present[fileID] {
				result = append(result, fileID)
			}
		}
	}
	return result, nil
}
