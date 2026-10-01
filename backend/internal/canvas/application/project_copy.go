package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

// ProjectCopyBinding is a trusted workspace admission's immutable module binding.
// It is never accepted directly from a public canvas request.
type ProjectCopyBinding struct {
	JobID, OrgID, SourceProjectID, TargetProjectID uuid.UUID
}

// Validate rejects identities that could reuse another project's content.
func (b ProjectCopyBinding) Validate() error {
	if b.JobID == uuid.Nil || b.OrgID == uuid.Nil || b.SourceProjectID == uuid.Nil || b.TargetProjectID == uuid.Nil || b.SourceProjectID == b.TargetProjectID {
		return domain.ErrInvalidProjectCopy
	}
	return nil
}

// ProjectCopySnapshot identifies immutable canvas-owned frozen documents.
type ProjectCopySnapshot struct {
	ID             uuid.UUID
	ManifestSHA256 string
	Documents      int
}

// ProjectCopyReceipt proves the exact frozen set and deterministic target graphs.
type ProjectCopyReceipt struct {
	ManifestSHA256           string
	ContentSHA256            string
	Documents                int
	ClearedOperationBindings int
}

// PrepareProjectCopy validates every frozen document and all its media mappings.
// The document limit bounds one project's atomic graph snapshot without truncation.
func PrepareProjectCopy(binding ProjectCopyBinding, documents []domain.Document, assets map[uuid.UUID]uuid.UUID) ([]domain.ProjectDocumentCopy, string, error) {
	if err := binding.Validate(); err != nil {
		return nil, "", err
	}
	if len(documents) > 256 {
		return nil, "", fmt.Errorf("%w: project document limit", domain.ErrUnsupportedProjectCopy)
	}
	result := make([]domain.ProjectDocumentCopy, 0, len(documents))
	seen := make(map[uuid.UUID]bool, len(documents))
	for _, document := range documents {
		if document.ProjectID != binding.SourceProjectID || seen[document.ID] {
			return nil, "", domain.ErrInvalidProjectCopy
		}
		seen[document.ID] = true
		copied, err := domain.CopyProjectDocument(document, binding.TargetProjectID, binding.JobID, assets)
		if err != nil {
			return nil, "", err
		}
		result = append(result, copied)
	}
	body, err := json.Marshal(result)
	if err != nil || len(body) > 32<<20 {
		return nil, "", fmt.Errorf("%w: project canvas metadata limit", domain.ErrUnsupportedProjectCopy)
	}
	hash := sha256.Sum256(body)
	return result, hex.EncodeToString(hash[:]), nil
}
