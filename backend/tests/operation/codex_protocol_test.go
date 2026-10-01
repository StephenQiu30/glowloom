package operation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/codex"
	"github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func TestM1CodexImageProtocol(t *testing.T) {
	cases := []struct {
		name, behavior, outcome string
		starts, receipts        int
	}{
		{"complete", "success", "completed", 1, 1},
		{"missing capability", "missing", "not_submitted", 0, 0},
		{"disabled capability", "disabled", "not_submitted", 0, 0},
		{"malformed capability", "malformed", "not_submitted", 0, 0},
		{"wrong thread ignored", "foreign", "completed", 1, 1},
		{"wrong turn ignored", "foreign-turn", "completed", 1, 1},
		{"duplicate image", "duplicate", "completed", 1, 1},
		{"conflicting image", "conflict", "unknown", 1, 0},
		{"result encoding unknown", "empty-path", "unknown", 1, 0},
		{"outside path", "outside", "unknown", 1, 0},
		{"symlink escape", "symlink", "unknown", 1, 0},
		{"fake image", "fake", "unknown", 1, 0},
		{"oversize", "oversize", "unknown", 1, 0},
		{"EOF", "eof", "unknown", 1, 0},
		{"failed image", "failed-image", "unknown", 1, 0},
		{"failed turn", "failed-turn", "unknown", 1, 0},
		{"no image", "no-image", "unknown", 1, 0},
		{"item precedes turn response", "early", "completed", 1, 1},
		{"model mismatch", "model-mismatch", "not_submitted", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			launch := &codexTestLauncher{t: t, behavior: tc.behavior}
			receipts := &codexTestReceipts{}
			adapter, err := codex.New(&codexTestDispatch{}, launch, receipts, codex.Config{WorkRoot: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			input := codexTestInput()
			got, err := adapter.Submit(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if got.Outcome != tc.outcome || launch.starts != tc.starts || receipts.writes != tc.receipts {
				t.Fatalf("outcome=%s starts=%d receipts=%d; want %s/%d/%d", got.Outcome, launch.starts, receipts.writes, tc.outcome, tc.starts, tc.receipts)
			}
			if launch.conn != nil && !launch.conn.closed {
				t.Fatal("connection not closed")
			}
			if launch.starts > 0 && (launch.model != input.Model || launch.fallback || launch.approval != "never" || launch.sandbox != "workspace-write") {
				t.Fatal("model or thread boundaries changed")
			}
			if got.Outcome == "completed" && (got.Receipt == nil || got.Receipt.Validate() != nil || got.Receipt.Identity != input.Identity) {
				t.Fatal("invalid receipt identity")
			}
		})
	}
}
func TestM1CodexImageProtocolReplay(t *testing.T) {
	input := codexTestInput()
	receipt := codexTestReceipt(input.Identity)
	for _, prior := range []*application.ProviderReceipt{nil, &receipt} {
		launch := &codexTestLauncher{t: t, behavior: "success"}
		a, err := codex.New(&codexTestDispatch{prior: true, receipt: prior}, launch, &codexTestReceipts{}, codex.Config{WorkRoot: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		got, err := a.Submit(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		want := "unknown"
		if prior != nil {
			want = "completed"
		}
		if got.Outcome != want || launch.opens != 0 || launch.starts != 0 {
			t.Fatalf("replay=%s opens=%d starts=%d", got.Outcome, launch.opens, launch.starts)
		}
	}
}
func TestM1CodexImageProtocolCancellation(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before", false: "after"}[before], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if before {
				cancel()
			}
			launch := &codexTestLauncher{t: t, behavior: "cancel", cancel: cancel}
			a, err := codex.New(&codexTestDispatch{}, launch, &codexTestReceipts{}, codex.Config{WorkRoot: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			got, err := a.Submit(ctx, codexTestInput())
			if err != nil {
				t.Fatal(err)
			}
			want := "unknown"
			starts := 1
			if before {
				want = "not_submitted"
				starts = 0
			}
			if got.Outcome != want || launch.starts != starts {
				t.Fatalf("cancel=%s starts=%d", got.Outcome, launch.starts)
			}
			if launch.conn != nil && !launch.conn.closed {
				t.Fatal("cancel did not close connection")
			}
		})
	}
}
func TestM1CodexImageProtocolTimeout(t *testing.T) {
	launch := &codexTestLauncher{t: t, behavior: "stall"}
	a, err := codex.New(&codexTestDispatch{}, launch, &codexTestReceipts{}, codex.Config{WorkRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	got, err := a.Submit(ctx, codexTestInput())
	if err != nil || got.Outcome != "unknown" || launch.starts != 1 || !launch.conn.closed {
		t.Fatalf("timeout=%s error=%v starts=%d", got.Outcome, err, launch.starts)
	}
}
func TestM1CodexImageProtocolReceiptFailure(t *testing.T) {
	launch := &codexTestLauncher{t: t, behavior: "success"}
	receipts := &codexTestReceipts{fail: true}
	a, err := codex.New(&codexTestDispatch{}, launch, receipts, codex.Config{WorkRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Submit(context.Background(), codexTestInput())
	if err != nil || got.Outcome != "unknown" || receipts.writes != 1 {
		t.Fatalf("receipt failure=%s %v", got.Outcome, err)
	}
	if _, err := os.Stat(launch.artifact); err != nil {
		t.Fatal("uncommitted generated image deleted")
	}
}
func codexTestInput() application.ProviderImageInput {
	return application.ProviderImageInput{Identity: application.ProviderDispatchIdentity{ProjectID: uuid.New(), OperationID: uuid.New(), Action: "submit", Attempt: 1, RequestKey: "fixture/request", ModelProfileVersionID: uuid.New(), PriceRuleVersionID: uuid.New()}, Model: "fixture-model", Prompt: "生成测试图片"}
}

type codexTestDispatch struct {
	prior     bool
	cancelled bool
	receipt   *application.ProviderReceipt
}

func (d *codexTestDispatch) ProviderImageModel(context.Context, application.ProviderDispatchIdentity) (string, error) {
	return "fixture-model", nil
}

func (d *codexTestDispatch) ClaimProviderDispatch(ctx context.Context, _ application.ProviderDispatchIdentity) (application.ProviderDispatchResult, error) {
	if d.cancelled {
		return application.ProviderDispatchResult{}, application.ErrProviderDispatchCancelled
	}
	if err := ctx.Err(); err != nil {
		return application.ProviderDispatchResult{}, err
	}
	now := time.Now()
	return application.ProviderDispatchResult{Claimed: !d.prior, DispatchStartedAt: &now, Receipt: d.receipt}, nil
}

func TestM1CodexImageProtocolPersistedCancellationStaysUnsent(t *testing.T) {
	launch := &codexTestLauncher{t: t, behavior: "success"}
	adapter, err := codex.New(&codexTestDispatch{cancelled: true}, launch, &codexTestReceipts{}, codex.Config{WorkRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Submit(t.Context(), codexTestInput())
	if err != nil || result.Outcome != "not_submitted" || result.FailureCode != "provider_dispatch_cancelled" || launch.opens != 0 || launch.starts != 0 {
		t.Fatalf("persisted cancellation lost definite no-send evidence: result=%+v opens=%d err=%v", result, launch.opens, err)
	}
}

func TestM1CodexImageProtocolCallerCannotOverrideFrozenModel(t *testing.T) {
	launch := &codexTestLauncher{t: t, behavior: "success"}
	adapter, err := codex.New(&codexTestDispatch{}, launch, &codexTestReceipts{}, codex.Config{WorkRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	input := codexTestInput()
	input.Model = "another-model"
	result, err := adapter.Submit(t.Context(), input)
	if err != nil || result.Outcome != "not_submitted" || result.FailureCode != "codex:frozen_model_mismatch" || launch.opens != 0 {
		t.Fatalf("caller model overrode frozen model: result=%+v err=%v", result, err)
	}
}

type codexTestReceipts struct {
	writes int
	fail   bool
}

func (r *codexTestReceipts) RecoverImage(context.Context, application.ProviderDispatchIdentity) (*application.ProviderReceipt, error) {
	return nil, nil
}

func (r *codexTestReceipts) StageImage(_ context.Context, id application.ProviderDispatchIdentity, e codex.ImageEvidence, o application.ProviderReceiptOutput, data []byte) (application.ProviderReceipt, error) {
	r.writes++
	if r.fail {
		return application.ProviderReceipt{}, errors.New("fixture stage unavailable")
	}
	if e.ThreadID != "fixture-thread" || e.TurnID != "fixture-turn" || e.ItemID != "fixture-item" || int64(len(data)) != o.SizeBytes || o.MIMEType != "image/png" {
		return application.ProviderReceipt{}, errors.New("invalid image evidence")
	}
	receipt := codexTestReceipt(id)
	receipt.Outputs = []application.ProviderReceiptOutput{o}
	return receipt, nil
}
func codexTestReceipt(id application.ProviderDispatchIdentity) application.ProviderReceipt {
	return application.ProviderReceipt{Version: 1, Identity: id, ManifestSHA256: strings.Repeat("a", 64), Outputs: []application.ProviderReceiptOutput{{Sequence: 1, SizeBytes: 80, MIMEType: "image/png", SHA256: strings.Repeat("b", 64)}}}
}

type codexTestLauncher struct {
	t                                  *testing.T
	behavior                           string
	cancel                             context.CancelFunc
	opens, starts                      int
	artifact, model, approval, sandbox string
	fallback                           bool
	conn                               *codexTestConn
}

func (l *codexTestLauncher) Open(_ context.Context, dir string) (io.ReadWriteCloser, error) {
	l.opens++
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		return nil, err
	}
	l.artifact = filepath.Join(dir, "result.png")
	if err := os.WriteFile(l.artifact, buf.Bytes(), 0600); err != nil {
		return nil, err
	}
	switch l.behavior {
	case "outside":
		l.artifact = filepath.Join(l.t.TempDir(), "outside.png")
		if err := os.WriteFile(l.artifact, buf.Bytes(), 0600); err != nil {
			return nil, err
		}
	case "symlink":
		outside := filepath.Join(l.t.TempDir(), "outside.png")
		if err := os.WriteFile(outside, buf.Bytes(), 0600); err != nil {
			return nil, err
		}
		if err := os.Remove(l.artifact); err != nil {
			return nil, err
		}
		if err := os.Symlink(outside, l.artifact); err != nil {
			return nil, err
		}
	case "fake":
		if err := os.WriteFile(l.artifact, []byte("fake"), 0600); err != nil {
			return nil, err
		}
	case "oversize":
		f, err := os.OpenFile(l.artifact, os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		err = f.Truncate(32*1024*1024 + 1)
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	c := &codexTestConn{messages: make(chan []byte, 16), done: make(chan struct{})}
	l.conn = c
	c.respond = func(raw []byte) {
		var req struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(raw, &req) != nil {
			return
		}
		var result map[string]any
		switch req.Method {
		case "initialize":
			result = map[string]any{"userAgent": "fixture"}
		case "initialized":
			return
		case "modelProvider/capabilities/read":
			result = map[string]any{"imageGeneration": true}
			if l.behavior == "missing" {
				result = map[string]any{}
			}
			if l.behavior == "disabled" {
				result["imageGeneration"] = false
			}
			if l.behavior == "malformed" {
				result["imageGeneration"] = "yes"
			}
		case "thread/start":
			l.model, _ = req.Params["model"].(string)
			l.fallback, _ = req.Params["allowProviderModelFallback"].(bool)
			l.approval, _ = req.Params["approvalPolicy"].(string)
			l.sandbox, _ = req.Params["sandbox"].(string)
			model := l.model
			if l.behavior == "model-mismatch" {
				model = "other"
			}
			result = map[string]any{"model": model, "thread": map[string]any{"id": "fixture-thread"}}
		case "turn/start":
			l.starts++
			if l.behavior == "eof" {
				close(c.messages)
				return
			}
			if l.behavior == "cancel" {
				l.cancel()
				return
			}
			if l.behavior == "early" {
				l.item(c, "fixture-thread", "fixture-turn", false)
			}
			c.emit(map[string]any{"id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "fixture-turn", "status": "inProgress"}}})
			if l.behavior == "stall" {
				return
			}
			if l.behavior == "foreign" {
				l.item(c, "foreign-thread", "fixture-turn", false)
			}
			if l.behavior == "foreign-turn" {
				l.item(c, "fixture-thread", "foreign-turn", false)
			}
			if l.behavior != "early" && l.behavior != "no-image" {
				l.item(c, "fixture-thread", "fixture-turn", false)
			}
			if l.behavior == "duplicate" {
				l.item(c, "fixture-thread", "fixture-turn", false)
			}
			if l.behavior == "conflict" {
				l.item(c, "fixture-thread", "fixture-turn", true)
			}
			status := "completed"
			if l.behavior == "failed-turn" {
				status = "failed"
			}
			c.emit(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "fixture-thread", "turn": map[string]any{"id": "fixture-turn", "status": status}}})
			return
		default:
			return
		}
		c.emit(map[string]any{"id": req.ID, "result": result})
	}
	return c, nil
}
func (l *codexTestLauncher) item(c *codexTestConn, thread, turn string, conflict bool) {
	path := l.artifact
	if l.behavior == "empty-path" {
		path = ""
	}
	if conflict {
		path += ".other"
	}
	status := "completed"
	if l.behavior == "failed-image" {
		status = "failed"
	}
	c.emit(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"id": "fixture-item", "type": "imageGeneration", "status": status, "savedPath": path, "result": "encoding-is-unknown"}}})
}

type codexTestConn struct {
	messages chan []byte
	done     chan struct{}
	once     sync.Once
	respond  func([]byte)
	pending  []byte
	closed   bool
}

func (c *codexTestConn) emit(v any)                  { raw, _ := json.Marshal(v); c.messages <- append(raw, '\n') }
func (c *codexTestConn) Write(p []byte) (int, error) { c.respond(p); return len(p), nil }
func (c *codexTestConn) Read(p []byte) (int, error) {
	if len(c.pending) == 0 {
		select {
		case <-c.done:
			return 0, io.EOF
		case msg, ok := <-c.messages:
			if !ok {
				return 0, io.EOF
			}
			c.pending = msg
		}
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}
func (c *codexTestConn) Close() error {
	c.once.Do(func() { c.closed = true; close(c.done) })
	return nil
}

func TestM1CodexImageProtocolProcessExit(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("fixture executable uses a POSIX interpreter")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "stdio-fixture")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexec /bin/cat\n"), 0700); err != nil {
		t.Fatal(err)
	}
	launcher, err := codex.NewProcessLauncher(binary)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := launcher.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close fixture connection: %v", err)
		}
	})
	message := []byte("fixture-only-no-model-call\n")
	if _, err := conn.Write(message); err != nil {
		t.Fatal(err)
	}
	echoed := make([]byte, len(message))
	if _, err := io.ReadFull(conn, echoed); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(message, echoed) {
		t.Fatal("stdio did not round trip")
	}
	readDone := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, conn); readDone <- err }()
	cancel()
	closeDone := make(chan struct{})
	go func() { _ = conn.Close(); close(closeDone) }()
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("process exit was not awaited")
	}
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("reader did not exit")
	}
	if err := conn.Close(); err != nil {
		t.Fatal("idempotent close failed")
	}
}
