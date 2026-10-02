// Package postgres persists project-scoped media metadata.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

var (
	// ErrNotFound hides missing and out-of-project media records.
	ErrNotFound = application.ErrNotFound
	// ErrUnavailable means the store has no database handle.
	ErrUnavailable = application.ErrUnavailable
	// ErrProjectStateConflict means an inactive project cannot accept new media.
	ErrProjectStateConflict = workspacedomain.ErrProjectStateConflict
)

// Store keeps the database handle injected by the composition root.
type Store struct{ db *gorm.DB }

// NewStore creates a media store using the caller's database handle.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// CreateAsset records metadata for a new upload or generated result. Object
// transfer and ingest are separate steps; this method does not mark it ready.
func (s *Store) CreateAsset(ctx context.Context, actor identityapp.Principal, asset domain.MediaAsset) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if err := asset.Validate(); err != nil {
		return fmt.Errorf("validate media asset: %w", err)
	}
	if asset.IsDelete || asset.Revision != 1 ||
		(asset.Status != domain.StatusUploading && asset.Status != domain.StatusProcessing) {
		return domain.ErrInvalidMediaAsset
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := requireProject(tx, actor, asset.ProjectID, true); err != nil {
			return err
		}
		result := tx.Exec(`
			INSERT INTO media.media_asset
			  (id, project_id, kind, origin, status, object_key, file_name, mime_type,
			   byte_size, sha256, width, height, duration_ms, fps, audio_channels,
			   codec, source_operation_id, provider_key, model_key, region,
			   moderation_status, moderation_detail, aigc_marked, contains_real_person,
			   consent_record_id, upload_id, failure_reason, delete_time, purge_after,
			   revision, create_time, update_time)
			VALUES (?::uuid, ?::uuid, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			        ?::uuid, ?, ?, ?, ?, ?::jsonb, ?, ?, ?::uuid, ?, ?, ?, ?, ?, ?, ?)
		`, asset.ID.String(), asset.ProjectID.String(), string(asset.Kind),
			string(asset.Origin), string(asset.Status), asset.ObjectKey,
			asset.FileName, asset.MimeType, asset.ByteSize, asset.SHA256,
			asset.Width, asset.Height, asset.DurationMS, asset.FPS,
			asset.AudioChannels, asset.Codec, asset.SourceOperationID,
			asset.ProviderKey, asset.ModelKey, asset.Region,
			string(asset.ModerationStatus), nullableJSON(asset.ModerationDetail),
			asset.AIGCMarked, asset.ContainsRealPerson, asset.ConsentRecordID,
			asset.UploadID, asset.FailureReason, asset.DeleteTime, asset.PurgeAfter,
			asset.Revision, asset.CreateTime, asset.UpdateTime)
		if result.Error != nil {
			return fmt.Errorf("insert media asset: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("insert media asset: wrote %d rows", result.RowsAffected)
		}
		return nil
	})
}

