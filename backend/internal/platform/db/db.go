// Package db owns the PostgreSQL connection used by backend processes.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ErrDSNRequired means the process has no PostgreSQL connection string.
var ErrDSNRequired = errors.New("LV_DB_DSN is required")

// Connection owns both the GORM handle and its underlying SQL pool.
type Connection struct {
	DB   *gorm.DB
	pool *sql.DB
}

// Open establishes a PostgreSQL connection and confirms it is usable.
func Open(ctx context.Context, dsn string, tracerProvider trace.TracerProvider) (*Connection, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, ErrDSNRequired
	}
	orm, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("initialize PostgreSQL: %w", err)
	}
	pool, err := orm.DB()
	if err != nil {
		return nil, fmt.Errorf("get PostgreSQL pool: %w", err)
	}
	if err := orm.Use(otelgorm.NewPlugin(
		otelgorm.WithTracerProvider(tracerProvider),
		otelgorm.WithoutMetrics(),
		otelgorm.WithoutQueryVariables(),
		otelgorm.WithQueryFormatter(func(query string) string {
			fields := strings.Fields(query)
			if len(fields) == 0 {
				return ""
			}
			return strings.ToUpper(fields[0])
		}),
	)); err != nil {
		if closeErr := pool.Close(); closeErr != nil {
			return nil, errors.Join(fmt.Errorf("instrument PostgreSQL: %w", err), fmt.Errorf("close PostgreSQL pool: %w", closeErr))
		}
		return nil, fmt.Errorf("instrument PostgreSQL: %w", err)
	}
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(30 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		if closeErr := pool.Close(); closeErr != nil {
			return nil, errors.Join(fmt.Errorf("ping PostgreSQL: %w", err), fmt.Errorf("close PostgreSQL pool: %w", closeErr))
		}
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return &Connection{DB: orm, pool: pool}, nil
}

// Close releases the PostgreSQL connection pool.
func (c *Connection) Close() error {
	if err := c.pool.Close(); err != nil {
		return fmt.Errorf("close PostgreSQL pool: %w", err)
	}
	return nil
}

// Ping checks the existing PostgreSQL connection pool for readiness.
func (c *Connection) Ping(ctx context.Context) error {
	if err := c.pool.PingContext(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return nil
}
