package database

import (
	"context"
	"database/sql"

	"gorm.io/gorm"
)

func WithinTransaction(ctx context.Context, database *gorm.DB, operation func(*gorm.DB) error) error {
	return database.WithContext(ctx).Transaction(operation)
}

func WithinSerializableTransaction(ctx context.Context, database *gorm.DB, operation func(*gorm.DB) error) error {
	return database.WithContext(ctx).Transaction(operation, &sql.TxOptions{Isolation: sql.LevelSerializable})
}
