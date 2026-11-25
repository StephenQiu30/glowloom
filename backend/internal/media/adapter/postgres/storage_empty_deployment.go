package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// These owners can retain work even when no physical-object row exists yet.
var emptyStorageWorkTables = []string{
	"operation.operation", "media.storage_work_intent", "media.upload_admission", "media.upload_request",
	"media.package_job", "media.transfer_job", "media.purge_job", "media.project_copy_snapshot",
	"mediatool.depth_job", "mediatool.export_job", "mediatool.transcription_job",
	"script.command", "script.import_job", "script.copy_snapshot", "script.request", "workspace.project_copy_job", "subject.voice_job",
}

func storageOwnerTables() []string {
	seen := map[string]bool{}
	var tables []string
	for _, column := range storageOwnerColumns {
		if !seen[column.table] {
			tables = append(tables, column.table)
			seen[column.table] = true
		}
	}
	for _, table := range append([]string{"media.object_location"}, emptyStorageWorkTables...) {
		if !seen[table] {
			tables = append(tables, table)
			seen[table] = true
		}
	}
	sort.Strings(tables)
	return tables
}

// lockStorageOwners serializes an empty-state check with storage admissions.
func lockStorageOwners(tx *gorm.DB) error { return lockStorageRoute(tx) }

func storageHasNoRows(tx *gorm.DB) (bool, error) {
	for _, table := range storageOwnerTables() {
		var count int64
		if err := tx.Table(table).Limit(1).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}
	for _, model := range []any{&storageRouteRecord{}} {
		used, err := storageExists(tx.Model(model))
		if err != nil {
			return false, err
		}
		if used {
			return false, nil
		}
	}
	return true, nil
}

// ProveEmptyStorageDeployment measures a new database before environment setup. It
// records an immutable proof so later schema migration does not infer a bucket.
func (s *StorageRoutes) ProveEmptyStorageDeployment(ctx context.Context, configSHA string) (string, error) {
	if s == nil || s.db == nil || !storageDigest(configSHA) {
		return "", application.ErrStorageNotReady
	}
	var evidence string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockStorageOwners(tx); err != nil {
			return err
		}
		empty, err := storageHasNoRows(tx)
		if err != nil {
			return err
		}
		if !empty {
			return application.ErrStorageNotReady
		}
		body, digest, err := emptyStorageInventory(configSHA)
		if err != nil {
			return err
		}
		evidence = digest
		record := storageEmptyDeploymentEvidenceRecord{ConfigSHA256: configSHA, EvidenceSHA256: digest, Inventory: jsonDocument(body)}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "config_sha256"}}, DoNothing: true}).Create(&record).Error; err != nil {
			return err
		}
		return checkEmptyStorageInventory(tx, configSHA)
	})
	return evidence, err
}

func emptyStorageInventory(configSHA string) ([]byte, string, error) {
	body, err := json.Marshal(struct {
		SchemaVersion int      `json:"schema_version"`
		ConfigSHA256  string   `json:"config_sha256"`
		Tables        []string `json:"empty_tables"`
	}{1, configSHA, storageOwnerTables()})
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), nil
}

func checkEmptyStorageInventory(tx *gorm.DB, configSHA string) error {
	_, digest, err := emptyStorageInventory(configSHA)
	if err != nil {
		return err
	}
	var record storageEmptyDeploymentEvidenceRecord
	err = tx.Where("config_sha256 = ? AND evidence_sha256 = ?", configSHA, digest).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrStorageNotReady
	}
	if err != nil {
		return err
	}
	var inventory struct {
		SchemaVersion int      `json:"schema_version"`
		ConfigSHA256  string   `json:"config_sha256"`
		Tables        []string `json:"empty_tables"`
	}
	if json.Unmarshal(record.Inventory, &inventory) != nil || inventory.SchemaVersion != 1 || inventory.ConfigSHA256 != configSHA {
		return application.ErrStorageNotReady
	}
	_, expectedDigest, err := emptyStorageInventory(configSHA)
	if err != nil {
		return err
	}
	if expectedDigest != record.EvidenceSHA256 || len(inventory.Tables) != len(storageOwnerTables()) {
		return application.ErrStorageNotReady
	}
	for i, table := range storageOwnerTables() {
		if inventory.Tables[i] != table {
			return application.ErrStorageNotReady
		}
	}
	return nil
}
