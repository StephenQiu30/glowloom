package canvas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	canvas "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

// TestYjsSyncPerformance is opt-in: use a dedicated, freshly initialized database
// with pg_stat_statements enabled. It never creates schema or resets global stats.
func TestYjsSyncPerformance(t *testing.T) {
	if os.Getenv("GLOWLOOM_CANVAS_PERF") != "1" {
		t.Skip("set GLOWLOOM_CANVAS_PERF=1 with a dedicated isolated PostgreSQL DSN")
	}
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	runtime := yjsRuntime(t)
	for _, size := range []int{50, 200, 2000} {
		for _, mode := range []string{"text", "drag"} {
			for repeat := range 3 {
				t.Run(fmt.Sprintf("%d/%s/%d", size, mode, repeat), func(t *testing.T) {
					actor, project := seedCanvasActor(t, database)
					store := canvasStore(database)
					doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Sync performance"})
					if err != nil {
						t.Fatal(err)
					}
					nodes := make([]domain.Node, size)
					for i := range nodes {
						nodes[i] = resourceNode("text")
						nodes[i].ID = uuid.NewSHA1(doc.ID, []byte(fmt.Sprint(i)))
						// A reproducible, non-uniform body prevents unrealistically perfect compression.
						text := ""
						for j := range 8 {
							text += uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%d/%d", i, j))).String()
						}
						nodes[i].Config.Text = &text
					}
					for start := 0; start < size; start += 100 {
						_, err = canvasfixture.NewLegacySeeder(database, store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes(nodes[start:min(start+100, size)])})
						if err != nil {
							t.Fatal(err)
						}
					}
					measuredRuntime := &perfRuntime{YjsRuntime: runtime}
					measuredStore := &perfStore{YjsStore: store}
					service := canvas.NewYjsService(measuredStore, measuredRuntime)
					initial, err := service.Initialize(t.Context(), actor, doc.ID, canvas.Revision(1+(size+99)/100))
					if err != nil {
						t.Fatal(err)
					}
					const count = 12
					body, err := json.Marshal(map[string]any{"state": initial.State, "nodeId": nodes[0].ID, "mode": mode, "count": count})
					if err != nil {
						t.Fatal(err)
					}
					command := exec.CommandContext(t.Context(), "node", "../../../canvas/tests/sync-performance-fixture.mjs")
					command.Stdin = bytes.NewReader(body)
					output, err := command.CombinedOutput()
					if err != nil {
						t.Fatalf("fixture: %v: %s", err, output)
					}
					var updates []canvas.YjsUpdate
					if err = json.Unmarshal(output, &updates); err != nil {
						t.Fatal(err)
					}
					if len(updates) != count {
						t.Fatalf("fixture produced %d updates", len(updates))
					}
					var startLSN string
					if err = database.Raw("SELECT pg_current_wal_insert_lsn()::text").Scan(&startLSN).Error; err != nil {
						t.Fatal(err)
					}
					var beforeStats, afterStats []perfStatement
					readStats := func(target *[]perfStatement) {
						if err := database.Raw(`SELECT query,calls,wal_bytes::bigint AS wal_bytes,total_exec_time FROM pg_stat_statements WHERE dbid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND (query LIKE 'INSERT INTO canvas.collab_version%' OR query LIKE 'UPDATE canvas.collab_document SET commit_seq%' OR query LIKE 'WITH checkpoint AS%') ORDER BY query`).Scan(target).Error; err != nil {
							t.Fatal(err)
						}
					}
					readStats(&beforeStats)
					latencies := make([]float64, 0, count)
					deltaBytes := 0
					for i := range updates {
						updates[i].Epoch, updates[i].SessionID, updates[i].UpdateID = initial.Epoch, uuid.NewSHA1(doc.ID, []byte("session")), uuid.NewSHA1(doc.ID, []byte(fmt.Sprint(i)))
						deltaBytes += len(updates[i].Update)
						start := time.Now()
						receipt, err := service.Submit(t.Context(), actor, doc.ID, updates[i])
						latencies = append(latencies, float64(time.Since(start).Microseconds())/1000)
						if err != nil || receipt.Status != "accepted" || receipt.CommitSeq != canvas.Revision(i+1) {
							t.Fatalf("submit: %+v %v", receipt, err)
						}
					}
					var walBytes int64
					if err = database.Raw("SELECT pg_wal_lsn_diff(pg_current_wal_insert_lsn(),?::pg_lsn)::bigint", startLSN).Scan(&walBytes).Error; err != nil {
						t.Fatal(err)
					}
					readStats(&afterStats)
					for i := range afterStats {
						for _, before := range beforeStats {
							if before.Query == afterStats[i].Query {
								afterStats[i].Calls -= before.Calls
								afterStats[i].WalBytes -= before.WalBytes
								afterStats[i].TotalExecTime -= before.TotalExecTime
							}
						}
					}
					var history struct {
						Versions   int64
						JSONBytes  int64
						StateBytes int64
					}
					if err = database.Raw(`SELECT count(*) AS versions,sum(octet_length(document_json::text)) AS json_bytes,sum(octet_length(c.state)) AS state_bytes FROM canvas.collab_version v JOIN canvas.collab_checkpoint c ON c.document_id=v.document_id AND c.id=v.state_id WHERE v.document_id=?`, doc.ID).Scan(&history).Error; err != nil {
						t.Fatal(err)
					}
					if history.Versions != count+1 {
						t.Fatalf("lost per-revision history: %+v", history)
					}
					loaded, err := service.Load(t.Context(), actor, doc.ID)
					if err != nil || loaded.CommitSeq != count {
						t.Fatalf("reload: %+v %v", loaded.YjsBinding, err)
					}
					sort.Float64s(latencies)
					result := map[string]any{"nodes": size, "mode": mode, "repeat": repeat, "edits": count, "delta_bytes": deltaBytes, "head_state_bytes": len(loaded.State), "wal_bytes": walBytes, "history": history, "statements": afterStats, "latency_ms": latencies, "runtime_ms": measuredRuntime.Milliseconds, "commit_ms": measuredStore.Milliseconds}
					encoded, err := json.Marshal(result)
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("CANVAS_PERF %s", encoded)
				})
			}
		}
	}
}

type perfStatement struct {
	Query         string
	Calls         int64
	WalBytes      int64
	TotalExecTime float64
}

type perfRuntime struct {
	canvas.YjsRuntime
	Milliseconds []float64
}

func (p *perfRuntime) Prepare(ctx context.Context, state canvas.YjsState, update canvas.YjsUpdate) (canvas.YjsCandidate, error) {
	start := time.Now()
	result, err := p.YjsRuntime.Prepare(ctx, state, update)
	p.Milliseconds = append(p.Milliseconds, float64(time.Since(start).Microseconds())/1000)
	return result, err
}

type perfStore struct {
	canvas.YjsStore
	Milliseconds []float64
}

func (p *perfStore) CommitYjs(ctx context.Context, actor identity.Principal, state canvas.YjsState, update canvas.YjsUpdate, candidate *canvas.YjsCandidate, rejection string) (canvas.YjsReceipt, error) {
	start := time.Now()
	result, err := p.YjsStore.CommitYjs(ctx, actor, state, update, candidate, rejection)
	p.Milliseconds = append(p.Milliseconds, float64(time.Since(start).Microseconds())/1000)
	return result, err
}
