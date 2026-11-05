package agentbusiness_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/yjsruntime"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

type commandStore struct {
	base                   canvas.YjsState
	initialized, committed int
}

func (s *commandStore) LoadYjs(context.Context, identity.Principal, uuid.UUID) (canvas.YjsState, error) {
	return s.base, nil
}
func (s *commandStore) InitializeYjs(_ context.Context, _ identity.Principal, b canvas.YjsState, c canvas.YjsCandidate) (canvas.YjsState, error) {
	s.initialized++
	b.State = c.State
	b.GraphHash = c.GraphHash
	s.base = b
	return b, nil
}
func (s *commandStore) ResolveYjs(context.Context, identity.Principal, uuid.UUID, canvas.YjsUpdate) (*canvas.YjsReceipt, error) {
	return nil, nil
}
func (s *commandStore) CommitYjs(_ context.Context, _ identity.Principal, b canvas.YjsState, u canvas.YjsUpdate, c *canvas.YjsCandidate, _ string) (canvas.YjsReceipt, error) {
	s.committed++
	if b.CommitSeq != s.base.CommitSeq {
		return canvas.YjsReceipt{}, canvas.ErrYjsContention
	}
	return canvas.YjsReceipt{Status: "accepted", UpdateID: u.UpdateID, CommitSeq: b.CommitSeq + 1, ContentRevision: b.Document.Revision + 1, GraphHash: c.GraphHash}, nil
}
func (s *commandStore) CommitYjsCommands(ctx context.Context, a identity.Principal, in canvas.PreparedYjsCommands) (canvas.YjsReceipt, error) {
	if in.Initialization != nil {
		if _, err := s.InitializeYjs(ctx, a, in.Base, *in.Initialization); err != nil {
			return canvas.YjsReceipt{}, err
		}
	}
	return s.CommitYjs(ctx, a, in.Base, in.Update, &in.Candidate, "")
}
func commandRuntime(t *testing.T) *yjsruntime.Client {
	t.Helper()
	root, err := filepath.Abs("../../../canvas-collab")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "node", "--input-type=module", "-e", `import {runtimeServer}from'./src/server.mjs';const s=runtimeServer({token:'isolated-agent-command-runtime-token'});s.listen(0,'127.0.0.1',()=>console.log(s.address().port));`)
	cmd.Dir = root
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stop := sync.OnceFunc(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	t.Cleanup(stop)
	scanner := bufio.NewScanner(pipe)
	if !scanner.Scan() {
		stop()
		t.Fatal(stderr.String())
	}
	return yjsruntime.New("http://127.0.0.1:"+scanner.Text(), "isolated-agent-command-runtime-token")
}
func TestFormalYjsCommandPreparesOutsideCommitAndPreservesStrictRevision(t *testing.T) {
	doc := canvasdomain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "Agent真实结构化写入", Revision: 1, SchemaVersion: 1, OperationVersion: 1, Writable: true, Nodes: []canvasdomain.Node{}, Edges: []canvasdomain.Edge{}}
	store := &commandStore{base: canvas.YjsState{YjsBinding: canvas.YjsBinding{DocumentID: doc.ID}, Document: canvas.Snapshot(doc), CommandDocument: doc}}
	service := canvas.NewYjsService(store, commandRuntime(t))
	text := "雨夜分镜。"
	input := canvas.YjsCommandInput{CommandID: uuid.New(), SessionID: uuid.New(), ExpectedRevision: 1, Operations: []canvasdomain.Operation{{Type: "create_node", Node: &canvasdomain.Node{ID: uuid.New(), NodeType: "text", NodeAction: "resource", Title: "分镜", Config: canvasdomain.NodeConfig{Text: &text}}}}}
	prepared, err := service.PrepareCommands(t.Context(), identity.Principal{}, doc.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if store.initialized != 0 || store.committed != 0 {
		t.Fatal("preparation committed side effects")
	}
	if len(prepared.Candidate.State) == 0 || len(prepared.Candidate.AppliedUpdate) == 0 {
		t.Fatal("not real Yjs output")
	}
	encoded, _ := json.Marshal(prepared.Candidate.Projection)
	if !bytes.Contains(encoded, []byte("雨夜分镜")) {
		t.Fatal(string(encoded))
	}
	result, err := service.CommitCommands(t.Context(), identity.Principal{}, prepared)
	if err != nil || result.Status != "accepted" || store.initialized != 1 || store.committed != 1 {
		t.Fatal(result, err)
	}
	stale := input
	stale.ExpectedRevision = 2
	if _, err = service.PrepareCommands(t.Context(), identity.Principal{}, doc.ID, stale); !errors.Is(err, canvas.ErrYjsContention) {
		t.Fatal("stale revision prepared", err)
	}
}
