package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestProjectCopyRecordedPolicy(t *testing.T) {
	parser := auditapp.NewRecordedActionParser()
	projectID, jobID := uuid.NewString(), uuid.NewString()
	for _, action := range []string{"project.copy_requested", "project.copy_retry", "project.copy_cancel", "project.copy_reconcile", "project.copy_completed", "project.copy_failed", "project.copy_cancelled"} {
		t.Run(action, func(t *testing.T) {
			after := map[string]any{
				"copy_job_id": jobID, "source_project_id": projectID, "target_project_id": uuid.NewString(),
				"status": "failed", "stage": "media", "revision": 4, "documents": 2, "assets": 3,
				"renditions": 4, "needs_reconciliation": true, "failure_code": "object_absence_unknown",
			}
			record := auditRecord(t, uuid.NewString(), uuid.NewString(), projectID, uuid.NewString(), time.Now().UTC(), after)
			var body map[string]any
			if err := json.Unmarshal(record.Value, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			data["action"] = action
			data["after"] = after
			data["object"] = map[string]any{"type": "project_copy", "id": jobID}
			data["request_id"] = uuid.NewString()
			delete(data, "before")
			record.Value, _ = json.Marshal(body)
			parsed, err := parser.Parse(record)
			if err != nil || parsed.Action != action || parsed.ObjectID != jobID || parsed.ObjectType != "project_copy" {
				t.Fatalf("durable copy summary rejected: %v", err)
			}
			for _, field := range []string{"object_key", "frozen", "prompt", "worker_id"} {
				after[field] = "private implementation value"
				record.Value, _ = json.Marshal(body)
				if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
					t.Fatalf("private field %s accepted: %v", field, err)
				}
				delete(after, field)
			}
		})
	}
}
