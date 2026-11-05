//go:build browserqa

package canvas_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trialtask"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	copytask "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/tests/taskfixture"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	copywire "github.com/StephenQiu30/glowloom-server/backend/internal/app/projectcopy"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trial"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

// startAdminCopyBrowserWorkers uses actual workflows, activities, stores and
// committed outbox proofs. Only the upstream chat protocol is synthetic. The
// relay is scoped to this fixture's source project and never consumes other jobs.
func startAdminCopyBrowserWorkers(t *testing.T, ctx context.Context, db *gorm.DB, cfg config.Config, project uuid.UUID) {
	t.Helper()
	copyWorker, copies := copywire.ProvideProjectCopyWorker(db, nil)
	runner := &trial.Runner{Store: trial.NewStore(db, app.NewCatalogStore(db)), Launcher: browserChatLauncher{}, WorkRoot: t.TempDir()}
	taskfixture.Start(t, db, map[string]taskruntime.Handler{trialtask.Codex: trialtask.NewSessions(db).Handler(runner), copytask.Copy: copytask.CopyHandler(copyWorker, copies)})

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
