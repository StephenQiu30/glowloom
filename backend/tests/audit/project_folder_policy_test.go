package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestProjectFolderRecordedPolicy(t *testing.T) {
	parser := auditapp.NewRecordedActionParser()
	for _, action := range []string{"created", "updated", "moved", "recycled"} {
		t.Run(action, func(t *testing.T) {
			id := uuid.NewString()
			after := map[string]any{"folder_id": nil, "project_id": uuid.NewString(), "revision": 2, "placement_revision": 1, "recycled_count": 3, "name_updated": true, "cover_updated": false}
			record := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), after)
			var body map[string]any
			if err := json.Unmarshal(record.Value, &body); err != nil {
				t.Fatal(err)
			}
			data := body["data"].(map[string]any)
			data["action"] = "project_folder." + action
			data["object"] = map[string]any{"type": "project_folder", "id": id}
			data["before"] = nil
			data["after"] = after
			data["request_id"] = uuid.NewString()
			record.Value, _ = json.Marshal(body)
			parsed, err := parser.Parse(record)
			if err != nil || parsed.Action != "project_folder."+action || parsed.ObjectID != id {
				t.Fatal("directory safe summary rejected", err)
			}
			for _, field := range []string{"name", "object_key", "signed_url", "actor_id", "recycled_project_ids"} {
				after[field] = "private value"
				record.Value, _ = json.Marshal(body)
				if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
					t.Fatal("unreviewed directory field accepted", field, err)
				}
				delete(after, field)
			}
		})
	}
}
