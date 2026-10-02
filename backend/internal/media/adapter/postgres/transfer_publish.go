package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func insertTransferAsset(tx *gorm.DB, asset domain.MediaAsset, renditions []domain.Rendition) error {
	if asset.Validate() != nil || asset.IsDelete || asset.Status != domain.StatusReady || asset.ModerationStatus != domain.ModerationPassed || asset.ContainsRealPerson || asset.ConsentRecordID != nil {
		return application.ErrProjectCopyMediaUnavailable
	}
	var project, personalOrg, personalActor *uuid.UUID
	if asset.Personal == nil {
		project = &asset.ProjectID
	} else {
		personalOrg, personalActor = &asset.Personal.OrgID, &asset.Personal.ActorID
	}
	if err := libraryChanged(tx, `INSERT INTO media.media_asset(id,project_id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,duration_ms,fps,audio_channels,codec,moderation_status,moderation_detail,aigc_marked,contains_real_person,revision,create_time,update_time) VALUES(?,?,?,?,?,?,'ready',?,?,?,?,?,?,?,?,?,?,?,'passed',?::jsonb,?,false,1,?,?)`, asset.ID, project, personalOrg, personalActor, string(asset.Kind), string(asset.Origin), asset.ObjectKey, asset.FileName, asset.MimeType, asset.ByteSize, asset.SHA256, asset.Width, asset.Height, asset.DurationMS, asset.FPS, asset.AudioChannels, asset.Codec, nullableJSON(asset.ModerationDetail), asset.AIGCMarked, asset.CreateTime, asset.UpdateTime); err != nil {
		return err
	}
	for _, r := range renditions {
		if r.Validate() != nil || r.MediaAssetID != asset.ID || r.IsDelete {
			return application.ErrProjectCopyMediaUnavailable
		}
		if err := libraryChanged(tx, `INSERT INTO media.rendition(id,media_asset_id,kind,object_key,width,height,byte_size,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?)`, r.ID, r.MediaAssetID, string(r.Kind), r.ObjectKey, r.Width, r.Height, r.ByteSize, r.CreateTime, r.UpdateTime); err != nil {
			return err
		}
	}
	return nil
}

// PublishTransferItem atomically registers a complete independently owned item.
// The source is unchanged; failed/unknown objects never enter formal media reads.
func (s *TransferStore) PublishTransferItem(ctx context.Context, lease application.TransferLease, index int) error {
	return s.withLease(ctx, lease, false, false, func(tx *gorm.DB, row transferJobRow, source, target libraryRow) error {
		state, item, err := readTransferItem(tx, row.ID, index)
		if err != nil {
			return err
		}
		if state.Status == "succeeded" {
			return nil
		}
		if state.Status != "running" {
			return domain.ErrTransferConflict
		}
		if err := validateTransferSource(tx, lease.Actor, row, source, item); err != nil {
			return err
		}
		if err := validateTransferFolder(tx, row, target); err != nil {
			return err
		}
		var objects []transferObjectRow
		if err := tx.Raw(`SELECT * FROM media.transfer_object WHERE job_id=? AND item_index=? ORDER BY rendition_kind FOR UPDATE`, row.ID, index).Scan(&objects).Error; err != nil {
			return err
		}
		if len(objects) != len(item.Objects) {
			return application.ErrObjectMismatch
		}
		byKey := make(map[string]transferObjectRow, len(objects))
		for _, object := range objects {
			if object.Status != "verified" || !object.SourceVerified || !object.WriteStarted || object.SHA256 == nil || object.ByteSize == nil || !validTransferDigest(*object.SHA256, *object.ByteSize) {
				return application.ErrObjectMismatch
			}
			byKey[object.TargetObjectKey] = object
		}
		if item.TargetAsset != nil {
			a := item.TargetAsset
			object, ok := byKey[a.ObjectKey]
			if !ok || object.RenditionKind != "" || object.SHA256 == nil || a.SHA256 == nil || *object.SHA256 != *a.SHA256 || object.ByteSize == nil || *object.ByteSize != a.ByteSize {
				return application.ErrObjectMismatch
			}
			for i := range item.TargetRenditions {
				r := &item.TargetRenditions[i]
				object, ok := byKey[r.ObjectKey]
				if !ok || object.RenditionKind != string(r.Kind) {
					return application.ErrObjectMismatch
				}
				r.ByteSize = object.ByteSize
			}
			if err := insertTransferAsset(tx, *a, item.TargetRenditions); err != nil {
				return &application.TransferItemError{Code: "registration_failed", Cause: err}
			}
		}
		if item.TargetItem.LibraryID != target.ID || item.TargetItem.ID == item.SourceItem.ID || item.TargetItem.Validate() != nil {
			return application.ErrUnavailable
		}
		if err := persistLibraryItem(tx, item.TargetItem, 0); err != nil {
			return &application.TransferItemError{Code: "registration_failed", Cause: err}
		}
		if err := libraryChanged(tx, `UPDATE media.library SET revision=revision+1,update_time=? WHERE id=? AND revision=?`, s.clock().UTC(), target.ID, target.Revision); err != nil {
			return err
		}
		_, targetScope := row.scopes()
		if targetScope.Kind == domain.LibraryProject {
			owner := s.project(tx)
			facts, err := owner.Authorize(ctx, lease.Actor, *targetScope.ProjectID, true)
			if err != nil {
				return err
			}
			if _, err := owner.TouchContent(ctx, lease.Actor, facts.ProjectID, facts.Revision); err != nil {
				return err
			}
		}
		if err := libraryChanged(tx, `UPDATE media.transfer_item SET status='succeeded',failure_code=NULL WHERE job_id=? AND item_index=? AND status='running'`, row.ID, index); err != nil {
			return err
		}
		return libraryChanged(tx, `UPDATE media.transfer_job SET stage='registering',revision=revision+1,updated_at=? WHERE id=? AND worker_fence=?`, s.clock().UTC(), row.ID, lease.Fence)
	})
}

var _ application.TransferWorkerRepository = (*TransferStore)(nil)
