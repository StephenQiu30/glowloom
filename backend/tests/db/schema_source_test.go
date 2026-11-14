package db_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaDoesNotScheduleRetiredRuntimeDDL(t *testing.T) {
	schema, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(schema), "'maintenance.partitions'") {
		t.Fatal("canonical schema must not schedule runtime partition DDL")
	}
}

func TestBackendSQLHasCanonicalSchemaAndPresetData(t *testing.T) {
	backend := filepath.Join("..", "..")
	allowed := map[string]bool{
		filepath.Join(backend, "db", "schema.sql"): false,
		filepath.Join(backend, "db", "data.sql"):   false,
	}
	err := filepath.WalkDir(backend, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".sql") {
			return nil
		}
		if _, ok := allowed[path]; !ok {
			t.Errorf("unexpected SQL file %s; expected schema.sql or data.sql", path)
		} else {
			allowed[path] = entry.Type().IsRegular()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, found := range allowed {
		if !found {
			t.Errorf("missing canonical SQL file: %s", path)
		}
	}
}
