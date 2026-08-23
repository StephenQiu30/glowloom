package architecture_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSourceAndMediaSchemaSeparateLogicalArtifactsFromPhysicalLocations(t *testing.T) {
	t.Parallel()

	schemaBytes, err := os.ReadFile(filepath.Join(repositoryRoot(t), "backend", "schema", "current.sql"))
	if err != nil {
		t.Fatalf("read current schema: %v", err)
	}
	schema := string(schemaBytes)
	source := tableDefinition(t, schema, "nar_source_revisions")
	artifact := tableDefinition(t, schema, "media_artifacts")
	location := tableDefinition(t, schema, "media_artifact_locations")

	if !strings.Contains(source, "artifact_id uuid NOT NULL REFERENCES media_artifacts(id)") {
		t.Error("nar_source_revisions must reference a logical MediaArtifact")
	}
	for _, physicalColumn := range []string{"object_key", "object_version_id", "content_hash", "content_length"} {
		if strings.Contains(source, physicalColumn) {
			t.Errorf("nar_source_revisions still owns physical object field %s", physicalColumn)
		}
	}
	for _, physicalColumn := range []string{"object_key", "object_version_id", "bucket", "etag"} {
		if strings.Contains(artifact, physicalColumn) {
			t.Errorf("media_artifacts still owns physical location field %s", physicalColumn)
		}
	}
	for _, required := range []string{"storage_profile", "bucket", "object_key", "object_version_id", "content_hash", "size_bytes", "etag", "status"} {
		if !strings.Contains(location, required) {
			t.Errorf("media_artifact_locations is missing %s", required)
		}
	}
}

func TestPublicCandidateContractDoesNotExposePhysicalObjectLocation(t *testing.T) {
	t.Parallel()

	swaggerBytes, err := os.ReadFile(filepath.Join(repositoryRoot(t), "backend", "docs", "swagger.json"))
	if err != nil {
		t.Fatalf("read Swagger document: %v", err)
	}
	var document struct {
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(swaggerBytes, &document); err != nil {
		t.Fatalf("decode Swagger document: %v", err)
	}
	candidate, ok := document.Definitions["scripts.Candidate"]
	if !ok {
		t.Fatal("Swagger document has no scripts.Candidate definition")
	}
	for _, forbidden := range []string{"object_key", "object_version_id", "bucket", "storage_profile"} {
		if _, exists := candidate.Properties[forbidden]; exists {
			t.Errorf("public Candidate contract exposes physical storage field %s", forbidden)
		}
	}
}

func tableDefinition(t *testing.T, schema, table string) string {
	t.Helper()
	pattern := regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS ` + regexp.QuoteMeta(table) + ` \((.*?)\n\);`)
	match := pattern.FindStringSubmatch(schema)
	if len(match) != 2 {
		t.Fatalf("current schema has no %s table definition", table)
	}
	return match[1]
}
