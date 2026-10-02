package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestScriptRecordedPolicyOnlyReviewedSummaries(t *testing.T) {
	for _, action := range []string{"script.source_create", "script.source_update", "script.source_delete", "script.source_import", "script.source_reorder", "script.rules_split_saved", "script.split_confirmed", "script.structure_saved", "script.structure_confirmed", "script.source_write_cancel", "script.source_write_reconcile", "script.version_adopted"} {
		t.Run(action, func(t *testing.T) {
			after := map[string]any{"script_revision": 2, "version_id": uuid.NewString()}
			switch action {
			case "script.source_create", "script.source_update", "script.source_delete", "script.source_import", "script.source_reorder":
				after["project_revision"] = 3
				after["split_set_id"] = uuid.NewString()
				after["source_count"] = 2
				for _, field := range []string{"content_hash", "document_sha256", "source_manifest_sha256"} {
					after[field] = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
				}
			case "script.structure_saved", "script.structure_confirmed":
				after["episode_id"] = uuid.NewString()
			}
			record := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), after)
			var body map[string]any
			if err := json.Unmarshal(record.Value, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			data["action"], data["object"], data["before"], data["after"], data["request_id"] = action, map[string]any{"type": "script_version", "id": uuid.NewString()}, nil, after, uuid.NewString()
			record.Value, _ = json.Marshal(body)
			parser := auditapp.NewRecordedActionParser()
			if parsed, err := parser.Parse(record); err != nil || parsed.Action != action {
				t.Fatal("actual safe script summary rejected", action, err)
			}
			for _, field := range []string{"source_title", "body", "object_key", "document", "file_name", "signed_url", "request_key", "actor_role"} {
				after[field] = "private payload"
				record.Value, _ = json.Marshal(body)
				if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
					t.Fatal("unreviewed script field accepted", field, err)
				}
				delete(after, field)
			}
		})
	}
}
