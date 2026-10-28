package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

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
			LocationKey, LocationState                          string
		}
		q := tx.Raw(`SELECT a.id AS asset_id,a.project_id,a.object_location_id AS location_id,a.object_key AS key,a.sha256,a.kind,a.byte_size,i.role,i.ref_type,i.ref_version,coalesce(l.profile_id,'') AS profile_id,coalesce(l.bucket,'') AS bucket,l.sha256 AS location_sha,l.byte_size AS location_size,coalesce(l.object_key,'') AS location_key,coalesce(l.state,'') AS location_state FROM operation.operation_input i JOIN operation.operation o ON o.id=i.operation_id JOIN media.media_asset a ON a.id=i.media_asset_id AND a.project_id=o.project_id LEFT JOIN media.object_location l ON l.id=a.object_location_id WHERE i.id=? AND i.operation_id=? AND o.project_id=? AND i.ref_type<>'subject_media' AND i.mask_asset_id IS NULL AND NOT i.is_delete AND NOT o.is_delete AND NOT a.is_delete AND a.status='ready' AND a.moderation_status IN ('passed','skipped') FOR SHARE OF o,a`, input, op, project).Scan(&r)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 || r.AssetID == uuid.Nil || r.Key == "" {
			return application.ErrWorkflowInputNotReady
		}
		if r.RefType == "media_asset" && r.RefVersion != nil && *r.RefVersion != r.SHA256 {
			return application.ErrWorkflowInputNotReady
		}
		if r.LocationID != nil && (r.ProfileID == "" || r.Bucket == "" || r.LocationState != "accepted" || r.LocationKey != r.Key || r.LocationSHA == nil || *r.LocationSHA != r.SHA256 || r.LocationSize == nil || *r.LocationSize != r.ByteSize) {
			return application.ErrWorkflowInputNotReady
		}
		var err error
		out, err = s.projectMedia(tx).SignProjectMedia(ctx, application.ProjectMediaObject{AssetID: r.AssetID, ProjectID: r.ProjectID, LocationID: r.LocationID, ProfileID: r.ProfileID, Bucket: r.Bucket, Key: r.Key, SHA256: r.SHA256, Kind: r.Kind, Role: r.Role, ByteSize: r.ByteSize}, 15*time.Minute)
		return err
	})
	return out, true, err
}
