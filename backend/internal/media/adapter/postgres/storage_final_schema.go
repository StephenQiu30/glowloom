package postgres

import (
	"context"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// CheckStorageFinalSchema is the executable operator's finalization gate. The
// caller holds its transaction through activation, retaining these relation
// locks. Expanded nullable columns and global bare-key uniqueness are a staged
// migration, never an accepted multi-profile final schema.
func (s *StorageRoutes) CheckStorageFinalSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return application.ErrUnavailable
	}
	tables := make([]string, 0, len(legacyObjectColumns))
	seen := map[string]bool{}
	for _, column := range legacyObjectColumns {
		if !seen[column.table] {
			tables = append(tables, column.table)
			seen[column.table] = true
		}
	}
	if err := s.db.WithContext(ctx).Exec(`LOCK TABLE ` + strings.Join(tables, ",") + ` IN SHARE UPDATE EXCLUSIVE MODE`).Error; err != nil {
		return err
	}
	for _, column := range legacyObjectColumns {
		var ready bool
		if err := s.db.WithContext(ctx).Raw(`SELECT a.attnotnull AND EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_attribute k ON k.attrelid=c.conrelid AND k.attname=? WHERE c.conrelid=a.attrelid AND c.contype='f' AND c.convalidated AND c.confrelid='media.object_location'::regclass AND c.conkey @> ARRAY[a.attnum,k.attnum]::smallint[]) AND NOT EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_attribute k ON k.attrelid=c.conrelid AND k.attname=? WHERE c.conrelid=a.attrelid AND c.contype IN ('p','u') AND c.conkey=ARRAY[k.attnum]::smallint[]) AND NOT EXISTS(SELECT 1 FROM pg_index i JOIN pg_attribute k ON k.attrelid=i.indrelid AND k.attname=? WHERE i.indrelid=a.attrelid AND i.indisunique AND i.indnkeyatts=1 AND i.indkey[0]=k.attnum) FROM pg_attribute a WHERE a.attrelid=?::regclass AND a.attname=? AND NOT a.attisdropped`, column.key, column.key, column.key, column.table, column.location).Scan(&ready).Error; err != nil {
			return err
		}
		if !ready {
			return application.ErrStorageNotReady
		}
	}
	var bareKeyCheck bool
	if err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid IN ('media.project_copy_object'::regclass,'media.transfer_object'::regclass) AND contype='c' AND pg_get_constraintdef(oid) LIKE '%source_object_key%<>%target_object_key%')`).Scan(&bareKeyCheck).Error; err != nil {
		return err
	}
	if bareKeyCheck {
		return application.ErrStorageNotReady
	}
	return nil
}
