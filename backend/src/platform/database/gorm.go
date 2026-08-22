package database

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenGORM wraps the already-created pgx pool. There is one PostgreSQL pool in
// the process; GORM is the only business Repository API layered on top of it.
// Schema ownership stays with schema/current.sql, so this function never calls
// AutoMigrate.
func OpenGORM(pool *pgxpool.Pool) (*gorm.DB, error) {
	if pool == nil {
		return nil, fmt.Errorf("database pool is nil")
	}
	connection := stdlib.OpenDBFromPool(pool)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: connection}), &gorm.Config{
		SkipDefaultTransaction: false,
		PrepareStmt:            true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open gorm database: %w", err)
	}
	return db, nil
}