// FindAsset returns one visible asset after rechecking the current actor and
// project. A caller cannot discover another project's asset by its ID.
func (s *Store) FindAsset(ctx context.Context, actor identityapp.Principal, projectID, assetID uuid.UUID) (domain.MediaAsset, error) {
	if s == nil || s.db == nil {
		return domain.MediaAsset{}, ErrUnavailable
	}
	if projectID == uuid.Nil || assetID == uuid.Nil {
		return domain.MediaAsset{}, ErrNotFound
	}
	var row assetRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		result := tx.Raw(`
			SELECT a.id, a.project_id, a.kind, a.origin, a.status, a.object_key,
			       a.file_name, a.mime_type, a.byte_size, a.sha256, a.width,
			       a.height, a.duration_ms, a.fps, a.audio_channels, a.codec,
			       a.source_operation_id, a.provider_key, a.model_key, a.region,
			       a.moderation_status, a.moderation_detail, a.aigc_marked,
			       a.contains_real_person, a.consent_record_id, a.upload_id,
			       a.failure_reason, a.delete_time, a.purge_after, a.revision,
			       a.create_time, a.update_time, a.is_delete
			FROM media.media_asset AS a
			JOIN workspace.project AS p ON p.id = a.project_id
			WHERE a.id = ?::uuid AND a.project_id = ?::uuid AND p.org_id = ?::uuid
			  AND NOT a.is_delete AND NOT p.is_delete AND p.status IN ('active','archived')
			FOR SHARE OF a, p
		`, assetID.String(), projectID.String(), actor.OrgID.String()).Scan(&row)
		if result.Error != nil {
			return fmt.Errorf("read media asset: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return domain.MediaAsset{}, fmt.Errorf("find media asset: %w", err)
	}
	asset := row.domain()
	if err := asset.Validate(); err != nil {
		return domain.MediaAsset{}, fmt.Errorf("validate stored media asset: %w", err)
	}
	return asset, nil
}

// AddRendition appends a derived object only after checking the parent asset's
// project scope. The parent foreign key carries that scope in the database.
func (s *Store) AddRendition(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, rendition domain.Rendition) error {
	if s == nil || s.db == nil {
		return ErrUnavailable
	}
	if projectID == uuid.Nil {
		return ErrNotFound
	}
	if err := rendition.Validate(); err != nil {
		return fmt.Errorf("validate rendition: %w", err)
	}
	if rendition.IsDelete {
		return domain.ErrInvalidRendition
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		parentKey, err := visibleAssetKey(tx, actor, projectID, rendition.MediaAssetID)
		if err != nil {
			return err
		}
		prefix := strings.TrimSuffix(parentKey, path.Ext(parentKey)) + "/"
		name := strings.TrimPrefix(rendition.ObjectKey, prefix)
		if !strings.HasPrefix(rendition.ObjectKey, prefix) ||
			strings.Contains(name, "/") || path.Ext(name) == "" ||
			strings.TrimSuffix(name, path.Ext(name)) != string(rendition.Kind) {
			return domain.ErrInvalidRendition
		}
		result := tx.Exec(`
			INSERT INTO media.rendition
			  (id, media_asset_id, kind, object_key, width, height, byte_size,
			   create_time, update_time)
			VALUES (?::uuid, ?::uuid, ?, ?, ?, ?, ?, ?, ?)
		`, rendition.ID.String(), rendition.MediaAssetID.String(),
			string(rendition.Kind), rendition.ObjectKey, rendition.Width,
			rendition.Height, rendition.ByteSize, rendition.CreateTime,
			rendition.UpdateTime)
		if result.Error != nil {
			return fmt.Errorf("insert rendition: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("insert rendition: wrote %d rows", result.RowsAffected)
		}
		return nil
	})
}

// FindRenditions returns the visible derived objects of a scoped parent.
func (s *Store) FindRenditions(ctx context.Context, actor identityapp.Principal, projectID, assetID uuid.UUID) ([]domain.Rendition, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if projectID == uuid.Nil || assetID == uuid.Nil {
		return nil, ErrNotFound
	}
	var rows []renditionRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if _, err := visibleAssetKey(tx, actor, projectID, assetID); err != nil {
			return err
		}
		result := tx.Raw(`
			SELECT id, media_asset_id, kind, object_key, width, height,
			       byte_size, create_time, update_time, is_delete
			FROM media.rendition
			WHERE media_asset_id = ?::uuid AND NOT is_delete
			ORDER BY kind, id
		`, assetID.String()).Scan(&rows)
		if result.Error != nil {
			return fmt.Errorf("read renditions: %w", result.Error)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("find renditions: %w", err)
	}
	result := make([]domain.Rendition, 0, len(rows))
	for _, row := range rows {
		r := row.domain()
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("validate stored rendition: %w", err)
		}
		result = append(result, r)
	}
	return result, nil
}

