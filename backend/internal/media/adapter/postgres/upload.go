package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	workspacedomain "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

// AuthorizeUpload checks the current actor and active project before body reading.
func (s *Store) AuthorizeUpload(ctx context.Context, actor identityapp.Principal, projectID uuid.UUID) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrUnavailable
	}
	var aspect string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		aspect, err = s.authorizeUpload(tx, actor, projectID, false)
		return err
	})
	return aspect, err
}

func (s scopeAuthority) authorizeUpload(tx *gorm.DB, actor identityapp.Principal, projectID uuid.UUID, lock bool) (string, error) {
	if err := s.requireCurrentActor(tx, actor); err != nil {
		return "", err
	}
	if err := s.requireProject(tx, actor, projectID, true); err != nil {
		return "", err
	}
	if projectID == uuid.Nil {
		return "", ErrNotFound
	}
	query := tx.Table("workspace.project").Select("status", "aspect_ratio").Where("id = ? AND org_id = ? AND is_delete = ? AND status IN ?", projectID, actor.OrgID, false, []string{workspacedomain.ProjectStatusActive, workspacedomain.ProjectStatusArchived})
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	} else {
		query = query.Clauses(clause.Locking{Strength: "SHARE"})
	}
	var row struct {
		Status      string
		AspectRatio string
	}
	r := query.Take(&row)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	if r.Error != nil {
		return "", fmt.Errorf("authorize upload project: %w", r.Error)
	}
	if row.Status != workspacedomain.ProjectStatusActive {
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
		if _, err := s.authorizeUpload(tx, actor, r.ProjectID, false); err != nil {
			return err
		}
		var err error
		result, found, err = replayUpload(tx, actor, r)
		return err
	})
	return result, found, err
}

func replayUpload(tx *gorm.DB, actor identityapp.Principal, r application.UploadRequest) (application.UploadResult, bool, error) {
	var row uploadRequestRecord
	read := tx.Where(map[string]any{"project_id": r.ProjectID, "principal_id": actor.ID, "request_key": r.Key}).Take(&row)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.UploadResult{}, false, nil
	}
	if read.Error != nil {
		return application.UploadResult{}, false, fmt.Errorf("read upload receipt: %w", read.Error)
	}
	if row.SHA256 != r.SHA256 || row.FileName != r.FileName || row.ByteSize != r.ByteSize || row.SourceKind != r.SourceKind() {
		return application.UploadResult{}, false, application.ErrUploadConflict
	}
	var result application.UploadResult
	if err := json.Unmarshal(row.Response, &result); err != nil || result.Asset.ID != row.AssetID || result.Asset.ProjectID != r.ProjectID {
		return result, false, ErrUnavailable
	}
	var available assetRow
	read = tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(map[string]any{"id": row.AssetID, "project_id": r.ProjectID, "origin": domain.OriginUpload, "status": domain.StatusReady, "moderation_status": domain.ModerationPassed, "is_delete": false, "contains_real_person": false}).Take(&available)
	if errors.Is(read.Error, gorm.ErrRecordNotFound) {
		return application.UploadResult{}, false, application.ErrUploadConflict
	}
	if read.Error != nil {
		return application.UploadResult{}, false, read.Error
	}
	if err := available.loadLocation(tx); err != nil {
		return application.UploadResult{}, false, err
	}
	if available.domain().Validate() != nil {
		return application.UploadResult{}, false, ErrUnavailable
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
		if _, err := s.authorizeUpload(tx, actor, r.ProjectID, true); err != nil {
			return err
		}
		if prior, found, err := replayUpload(tx, actor, r); err != nil || found {
			result = prior
			return err
		}
		var existing assetRow
		read := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("project_id = ?", r.ProjectID).
			Where("(sha256 = ? OR (? AND moderation_detail->'normalization'->'source'->>'sha256' = ? AND moderation_detail->'normalization'->>'method' = ? AND moderation_detail->'normalization'->>'version' = ?))", *asset.SHA256, *asset.SHA256 != r.SHA256, r.SHA256, "webm_vp8_vp9_to_mp4_h264", "1").
			Where(map[string]any{"origin": domain.OriginUpload, "status": domain.StatusReady, "moderation_status": domain.ModerationPassed, "is_delete": false, "contains_real_person": false}).
			Where("moderation_detail->>'method' = ?", "local_workspace_owner_review").Order("create_time").Order("id").Limit(1).Take(&existing)
		if read.Error != nil {
			if errors.Is(read.Error, gorm.ErrRecordNotFound) {
				read.Error = nil
			} else {
				return fmt.Errorf("find identical local upload: %w", read.Error)
			}
		}
		if read.Error == nil && existing.ID != uuid.Nil {
			if err := existing.loadLocation(tx); err != nil {
				return err
			}
			duplicate := existing.domain()
			if err := duplicate.Validate(); err != nil {
				return ErrUnavailable
			}
			id := duplicate.ID
			result = application.UploadResult{Asset: application.UploadSummary(duplicate), DuplicateOf: &id}
		} else {
			if err := insertReviewedUpload(tx, asset, renditions); err != nil {
				return err
			}
			result = application.UploadResult{Asset: application.UploadSummary(asset)}
		}
		if err := acceptUploadLocations(tx, actor, r, asset, renditions, result.Asset.ID); err != nil {
			return err
		}
		body, err := json.Marshal(result)
		if err != nil {
			return err
		}
		receipt := uploadRequestRecord{ProjectID: &r.ProjectID, PrincipalID: actor.ID, RequestKey: r.Key, SHA256: r.SHA256, FileName: r.FileName, SourceKind: r.SourceKind(), ByteSize: r.ByteSize, AssetID: result.Asset.ID, Response: jsonDocument(body)}
		if err := tx.Create(&receipt).Error; err != nil {
			return fmt.Errorf("insert upload receipt: %w", err)
		}
		after, err := json.Marshal(map[string]any{"kind": result.Asset.Kind, "byte_size": result.Asset.ByteSize, "sha256": r.SHA256, "review_method": "local_workspace_owner_review", "reused": result.DuplicateOf != nil})
		if err != nil {
			return err
		}
		audit := uploadAuditRecord{ID: uuid.New(), OrgID: actor.OrgID, ProjectID: &r.ProjectID, ActorID: actor.ID, ActorKind: "user", Action: "media.uploaded", ObjectType: "media_asset", ObjectID: result.Asset.ID.String(), After: jsonDocument(after), RequestID: r.RequestID.String(), CreateTime: asset.CreateTime, UpdateTime: asset.CreateTime}
		return tx.Create(&audit).Error
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
	if asset.ProjectID != r.ProjectID || !sameUploadOwnership(actor, r, asset) || asset.Origin != domain.OriginUpload || asset.Status != domain.StatusReady || asset.ModerationStatus != domain.ModerationPassed || asset.IsDelete || asset.ContainsRealPerson || asset.ConsentRecordID != nil || asset.Revision != 1 || asset.SHA256 == nil {
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
	case domain.KindModel, domain.KindDocument:
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
		var project struct{ ID uuid.UUID }
		read := tx.Table("workspace.project").Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", projectID).Take(&project)
		if errors.Is(read.Error, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if read.Error != nil {
			return read.Error
		}
		return tx.Model(&assetRow{}).Where("id = ? AND project_id = ?", id, projectID).Count(&count).Error
	})
	return count != 0, err
}
