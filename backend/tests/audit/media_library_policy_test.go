package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestMediaLibraryRecordedPolicyOnlyReviewedSummaries(t *testing.T) {
	for _, action := range []string{"create_folder", "update_folder", "delete_folder", "create_text", "update_item", "move_items", "recycle_items", "restore_items", "remove_items"} {
		t.Run(action, func(t *testing.T) {
			after := map[string]any{"library_id": uuid.NewString(), "revision": 3, "count": 2}
			record := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), after)
			var body map[string]any
			if err := json.Unmarshal(record.Value, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			data["action"] = "media.library." + action
			data["object"] = map[string]any{"type": "media.library", "id": after["library_id"]}
			data["before"], data["after"], data["request_id"] = nil, after, uuid.NewString()
			record.Value, _ = json.Marshal(body)
			parser := auditapp.NewRecordedActionParser()
			if parsed, err := parser.Parse(record); err != nil || parsed.Action != data["action"] {
				t.Fatal("safe library summary rejected", err)
			}
			for _, field := range []string{"text", "tags", "notes", "file_name", "object_key", "signed_url", "personal_actor_id", "request_key"} {
				after[field] = "private payload"
				record.Value, _ = json.Marshal(body)
				if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
					t.Fatal("unreviewed library field accepted", field, err)
				}
				delete(after, field)
			}
		})
	}
}
