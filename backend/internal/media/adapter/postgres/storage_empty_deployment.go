package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// These owners can retain old work even when no physical-object row exists yet.
var emptyStorageWorkTables = []string{
	"operation.operation", "media.storage_work_intent", "media.upload_admission", "media.upload_request",
	"media.package_job", "media.transfer_job", "media.purge_job", "media.project_copy_snapshot",
	"mediatool.depth_job", "mediatool.export_job", "mediatool.transcription_job",
	"script.command", "script.import_job", "script.copy_snapshot", "script.request", "workspace.project_copy_job", "subject.voice_job",
}

func storageOwnerTables() []string {
	seen := map[string]bool{}
	var tables []string
	for _, c := range legacyObjectColumns {
		if !seen[c.table] {
			tables, seen[c.table] = append(tables, c.table), true
		}
	}
	for _, t := range append([]string{"media.object_location"}, emptyStorageWorkTables...) {
		if !seen[t] {
			tables, seen[t] = append(tables, t), true
		}
	}
	sort.Strings(tables)
	return tables
}

func lockStorageOwners(tx *gorm.DB) error {
	return tx.Exec(`LOCK TABLE ` + strings.Join(storageOwnerTables(), ",") + ` IN SHARE ROW EXCLUSIVE MODE`).Error
}

func storageHasNoLegacyRows(tx *gorm.DB) (bool, error) {
	for _, table := range storageOwnerTables() {
		var exists bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM ` + table + `)`).Scan(&exists).Error; err != nil {
			return false, err
		}
		if exists {
			return false, nil
		}
	}
	var used bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_route) OR EXISTS(SELECT 1 FROM media.storage_activation) OR EXISTS(SELECT 1 FROM media.storage_activation_guard) OR EXISTS(SELECT 1 FROM media.storage_legacy_binding)`).Scan(&used).Error; err != nil {
		return false, err
	}
	return !used, nil
}

// ProveEmptyStorageDeployment measures the locked database, never inventing an
// original MinIO identity. Only a maintenance owner can insert this evidence.
func (s *StorageRoutes) ProveEmptyStorageDeployment(ctx context.Context, configSHA string) (string, error) {
	if s == nil || s.db == nil || !storageDigest(configSHA) {
		return "", application.ErrStorageNotReady
	}
	var evidence string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL lock_timeout='5s'`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('media.storage_route',0))`).Error; err != nil {
			return err
		}
		if err := lockStorageOwners(tx); err != nil {
			return err
		}
		empty, err := storageHasNoLegacyRows(tx)
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
		if err := tx.Exec(`INSERT INTO media.storage_empty_deployment_evidence(config_sha256,evidence_sha256,inventory) VALUES(?,?,?::jsonb) ON CONFLICT(config_sha256) DO NOTHING`, configSHA, evidence, string(body)).Error; err != nil {
			return err
		}
		return checkEmptyStorageInventory(tx, configSHA)
	})
	return evidence, err
}

func requireEmptyStorageProof(tx *gorm.DB, configSHA string) error {
	if err := checkEmptyStorageInventory(tx, configSHA); err != nil {
		return err
	}
	if err := lockStorageOwners(tx); err != nil {
		return err
	}
	empty, err := storageHasNoLegacyRows(tx)
	if err != nil {
		return err
	}
	if !empty {
		return application.ErrStorageNotReady
	}
	return nil
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
	body, digest, err := emptyStorageInventory(configSHA)
	if err != nil {
		return err
	}
	var present bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM media.storage_empty_deployment_evidence WHERE config_sha256=? AND evidence_sha256=? AND inventory=?::jsonb)`, configSHA, digest, string(body)).Scan(&present).Error; err != nil {
		return err
	}
	if !present {
		return application.ErrStorageNotReady
	}
	return nil
}

// requireNativeBootstrap preserves a measured first deployment across later
// guarded revisions. Subsequent data is governed by the finalized owner FKs;
// it cannot be required to remain an empty database forever.
func requireNativeBootstrap(tx *gorm.DB, configSHA string) error {
	var original string
	read := tx.Raw(`SELECT e.config_sha256 FROM media.storage_empty_deployment_evidence e JOIN media.storage_activation_guard g ON g.config_sha256=e.config_sha256 JOIN media.storage_activation a ON a.revision=g.revision ORDER BY a.activated_at,a.revision LIMIT 1`).Scan(&original)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected == 1 {
		return checkEmptyStorageInventory(tx, original)
	}
	return requireEmptyStorageProof(tx, configSHA)
}
