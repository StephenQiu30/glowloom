package architecture_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAuditEventSchemaPreservesRestorableSuccessfulChangeBaseline(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve audit schema contract test path")
	}
	schemaPath := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "schema", "current.sql"))
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read current schema: %v", err)
	}
	schema := string(schemaBytes)

	for _, required := range []string{
		"before_state jsonb NOT NULL",
		"after_state jsonb NOT NULL",
		"request_id text NOT NULL",
		"reason text NOT NULL CHECK (length(trim(reason)) BETWEEN 1 AND 500)",
		"result text NOT NULL CHECK (result IN ('succeeded', 'denied', 'failed'))",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("current schema is missing audit contract %q", required)
		}
	}
}
