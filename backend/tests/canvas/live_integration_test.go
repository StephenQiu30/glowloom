package canvas_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

const liveOrigin = "http://127.0.0.1:3198"

type livePacket struct {
	Type     string            `json:"type"`
	Revision int64             `json:"revision"`
	Writable bool              `json:"writable"`
	OpID     string            `json:"op_id"`
	Code     string            `json:"code"`
	Status   int               `json:"status"`
	Document *domain.Document  `json:"document"`
	Result   *canvasapp.Result `json:"result"`
}
type liveFixture struct {
	db      *gorm.DB
	actor   identity.Principal
	auth    identity.AuthResult
	doc     domain.Document
	server  *httptest.Server
	service *canvasapp.Service
}

func newLiveFixture(t *testing.T, lifetime ...context.Context) liveFixture {
	t.Helper()
	db := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	cfg := identityfixture.Configure(t, config.Config{Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: liveOrigin})
	auth := identityfixture.Create(t, db, cfg, "WS fixture", "WS fixture-only password!")
	actor := identity.Principal{ID: auth.User.ID, OrgID: auth.User.OrgID, SessionID: auth.Session.ID, CredentialEpoch: auth.User.SessionEpoch, PlatformEpoch: auth.User.PlatformEpoch, PlatformRole: auth.User.PlatformRole, ScopeID: identityfixture.Scope(t, db, auth.User.ID)}
	project := uuid.New()
	if err := db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'WS fixture','16:9','realistic')`, project, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	service := canvasapp.NewService(canvasStore(db))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "WS fixture"})
	if err != nil {
		t.Fatal(err)
	}
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, db, nil, identityruntime.SessionDependencies{}, lifetime...)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return liveFixture{db, actor, auth, doc, server, service}
}
func (f liveFixture) dial(t *testing.T) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.server.URL, "http")+"/api/canvases/"+f.doc.ID.String()+"/live", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {liveOrigin}, "Cookie": {"glowloom_session=" + f.auth.Token}}})
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("real canvas WS handshake failed: status=%d error=%v", status, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}
func liveRead(t *testing.T, c *websocket.Conn, kind string, revision int64) livePacket {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	for {
		var p livePacket
		if err := wsjson.Read(ctx, c, &p); err != nil {
			t.Fatalf("waiting %s r%d: %v", kind, revision, err)
		}
		if p.Type == "error" {
			t.Fatalf("unexpected WS error: code=%s status=%d", p.Code, p.Status)
		}
		if p.Type == kind && (revision == 0 || p.Revision >= revision) {
			return p
		}
	}
}
func liveSend(t *testing.T, c *websocket.Conn, p any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, c, p); err != nil {
		t.Fatal(err)
	}
}
func (f liveFixture) join(t *testing.T, c *websocket.Conn) {
	t.Helper()
	liveSend(t, c, map[string]any{"type": "join", "protocol_version": 1, "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
	p := liveRead(t, c, "snapshot", 0)
	if p.Document == nil || p.Document.ID != f.doc.ID {
		t.Fatal("missing authorized database snapshot")
	}
	liveRead(t, c, "ready", p.Revision)
}
func TestCanvasWebSocketDatabaseSaveBroadcastAndReceiptReplay(t *testing.T) {
	f := newLiveFixture(t)
	a, b := f.dial(t), f.dial(t)
	f.join(t, a)
	f.join(t, b)
	first, second := resourceNode("text"), resourceNode("text")
	edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: first.ID, TargetNodeID: second.ID}
	body := canvasapp.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{first, second}}, {Type: "Connect", Edges: []domain.Edge{edge}}}}
	key := uuid.NewString()
	submit := map[string]any{"type": "submit", "op_id": key, "body": body}
	liveSend(t, a, submit)
	ack := liveRead(t, a, "ack", 2)
	if ack.Result == nil || len(ack.Result.Nodes) != 2 || len(ack.Result.Edges) != 1 || ack.OpID != key {
		t.Fatal("save was not confirmed by committed graph")
	}
	observed := liveRead(t, b, "snapshot", 2)
	if observed.Document == nil || len(observed.Document.Edges) != 1 {
		t.Fatal("other client missed database edge")
	}
	loaded, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
	if err != nil || loaded.Revision != 2 || len(loaded.Edges) != 1 {
		t.Fatal("DB reload mismatch", err)
	}
	_ = a.CloseNow()
	reconnected := f.dial(t)
	f.join(t, reconnected)
	liveSend(t, reconnected, map[string]any{"type": "resolve", "op_id": key, "body": body})
	receipt := liveRead(t, reconnected, "ack", 2)
	if receipt.OpID != key || receipt.Result == nil || receipt.Result.Revision != 2 {
		t.Fatal("durable receipt not resolved")
	}
	liveSend(t, reconnected, submit)
	liveRead(t, reconnected, "ack", 2)
	loaded, err = f.service.Get(t.Context(), f.actor, f.doc.ID)
	if err != nil || loaded.Revision != 2 {
		t.Fatal("replay duplicated a revision", err)
	}
	var count int64
	if err := f.db.Raw(`SELECT count(*) FROM canvas.save_receipt WHERE document_id=?`, f.doc.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("not exactly one durable receipt", err)
	}
}
func TestCanvasWebSocketHeartbeatFindsMissedWritesAndArchive(t *testing.T) {
	f := newLiveFixture(t)
	c := f.dial(t)
	f.join(t, c)
	_, err := f.service.Execute(t.Context(), f.actor, f.doc.ID, uuid.NewString(), canvasapp.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{resourceNode("text")}}}})
	if err != nil {
		t.Fatal(err)
	}
	liveRead(t, c, "snapshot", 2)
	if err := f.db.Exec(`UPDATE workspace.project SET status='archived',archived_at=now() WHERE id=?`, f.doc.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	p := liveRead(t, c, "snapshot", 2)
	if p.Writable {
		t.Fatal("heartbeat retained write permission after archive")
	}
}
func TestCanvasWebSocketRejectsForeignOriginBeforeUpgrade(t *testing.T) {
	f := newLiveFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.server.URL, "http")+fmt.Sprintf("/api/canvases/%s/live", f.doc.ID), &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"http://foreign.invalid"}, "Cookie": {"glowloom_session=" + f.auth.Token}}})
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if conn != nil {
		_ = conn.CloseNow()
	}
	if err == nil || resp == nil || resp.StatusCode != 403 {
		t.Fatalf("foreign origin upgrade was not rejected: %v", err)
	}
}

// liveError gives each protocol assertion its own budget, including the server's
// bounded database operation. Race tests can run alongside CPU-heavy packages.
func liveError(t *testing.T, c *websocket.Conn) livePacket {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	for {
		var p livePacket
		if err := wsjson.Read(ctx, c, &p); err != nil {
			t.Fatal(err)
		}
		if p.Type == "error" {
			return p
		}
	}
}

func TestCanvasWebSocketConflictDoesNotOverwriteAndArchiveStillResolves(t *testing.T) {
	f := newLiveFixture(t)
	a, b := f.dial(t), f.dial(t)
	f.join(t, a)
	f.join(t, b)
	key := uuid.NewString()
	body := canvasapp.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{resourceNode("text")}}}}
	liveSend(t, a, map[string]any{"type": "submit", "op_id": key, "body": body})
	liveRead(t, a, "ack", 2)
	staleKey := uuid.NewString()
	liveSend(t, b, map[string]any{"type": "submit", "op_id": staleKey, "body": body})
	conflict := liveError(t, b)
	if conflict.OpID != staleKey || conflict.Status != 409 || conflict.Code != "revision_conflict" {
		t.Fatalf("unsafe conflict result: %+v", conflict)
	}
	loaded, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
	if err != nil || loaded.Revision != 2 || len(loaded.Nodes) != 1 {
		t.Fatal("stale write modified database", err)
	}
	if err := f.db.Exec(`UPDATE workspace.project SET status='archived',archived_at=now() WHERE id=?`, f.doc.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	liveSend(t, a, map[string]any{"type": "resolve", "op_id": key, "body": body})
	liveRead(t, a, "ack", 2)
	// A reused UUID with another body can never resolve the old success.
	body.ExpectedRevision = 2
	liveSend(t, a, map[string]any{"type": "resolve", "op_id": key, "body": body})
	if reused := liveError(t, a); reused.Code != "idempotency_key_reused" {
		t.Fatalf("hash mismatch accepted: %+v", reused)
	}
}

func TestCanvasWebSocketHeartbeatDoesNotExtendSessionAndRevocationCloses(t *testing.T) {
	f := newLiveFixture(t)
	c := f.dial(t)
	f.join(t, c)
	var before, after time.Time
	if err := f.db.Raw(`SELECT last_active_at FROM identity.user_session WHERE id=?`, f.auth.Session.ID).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	liveRead(t, c, "heartbeat", 1)
	if err := f.db.Raw(`SELECT last_active_at FROM identity.user_session WHERE id=?`, f.auth.Session.ID).Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Fatal("background heartbeat extended session")
	}
	probe, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/api/canvases/"+f.doc.ID.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probe.Header.Set("Cookie", "glowloom_session="+f.auth.Token)
	probe.Header.Set("X-Creation-Scope", f.actor.ScopeID.String())
	response, err := http.DefaultClient.Do(probe)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("background canvas probe failed", response.StatusCode)
	}
	if err := f.db.Raw(`SELECT last_active_at FROM identity.user_session WHERE id=?`, f.auth.Session.ID).Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Fatal("background recovery probe extended session")
	}

	if err := f.db.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, f.auth.Session.ID).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	var p livePacket
	if err := wsjson.Read(ctx, c, &p); err != nil {
		t.Fatal(err)
	}
	if p.Type != "error" || p.Status != 401 || p.Document != nil || p.Result != nil {
		t.Fatalf("private delivery after revoke: %+v", p)
	}
	if err := wsjson.Read(ctx, c, &p); err == nil {
		t.Fatal("revoked socket remained open")
	}
}

func TestCanvasWebSocketArchivedWriteKeepsReadableConnection(t *testing.T) {
	f := newLiveFixture(t)
	c := f.dial(t)
	f.join(t, c)
	if err := f.db.Exec(`UPDATE workspace.project SET status='archived',archived_at=now() WHERE id=?`, f.doc.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	liveSend(t, c, map[string]any{"type": "submit", "op_id": uuid.NewString(), "body": canvasapp.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{resourceNode("text")}}}}})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var p livePacket
	if err := wsjson.Read(ctx, c, &p); err != nil {
		t.Fatal(err)
	}
	if p.Type != "error" || p.Status != 409 || p.Code != "canvas_read_only" {
		t.Fatalf("read-only write must preserve readable page, got %+v", p)
	}
	liveSend(t, c, map[string]any{"type": "sync"})
	p = liveRead(t, c, "snapshot", 1)
	if p.Writable {
		t.Fatal("archived canvas still writable")
	}
}

func TestCanvasWebSocketStopsWithAPILifetime(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f := newLiveFixture(t, ctx)
	c := f.dial(t)
	f.join(t, c)
	cancel()
	read, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	var p livePacket
	if err := wsjson.Read(read, c, &p); err == nil {
		t.Fatal("socket outlived API shutdown")
	}
}

func TestCanvasWebSocketSyncBurstDoesNotBlockHeartbeatPong(t *testing.T) {
	f := newLiveFixture(t)
	c := f.dial(t)
	f.join(t, c)
	tx := f.db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	var locked struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM canvas.document WHERE id=? FOR UPDATE`, f.doc.ID).Scan(&locked).Error; err != nil {
		t.Fatal(err)
	}
	// Hold the authoritative snapshot read while sync requests accumulate. There
	// must still be a reader available for control frames when heartbeat runs.
	liveSend(t, c, map[string]any{"type": "sync"})
	for range 100 {
		liveSend(t, c, map[string]any{"type": "sync"})
	}
	timer := time.NewTimer(5100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	liveRead(t, c, "heartbeat", 1)
}
