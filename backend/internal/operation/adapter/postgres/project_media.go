package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type projectMediaLocationRecord struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	ProfileID string
	Bucket    string
	ObjectKey string
	SHA256    *string
	ByteSize  *int64
	State     mediadomain.ObjectLocationState
}

func (projectMediaLocationRecord) TableName() string { return "media.object_location" }

// WithProjectMedia installs exact-location signing without widening ordinary media.
func (s *Store) WithProjectMedia(f func(*gorm.DB) application.ProjectMediaSigner) *Store {
	s.projectMedia = f
	return s
}

// ManagedProjectMediaReference keeps the original project/SHA/consent guards and
// refuses a routed failure rather than falling back to the current bucket.
func (s *Store) ManagedProjectMediaReference(ctx context.Context, op, project, input uuid.UUID) (application.ProviderReference, bool, error) {
	if s == nil || s.projectMedia == nil {
		return application.ProviderReference{}, false, nil
	}
	var out application.ProviderReference
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireWorker(tx, op, true); err != nil {
			return err
		}
		if err := s.withDB(tx).CheckSubmissionInputs(ctx, op); err != nil {
			return err
		}
		var r struct {
			AssetID, ProjectID                                  uuid.UUID
			LocationID                                          *uuid.UUID
			ProfileID, Bucket, Key, SHA256, Kind, Role, RefType string
			ByteSize                                            int64
			RefVersion                                          *string
			LocationSHA                                         *string
			LocationSize                                        *int64
			LocationKey                                         string
			LocationState                                       mediadomain.ObjectLocationState
		}
		query := tx.Table("operation.operation_input AS i").
			Select("a.id AS asset_id, a.project_id, a.object_location_id AS location_id, a.object_key AS key, a.sha256, a.kind, a.byte_size, i.role, i.ref_type, i.ref_version").
			Joins("JOIN operation.operation AS o ON o.id = i.operation_id").
			Joins("JOIN media.media_asset AS a ON a.id = i.media_asset_id AND a.project_id = o.project_id").
			Where("i.id = ? AND i.operation_id = ? AND o.project_id = ?", input, op, project).
			Where("i.ref_type <> ? AND i.mask_asset_id IS NULL AND i.is_delete = ? AND o.is_delete = ? AND a.is_delete = ?", "subject_media", false, false, false).
			Where("a.status = ? AND a.moderation_status IN ?", mediadomain.StatusReady, []mediadomain.ModerationStatus{mediadomain.ModerationPassed, mediadomain.ModerationSkipped}).
			Clauses(clause.Locking{Strength: "SHARE", Table: clause.Table{Name: "a"}})
		if err := query.Take(&r).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return application.ErrWorkflowInputNotReady
		} else if err != nil {
			return err
		}
		if r.AssetID == uuid.Nil || r.Key == "" {
			return application.ErrWorkflowInputNotReady
		}
		if r.LocationID != nil {
			var location projectMediaLocationRecord
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ? AND object_key = ?", *r.LocationID, r.Key).Take(&location).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return application.ErrWorkflowInputNotReady
				}
				return err
			}
			r.ProfileID, r.Bucket, r.LocationSHA, r.LocationSize, r.LocationKey, r.LocationState = location.ProfileID, location.Bucket, location.SHA256, location.ByteSize, location.ObjectKey, location.State
		}
		if r.RefType == "media_asset" && r.RefVersion != nil && *r.RefVersion != r.SHA256 {
			return application.ErrWorkflowInputNotReady
		}
		if r.LocationID != nil && (r.ProfileID == "" || r.Bucket == "" || r.LocationState != mediadomain.ObjectLocationAccepted || r.LocationKey != r.Key || r.LocationSHA == nil || *r.LocationSHA != r.SHA256 || r.LocationSize == nil || *r.LocationSize != r.ByteSize) {
			return application.ErrWorkflowInputNotReady
		}
		var err error
		out, err = s.projectMedia(tx).SignProjectMedia(ctx, application.ProjectMediaObject{AssetID: r.AssetID, ProjectID: r.ProjectID, LocationID: r.LocationID, ProfileID: r.ProfileID, Bucket: r.Bucket, Key: r.Key, SHA256: r.SHA256, Kind: r.Kind, Role: r.Role, ByteSize: r.ByteSize}, 15*time.Minute)
		return err
	})
	return out, true, err
}
