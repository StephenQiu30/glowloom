package operation_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestWorkflowRecordsEnforceOperationProjectScope(t *testing.T) {
	database := operationSchemaDB(t)
	projectA := operationSchemaProject(t, database)
	projectB := operationSchemaProject(t, database)
	operationID := operationSchemaQuoted(t, database, projectA, nil)

	insertOutput := func(projectID string) error {
		return database.Exec(`
			INSERT INTO operation.operation_output
			  (id, project_id, operation_id, seq_no, kind, json_payload)
			VALUES (?::uuid, ?::uuid, ?::uuid, 0, 'json', '{}'::jsonb)
		`, uuid.NewString(), projectID, operationID).Error
	}
	if err := insertOutput(projectB); err == nil {
		t.Fatal("output crossed its operation project")
	}
	if err := insertOutput(projectA); err != nil {
		t.Fatalf("insert scoped output: %v", err)
	}
	if err := insertOutput(projectA); err == nil {
		t.Fatal("duplicate output sequence was accepted")
	}

	insertCall := func(projectID string) error {
		return database.Exec(`
			INSERT INTO operation.provider_call
			  (id, project_id, operation_id, attempt, action, provider_key,
			   request_summary, outcome, region)
			VALUES (?::uuid, ?::uuid, ?::uuid, 1, 'submit', 'mock',
			        '{}'::jsonb, 'ok', 'domestic')
		`, uuid.NewString(), projectID, operationID).Error
	}
	if err := insertCall(projectB); err == nil {
		t.Fatal("provider call crossed its operation project")
	}
	if err := insertCall(projectA); err != nil {
		t.Fatalf("insert scoped provider call: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO operation.operation_event
		  (id, operation_id, from_status, to_status)
		VALUES (?::uuid, ?::uuid, 'quoted', 'confirmed')
	`, uuid.NewString(), operationID).Error; err != nil {
		t.Fatalf("insert operation event: %v", err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation_event WHERE operation_id = ?::uuid`, operationID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("operation event count = %d: %v", count, err)
	}
}

func TestWorkflowOutputRequiresValidMediaShape(t *testing.T) {
	database := operationSchemaDB(t)
	projectID := operationSchemaProject(t, database)
	operationID := operationSchemaQuoted(t, database, projectID, nil)
	for _, tc := range []struct {
		name, kind string
		mediaID    any
		payload    any
	}{
		{"media without asset", "media", nil, nil},
		{"json without payload", "json", nil, nil},
		{"json with asset", "json", uuid.NewString(), `{}`},
		{"media with payload", "media", uuid.NewString(), `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := database.Exec(`
				INSERT INTO operation.operation_output
				  (id, project_id, operation_id, seq_no, kind,
				   media_asset_id, json_payload)
				VALUES (?::uuid, ?::uuid, ?::uuid, 0, ?, ?::uuid, ?::jsonb)
			`, uuid.NewString(), projectID, operationID, tc.kind, tc.mediaID, tc.payload).Error
			if err == nil {
				t.Fatal("invalid output shape was accepted")
			}
		})
	}
	assertNoWorkflowOutputs(t, database, operationID)
}

func assertNoWorkflowOutputs(t *testing.T, database *gorm.DB, operationID string) {
	t.Helper()
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation_output WHERE operation_id = ?::uuid`, operationID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("invalid outputs survived: count=%d err=%v", count, err)
	}
}
