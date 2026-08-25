package workflow_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	temporaladapter "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/temporal"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestTemporalStarterPersistsStableWorkflowIdentityAndInputHash(t *testing.T) {
	address := os.Getenv("LANVERSE_TEST_TEMPORAL_ADDRESS")
	if address == "" {
		t.Skip("set LANVERSE_TEST_TEMPORAL_ADDRESS to run the real Temporal starter journey")
	}
	starter, err := temporaladapter.NewStarter(temporaladapter.Config{
		Address: address, Namespace: "default", TaskQueue: "lanverse-workflow-test",
	})
	if err != nil {
		t.Fatalf("connect Temporal starter: %v", err)
	}
	t.Cleanup(starter.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request := workflow.StartRequest{
		WorkflowID: "lanverse:test:" + uuid.NewString(), WorkflowType: "lanverse.test.unregistered",
		WorkflowTypeVersion: "1.0.0", WorkflowRunID: uuid.NewString(), DefinitionVersionID: uuid.NewString(),
		RunInputSnapshotID: uuid.NewString(), DefinitionContentHash: strings.Repeat("a", 64),
		InputSnapshotHash: strings.Repeat("b", 64), InputHash: strings.Repeat("c", 64),
	}
	started, err := starter.Start(ctx, request)
	if err != nil || started.Outcome != workflow.StartOutcomeStarted || started.ObservedInputHash != request.InputHash {
		t.Fatalf("start real Temporal workflow: observation=%#v err=%v", started, err)
	}
	replayed, err := starter.Start(ctx, request)
	if err != nil || replayed.Outcome != workflow.StartOutcomeAlreadyStarted || replayed.ObservedInputHash != request.InputHash {
		t.Fatalf("describe already-started workflow: observation=%#v err=%v", replayed, err)
	}
	different := request
	different.InputHash = strings.Repeat("d", 64)
	conflict, err := starter.Start(ctx, different)
	if err != nil || conflict.Outcome != workflow.StartOutcomeAlreadyStarted || conflict.ObservedInputHash != request.InputHash {
		t.Fatalf("already-started input hash was not recovered from Temporal: observation=%#v err=%v", conflict, err)
	}
}
