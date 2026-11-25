package postgres

import (
	"context"
	"errors"

	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// ReadyForWork checks only explicit existing work-storage dependencies and route.
// Missing required structure is an expected unavailable state. Permission, network
// and other database failures are returned; this read does not apply configuration.
func (s *StorageRoutes) ReadyForWork(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	requiredTables := []struct {
		model   interface{ TableName() string }
		columns []string
	}{
		{&storageProfileRecord{}, []string{"id", "bucket", "resource_identity"}},
		{&storageRouteRecord{}, []string{"singleton", "profile_id", "revision"}},
		{&storageWorkRow{}, []string{"id", "kind", "owner_id", "work_id", "profile_id", "bucket", "config_revision", "state", "closed_at"}},
		{&objectLocationRow{}, []string{"id", "profile_id", "bucket", "object_key", "state", "byte_size", "sha256", "mime_type", "owner_intent_id", "owner_session_id", "write_until"}},
	}
	for _, required := range requiredTables {
		name := strings.SplitN(required.model.TableName(), ".", 2)
		var columns []struct{ Name string }
		// Catalog discovery does not abort the caller transaction when a dependency
		// is absent and does not hide existing columns merely because SELECT is denied.
		if err := s.db.WithContext(ctx).Table("pg_catalog.pg_attribute AS a").
			Select("a.attname AS name").
			Joins("JOIN pg_catalog.pg_class AS c ON c.oid = a.attrelid").
			Joins("JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace").
			Where("n.nspname = ? AND c.relname = ? AND a.attnum > 0 AND NOT a.attisdropped", name[0], name[1]).Find(&columns).Error; err != nil {
			return false, err
		}
		present := make(map[string]bool, len(columns))
		for _, column := range columns {
			present[column.Name] = true
		}
		for _, column := range required.columns {
			if !present[column] {
				return false, nil
			}
		}
		rows, err := s.db.WithContext(ctx).Model(required.model).Select(required.columns).Limit(0).Rows()
		if err != nil {
			return false, err
		}
		if err := rows.Close(); err != nil {
			return false, err
		}
	}
	route, err := s.ReadConfigured(ctx)
	if errors.Is(err, application.ErrStorageNotReady) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return route.ProfileID != "" && route.Bucket != "" && route.Revision != "", nil
}
