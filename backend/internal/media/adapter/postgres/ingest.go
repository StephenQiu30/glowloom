package postgres

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

var (
	// ErrOperationNotIngesting means the source is absent or cannot accept media.
	ErrOperationNotIngesting = errors.New("operation cannot ingest media")
	// ErrOutputConflict means a replay attempted to rewrite an existing output.
	ErrOutputConflict = errors.New("operation media output conflict")
)

// IngestRepository writes one generated media asset and its operation output
// in the same database transaction. It owns no other operation lifecycle state.
type IngestRepository struct{ db *gorm.DB }

// NewIngestRepository binds generated-media writes to the caller's database.
func NewIngestRepository(db *gorm.DB) *IngestRepository { return &IngestRepository{db: db} }

// FindOutput returns an existing candidate so a replay can skip re-downloading.
func (r *IngestRepository) FindOutput(ctx context.Context, operationID uuid.UUID, seq int) (application.IngestOutput, bool, error) {
	if r == nil || r.db == nil {
		return application.IngestOutput{}, false, ErrUnavailable
	}
	return findOutput(r.db.WithContext(ctx), operationID, seq)
}

func findOutput(tx *gorm.DB, operationID uuid.UUID, seq int) (application.IngestOutput, bool, error) {
	var row struct {
		OutputID     uuid.UUID
		MediaAssetID uuid.UUID
		ObjectKey    string
		Kind         string
		MimeType     string
		ByteSize     int64
		SHA256       *string
	}
	result := tx.Raw(`
		SELECT o.id AS output_id, a.id AS media_asset_id, a.object_key,
		       a.kind, a.mime_type, a.byte_size, a.sha256
		FROM operation.operation_output AS o
		JOIN media.media_asset AS a ON a.id = o.media_asset_id AND a.project_id = o.project_id
		WHERE o.operation_id = ?::uuid AND o.seq_no = ? AND o.kind = 'media'
		  AND NOT o.is_delete AND NOT a.is_delete
	`, operationID.String(), seq).Scan(&row)
	if result.Error != nil {
		return application.IngestOutput{}, false, fmt.Errorf("read operation media output: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return application.IngestOutput{}, false, nil
	}
	if result.RowsAffected != 1 || row.SHA256 == nil {
		return application.IngestOutput{}, false, ErrOutputConflict
	}
	return application.IngestOutput{
		OutputID: row.OutputID.String(), MediaAssetID: row.MediaAssetID.String(),
		ObjectKey: row.ObjectKey, Kind: row.Kind, MIMEType: row.MimeType,
		ByteSize: row.ByteSize, SHA256: *row.SHA256,
	}, true, nil
}

// LoadOperation reads the persisted source and rejects non-ingesting operations.
func (r *IngestRepository) LoadOperation(ctx context.Context, operationID uuid.UUID) (application.SourceOperation, error) {
	if r == nil || r.db == nil {
		return application.SourceOperation{}, ErrUnavailable
	}
	var row application.SourceOperation
	result := r.db.WithContext(ctx).Raw(`
		SELECT o.id, o.project_id, provider.key AS provider_key,
		       profile.model_key, o.region, project.aspect_ratio,
		       capability.output_type AS output_kind, o.create_time
		FROM operation.operation AS o
		JOIN workspace.project AS project ON project.id = o.project_id
		JOIN catalog.model_profile_version AS version ON version.id = o.model_profile_version_id
		JOIN catalog.model_profile AS profile ON profile.id = version.model_profile_id
		JOIN catalog.capability AS capability ON capability.key = profile.capability
		JOIN catalog.provider AS provider ON provider.id = profile.provider_id
		WHERE o.id = ?::uuid AND o.status = 'ingesting' AND NOT o.is_delete
		  AND NOT project.is_delete AND NOT version.is_delete
		  AND NOT profile.is_delete AND NOT provider.is_delete
	`, operationID.String()).Scan(&row)
	if result.Error != nil {
		return application.SourceOperation{}, fmt.Errorf("load media source operation: %w", result.Error)
	}
	if result.RowsAffected != 1 || row.ID == uuid.Nil || row.ProjectID == uuid.Nil ||
		row.ProviderKey == "" || row.ModelKey == "" || row.Region == "" ||
		(row.AspectRatio != "9:16" && row.AspectRatio != "16:9") ||
		(row.OutputKind != domain.KindImage && row.OutputKind != domain.KindVideo && row.OutputKind != domain.KindAudio) ||
		row.CreateTime.IsZero() {
		return application.SourceOperation{}, ErrOperationNotIngesting
	}
	return row, nil
}

// SaveOutput creates an asset and candidate in one transaction, or returns the prior candidate.
func (r *IngestRepository) SaveOutput(ctx context.Context, asset domain.MediaAsset, renditions []domain.Rendition, seq int, outputID uuid.UUID) (application.IngestOutput, error) {
	if r == nil || r.db == nil {
		return application.IngestOutput{}, ErrUnavailable
	}
	if err := asset.Validate(); err != nil || seq < 1 || seq > 8 || outputID == uuid.Nil || asset.SourceOperationID == nil {
		return application.IngestOutput{}, domain.ErrInvalidMediaAsset
	}
	for _, rendition := range renditions {
		if err := rendition.Validate(); err != nil || rendition.MediaAssetID != asset.ID || rendition.IsDelete {
			return application.IngestOutput{}, domain.ErrInvalidRendition
		}
		prefix := strings.TrimSuffix(asset.ObjectKey, path.Ext(asset.ObjectKey)) + "/"
		name := strings.TrimPrefix(rendition.ObjectKey, prefix)
		if !strings.HasPrefix(rendition.ObjectKey, prefix) || strings.Contains(name, "/") ||
			path.Ext(name) == "" || strings.TrimSuffix(name, path.Ext(name)) != string(rendition.Kind) {
			return application.IngestOutput{}, domain.ErrInvalidRendition
		}
	}
	var saved application.IngestOutput
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ Status string }
		result := tx.Raw(`
			SELECT status FROM operation.operation
			WHERE id = ?::uuid AND project_id = ?::uuid AND NOT is_delete
			FOR UPDATE
		`, asset.SourceOperationID.String(), asset.ProjectID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("lock media source operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrOperationNotIngesting
		}
		if existing, found, err := findOutput(tx, *asset.SourceOperationID, seq); err != nil {
			return err
		} else if found {
			if existing.MediaAssetID != asset.ID.String() || existing.OutputID != outputID.String() {
				return ErrOutputConflict
			}
			saved = existing
			return nil
		}
		if row.Status != "ingesting" {
			return ErrOperationNotIngesting
		}
		result = tx.Exec(`
			INSERT INTO media.media_asset
			  (id, project_id, kind, origin, status, object_key, file_name, mime_type,
			   byte_size, sha256, width, height, duration_ms, fps, audio_channels,
			   codec, source_operation_id, provider_key, model_key, region,
			   moderation_status, aigc_marked, contains_real_person, revision,
			   create_time, update_time)
			VALUES (?::uuid, ?::uuid, ?, 'generated', 'processing', ?, '', ?,
			        ?, ?, ?, ?, ?, ?, ?, ?, ?::uuid, ?, ?, ?, 'pending', ?, ?, 1, ?, ?)
		`, asset.ID.String(), asset.ProjectID.String(), string(asset.Kind), asset.ObjectKey,
			asset.MimeType, asset.ByteSize, asset.SHA256, asset.Width, asset.Height,
			asset.DurationMS, asset.FPS, asset.AudioChannels, asset.Codec,
			asset.SourceOperationID.String(), asset.ProviderKey, asset.ModelKey, asset.Region,
			asset.AIGCMarked, asset.ContainsRealPerson, asset.CreateTime, asset.UpdateTime)
		if result.Error != nil {
			return fmt.Errorf("insert generated media asset: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrOutputConflict
		}
		for _, rendition := range renditions {
			result = tx.Exec(`
				INSERT INTO media.rendition
				  (id, media_asset_id, kind, object_key, width, height, byte_size,
				   create_time, update_time)
				VALUES (?::uuid, ?::uuid, ?, ?, ?, ?, ?, ?, ?)
			`, rendition.ID.String(), asset.ID.String(), string(rendition.Kind),
				rendition.ObjectKey, rendition.Width, rendition.Height, rendition.ByteSize,
				rendition.CreateTime, rendition.UpdateTime)
			if result.Error != nil {
				return fmt.Errorf("insert media rendition: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrOutputConflict
			}
		}
		result = tx.Exec(`
			INSERT INTO operation.operation_output
			  (id, project_id, operation_id, seq_no, kind, media_asset_id, moderation_status)
			VALUES (?::uuid, ?::uuid, ?::uuid, ?, 'media', ?::uuid, 'pending')
		`, outputID.String(), asset.ProjectID.String(), asset.SourceOperationID.String(), seq, asset.ID.String())
		if result.Error != nil {
			return fmt.Errorf("insert operation media output: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrOutputConflict
		}
		saved = application.IngestOutput{
			OutputID: outputID.String(), MediaAssetID: asset.ID.String(),
			ObjectKey: asset.ObjectKey, Kind: string(asset.Kind), MIMEType: asset.MimeType,
			ByteSize: asset.ByteSize, SHA256: *asset.SHA256,
		}
		return nil
	})
	if err != nil {
		return application.IngestOutput{}, fmt.Errorf("save generated media output: %w", err)
	}
	return saved, nil
}

// FindRenditions returns the durable preview rows attached to one media asset.
func (r *IngestRepository) FindRenditions(ctx context.Context, assetID uuid.UUID) ([]domain.Rendition, error) {
	if r == nil || r.db == nil {
		return nil, ErrUnavailable
	}
	var rows []struct {
		ID           uuid.UUID
		MediaAssetID uuid.UUID
		Kind         string
		ObjectKey    string
		Width        *int32
		Height       *int32
		ByteSize     *int64
		CreateTime   time.Time
		UpdateTime   time.Time
	}
	if err := r.db.WithContext(ctx).Raw(`
		SELECT id, media_asset_id, kind, object_key, width, height, byte_size,
		       create_time, update_time
		FROM media.rendition WHERE media_asset_id = ?::uuid AND NOT is_delete
		ORDER BY kind
	`, assetID.String()).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read generated media renditions: %w", err)
	}
	result := make([]domain.Rendition, 0, len(rows))
	for _, row := range rows {
		rendition := domain.Rendition{
			ID: row.ID, MediaAssetID: row.MediaAssetID, Kind: domain.RenditionKind(row.Kind),
			ObjectKey: row.ObjectKey, Width: row.Width, Height: row.Height,
			ByteSize: row.ByteSize, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
		}
		if err := rendition.Validate(); err != nil {
			return nil, fmt.Errorf("validate stored media rendition: %w", err)
		}
		result = append(result, rendition)
	}
	return result, nil
}

// RecordModeration updates candidate and media states atomically. Repeating the
// same decision is safe; a conflicting decision cannot rewrite a reviewed result.
func (r *IngestRepository) RecordModeration(ctx context.Context, operationID, outputID uuid.UUID, status domain.ModerationStatus, reason string) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	if operationID == uuid.Nil || outputID == uuid.Nil ||
		(status != domain.ModerationPassed && status != domain.ModerationRejected) {
		return ErrOutputConflict
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var op struct{ Status string }
		result := tx.Raw(`SELECT status FROM operation.operation WHERE id = ?::uuid AND NOT is_delete FOR UPDATE`,
			operationID.String()).Scan(&op)
		if result.Error != nil {
			return fmt.Errorf("lock moderation source operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrOutputConflict
		}
		var row struct {
			MediaAssetID     uuid.UUID
			ModerationStatus string
			ModerationReason *string
			AssetStatus      string
			AssetModeration  string
			Revision         int32
		}
		result = tx.Raw(`
			SELECT o.media_asset_id, o.moderation_status, o.moderation_reason,
			       a.status AS asset_status, a.moderation_status AS asset_moderation,
			       a.revision
			FROM operation.operation_output AS o
			JOIN media.media_asset AS a ON a.id = o.media_asset_id AND a.project_id = o.project_id
			WHERE o.id = ?::uuid AND o.operation_id = ?::uuid AND o.kind = 'media'
			  AND NOT o.is_delete AND NOT a.is_delete
			FOR UPDATE OF o, a
		`, outputID.String(), operationID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("lock media moderation output: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrOutputConflict
		}
		if row.ModerationStatus == string(status) && row.AssetModeration == string(status) &&
			((row.ModerationReason == nil && reason == "") || (row.ModerationReason != nil && *row.ModerationReason == reason)) {
			return nil
		}
		if op.Status != "ingesting" || row.ModerationStatus != string(domain.ModerationPending) ||
			row.AssetStatus != string(domain.StatusProcessing) || row.AssetModeration != string(domain.ModerationPending) {
			return ErrOutputConflict
		}
		next := domain.StatusReady
		if status == domain.ModerationRejected {
			next = domain.StatusRejected
		}
		now := time.Now().UTC()
		result = tx.Exec(`
			UPDATE media.media_asset SET status = ?, moderation_status = ?, revision = revision + 1,
			       update_time = ?
			WHERE id = ?::uuid AND status = 'processing' AND moderation_status = 'pending'
			  AND revision = ? AND NOT is_delete
		`, string(next), string(status), now, row.MediaAssetID.String(), row.Revision)
		if result.Error != nil {
			return fmt.Errorf("update reviewed media asset: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrOutputConflict
		}
		var reasonValue any
		if reason != "" {
			reasonValue = reason
		}
		result = tx.Exec(`
			UPDATE operation.operation_output
			SET moderation_status = ?, moderation_reason = ?, update_time = ?
			WHERE id = ?::uuid AND moderation_status = 'pending' AND NOT is_delete
		`, string(status), reasonValue, now, outputID.String())
		if result.Error != nil {
			return fmt.Errorf("update reviewed operation output: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrOutputConflict
		}
		return nil
	})
}
