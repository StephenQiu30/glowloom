package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestMediaDepthRecordedPolicy(t *testing.T) {
	parser := auditapp.NewRecordedActionParser()
	project, job := uuid.NewString(), uuid.NewString()
	for _, action := range []string{"requested", "cancel", "retry", "reconcile", "reviewed", "rendered", "failed", "cancelled"} {
		t.Run(action, func(t *testing.T) {
			after := map[string]any{
				"id": job, "project_id": project, "canvas_id": uuid.NewString(), "node_id": uuid.NewString(),
				"source_revision": 3, "source_asset_id": uuid.NewString(), "source_asset_revision": 2,
				"profile_id": "vda-small-relative-v1", "status": "failed", "stage": "inferring", "attempt": 1,
				"revision": 4, "asset_id": nil, "sha256": nil, "failure_code": "object_write_unknown",
				"needs_reconciliation": true, "execution_unconfirmed": false,
			}
			record := auditRecord(t, uuid.NewString(), uuid.NewString(), project, uuid.NewString(), time.Now().UTC(), after)
			var body map[string]any
			if err := json.Unmarshal(record.Value, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			data["action"] = "media.depth_" + action
			data["after"] = after
			data["before"] = nil
			data["object"] = map[string]any{"type": "media_depth", "id": job}
			data["request_id"] = uuid.NewString()
			record.Value, _ = json.Marshal(body)
			parsed, err := parser.Parse(record)
			if err != nil || parsed.Action != "media.depth_"+action || parsed.ObjectID != job {
				t.Fatal("durable depth summary rejected", err)
			}
			for _, field := range []string{"object_key", "frozen", "input_path", "source_tree", "native_receipt", "worker_id"} {
				after[field] = "private implementation value"
				record.Value, _ = json.Marshal(body)
				if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
					t.Fatal("private depth field accepted", field, err)
				}
				delete(after, field)
			}
		})
	}
}
