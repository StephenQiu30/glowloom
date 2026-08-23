package architecture_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCurrentSchemaContainsOnlyCanonicalNarrativeFacts(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve schema contract test path")
	}
	schemaPath := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "schema", "current.sql"))
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read current schema: %v", err)
	}
	schema := string(schemaBytes)

	for _, forbidden := range []string{
		"CREATE TABLE IF NOT EXISTS script_revisions",
		"CREATE TABLE IF NOT EXISTS script_analysis_drafts",
		"CREATE TABLE IF NOT EXISTS content_units",
		"CREATE TABLE IF NOT EXISTS narrative_units",
		"CREATE TABLE IF NOT EXISTS entities",
		"CREATE TABLE IF NOT EXISTS entity_mentions",
		"CREATE TABLE IF NOT EXISTS production_requirements",
		"ALTER TABLE operations DROP CONSTRAINT",
		"ALTER TABLE iam_users ADD COLUMN",
		"ALTER TABLE iam_sessions ADD COLUMN",
		"ALTER TABLE iam_roles ADD COLUMN",
		"Normalize legacy role rows",
	} {
		if strings.Contains(schema, forbidden) {
			t.Errorf("current schema still contains compatibility contract %q", forbidden)
		}
	}

	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS nar_source_revisions",
		"CREATE TABLE IF NOT EXISTS nar_analysis_drafts",
		"source_revision_id uuid PRIMARY KEY REFERENCES nar_source_revisions(id)",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("current schema is missing canonical narrative contract %q", required)
		}
	}
}
