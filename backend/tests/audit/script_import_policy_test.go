package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestScriptImportRecordedPolicyOnlyReviewedSummaries(t *testing.T) {
	for _, action := range []string{"create", "cancel", "retry", "reconcile", "completed", "cancelled"} {
		t.Run(action, func(t *testing.T) {
			after := map[string]any{"id": uuid.NewString(), "revision": 3, "attempt": 2, "status": "partial", "stage": "extract", "file_count": 2, "script_revision": 4}
			record := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), after)
			var body map[string]any
			if err := json.Unmarshal(record.Value, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			data["action"] = "script.file_import_" + action
			data["object"] = map[string]any{"type": "script_import", "id": after["id"]}
			data["before"], data["after"], data["request_id"] = nil, after, uuid.NewString()
			record.Value, _ = json.Marshal(body)
			parser := auditapp.NewRecordedActionParser()
			if parsed, err := parser.Parse(record); err != nil || parsed.Action != data["action"] {
				t.Fatal("safe import summary rejected", err)
			}
			for _, field := range []string{"file_name", "asset_ids", "body", "warnings", "object_key", "source_mapping", "rights_at", "request_key", "worker_owner"} {
				after[field] = "private payload"
				record.Value, _ = json.Marshal(body)
				if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
					t.Fatal("unreviewed import field accepted", field, err)
				}
				delete(after, field)
			}
		})
	}
}
