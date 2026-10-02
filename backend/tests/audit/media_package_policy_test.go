package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestMediaPackageRecordedPolicyOnlyReviewedSummaries(t *testing.T) {
	for _, action := range []string{"media.package.imported", "media.package.cancelled"} {
		t.Run(action, func(t *testing.T) {
			after := map[string]any{"revision": 2, "status": "succeeded", "item_count": 3, "folder_count": 2}
			record := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), after)
			var body map[string]any
			if err := json.Unmarshal(record.Value, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			data["action"] = action
			data["object"] = map[string]any{"type": "media.package", "id": uuid.NewString()}
			data["before"], data["after"], data["request_id"] = nil, after, uuid.NewString()
			parser := auditapp.NewRecordedActionParser()
			record.Value, _ = json.Marshal(body)
			if parsed, err := parser.Parse(record); err != nil || parsed.Action != action {
				t.Fatal("safe package summary rejected", err)
			}
			for _, field := range []string{"manifest", "source_object_key", "object_key", "signed_url", "text", "file_name", "request_key"} {
				after[field] = "private payload"
				record.Value, _ = json.Marshal(body)
				if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
					t.Fatal("unreviewed package field accepted", field, err)
				}
				delete(after, field)
			}
			data["action"] = "media.package.unreviewed"
			record.Value, _ = json.Marshal(body)
			if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
				t.Fatal("unreviewed package action accepted", err)
			}
		})
	}
}
