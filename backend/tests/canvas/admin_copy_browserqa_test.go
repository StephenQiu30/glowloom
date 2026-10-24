//go:build browserqa

package canvas_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	copywire "github.com/StephenQiu30/glowloom-server/backend/internal/app/projectcopy"
	inbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	record "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trial"
	trialflow "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/workflow"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	copyevent "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/event"
	copyflow "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/workflow"
)

// startAdminCopyBrowserWorkers uses actual workflows, activities, stores and
// committed outbox proofs. Only the upstream chat protocol is synthetic. The
// relay is scoped to this fixture's source project and never consumes other jobs.
func startAdminCopyBrowserWorkers(t *testing.T, ctx context.Context, db *gorm.DB, cfg config.Config, project uuid.UUID) {
	t.Helper()
	if cfg.TemporalAddr == "" || cfg.TemporalNamespace == "" {
		t.Fatal("explicit isolated Temporal required")
	}
	c, err := client.Dial(client.Options{HostPort: cfg.TemporalAddr, Namespace: cfg.TemporalNamespace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	flow := worker.New(c, "flow", worker.Options{})
	flow.RegisterWorkflow(trialflow.TrialWorkflow)
	copyflow.RegisterProjectCopyWorkflow(flow)
	media := worker.New(c, "media", worker.Options{})
	copyWorker, copies := copywire.ProvideProjectCopyWorker(db, nil)
	copyflow.RegisterProjectCopyActivities(media, copyflow.NewProjectCopyActivities(copyWorker, copies))
	native := worker.New(c, "agent.codex", worker.Options{})
	runner := &trial.Runner{Store: trial.NewStore(db, app.NewCatalogStore(db)), Launcher: browserChatLauncher{}, WorkRoot: t.TempDir()}
	native.RegisterActivityWithOptions(runner.Run, activity.RegisterOptions{Name: "trial.Run"})
	for _, w := range []worker.Worker{flow, media, native} {
		if err := w.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(w.Stop)
	}
	handler := copyevent.NewProjectCopyHandler(inbox.NewStore(db), copies, copyflow.NewProjectCopyStarter(c))
	relayCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	t.Cleanup(func() { cancel(); <-done })
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-relayCtx.Done():
				return
			case <-ticker.C:
				var events []struct {
					PartitionKey string
					Payload      []byte
				}
				if err := db.WithContext(relayCtx).Raw(`SELECT partition_key,payload FROM infra.outbox WHERE topic=? AND payload->'data'->>'source_project_id'=? ORDER BY create_time`, copyevent.ProjectCopyTopic, project.String()).Scan(&events).Error; err != nil {
					if relayCtx.Err() == nil {
						t.Error(err)
					}
					return
				}
				for _, event := range events {
					if err := handler.Handle(relayCtx, record.Record{Topic: copyevent.ProjectCopyTopic, Key: []byte(event.PartitionKey), Value: event.Payload}); err != nil && relayCtx.Err() == nil {
						t.Error(err)
						return
					}
				}
			}
		}
	}()
}

// browserChatLauncher exercises the production RPC parser without credentials,
// network inference, filesystem tools, or supplier charges.
type browserChatLauncher struct{}

func (browserChatLauncher) Open(ctx context.Context, _ string) (io.ReadWriteCloser, error) {
	appSide, fixture := net.Pipe()
	go func() {
		defer fixture.Close()
		stop := context.AfterFunc(ctx, func() { _ = fixture.Close() })
		defer stop()
		scanner := bufio.NewScanner(fixture)
		encoder := json.NewEncoder(fixture)
		thread := "qa-" + uuid.NewString()
		for scanner.Scan() {
			var req struct {
				ID     int            `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if json.Unmarshal(scanner.Bytes(), &req) != nil {
				return
			}
			var result any
			switch req.Method {
			case "initialize":
				result = map[string]any{"userAgent": "isolated-qa"}
			case "initialized":
				continue
			case "account/read":
				result = map[string]any{"account": map[string]any{"type": "fixture"}}
			case "modelProvider/capabilities/read":
				result = map[string]any{"imageGeneration": false}
			case "model/list":
				result = map[string]any{"data": []any{map[string]any{"model": "qa-chat", "hidden": false}}, "nextCursor": nil}
			case "thread/start", "thread/resume":
				if id, ok := req.Params["threadId"].(string); ok {
					thread = id
				}
				result = map[string]any{"model": "qa-chat", "thread": map[string]any{"id": thread}}
			case "turn/start":
				turn := uuid.NewString()
				if encoder.Encode(map[string]any{"id": req.ID, "result": map[string]any{"turn": map[string]any{"id": turn}}}) != nil {
					return
				}
				if encoder.Encode(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"id": uuid.NewString(), "type": "agentMessage", "text": "隔离验收回复：中文与 emoji 🌏，请求已完成。", "phase": "final_answer"}}}) != nil {
					return
				}
				if encoder.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": thread, "turn": map[string]any{"id": turn, "status": "completed"}}}) != nil {
					return
				}
				continue
			default:
				return
			}
			if encoder.Encode(map[string]any{"id": req.ID, "result": result}) != nil {
				return
			}
		}
	}()
	return appSide, nil
}
