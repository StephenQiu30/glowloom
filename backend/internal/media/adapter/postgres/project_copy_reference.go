package postgres

import (
	"bytes"
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

func (s scopeAuthority) copyReferenceAccess(tx *gorm.DB, actor identityapp.Principal, binding application.ProjectCopyBinding) error {
	if binding.Validate() != nil || actor.OrgID != binding.OrgID {
		return application.ErrProjectCopyMediaUnavailable
	}
	if err := s.requireCurrentActor(tx, actor); err != nil {
		return err
	}
	if err := s.requireProject(tx, actor, binding.SourceProjectID, false); err != nil {
		return err
	}
	if err := s.requireCopyTarget(tx, actor, binding.TargetProjectID); err != nil {
		return err
	}
	ids := []uuid.UUID{binding.SourceProjectID, binding.TargetProjectID}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	var rows []struct {
		ID     uuid.UUID
		Status string
	}
	if err := tx.Table("workspace.project").Select("id", "status").Clauses(clause.Locking{Strength: "SHARE"}).Where("org_id = ? AND id IN ? AND is_delete = ?", actor.OrgID, ids, false).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != 2 {
		return application.ErrNotFound
	}
	for _, row := range rows {
		if row.ID == binding.TargetProjectID && row.Status != workspacedomain.ProjectStatusCopying || row.ID == binding.SourceProjectID && row.Status != workspacedomain.ProjectStatusActive && row.Status != workspacedomain.ProjectStatusArchived {
			return application.ErrNotFound
		}
	}
	return nil
}

// ReadCopyReference reads only SQL. The trusted manifest and its actual object
// mapping determine target identities; rendition digests are not manufactured.
func (s *ProjectCopyStore) ReadCopyReference(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, fact application.ReferenceFact) (application.ProjectCopyReferenceFiles, error) {
	var result application.ProjectCopyReferenceFiles
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	tx := s.db.WithContext(ctx)
	if err := s.copyReferenceAccess(tx, actor, binding); err != nil {
		return result, err
	}
	content, err := s.load(ctx, actor, binding, snapshot)
	if err != nil {
		return result, err
	}
	var manifest struct{ AssetMapping jsonDocument }
	if err := tx.Model(&projectCopySnapshotRecord{}).Select("asset_mapping").Where("id = ?", snapshot.ID).Take(&manifest).Error; err != nil {
		return result, err
	}
	var mapping map[uuid.UUID]uuid.UUID
	if strictCopyMedia(manifest.AssetMapping, &mapping) != nil || mapping[fact.AssetID] == uuid.Nil {
		return result, application.ErrProjectCopyMediaUnavailable
	}
	var source assetRow
	read := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"id": fact.AssetID, "project_id": binding.SourceProjectID, "personal_org_id": nil, "personal_actor_id": nil}).Take(&source)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return result, application.ErrNotFound
	}
	if read.Error != nil {
		return result, read.Error
	}
	if err := hydrateCopySourceAsset(tx, &source); err != nil {
		return result, err
	}
	result.Source.Asset = source.domain()
	var rows []renditionRow
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("media_asset_id = ? AND is_delete = ?", fact.AssetID, false).Order("kind").Order("id").Limit(17).Find(&rows).Error; err != nil {
		return result, err
	}
	if len(rows) > 16 {
		return result, application.ErrProjectCopyMediaUnavailable
	}
	result.Source.Renditions = []domain.Rendition{}
	for _, r := range rows {
		if err := hydrateCopySourceRendition(tx, &r); err != nil {
			return result, err
		}
		result.Source.Renditions = append(result.Source.Renditions, r.domain())
	}
	var found bool
	for _, item := range content.Assets {
		if item.Asset.ID == mapping[fact.AssetID] {
			result.Target = application.LibraryMediaFile{Asset: item.Asset, Renditions: item.Renditions}
			result.RetainedHistory = item.RetainedHistory
			found = true
		}
	}
	if !found {
		return result, application.ErrProjectCopyMediaUnavailable
	}
	var originalBound bool
	for _, object := range content.Objects {
		if object.TargetAssetID == result.Target.Asset.ID && object.RenditionKind == "" && object.SourceObjectKey == result.Source.Asset.ObjectKey && object.TargetObjectKey == result.Target.Asset.ObjectKey {
			originalBound = true
		}
	}
	if !originalBound {
		return result, application.ErrObjectMismatch
	}
	if fact.RenditionID != nil {
		for _, sourceRend := range result.Source.Renditions {
			if sourceRend.ID != *fact.RenditionID {
				continue
			}
			for _, object := range content.Objects {
				if object.TargetAssetID != result.Target.Asset.ID || object.SourceObjectKey != sourceRend.ObjectKey || object.RenditionKind != string(sourceRend.Kind) {
					continue
				}
				for _, targetRend := range result.Target.Renditions {
					if targetRend.ObjectKey == object.TargetObjectKey && targetRend.MediaAssetID == result.Target.Asset.ID && targetRend.Kind == sourceRend.Kind {
						id := targetRend.ID
						result.TargetRenditionID = &id
					}
				}
			}
		}
	}
	return result, nil
}

