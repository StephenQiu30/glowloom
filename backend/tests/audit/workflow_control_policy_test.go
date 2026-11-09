package audit_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
)

func TestWorkflowControlAuditPolicyAcceptsOnlyDeclaredActionsAndStatus(t *testing.T) {
	parser := auditapp.NewRecordedActionParser()
	for _, action := range []string{"operation.cancel_requested", "operation.recover_requested", "batch.cancel_requested", "batch.resume_requested"} {
		t.Run(action, func(t *testing.T) {
			input := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), map[string]any{"status": "submitted"})
			var body map[string]any
			require.NoError(t, json.Unmarshal(input.Value, &body))
			data := body["data"].(map[string]any)
			data["action"] = action
			data["object"] = map[string]any{"type": "operation", "id": uuid.NewString()}
			data["request_id"] = uuid.NewString()
			delete(data, "before")
			input.Value, _ = json.Marshal(body)
			got, err := parser.Parse(input)
			require.NoError(t, err)
			require.Equal(t, action, got.Action)
			for _, field := range []string{"provider_task_id", "request_key", "signed_url", "secret_key", "prompt"} {
				data["after"] = map[string]any{"status": "submitted", field: "private"}
				input.Value, _ = json.Marshal(body)
				_, err = parser.Parse(input)
				require.ErrorIs(t, err, auditapp.ErrInvalidEvent)
			}
		})
	}
	for _, action := range []string{"operation.unreviewed_requested", "operation.deliver_original_requested", "batch.recover_requested"} {
		input := auditRecord(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), time.Now().UTC(), nil)
		var body map[string]any
		require.NoError(t, json.Unmarshal(input.Value, &body))
		body["data"].(map[string]any)["action"] = action
		input.Value, _ = json.Marshal(body)
		_, err := parser.Parse(input)
		require.ErrorIs(t, err, auditapp.ErrInvalidEvent)
	}
}
