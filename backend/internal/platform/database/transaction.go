package database

import (
	"context"

	"gorm.io/gorm"
)

func WithinTransaction(ctx context.Context, database *gorm.DB, operation func(*gorm.DB) error) error {
	return database.WithContext(ctx).Transaction(operation)
}