// ReadTransferredCopyReference requires owning registration/object receipts
// and exact current target facts. The caller separately proves actual bytes.
func (s *ProjectCopyStore) ReadTransferredCopyReference(ctx context.Context, actor identityapp.Principal, binding application.ProjectCopyBinding, snapshot application.ProjectCopySnapshot, fact application.ReferenceFact) (application.ProjectCopyReferenceFiles, error) {
	result, err := s.ReadCopyReference(ctx, actor, binding, snapshot, fact)
	if err != nil {
		return result, err
	}
	tx := s.db.WithContext(ctx)
	var registered projectCopyReceiptRecord
	read := tx.Where("snapshot_id = ? AND content_sha256 IS NOT NULL AND asset_count = ? AND rendition_count = ?", snapshot.ID, snapshot.Assets, snapshot.Renditions).Take(&registered)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return result, application.ErrObjectMismatch
	}
	if read.Error != nil {
		return result, read.Error
	}
	objects, err := s.Objects(ctx, actor, binding, snapshot)
	if err != nil {
		return result, err
	}
	byKey := make(map[string]application.ProjectCopyObject)
	for _, object := range objects {
		if object.TargetAssetID != result.Target.Asset.ID {
			continue
		}
		if object.Status != application.ProjectCopyObjectVerified || !object.SourceVerified || object.SHA256 == nil || object.ByteSize == nil {
			return result, application.ErrObjectMismatch
		}
		byKey[object.TargetObjectKey] = object
	}
	if len(byKey) != len(result.Target.Renditions)+1 {
		return result, application.ErrObjectMismatch
	}
	for i := range result.Target.Renditions {
		r := &result.Target.Renditions[i]
		object, ok := byKey[r.ObjectKey]
		if !ok || object.RenditionKind != string(r.Kind) {
			return result, application.ErrObjectMismatch
		}
		r.ByteSize = object.ByteSize
	}
	if err := verifyCopiedReference(tx, &result.Target, binding.TargetProjectID); err != nil {
		return result, err
	}
	return result, nil
}

func verifyCopiedReference(tx *gorm.DB, target *application.LibraryMediaFile, project uuid.UUID) error {
	var row assetRow
	read := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND project_id = ? AND is_delete = ?", target.Asset.ID, project, false).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.ErrObjectMismatch
	}
	if read.Error != nil {
		return read.Error
	}
	if err := row.loadLocation(tx); err != nil {
		return err
	}
	if !equalCopyMedia(target.Asset, row.domain()) {
		return application.ErrObjectMismatch
	}
	var rows []renditionRow
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("media_asset_id = ? AND is_delete = ?", target.Asset.ID, false).Order("kind").Order("id").Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(target.Renditions) {
		return application.ErrObjectMismatch
	}
	byID := make(map[uuid.UUID]domain.Rendition, len(rows))
	for _, row := range rows {
		if err := row.loadLocation(tx); err != nil {
			return err
		}
		byID[row.ID] = row.domain()
	}
	for i, rendition := range target.Renditions {
		if !equalCopyMedia(rendition, byID[rendition.ID]) {
			return application.ErrObjectMismatch
		}
		target.Renditions[i] = byID[rendition.ID]
	}
	target.Asset = row.domain()
	return nil
}
