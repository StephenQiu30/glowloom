package operation_test

import (
	"os"
	"path/filepath"
	"testing"

	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/workflow"
)

// TestM1SyncWorkflowReplayLegacyTemporalHistories consumes exported fixture
// histories produced by baseline 84de390f on an isolated real Temporal namespace.
// They use mock Activities; passing replay does not prove a real supplier works.
func TestM1SyncWorkflowReplayLegacyTemporalHistories(t *testing.T) {
	dir := os.Getenv("LV_TEST_SYNC_REPLAY_HISTORY_DIR")
	if dir == "" {
		t.Skip("set LV_TEST_SYNC_REPLAY_HISTORY_DIR to the task's exported baseline fixture histories")
	}
	for _, scenario := range []string{"success", "unknown", "manual", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			name := filepath.Join(dir, scenario+".json")
			info, err := os.Stat(name)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 4*1024*1024 {
				t.Fatal("baseline fixture history missing or outside its capacity bound")
			}
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			var history historypb.History
			if err := protojson.Unmarshal(raw, &history); err != nil || len(history.Events) < 10 {
				t.Fatal("baseline history is not a complete Temporal fixture")
			}
			for _, event := range history.Events {
				if marker := event.GetMarkerRecordedEventAttributes(); marker != nil && marker.MarkerName == "Version" {
					t.Fatal("old baseline fixture unexpectedly contains a new version decision")
				}
			}
			replayer := worker.NewWorkflowReplayer()
			replayer.RegisterWorkflowWithOptions(workflow.OperationWorkflow, temporalworkflow.RegisterOptions{Name: "OperationWorkflow"})
			if err := replayer.ReplayWorkflowHistory(nil, &history); err != nil {
				t.Fatalf("new synchronous branch broke baseline %s command history: %v", scenario, err)
			}
			t.Logf("baseline 84de390f %s: %d real Temporal events replayed", scenario, len(history.Events))
		})
	}
}
