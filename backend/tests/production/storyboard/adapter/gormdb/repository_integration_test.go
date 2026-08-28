package storyboard_test

import (
	"context"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"gorm.io/gorm"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
)

func TestPostgreSQLCatalogMatchesSingleGORMSource(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL catalog test")
	}

	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	assertSingleCatalog(t, database)
}

func assertSingleCatalog(t *testing.T, database *gorm.DB) {
	t.Helper()
	tables, err := database.Migrator().GetTables()
	if err != nil {
		t.Fatalf("list synchronized tables: %v", err)
	}
	sort.Strings(tables)
	for _, table := range tables {
		if strings.Contains(strings.ToLower(table), "migration") {
			t.Fatalf("migration bookkeeping table must not exist: %s", table)
		}
	}
	expected := make([]string, 0, len(schema.Catalog()))
	for _, value := range schema.Catalog() {
		statement := &gorm.Statement{DB: database}
		if err = statement.Parse(value); err != nil {
			t.Fatalf("parse catalog model: %v", err)
		}
		expected = append(expected, statement.Schema.Table)
	}
	sort.Strings(expected)
	if strings.Join(tables, ",") != strings.Join(expected, ",") {
		t.Fatalf("database tables differ from the GORM catalog\nexpected: %v\nactual:   %v", expected, tables)
	}
}