func requireCurrentActor(tx *gorm.DB, actor identityapp.Principal) error {
	if actor.ID == uuid.Nil || actor.OrgID == uuid.Nil || actor.MustChangePassword ||
		(actor.Role != identitydomain.RoleAdmin && actor.Role != identitydomain.RoleProducer) {
		return identityapp.ErrForbidden
	}
	var present int
	result := tx.Raw(`
		SELECT 1 FROM identity."user" AS u
		JOIN workspace.organization AS org ON org.id = u.org_id
		WHERE u.id = ?::uuid AND u.org_id = ?::uuid AND u.role = ?
		  AND u.status = 'active' AND NOT u.is_delete AND NOT u.must_change_password
		  AND org.status = 'active' AND NOT org.is_delete
		FOR SHARE OF u, org
	`, actor.ID.String(), actor.OrgID.String(), string(actor.Role)).Scan(&present)
	if result.Error != nil {
		return fmt.Errorf("check current media actor: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return identityapp.ErrForbidden
	}
	return nil
}

func requireProject(tx *gorm.DB, actor identityapp.Principal, projectID uuid.UUID, forWrite bool) error {
	if projectID == uuid.Nil {
		return ErrNotFound
	}
	var row struct{ Status string }
	result := tx.Raw(`
		SELECT status FROM workspace.project
		WHERE id = ?::uuid AND org_id = ?::uuid AND NOT is_delete AND status IN ('active','archived')
		FOR SHARE
	`, projectID.String(), actor.OrgID.String()).Scan(&row)
	if result.Error != nil {
		return fmt.Errorf("check media project: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrNotFound
	}
	if forWrite && row.Status != "active" {
		return ErrProjectStateConflict
	}
	return nil
}

func visibleAssetKey(tx *gorm.DB, actor identityapp.Principal, projectID, assetID uuid.UUID) (string, error) {
	if projectID == uuid.Nil || assetID == uuid.Nil {
		return "", ErrNotFound
	}
	var row struct{ ObjectKey string }
	result := tx.Raw(`
		SELECT a.object_key FROM media.media_asset AS a
		JOIN workspace.project AS p ON p.id = a.project_id
		WHERE a.id = ?::uuid AND a.project_id = ?::uuid AND p.org_id = ?::uuid
		  AND NOT a.is_delete AND NOT p.is_delete AND p.status IN ('active','archived')
		FOR SHARE OF a, p
	`, assetID.String(), projectID.String(), actor.OrgID.String()).Scan(&row)
	if result.Error != nil {
		return "", fmt.Errorf("check media asset scope: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return "", ErrNotFound
	}
	return row.ObjectKey, nil
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

type assetRow struct {
	ID                 uuid.UUID  `json:"id"`
	ProjectID          uuid.UUID  `json:"project_id"`
	PersonalOrgID      *uuid.UUID `json:"personal_org_id"`
	PersonalActorID    *uuid.UUID `json:"personal_actor_id"`
	Kind               string     `json:"kind"`
	Origin             string     `json:"origin"`
	Status             string     `json:"status"`
	ObjectKey          string     `json:"object_key"`
	FileName           string     `json:"file_name"`
	MimeType           string     `json:"mime_type"`
	ByteSize           int64      `json:"byte_size"`
	SHA256             *string    `json:"sha256"`
	Width              *int32     `json:"width"`
	Height             *int32     `json:"height"`
	DurationMS         *int32     `json:"duration_ms"`
	FPS                *float64   `json:"fps"`
	AudioChannels      *int32     `json:"audio_channels"`
	Codec              *string    `json:"codec"`
	SourceOperationID  *uuid.UUID `json:"source_operation_id"`
	ProviderKey        *string    `json:"provider_key"`
	ModelKey           *string    `json:"model_key"`
	Region             *string    `json:"region"`
	ModerationStatus   string     `json:"moderation_status"`
	ModerationDetail   []byte     `json:"moderation_detail"`
	AIGCMarked         bool       `json:"aigc_marked"`
	ContainsRealPerson bool       `json:"contains_real_person"`
	ConsentRecordID    *uuid.UUID `json:"consent_record_id"`
	UploadID           *string    `json:"upload_id"`
	FailureReason      *string    `json:"failure_reason"`
	DeleteTime         *time.Time `json:"delete_time"`
	PurgeAfter         *time.Time `json:"purge_after"`
	Revision           int64      `json:"revision"`
	CreateTime         time.Time  `json:"create_time"`
	UpdateTime         time.Time  `json:"update_time"`
	IsDelete           bool       `json:"is_delete"`
}

func (r assetRow) domain() domain.MediaAsset {
	var personal *domain.PersonalOwnership
	if r.PersonalOrgID != nil || r.PersonalActorID != nil {
		personal = &domain.PersonalOwnership{}
		if r.PersonalOrgID != nil {
			personal.OrgID = *r.PersonalOrgID
		}
		if r.PersonalActorID != nil {
			personal.ActorID = *r.PersonalActorID
		}
	}
	return domain.MediaAsset{
		ID: r.ID, ProjectID: r.ProjectID, Personal: personal, Kind: domain.Kind(r.Kind),
		Origin: domain.Origin(r.Origin), Status: domain.Status(r.Status),
		ObjectKey: r.ObjectKey, FileName: r.FileName, MimeType: r.MimeType,
		ByteSize: r.ByteSize, SHA256: r.SHA256, Width: r.Width, Height: r.Height,
		DurationMS: r.DurationMS, FPS: r.FPS, AudioChannels: r.AudioChannels,
		Codec: r.Codec, SourceOperationID: r.SourceOperationID,
		ProviderKey: r.ProviderKey, ModelKey: r.ModelKey, Region: r.Region,
		ModerationStatus: domain.ModerationStatus(r.ModerationStatus),
		ModerationDetail: json.RawMessage(r.ModerationDetail),
		AIGCMarked:       r.AIGCMarked, ContainsRealPerson: r.ContainsRealPerson,
		ConsentRecordID: r.ConsentRecordID, UploadID: r.UploadID,
		FailureReason: r.FailureReason, DeleteTime: r.DeleteTime,
		PurgeAfter: r.PurgeAfter, Revision: r.Revision,
		CreateTime: r.CreateTime, UpdateTime: r.UpdateTime, IsDelete: r.IsDelete,
	}
}

type renditionRow struct {
	ID           uuid.UUID
	MediaAssetID uuid.UUID
	Kind         string
	ObjectKey    string
	Width        *int32
	Height       *int32
	ByteSize     *int64
	CreateTime   time.Time
	UpdateTime   time.Time
	IsDelete     bool
}

func (r renditionRow) domain() domain.Rendition {
	return domain.Rendition{
		ID: r.ID, MediaAssetID: r.MediaAssetID,
		Kind: domain.RenditionKind(r.Kind), ObjectKey: r.ObjectKey,
		Width: r.Width, Height: r.Height, ByteSize: r.ByteSize,
		CreateTime: r.CreateTime, UpdateTime: r.UpdateTime, IsDelete: r.IsDelete,
	}
}

// ListReadyAssets uses an ID keyset after rechecking current actor and project visibility.
func (s *Store) ListReadyAssets(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID, kind string, after uuid.UUID, limit int) ([]domain.MediaAsset, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	var rows []assetRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := requireProject(tx, actor, projectID, false); err != nil {
			return err
		}
		query := `SELECT * FROM media.media_asset WHERE project_id=? AND NOT is_delete AND status='ready' AND moderation_status='passed'`
		if kind == "document" {
			query += ` AND kind='document'`
		} else {
			query += ` AND kind IN ('image','video','audio','model')`
		}
		args := []any{projectID}
		if kind != "" {
			query += ` AND kind=?`
			args = append(args, kind)
		}
		if after != uuid.Nil {
			query += ` AND id<?`
			args = append(args, after)
		}
		query += ` ORDER BY id DESC LIMIT ?`
		args = append(args, limit)
		return tx.Raw(query, args...).Scan(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	assets := make([]domain.MediaAsset, 0, len(rows))
	for _, r := range rows {
		a := r.domain()
		if err := a.Validate(); err != nil {
			return nil, fmt.Errorf("validate available media: %w", err)
		}
		assets = append(assets, a)
	}
	return assets, nil
}
