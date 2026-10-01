package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// AuthorizeUpload checks the current actor and active project before body reading.
func (s *Store) AuthorizeUpload(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrUnavailable
	}
	var aspect string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		aspect, err = authorizeUpload(tx, actor, projectID, false)
		return err
	})
	return aspect, err
}

func authorizeUpload(tx *gorm.DB, actor identityapp.Principal, projectID uuid.UUID, lock bool) (string, error) {
	if err := requireCurrentActor(tx, actor); err != nil {
		return "", err
	}
	if projectID == uuid.Nil {
		return "", ErrNotFound
	}
	query := `SELECT status, aspect_ratio FROM workspace.project WHERE id=? AND org_id=? AND NOT is_delete AND status IN ('active','archived')`
	if lock {
		query += ` FOR UPDATE`
	} else {
		query += ` FOR SHARE`
	}
	var row struct {
		Status      string
		AspectRatio string
	}
	r := tx.Raw(query, projectID, actor.OrgID).Scan(&row)
	if r.Error != nil {
		return "", fmt.Errorf("authorize upload project: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return "", ErrNotFound
	}
	if row.Status != "active" {
		return "", ErrProjectStateConflict
	}
	return row.AspectRatio, nil
}

// FindUpload returns the durable response only for an identical request body.
func (s *Store) FindUpload(ctx context.Context, actor identityapp.Principal, r application.UploadRequest) (application.UploadResult, bool, error) {
	if s == nil || s.db == nil {
		return application.UploadResult{}, false, ErrUnavailable
	}
	if err := application.ValidateUploadRequest(r); err != nil {
		return application.UploadResult{}, false, err
	}
	var result application.UploadResult
	var found bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeUpload(tx, actor, r.ProjectID, false); err != nil {
			return err
		}
		var err error
		result, found, err = replayUpload(tx, actor, r)
		return err
	})
	return result, found, err
}

func replayUpload(tx *gorm.DB, actor identityapp.Principal, r application.UploadRequest) (application.UploadResult, bool, error) {
	var row struct {
		SHA256   string
		FileName string
		ByteSize int64
		AssetID  uuid.UUID
		Response json.RawMessage
	}
	read := tx.Raw(`SELECT sha256,file_name,byte_size,asset_id,response FROM media.upload_request WHERE project_id=? AND principal_id=? AND request_key=?`, r.ProjectID, actor.ID, r.Key).Scan(&row)
	if read.Error != nil {
		return application.UploadResult{}, false, fmt.Errorf("read upload receipt: %w", read.Error)
	}
	if read.RowsAffected == 0 {
		return application.UploadResult{}, false, nil
	}
	if row.SHA256 != r.SHA256 || row.FileName != r.FileName || row.ByteSize != r.ByteSize {
		return application.UploadResult{}, false, application.ErrUploadConflict
	}
	var result application.UploadResult
	if err := json.Unmarshal(row.Response, &result); err != nil || result.Asset.ID != row.AssetID || result.Asset.ProjectID != r.ProjectID {
		return result, false, ErrUnavailable
	}
	var available int
	read = tx.Raw(`SELECT 1 FROM media.media_asset WHERE id=? AND project_id=? AND origin='upload' AND status='ready' AND moderation_status='passed' AND NOT is_delete AND NOT contains_real_person FOR SHARE`, row.AssetID, r.ProjectID).Scan(&available)
	if read.Error != nil {
		return application.UploadResult{}, false, read.Error
	}
	if read.RowsAffected != 1 {
		return application.UploadResult{}, false, application.ErrUploadConflict
	}
	return result, true, nil
}

