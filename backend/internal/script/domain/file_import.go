package domain

import "github.com/google/uuid"

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
func ImportOutcome(succeeded, failed int) string {
	if succeeded == 0 {
		return "failed"
	}
	if failed != 0 {
		return "partial"
	}
	return "succeeded"
}

// CanRetryImport requires explicit failed items and proven cessation of all I/O.
func CanRetryImport(status string, activeIO, uncertainIO, uncertainObjects bool, failures int) bool {
	return (status == "partial" || status == "failed") && !activeIO && !uncertainIO && !uncertainObjects && failures > 0
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
