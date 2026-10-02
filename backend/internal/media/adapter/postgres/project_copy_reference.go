package postgres

import (
	"bytes"
	"context"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func copyReferenceAccess(tx *gorm.DB, actor identityapp.Principal, binding application.ProjectCopyBinding) error {
	if binding.Validate() != nil || actor.OrgID != binding.OrgID {
		return application.ErrProjectCopyMediaUnavailable
	}
	if err := requireCurrentActor(tx, actor); err != nil {
		return err
	}
	ids := []uuid.UUID{binding.SourceProjectID, binding.TargetProjectID}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	var rows []struct {
		ID     uuid.UUID
		Status string
	}
	if err := tx.Raw(`SELECT id,status FROM workspace.project WHERE org_id=? AND id IN ? AND NOT is_delete ORDER BY id FOR SHARE`, actor.OrgID, ids).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) != 2 {
		return application.ErrNotFound
	}
	for _, row := range rows {
		if row.ID == binding.TargetProjectID && row.Status != "copying" || row.ID == binding.SourceProjectID && row.Status != "active" && row.Status != "archived" {
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
	if err := copyReferenceAccess(tx, actor, binding); err != nil {
		return result, err
	}
	content, err := s.load(ctx, actor, binding, snapshot)
	if err != nil {
		return result, err
	}
	var manifest struct{ AssetMapping []byte }
	if err := tx.Raw(`SELECT asset_mapping FROM media.project_copy_snapshot WHERE id=?`, snapshot.ID).Scan(&manifest).Error; err != nil {
		return result, err
	}
	var mapping map[uuid.UUID]uuid.UUID
	if strictCopyMedia(manifest.AssetMapping, &mapping) != nil || mapping[fact.AssetID] == uuid.Nil {
		return result, application.ErrProjectCopyMediaUnavailable
	}
	var source assetRow
	read := tx.Raw(`SELECT * FROM media.media_asset WHERE id=? AND project_id=? AND personal_org_id IS NULL AND personal_actor_id IS NULL FOR SHARE`, fact.AssetID, binding.SourceProjectID).Scan(&source)
	if read.Error != nil {
		return result, read.Error
	}
	if read.RowsAffected != 1 {
		return result, application.ErrNotFound
	}
	result.Source.Asset = source.domain()
	var rows []renditionRow
	if err := tx.Raw(`SELECT * FROM media.rendition WHERE media_asset_id=? AND NOT is_delete ORDER BY kind,id LIMIT 17 FOR SHARE`, fact.AssetID).Scan(&rows).Error; err != nil {
		return result, err
	}
	if len(rows) > 16 {
		return result, application.ErrProjectCopyMediaUnavailable
	}
	result.Source.Renditions = []domain.Rendition{}
	for _, r := range rows {
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
	var registered int
	read := tx.Raw(`SELECT 1 FROM media.project_copy_receipt r JOIN media.project_copy_snapshot s ON s.id=r.snapshot_id WHERE r.snapshot_id=? AND s.manifest_sha256=? AND r.asset_count=? AND r.rendition_count=?`, snapshot.ID, snapshot.ManifestSHA256, snapshot.Assets, snapshot.Renditions).Scan(&registered)
	if read.Error != nil {
		return result, read.Error
	}
	if read.RowsAffected != 1 {
		return result, application.ErrObjectMismatch
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
		if object.Status != "verified" || !object.SourceVerified || object.SHA256 == nil || object.ByteSize == nil {
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
	if err := verifyCopiedReference(tx, result.Target, binding.TargetProjectID); err != nil {
		return result, err
	}
	return result, nil
}

func verifyCopiedReference(tx *gorm.DB, target application.LibraryMediaFile, project uuid.UUID) error {
	var row assetRow
	read := tx.Raw(`SELECT * FROM media.media_asset WHERE id=? AND project_id=? AND NOT is_delete FOR SHARE`, target.Asset.ID, project).Scan(&row)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || !equalCopyMedia(target.Asset, row.domain()) {
		return application.ErrObjectMismatch
	}
	var rows []renditionRow
	if err := tx.Raw(`SELECT * FROM media.rendition WHERE media_asset_id=? AND NOT is_delete ORDER BY kind,id FOR SHARE`, target.Asset.ID).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(target.Renditions) {
		return application.ErrObjectMismatch
	}
	byID := make(map[uuid.UUID]domain.Rendition, len(rows))
	for _, row := range rows {
		byID[row.ID] = row.domain()
	}
	for _, rendition := range target.Renditions {
		if !equalCopyMedia(rendition, byID[rendition.ID]) {
			return application.ErrObjectMismatch
		}
	}
	return nil
}