// CommitUpload serializes project deduplication and saves assets, renditions,
// the explicit human declaration, audit and idempotency response atomically.
func (s *Store) CommitUpload(ctx context.Context, actor identityapp.Principal, r application.UploadRequest, asset domain.MediaAsset, renditions []domain.Rendition) (application.UploadResult, error) {
	if s == nil || s.db == nil {
		return application.UploadResult{}, ErrUnavailable
	}
	if err := validateReviewedUpload(actor, r, asset, renditions); err != nil {
		return application.UploadResult{}, err
	}
	var result application.UploadResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeUpload(tx, actor, r.ProjectID, true); err != nil {
			return err
		}
		if prior, found, err := replayUpload(tx, actor, r); err != nil || found {
			result = prior
			return err
		}
		var existing assetRow
		read := tx.Raw(`SELECT * FROM media.media_asset WHERE project_id=? AND (sha256=? OR (? AND moderation_detail->'normalization'->'source'->>'sha256'=? AND moderation_detail->'normalization'->>'method'='webm_vp8_vp9_to_mp4_h264' AND moderation_detail->'normalization'->>'version'='1')) AND origin='upload' AND status='ready' AND moderation_status='passed' AND NOT is_delete AND NOT contains_real_person AND moderation_detail->>'method'='local_workspace_owner_review' ORDER BY create_time,id LIMIT 1 FOR SHARE`, r.ProjectID, *asset.SHA256, *asset.SHA256 != r.SHA256, r.SHA256).Scan(&existing)
		if read.Error != nil {
			return fmt.Errorf("find identical local upload: %w", read.Error)
		}
		if read.RowsAffected == 1 {
			duplicate := existing.domain()
			if err := duplicate.Validate(); err != nil {
				return ErrUnavailable
			}
			id := duplicate.ID
			result = application.UploadResult{Asset: application.UploadSummary(duplicate), DuplicateOf: &id}
		} else {
			if err := tx.Exec(`INSERT INTO media.media_asset
				(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,duration_ms,fps,audio_channels,codec,moderation_status,moderation_detail,revision,create_time,update_time)
				VALUES(?, ?, ?, 'upload', 'ready', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'passed', ?::jsonb, 1, ?, ?)`,
				asset.ID, asset.ProjectID, string(asset.Kind), asset.ObjectKey, asset.FileName, asset.MimeType, asset.ByteSize, asset.SHA256,
				asset.Width, asset.Height, asset.DurationMS, asset.FPS, asset.AudioChannels, asset.Codec, string(asset.ModerationDetail), asset.CreateTime, asset.UpdateTime).Error; err != nil {
				return fmt.Errorf("insert reviewed upload: %w", err)
			}
			for _, rendition := range renditions {
				if err := tx.Exec(`INSERT INTO media.rendition(id,media_asset_id,kind,object_key,width,height,byte_size,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?)`,
					rendition.ID, rendition.MediaAssetID, string(rendition.Kind), rendition.ObjectKey, rendition.Width, rendition.Height, rendition.ByteSize, rendition.CreateTime, rendition.UpdateTime).Error; err != nil {
					return fmt.Errorf("insert upload rendition: %w", err)
				}
			}
			result = application.UploadResult{Asset: application.UploadSummary(asset)}
		}
		body, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO media.upload_request(project_id,principal_id,request_key,sha256,file_name,byte_size,asset_id,response) VALUES(?,?,?,?,?,?,?,?::jsonb)`,
			r.ProjectID, actor.ID, r.Key, r.SHA256, r.FileName, r.ByteSize, result.Asset.ID, string(body)).Error; err != nil {
			return fmt.Errorf("insert upload receipt: %w", err)
		}
		after, err := json.Marshal(map[string]any{"kind": result.Asset.Kind, "byte_size": result.Asset.ByteSize, "sha256": r.SHA256, "review_method": "local_workspace_owner_review", "reused": result.DuplicateOf != nil})
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO audit.audit_log(id,org_id,project_id,actor_id,actor_kind,action,object_type,object_id,after,request_id,create_time,update_time)
			VALUES(?,?,?,?, 'user','media.uploaded','media_asset',?,?::jsonb,?,?,?)`, uuid.New(), actor.OrgID, r.ProjectID, actor.ID, result.Asset.ID.String(), string(after), r.RequestID.String(), asset.CreateTime, asset.CreateTime).Error
	})
	return result, err
}

func validateReviewedUpload(actor identityapp.Principal, r application.UploadRequest, asset domain.MediaAsset, renditions []domain.Rendition) error {
	if err := application.ValidateUploadRequest(r); err != nil {
		return err
	}
	if err := asset.Validate(); err != nil {
		return err
	}
	if asset.ProjectID != r.ProjectID || asset.Origin != domain.OriginUpload || asset.Status != domain.StatusReady || asset.ModerationStatus != domain.ModerationPassed || asset.IsDelete || asset.ContainsRealPerson || asset.ConsentRecordID != nil || asset.Revision != 1 || asset.SHA256 == nil {
		return application.ErrInvalidUpload
	}
	var review application.LocalUploadReview
	decoder := json.NewDecoder(bytes.NewReader(asset.ModerationDetail))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&review); err != nil || review.Method != "local_workspace_owner_review" || review.PrincipalID != actor.ID || !review.ReviewedAt.Equal(asset.CreateTime) || review.SHA256 != r.SHA256 || !review.RightsConfirmed || !review.NoAuthorizationRequiredRealPerson {
		return application.ErrInvalidUpload
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return application.ErrInvalidUpload
	}
	if review.Normalization == nil {
		if *asset.SHA256 != r.SHA256 || asset.ByteSize != r.ByteSize || asset.FileName != r.FileName {
			return application.ErrInvalidUpload
		}
	} else if err := review.Normalization.Validate(r, asset); err != nil {
		return err
	}
	var required []domain.RenditionKind
	switch asset.Kind {
	case domain.KindImage:
		required = []domain.RenditionKind{domain.RenditionThumb256, domain.RenditionThumb640}
	case domain.KindVideo:
		required = []domain.RenditionKind{domain.RenditionPoster, domain.RenditionProxy720p}
	case domain.KindAudio:
		required = []domain.RenditionKind{domain.RenditionWaveform}
	case domain.KindModel:
		required = nil
	default:
		return application.ErrInvalidUpload
	}
	if len(renditions) != len(required) {
		return application.ErrInvalidUpload
	}
	prefix := strings.TrimSuffix(asset.ObjectKey, path.Ext(asset.ObjectKey)) + "/"
	for i, rendition := range renditions {
		if err := rendition.Validate(); err != nil {
			return err
		}
		if rendition.MediaAssetID != asset.ID || rendition.Kind != required[i] || rendition.IsDelete || rendition.Width == nil || rendition.Height == nil || rendition.ByteSize == nil || *rendition.ByteSize < 1 || !strings.HasPrefix(rendition.ObjectKey, prefix) || path.Base(rendition.ObjectKey) != string(rendition.Kind)+path.Ext(rendition.ObjectKey) {
			return application.ErrInvalidUpload
		}
	}
	return nil
}

// UploadAssetExists is the conservative cleanup check after an unknown commit.
// It intentionally includes deleted assets: their objects still have durable owners.
func (s *Store) UploadAssetExists(ctx context.Context, projectID, id uuid.UUID) (bool, error) {
	if s == nil || s.db == nil || projectID == uuid.Nil || id == uuid.Nil {
		return false, ErrUnavailable
	}
	var count int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var present int
		read := tx.Raw(`SELECT 1 FROM workspace.project WHERE id=? FOR UPDATE`, projectID).Scan(&present)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 {
			return ErrUnavailable
		}
		return tx.Raw(`SELECT count(*) FROM media.media_asset WHERE id=? AND project_id=?`, id, projectID).Scan(&count).Error
	})
	return count != 0, err
}
