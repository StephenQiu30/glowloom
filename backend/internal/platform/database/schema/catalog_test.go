package schema_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
)

type tabler interface {
	TableName() string
}

func TestCatalogDeclaresUniqueBusinessTablesWithoutSchemaMetadata(t *testing.T) {
	t.Parallel()

	models := schema.Catalog()
	if len(models) == 0 {
		t.Fatal("catalog must include at least one model")
	}
	tableNames := make(map[string]struct{}, len(models))
	for _, record := range models {
		model, ok := record.(tabler)
		if !ok {
			t.Fatalf("catalog record %T must declare TableName", record)
		}
		name := model.TableName()
		if name == "" {
			t.Fatalf("catalog record %T has an empty table name", record)
		}
		if strings.Contains(strings.ToLower(name), "migration") || strings.Contains(strings.ToLower(name), "schema_version") {
			t.Errorf("catalog must not contain schema bookkeeping table %q", name)
		}
		if _, duplicate := tableNames[name]; duplicate {
			t.Errorf("catalog contains duplicate table name %q", name)
		}
		tableNames[name] = struct{}{}
	}
}
