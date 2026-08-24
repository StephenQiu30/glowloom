package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/StephenQiu30/lanverse/backend/migrations"
	"github.com/jackc/pgx/v5"
)

const migrationLockKey = "lanverse.schema-migration"

type appliedMigration struct {
	Name     string
	Checksum *string
	Source   string
}

func ApplyMigrations(ctx context.Context, databaseURL string) error {
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect migration database: %w", err)
	}
	defer func() { _ = connection.Close(context.Background()) }()

	inventory, err := migrations.All()
	if err != nil {
		return err
	}
	transaction, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	if _, err = transaction.Exec(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtext($1))",
		migrationLockKey,
	); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if err = ensureMigrationTable(ctx, transaction); err != nil {
		return err
	}
	applied, err := loadAppliedMigrations(ctx, transaction)
	if err != nil {
		return err
	}
	if err = rejectUnknownAppliedVersions(applied, inventory); err != nil {
		return err
	}

	for _, migration := range inventory {
		existing, exists := applied[migration.Version]
		if exists {
			if err = verifyAppliedMigration(migration, existing); err != nil {
				return err
			}
			continue
		}
		if migration.Version == 1 {
			populated, checkErr := hasUnversionedApplicationTables(ctx, transaction)
			if checkErr != nil {
				return checkErr
			}
			if populated {
				return errors.New(
					"unversioned application schema detected; run the explicit Agent " +
						"adopt-baseline command after strict legacy schema validation",
				)
			}
		}
		if _, err = transaction.Exec(ctx, migration.SQL); err != nil {
			return fmt.Errorf("apply migration %06d_%s: %w", migration.Version, migration.Name, err)
		}
		if _, err = transaction.Exec(
			ctx,
			`INSERT INTO lanverse_migration.schema_migrations (version, name, checksum, source)
			 VALUES ($1, $2, $3, 'migration')`,
			migration.Version,
			migration.Name,
			migration.Checksum,
		); err != nil {
			return fmt.Errorf("record migration %d: %w", migration.Version, err)
		}
	}

	if err = transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func ensureMigrationTable(ctx context.Context, transaction pgx.Tx) error {
	if _, err := transaction.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS lanverse_migration"); err != nil {
		return fmt.Errorf("ensure migration schema: %w", err)
	}
	_, err := transaction.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS lanverse_migration.schema_migrations (
			version BIGINT PRIMARY KEY,
			name VARCHAR(160) NOT NULL,
			checksum CHAR(64),
			source VARCHAR(20) NOT NULL,
			applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_sys_schema_migration_source
				CHECK (source IN ('migration', 'adopted'))
		)`)
	if err != nil {
		return fmt.Errorf("ensure migration table: %w", err)
	}
	return nil
}

func loadAppliedMigrations(
	ctx context.Context,
	transaction pgx.Tx,
) (map[int64]appliedMigration, error) {
	rows, err := transaction.Query(
		ctx,
		`SELECT version, name, checksum, source
		 FROM lanverse_migration.schema_migrations
		 ORDER BY version`,
	)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int64]appliedMigration)
	for rows.Next() {
		var version int64
		var migration appliedMigration
		if err = rows.Scan(&version, &migration.Name, &migration.Checksum, &migration.Source); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = migration
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return applied, nil
}

func rejectUnknownAppliedVersions(
	applied map[int64]appliedMigration,
	inventory []migrations.Migration,
) error {
	known := make(map[int64]struct{}, len(inventory))
	for _, migration := range inventory {
		known[migration.Version] = struct{}{}
	}
	for version := range applied {
		if _, exists := known[version]; !exists {
			return fmt.Errorf("database migration version %d is newer than this binary", version)
		}
	}
	return nil
}

func verifyAppliedMigration(
	expected migrations.Migration,
	actual appliedMigration,
) error {
	if actual.Name != expected.Name {
		return fmt.Errorf(
			"migration %d name drift: database=%q binary=%q",
			expected.Version,
			actual.Name,
			expected.Name,
		)
	}
	switch actual.Source {
	case "adopted":
		if expected.Version != 1 {
			return fmt.Errorf("only the compatibility baseline may use adopted source")
		}
	case "migration":
		if actual.Checksum == nil || *actual.Checksum != expected.Checksum {
			return fmt.Errorf("migration %d checksum drift", expected.Version)
		}
	default:
		return fmt.Errorf("migration %d has unsupported source %q", expected.Version, actual.Source)
	}
	return nil
}

func hasUnversionedApplicationTables(ctx context.Context, transaction pgx.Tx) (bool, error) {
	var count int
	err := transaction.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_catalog.pg_tables
		WHERE schemaname = current_schema()
		  AND tablename <> 'alembic_version'`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("inspect unversioned schema: %w", err)
	}
	return count > 0, nil
}
