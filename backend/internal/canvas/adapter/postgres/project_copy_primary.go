package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

// InspectPrimary validates historical evidence without comparing live content to
// the frozen copy: a published target may legitimately have been edited since.
// The coordinator must already hold the source/target project and job locks.
func (s *ProjectCopyStore) InspectPrimary(ctx context.Context, binding application.ProjectCopyBinding, expected application.ProjectCopySnapshot) (application.ProjectCopyPrimaryProof, error) {
	if s == nil || s.db == nil || binding.Validate() != nil || expected.ID == uuid.Nil {
		return application.ProjectCopyPrimaryProof{}, domain.ErrInvalidProjectCopy
	}
	tx := s.db.WithContext(ctx)
	var row struct {
		ManifestSHA256 string
		DocumentCount  int
		Documents      []byte
		AssetMapping   []byte
	}
	read := tx.Raw(`SELECT manifest_sha256,document_count,documents,asset_mapping FROM canvas.project_copy_snapshot WHERE id=? AND job_id=? AND org_id=? AND source_project_id=? AND target_project_id=? FOR SHARE`, expected.ID, binding.JobID, binding.OrgID, binding.SourceProjectID, binding.TargetProjectID).Scan(&row)
	if read.Error != nil {
		return application.ProjectCopyPrimaryProof{}, fmt.Errorf("read frozen project canvases: %w", read.Error)
	}
	if read.RowsAffected != 1 || row.ManifestSHA256 != expected.ManifestSHA256 || row.DocumentCount != expected.Documents {
		return application.ProjectCopyPrimaryProof{}, domain.ErrInvalidProjectCopy
	}
	var documents []domain.Document
	var assets map[uuid.UUID]uuid.UUID
	if strictCopyJSON(row.Documents, &documents) != nil || strictCopyJSON(row.AssetMapping, &assets) != nil || len(documents) != expected.Documents {
		return application.ProjectCopyPrimaryProof{}, domain.ErrInvalidProjectCopy
	}
	body, err := json.Marshal(documents)
	if err != nil {
		return application.ProjectCopyPrimaryProof{}, err
	}
	mapping, err := json.Marshal(assets)
	if err != nil {
		return application.ProjectCopyPrimaryProof{}, err
	}
	hash := sha256.Sum256(append(append(body, '\n'), mapping...))
	if hex.EncodeToString(hash[:]) != expected.ManifestSHA256 {
		return application.ProjectCopyPrimaryProof{}, domain.ErrInvalidProjectCopy
	}
	copied, contentHash, err := application.PrepareProjectCopy(binding, documents, assets)
	if err != nil {
		return application.ProjectCopyPrimaryProof{}, err
	}
	result := application.ProjectCopyReceipt{ManifestSHA256: expected.ManifestSHA256, ContentSHA256: contentHash, Documents: len(copied)}
	for _, item := range copied {
		result.ClearedOperationBindings += item.ClearedOperationBindings
	}
	var previous struct {
		ContentSHA256            string
		DocumentCount            int
		ClearedOperationBindings int
	}
	replayed := tx.Raw(`SELECT content_sha256,document_count,cleared_operation_bindings FROM canvas.project_copy_receipt WHERE snapshot_id=? FOR SHARE`, expected.ID).Scan(&previous)
	if replayed.Error != nil {
		return application.ProjectCopyPrimaryProof{}, replayed.Error
	}

	if replayed.RowsAffected != 1 || previous.ContentSHA256 != result.ContentSHA256 || previous.DocumentCount != result.Documents || previous.ClearedOperationBindings != result.ClearedOperationBindings {
		return application.ProjectCopyPrimaryProof{}, domain.ErrInvalidProjectCopy
	}
	proof := application.ProjectCopyPrimaryProof{Receipt: result}
	for i, item := range copied {
		candidate := application.ProjectCopyPrimaryCandidate{SourceDocumentID: documents[i].ID, TargetDocumentID: item.Document.ID}
		read := tx.Raw(`SELECT revision FROM canvas.document WHERE id=? AND project_id=? AND NOT is_delete AND schema_version=? FOR SHARE`, item.Document.ID, binding.TargetProjectID, application.SchemaVersion).Scan(&candidate.Revision)
		if read.Error != nil {
			return application.ProjectCopyPrimaryProof{}, read.Error
		}
		if read.RowsAffected != 1 {
			return application.ProjectCopyPrimaryProof{}, domain.ErrInvalidProjectCopy
		}
		var collaboration struct {
			Epoch     uuid.UUID
			GraphHash string
		}
		if err := tx.Raw(`SELECT epoch,graph_hash FROM canvas.collab_document WHERE document_id=? FOR SHARE`, item.Document.ID).Scan(&collaboration).Error; err != nil {
			return application.ProjectCopyPrimaryProof{}, err
		}
		candidate.Epoch, candidate.GraphHash = collaboration.Epoch, collaboration.GraphHash
		proof.Candidates = append(proof.Candidates, candidate)
	}
	return proof, nil
}
