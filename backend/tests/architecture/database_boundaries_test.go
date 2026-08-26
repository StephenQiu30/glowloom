package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestDatabaseArchitectureBoundaries(t *testing.T) {
	t.Parallel()

	backendRoot := backendDirectory(t)
	for _, removedPath := range []string{
		"cmd/migrate",
		"internal/database",
		"migrations",
	} {
		if _, err := os.Stat(filepath.Join(backendRoot, removedPath)); !os.IsNotExist(err) {
			t.Errorf("obsolete database path must not exist: %s", removedPath)
		}
	}

	err := filepath.WalkDir(backendRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		relativePath, err := filepath.Rel(backendRoot, path)
		if err != nil {
			return err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if strings.HasPrefix(importPath, "github.com/jackc/pgx") {
				t.Errorf("%s imports PostgreSQL driver directly; use GORM through the database adapter", relativePath)
			}
			for _, secondORM := range []string{"github.com/jmoiron/sqlx", "github.com/uptrace/bun", "entgo.io/ent"} {
				if strings.HasPrefix(importPath, secondORM) {
					t.Errorf("%s imports second ORM/query framework %s; GORM is the only catalog", relativePath, importPath)
				}
			}
			if strings.HasPrefix(importPath, "gorm.io/") && !gormImportAllowed(relativePath) {
				t.Errorf("%s imports GORM outside the database platform or gormdb adapter", relativePath)
			}
		}
		if strings.HasSuffix(relativePath, "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, rawDatabaseCall := range []string{".Raw(", ".Exec("} {
			if strings.Contains(string(contents), rawDatabaseCall) {
				t.Errorf("%s uses raw database call %s; use GORM models and clauses", relativePath, rawDatabaseCall)
			}
		}
		for _, obsoleteIdentifier := range []string{
			"MigrationDatabaseURL",
			"LEGACY_API_URL",
			"MIGRATION_DATABASE_URL",
		} {
			if strings.Contains(string(contents), obsoleteIdentifier) {
				t.Errorf("%s contains obsolete runtime identifier %s", relativePath, obsoleteIdentifier)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect backend architecture: %v", err)
	}
}

func TestDatabaseModelsUseStructTagsRatherThanTableSQL(t *testing.T) {
	t.Parallel()

	backendRoot := backendDirectory(t)
	modelRoot := filepath.Join(backendRoot, "internal", "platform", "database", "model")
	err := filepath.WalkDir(modelRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr == nil && strings.Contains(strings.ToUpper(value), "CREATE TABLE") {
				t.Errorf("%s embeds table DDL; the GORM record catalog is the schema source", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("inspect database models: %v", err)
	}
}

func backendDirectory(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func gormImportAllowed(relativePath string) bool {
	segments := strings.Split(filepath.ToSlash(relativePath), "/")
	if strings.HasPrefix(filepath.ToSlash(relativePath), "internal/platform/database/") {
		return true
	}
	return slices.Contains(segments, "adapter") && slices.Contains(segments, "gormdb")
}
