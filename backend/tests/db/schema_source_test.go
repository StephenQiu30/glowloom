package db_test

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaSQLIsTheOnlyBackendSQL(t *testing.T) {
	backend := filepath.Join("..", "..")
	schema := filepath.Join(backend, "db", "schema.sql")
	found := false
	err := filepath.WalkDir(backend, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".sql") {
			return nil
		}
		if path != schema {
			t.Errorf("unexpected SQL file %s; the backend maintains only db/schema.sql", path)
		} else {
			found = entry.Type().IsRegular()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("missing canonical schema file: %s", schema)
	}
}
