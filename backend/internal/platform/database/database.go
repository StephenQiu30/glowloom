package database

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	defaultMaxOpenConnections = 20
	defaultMaxIdleConnections = defaultMaxOpenConnections
	defaultConnectionLifetime = 30 * time.Minute
)

func Open(ctx context.Context, databaseURL string, logOutput io.Writer) (*gorm.DB, error) {
	ormLogger := logger.New(
		log.New(logOutput, "", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			ParameterizedQueries:      true,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
	database, err := gorm.Open(
		postgres.New(postgres.Config{DSN: databaseURL, PreferSimpleProtocol: true}),
		&gorm.Config{
			Logger:                 ormLogger,
			SkipDefaultTransaction: true,
			TranslateError:         true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL with GORM: %w", err)
	}
	connection, err := database.DB()
	if err != nil {
		return nil, fmt.Errorf("get PostgreSQL connection pool: %w", err)
	}
	connection.SetMaxOpenConns(defaultMaxOpenConnections)
	connection.SetMaxIdleConns(defaultMaxIdleConnections)
	connection.SetConnMaxLifetime(defaultConnectionLifetime)
	if err = connection.PingContext(ctx); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return database, nil
}

func Close(database *gorm.DB) error {
	connection, err := database.DB()
	if err != nil {
		return err
	}
	return connection.Close()
}

func Ping(ctx context.Context, database *gorm.DB) error {
	connection, err := database.DB()
	if err != nil {
		return err
	}
	return connection.PingContext(ctx)
}
