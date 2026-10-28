package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func acceptIngestLocations(tx *gorm.DB, asset domain.MediaAsset, rends []domain.Rendition) error {
	if asset.ObjectLocation == nil {
		for _, r := range rends {
			if r.ObjectLocation != nil {
				return application.ErrStorageIdentityConflict
			}
		}
		return nil
	}
	if asset.SourceOperationID == nil {
		return application.ErrStorageIdentityConflict
	}
	owner := application.StorageWorkOwner{Kind: "operation", OwnerID: asset.ProjectID, WorkID: *asset.SourceOperationID}
	intent, found, err := readStorageWork(tx, owner)
	if err != nil {
		return err
	}
	if !found || intent.State != "open" {
		return application.ErrStorageRouteConflict
	}
	locations := []*domain.ObjectLocation{asset.ObjectLocation}
	keys := []string{asset.ObjectKey}
	sizes := []int64{asset.ByteSize}
	for _, r := range rends {
		if r.ObjectLocation == nil || r.ByteSize == nil {
			return application.ErrStorageIdentityConflict
		}
		locations = append(locations, r.ObjectLocation)
		keys = append(keys, r.ObjectKey)
		sizes = append(sizes, *r.ByteSize)
	}
	seen := map[uuid.UUID]bool{}
	for i, loc := range locations {
		if loc.ID == uuid.Nil || seen[loc.ID] || loc.Ref != (domain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: keys[i]}) || loc.ByteSize == nil || *loc.ByteSize != sizes[i] || loc.MIMEType == nil || *loc.MIMEType == "" || loc.SHA256 == nil || len(*loc.SHA256) != 64 {
			return application.ErrStorageIdentityConflict
		}
		if i == 0 && (*loc.MIMEType != asset.MimeType || asset.SHA256 == nil || *loc.SHA256 != *asset.SHA256) {
			return application.ErrObjectMismatch
		}
		seen[loc.ID] = true
		var row objectLocationRow
		q := tx.Raw(`SELECT * FROM media.object_location WHERE id=? FOR UPDATE`, loc.ID).Scan(&row)
		actual := row.domain()
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || row.OwnerIntentID == nil || *row.OwnerIntentID != intent.ID || row.OwnerSessionID != nil || actual.Ref != loc.Ref || (actual.State != "present" && actual.State != "accepted") || actual.ByteSize == nil || *actual.ByteSize != *loc.ByteSize || actual.MIMEType == nil || *actual.MIMEType != *loc.MIMEType || actual.SHA256 == nil || *actual.SHA256 != *loc.SHA256 {
			return application.ErrStorageIdentityConflict
		}
		if err := tx.Exec(`UPDATE media.object_location SET state='accepted' WHERE id=? AND owner_intent_id=? AND state IN ('present','accepted')`, loc.ID, intent.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

func insertIngestMedia(tx *gorm.DB, a domain.MediaAsset, rends []domain.Rendition) error {
	row := map[string]any{"id": a.ID, "project_id": a.ProjectID, "kind": string(a.Kind), "origin": "generated", "status": "processing", "object_key": a.ObjectKey, "file_name": "", "mime_type": a.MimeType, "byte_size": a.ByteSize, "sha256": a.SHA256, "width": a.Width, "height": a.Height, "duration_ms": a.DurationMS, "fps": a.FPS, "audio_channels": a.AudioChannels, "codec": a.Codec, "source_operation_id": a.SourceOperationID, "provider_key": a.ProviderKey, "model_key": a.ModelKey, "region": a.Region, "moderation_status": "pending", "aigc_marked": a.AIGCMarked, "contains_real_person": a.ContainsRealPerson, "revision": 1, "create_time": a.CreateTime, "update_time": a.UpdateTime}
	if a.ObjectLocation != nil {
		row["object_location_id"] = a.ObjectLocation.ID
	}
	q := tx.Table("media.media_asset").Create(row)
	if q.Error != nil {
		return fmt.Errorf("insert generated media: %w", q.Error)
	}
	if q.RowsAffected != 1 {
		return ErrOutputConflict
	}
	for _, r := range rends {
		row := map[string]any{"id": r.ID, "media_asset_id": a.ID, "kind": string(r.Kind), "object_key": r.ObjectKey, "width": r.Width, "height": r.Height, "byte_size": r.ByteSize, "create_time": r.CreateTime, "update_time": r.UpdateTime}
		if r.ObjectLocation != nil {
			row["object_location_id"] = r.ObjectLocation.ID
		}
		q := tx.Table("media.rendition").Create(row)
		if q.Error != nil {
			return fmt.Errorf("insert generated rendition: %w", q.Error)
		}
		if q.RowsAffected != 1 {
			return ErrOutputConflict
		}
	}
	return nil
}

// CompleteGeneratedWork closes a managed result only after every output and
// its renditions are accepted. Unknown staging debt is never silently discarded.
func (r *IngestRepository) CompleteGeneratedWork(ctx context.Context, operationID uuid.UUID, expected int) error {
	if r == nil || r.db == nil || operationID == uuid.Nil || expected < 1 || expected > 8 {
		return application.ErrInvalidIngest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.authorizeOperation(tx, operationID, true); err != nil {
			return err
		}
		var op struct {
			ProjectID uuid.UUID
			Status    string
		}
		q := tx.Raw(`SELECT project_id,status FROM operation.operation WHERE id=? AND NOT is_delete FOR UPDATE`, operationID).Scan(&op)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || op.Status != "ingesting" {
			return ErrOperationNotIngesting
		}
		intent, found, err := readStorageWork(tx, application.StorageWorkOwner{Kind: "operation", OwnerID: op.ProjectID, WorkID: operationID})
		if err != nil {
			return err
		}
		if !found {
			return application.ErrStorageRouteConflict
		}
		var n int64
		if err := tx.Raw(`SELECT count(*) FROM operation.operation_output o JOIN media.media_asset a ON a.id=o.media_asset_id AND a.project_id=o.project_id JOIN media.object_location l ON l.id=a.object_location_id AND l.object_key=a.object_key AND l.owner_intent_id=? AND l.state='accepted' WHERE o.operation_id=? AND o.kind='media' AND o.moderation_status='passed' AND a.status='ready' AND NOT o.is_delete AND NOT a.is_delete`, intent.ID, operationID).Scan(&n).Error; err != nil {
			return err
		}
		if n != int64(expected) {
			return application.ErrStorageRouteConflict
		}
		var outputs []struct {
			MediaAssetID uuid.UUID
			Kind         string
		}
		if err := tx.Raw(`SELECT a.id AS media_asset_id,a.kind FROM operation.operation_output o JOIN media.media_asset a ON a.id=o.media_asset_id WHERE o.operation_id=? AND o.kind='media' AND NOT o.is_delete AND NOT a.is_delete ORDER BY o.seq_no`, operationID).Scan(&outputs).Error; err != nil {
			return err
		}
		if len(outputs) != expected {
			return application.ErrStorageRouteConflict
		}
		for _, output := range outputs {
			rends, err := NewScopedIngestRepository(tx, r.scopes).findRenditions(ctx, output.MediaAssetID)
			if err != nil {
				return err
			}
			required := 0
			if output.Kind == "image" || output.Kind == "video" {
				required = 2
			}
			if len(rends) != required {
				return application.ErrStorageRouteConflict
			}
			for _, rendition := range rends {
				loc := rendition.ObjectLocation
				if loc == nil || loc.State != "accepted" || loc.Ref.ProfileID != intent.ProfileID || loc.Ref.Bucket != intent.Bucket {
					return application.ErrStorageIdentityConflict
				}
			}
		}
		var ids []uuid.UUID
		if err := tx.Raw(`SELECT id FROM media.object_location WHERE owner_intent_id=? ORDER BY id LIMIT 15001`, intent.ID).Scan(&ids).Error; err != nil {
			return err
		}
		return NewStorageRoutes(tx).AcceptStorageWork(ctx, intent.ID, ids)
	})
}
