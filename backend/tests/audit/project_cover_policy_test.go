package audit_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
)

func TestProjectCoverRecordedPolicy(t *testing.T) {
	parser := auditapp.NewRecordedActionParser()
	after := map[string]any{"revision": 2, "cover_changed": true}
	record := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), after)
	var body map[string]any
	if err := json.Unmarshal(record.Value, &body); err != nil {
		t.Fatal(err)
	}
	data := body["data"].(map[string]any)
	id := uuid.NewString()
	data["action"] = "project.updated"
	data["object"] = map[string]any{"type": "project", "id": id}
	data["before"] = nil
	data["after"] = after
	data["request_id"] = uuid.NewString()
	record.Value, _ = json.Marshal(body)
	parsed, err := parser.Parse(record)
	if err != nil || parsed.Action != "project.updated" || parsed.ObjectID != id {
		t.Fatal("safe project cover change rejected", err)
	}
	for _, field := range []string{"file_name", "object_key", "signed_url", "source_text", "cover_asset_id", "actor_id"} {
		after[field] = "private value"
		record.Value, _ = json.Marshal(body)
		if _, err := parser.Parse(record); !errors.Is(err, auditapp.ErrInvalidEvent) {
			t.Fatal("unreviewed cover field accepted", field, err)
		}
		delete(after, field)
	}
}
