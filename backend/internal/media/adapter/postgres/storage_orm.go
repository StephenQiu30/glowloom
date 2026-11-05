package postgres

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	storageIdempotencyPurge          = "purge"
	storageIdempotencyPackage        = "package"
	storageIdempotencyTransfer       = "transfer"
	storageIdempotencyUpload         = "upload"
	storageIdempotencyPersonalUpload = "personal_upload"
	storageIdempotencyLibrary        = "library"
)

// updateOne applies a parameterized GORM update and enforces the expected row
// fence for state transitions.
func updateOne(tx *gorm.DB, table string, where, values map[string]any, conflict error) error {
	result := tx.Table(table).Where(where).Updates(values)
	return requireOne(result, conflict)
}

func requireOne(result *gorm.DB, conflict error) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return conflict
	}
	return nil
}

// lockStorageIdempotencyKey serializes retries for one durable command key.
func lockStorageIdempotencyKey(tx *gorm.DB, namespace string, actorID, requestKey uuid.UUID) error {
	if actorID == uuid.Nil || requestKey == uuid.Nil {
		return fmt.Errorf("invalid storage idempotency identity")
	}
	record := storageIdempotencyLockRecord{Namespace: namespace, ActorID: actorID, RequestKey: requestKey, CreatedAt: time.Now().UTC()}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "namespace"}, {Name: "actor_id"}, {Name: "request_key"}}, DoNothing: true}).Create(&record).Error; err != nil {
		return fmt.Errorf("create storage idempotency lock: %w", err)
	}
	var locked storageIdempotencyLockRecord
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("namespace = ? AND actor_id = ? AND request_key = ?", namespace, actorID, requestKey).Take(&locked).Error
}
