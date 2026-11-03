package agent_test

import (
	"context"
	"net/url"
	"os"
	"testing"

	pg "github.com/StephenQiu30/glowloom-server/backend/internal/agent/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	platformdb "github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"
)

func checkpointDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GLOWLOOM_TEST_AGENT_CONTROL_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_AGENT_CONTROL_DB_DSN to an isolated migrated PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("options", "-c role=glowloom_app")
	parsed.RawQuery = q.Encode()

	conn, err := platformdb.Open(t.Context(), parsed.String(), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.DB
}

func TestPostgresCheckpointerSaveAndResume(t *testing.T) {
	db := checkpointDB(t)
	checkpointer := pg.NewCheckpointStore(db)

	sg := graph.NewStateGraph()

	_ = sg.AddNode("supervisor", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		s.Messages = append(s.Messages, graph.Message{Content: "supervisor planned"})
		return s, nil
	})

	_ = sg.AddNode("proposer", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		s.Proposals = append(s.Proposals, graph.Proposal{
			ID:                  uuid.New(),
			Action:              "generation.submit",
			Summary:             "生成分镜1视频",
			EstimatedCostMicros: 50000,
		})
		s.Interrupt("awaiting_user_payment_approval")
		return s, nil
	})

	_ = sg.AddNode("executor", func(ctx context.Context, s *graph.AgentState) (*graph.AgentState, error) {
		s.Messages = append(s.Messages, graph.Message{Content: "executor completed"})
		return s, nil
	})

	_ = sg.SetEntryPoint("supervisor")
	_ = sg.AddEdge("supervisor", "proposer")
	_ = sg.AddEdge("proposer", "executor")
	_ = sg.AddEdge("executor", graph.FinishPoint)

	compiled, err := sg.Compile(checkpointer)
	if err != nil {
		t.Fatal(err)
	}

	runID := uuid.New()
	initialState := &graph.AgentState{
		RunID:     runID,
		ProjectID: uuid.New(),
		SessionID: uuid.New(),
		ActorID:   uuid.New(),
	}

	// 1. Initial Invoke: should suspend at proposer
	suspendedState, err := compiled.Invoke(t.Context(), initialState)
	if err != nil {
		t.Fatal(err)
	}

	if suspendedState.Status != "suspended_for_approval" {
		t.Fatalf("expected suspended_for_approval, got %s", suspendedState.Status)
	}
	if suspendedState.CurrentNode != "proposer" {
		t.Fatalf("expected current node 'proposer', got %s", suspendedState.CurrentNode)
	}

	// 2. Verify database checkpoint exists
	latest, err := checkpointer.GetLatestCheckpoint(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.State == nil {
		t.Fatal("expected checkpoint to be found in PostgreSQL")
	}
	if latest.NodeName != "proposer" {
		t.Fatalf("expected latest checkpoint node 'proposer', got %s", latest.NodeName)
	}
	if len(latest.State.Proposals) != 1 {
		t.Fatalf("expected 1 proposal in checkpoint state, got %d", len(latest.State.Proposals))
	}

	// 3. Simulate User Approval and Resume
	resumedState, err := compiled.Resume(t.Context(), runID, "approved")
	if err != nil {
		t.Fatal(err)
	}

	if resumedState.Status != "completed" {
		t.Fatalf("expected completed status after resume, got %s", resumedState.Status)
	}
	if len(resumedState.Messages) != 2 {
		t.Fatalf("expected 2 messages after full execution, got %d", len(resumedState.Messages))
	}
	if resumedState.Messages[1].Content != "executor completed" {
		t.Fatalf("unexpected message: %s", resumedState.Messages[1].Content)
	}
}
