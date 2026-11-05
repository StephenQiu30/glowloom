package postgres

import (
	"encoding/hex"
	"strings"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediatoolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

// storageOwnerColumn is a closed registry of database rows that retain an
// object location. Identifiers come only from this package-level allowlist.
type storageOwnerColumn struct {
	table, key, location string
}

var storageOwnerColumns = []storageOwnerColumn{
	{"script.script_source", "original_key", "original_location_id"},
	{"script.script_source", "rich_key", "rich_location_id"},
	{"script.script_version", "text_key", "text_location_id"},
	{"script.script_version", "rich_key", "rich_location_id"},
	{"script.copy_object_intent", "source_key", "source_location_id"},
	{"script.copy_object_intent", "target_key", "target_location_id"},
	{"script.copy_object_state", "target_key", "target_location_id"},
	{"media.media_asset", "object_key", "object_location_id"},
	{"media.rendition", "object_key", "object_location_id"},
	{"media.package_object", "object_key", "object_location_id"},
	{"media.project_copy_object", "source_object_key", "source_location_id"},
	{"media.project_copy_object", "target_object_key", "target_location_id"},
	{"media.transfer_object", "source_object_key", "source_location_id"},
	{"media.transfer_object", "target_object_key", "target_location_id"},
	{"media.purge_object", "object_key", "object_location_id"},
	{"mediatool.depth_object", "object_key", "object_location_id"},
	{"script.object_intent", "object_key", "object_location_id"},
	{"script.object_state", "object_key", "object_location_id"},
}

func storageDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}

func storageOwnerRetired(tx *gorm.DB, owner storageOwnerColumn, row map[string]any) (bool, error) {
	var count int64
	switch owner.table {
	case "media.media_asset", "media.rendition":
		if !storageRowBool(row, "is_delete") {
			return false, nil
		}
		query := tx.Table("media.purge_object").
			Where("object_key = ? AND object_location_id = ? AND removed = ?", storageRowValue(row, owner.key), storageRowValue(row, owner.location), true)
		if err := query.Count(&count).Error; err != nil {
			return false, err
		}
		return count > 0, nil
	case "media.package_object":
		return storageRowString(row, "status") == string(application.PackageObjectStatusRemoved), nil
	case "mediatool.depth_object":
		return storageRowString(row, "status") == string(mediatoolapp.DepthObjectStatusRemoved), nil
	case "media.project_copy_object", "media.transfer_object":
		if owner.location == "target_location_id" {
			return storageRowString(row, "status") == string(application.ProjectCopyObjectRemoved), nil
		}
		query := tx.Table("media.purge_object").
			Where("object_key = ? AND object_location_id = ? AND removed = ?", storageRowValue(row, owner.key), storageRowValue(row, owner.location), true)
		if err := query.Count(&count).Error; err != nil {
			return false, err
		}
		return count > 0, nil
	case "media.purge_object":
		return storageRowBool(row, "removed"), nil
	case "script.object_intent":
		query := tx.Table("script.object_state").
			Where("object_key = ? AND object_location_id = ? AND removed = ?", storageRowValue(row, owner.key), storageRowValue(row, owner.location), true)
		if err := query.Count(&count).Error; err != nil {
			return false, err
		}
		return count > 0, nil
	case "script.object_state", "script.copy_object_state":
		return storageRowBool(row, "removed"), nil
	case "script.copy_object_intent":
		if owner.location != "target_location_id" {
			return false, nil
		}
		query := tx.Table("script.copy_object_state").
			Where("target_key = ? AND target_location_id = ? AND removed = ?", storageRowValue(row, owner.key), storageRowValue(row, owner.location), true)
		if err := query.Count(&count).Error; err != nil {
			return false, err
		}
		return count > 0, nil
	default:
		return false, nil
	}
}

func storageRowValue(row map[string]any, column string) any {
	return row[column]
}

func storageRowString(row map[string]any, column string) string {
	value, _ := row[column].(string)
	if value != "" {
		return value
	}
	if bytes, ok := row[column].([]byte); ok {
		return string(bytes)
	}
	return ""
}

func storageRowBool(row map[string]any, column string) bool {
	switch value := row[column].(type) {
	case bool:
		return value
	case string:
		return value == "true"
	case []byte:
		return string(value) == "true"
	default:
		return false
	}
}
