package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type ingestOperationRecord struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Status    string
	IsDelete  bool
}

func (ingestOperationRecord) TableName() string { return "operation.operation" }

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
	owner := application.StorageWorkOwner{Kind: application.StorageWorkKindOperation, OwnerID: asset.ProjectID, WorkID: *asset.SourceOperationID}
	intent, found, err := readStorageWork(tx, owner)
	if err != nil {
		return err
	}
	if !found || intent.State != application.StorageWorkOpen {
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
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", loc.ID).Take(&row).Error
		actual := row.domain()
		if err != nil {
			return err
		}
		if row.ID == uuid.Nil || row.OwnerIntentID == nil || *row.OwnerIntentID != intent.ID || row.OwnerSessionID != nil || actual.Ref != loc.Ref || (actual.State != domain.ObjectLocationPresent && actual.State != domain.ObjectLocationAccepted) || actual.ByteSize == nil || *actual.ByteSize != *loc.ByteSize || actual.MIMEType == nil || *actual.MIMEType != *loc.MIMEType || actual.SHA256 == nil || *actual.SHA256 != *loc.SHA256 {
			return application.ErrStorageIdentityConflict
		}
		updated := tx.Model(&objectLocationRow{}).Where("id = ? AND owner_intent_id = ? AND state IN ?", loc.ID, intent.ID, []domain.ObjectLocationState{domain.ObjectLocationPresent, domain.ObjectLocationAccepted}).Update("state", domain.ObjectLocationAccepted)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return application.ErrStorageRouteConflict
		}
	}
	return nil
}

func insertIngestMedia(tx *gorm.DB, a domain.MediaAsset, rends []domain.Rendition) error {
	row := map[string]any{"id": a.ID, "project_id": a.ProjectID, "kind": string(a.Kind), "origin": string(domain.OriginGenerated), "status": string(domain.StatusProcessing), "object_key": a.ObjectKey, "file_name": "", "mime_type": a.MimeType, "byte_size": a.ByteSize, "sha256": a.SHA256, "width": a.Width, "height": a.Height, "duration_ms": a.DurationMS, "fps": a.FPS, "audio_channels": a.AudioChannels, "codec": a.Codec, "source_operation_id": a.SourceOperationID, "provider_key": a.ProviderKey, "model_key": a.ModelKey, "region": a.Region, "moderation_status": string(domain.ModerationPending), "aigc_marked": a.AIGCMarked, "contains_real_person": a.ContainsRealPerson, "revision": 1, "create_time": a.CreateTime, "update_time": a.UpdateTime}
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
		var op ingestOperationRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND NOT is_delete", operationID).Take(&op).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOperationNotIngesting
		}
		if err != nil {
			return err
		}
		if err := r.requireIngestState(op.Status); err != nil {
			return err
		}
		intent, found, err := readStorageWork(tx, application.StorageWorkOwner{Kind: application.StorageWorkKindOperation, OwnerID: op.ProjectID, WorkID: operationID})
		if err != nil {
			return err
		}
		if !found {
			return application.ErrStorageRouteConflict
		}
		var n int64
		if err := tx.Table("operation.operation_output AS o").
			Joins("JOIN media.media_asset AS a ON a.id = o.media_asset_id AND a.project_id = o.project_id").
			Joins("JOIN media.object_location AS l ON l.id = a.object_location_id AND l.object_key = a.object_key").
			Where("l.owner_intent_id = ? AND l.state = ? AND o.operation_id = ? AND o.kind = ? AND o.moderation_status = ? AND a.status = ? AND NOT o.is_delete AND NOT a.is_delete", intent.ID, domain.ObjectLocationAccepted, operationID, "media", domain.ModerationPassed, domain.StatusReady).
			Count(&n).Error; err != nil {
			return err
		}
		if n != int64(expected) {
			return application.ErrStorageRouteConflict
		}
		var outputs []struct {
			MediaAssetID uuid.UUID
			Kind         string
		}
		if err := tx.Table("operation.operation_output AS o").Select("a.id AS media_asset_id, a.kind").
			Joins("JOIN media.media_asset AS a ON a.id = o.media_asset_id").
			Where("o.operation_id = ? AND o.kind = ? AND NOT o.is_delete AND NOT a.is_delete", operationID, "media").
			Order("o.seq_no").Find(&outputs).Error; err != nil {
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
				if loc == nil || loc.State != domain.ObjectLocationAccepted || loc.Ref.ProfileID != intent.ProfileID || loc.Ref.Bucket != intent.Bucket {
					return application.ErrStorageIdentityConflict
				}
			}
		}
		var ids []uuid.UUID
		if err := tx.Model(&objectLocationRow{}).Where("owner_intent_id = ?", intent.ID).Order("id").Limit(15001).Pluck("id", &ids).Error; err != nil {
			return err
		}
		return NewStorageRoutes(tx).AcceptStorageWork(ctx, intent.ID, ids)
	})
}
