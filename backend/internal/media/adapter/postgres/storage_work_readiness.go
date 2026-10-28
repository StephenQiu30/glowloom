package postgres

import (
	"context"
	"errors"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// ReadyForWork checks only explicit existing work-storage dependencies and route.
// Missing legacy structure is an expected unavailable state. Permission, network
// and other database failures are returned; no migration or activation occurs.
func (s *StorageRoutes) ReadyForWork(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	var complete bool
	if err := s.db.WithContext(ctx).Raw(`SELECT bool_and(EXISTS(SELECT 1 FROM pg_catalog.pg_attribute a WHERE a.attrelid=to_regclass(required.relation) AND a.attname=required.column_name AND a.attnum>0 AND NOT a.attisdropped)) FROM (VALUES
 ('media.storage_profile','id'),('media.storage_profile','bucket'),('media.storage_profile','resource_identity'),
 ('media.storage_route','singleton'),('media.storage_route','profile_id'),('media.storage_route','revision'),
 ('media.storage_work_intent','id'),('media.storage_work_intent','kind'),('media.storage_work_intent','owner_id'),('media.storage_work_intent','work_id'),('media.storage_work_intent','profile_id'),('media.storage_work_intent','bucket'),('media.storage_work_intent','config_revision'),('media.storage_work_intent','state'),('media.storage_work_intent','closed_at'),
 ('media.object_location','id'),('media.object_location','profile_id'),('media.object_location','bucket'),('media.object_location','object_key'),('media.object_location','state'),('media.object_location','byte_size'),('media.object_location','sha256'),('media.object_location','mime_type'),('media.object_location','owner_intent_id'),('media.object_location','owner_session_id'),('media.object_location','write_until')
 ) AS required(relation,column_name)`).Scan(&complete).Error; err != nil {
		return false, err
	}
	if !complete {
		return false, nil
	}
	route, err := s.ReadActive(ctx)
	if errors.Is(err, application.ErrStorageNotReady) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return route.ProfileID != "" && route.Bucket != "" && route.Revision != "", nil
}
