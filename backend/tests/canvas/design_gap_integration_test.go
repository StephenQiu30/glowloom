package canvas_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	live "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/websocket"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestCanvasHTTPCommandRouteRemoved(t *testing.T) {
	f := newLiveFixture(t)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+"/api/canvases/"+f.doc.ID.String()+"/commands", strings.NewReader(`{"expected_revision":1,"commands":[{"type":"SetViewport","viewport":{"x":0,"y":0,"zoom":1}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", "glowloom_session="+f.auth.Token)
	req.Header.Set("Origin", liveOrigin)
	req.Header.Set("X-CSRF-Token", f.auth.CSRFToken)
	req.Header.Set("X-Creation-Scope", f.actor.ScopeID.String())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("removed HTTP editing route returned %d", response.StatusCode)
	}
	doc, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
	if err != nil || doc.Revision != 1 {
		t.Fatalf("HTTP route changed graph: r%d %v", doc.Revision, err)
	}
}

func TestCanvasLegacySocketProtocolRejected(t *testing.T) {
	f := newLiveFixture(t)
	conn := f.dial(t)
	liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 1, "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
	var p livePacket
	if err := wsjson.Read(t.Context(), conn, &p); err != nil {
		t.Fatal(err)
	}
	if p.Type != "error" || p.Code != "unsupported_protocol_version" {
		t.Fatalf("legacy protocol admitted: %+v", p)
	}
}

// A real commit between Get and Changes makes the feed advance past the earlier
// metadata read. The cursor and every envelope must describe delivered changes.
type commitAfterGet struct {
	application.Store
	after func(context.Context, domain.Document)
}

func (s commitAfterGet) Get(ctx context.Context, a identity.Principal, id uuid.UUID) (domain.Document, error) {
	d, err := s.Store.Get(ctx, a, id)
	if err == nil {
		s.after(ctx, d)
	}
	return d, err
}

func TestCanvasChangesWatermarkMatchesConcurrentCommit(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	store := canvasStore(db)
	svc := application.NewCollaborationService(store)
	doc, err := application.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "watermark"})
	if err != nil {
		t.Fatal(err)
	}
	commit := func(ctx context.Context, base int64) {
		n := resourceNode("text")
		receipt, e := svc.Submit(ctx, actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: base, Operations: []domain.Operation{{Type: "create_node", Node: &n}}})
		if e != nil || receipt.Status != "accepted" {
			t.Errorf("concurrent commit: %+v %v", receipt, e)
		}
	}
	commit(t.Context(), 1)
	getter := commitAfterGet{Store: store, after: func(ctx context.Context, d domain.Document) {
		if d.Revision == 2 {
			commit(ctx, 2)
		}
	}}
	handler := live.NewHandler(application.NewService(getter), func(context.Context, *http.Request, uuid.UUID, uuid.UUID, string) (identity.Principal, bool, error) {
		return actor, true, nil
	}, liveOrigin).WithCollaboration(svc)
	router := gin.New()
	handler.Register(t.Context(), router.Group("/api"))
	server := httptest.NewServer(router)
	defer server.Close()
	conn, resp, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+"/api/canvases/"+doc.ID.String()+"/live", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {liveOrigin}}})
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 3, "client_id": canvasfixture.ClientID, "last_revision": "1", "scope_id": actor.ScopeID, "csrf_token": "test"})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var packet struct {
		Type     string                  `json:"type"`
		Revision int64                   `json:"revision,string"`
		From     int64                   `json:"from_revision,string"`
		To       int64                   `json:"to_revision,string"`
		Changes  []application.ChangeDTO `json:"changes"`
	}
	if err := wsjson.Read(ctx, conn, &packet); err != nil {
		t.Fatal(err)
	}
	if packet.Type != "changes" || len(packet.Changes) != 2 || packet.Revision != 3 || packet.From != 1 || packet.To != 3 {
		t.Fatalf("feed watermark does not match delivered range: %+v", packet)
	}
	ready := liveRead(t, conn, "ready", 3)
	if ready.Revision != 3 {
		t.Fatalf("ready lost delivered cursor: %+v", ready)
	}
}

func TestCanvasOversizedGraphRejectedBeforeSaveAndReportedOnLoad(t *testing.T) {
	f := newLiveFixture(t)
	// Model an existing large document. A new edit must not be accepted into an
	// unloadable graph, even when that edit and its history entry are small.
	if err := f.db.Exec(`INSERT INTO canvas.node(id,document_id,title,node_type,node_action,config,x,y) SELECT gen_random_uuid(),?,'large','text','resource',jsonb_build_object('text',repeat('x',10000)),0,0 FROM generate_series(1,1700)`, f.doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	var row struct{ ID uuid.UUID }
	if err := f.db.Raw(`SELECT id FROM canvas.node WHERE document_id=? LIMIT 1`, f.doc.ID).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	svc := application.NewCollaborationService(canvasStore(f.db))
	key := uuid.NewString()
	input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "set", NodeID: row.ID, Path: []string{"title"}, Value: json.RawMessage(`"changed"`)}}}
	receipt, err := svc.Submit(t.Context(), f.actor, f.doc.ID, key, input)
	if err != nil || receipt.Status != "rejected" || receipt.Code != "canvas_capacity_exceeded" {
		t.Fatalf("unloadable graph accepted: %+v %v", receipt, err)
	}
	var revision int64
	if err := f.db.Raw(`SELECT revision FROM canvas.document WHERE id=?`, f.doc.ID).Scan(&revision).Error; err != nil || revision != 1 {
		t.Fatalf("capacity rejection changed revision: %d %v", revision, err)
	}
	resolved, err := svc.Resolve(t.Context(), f.actor, f.doc.ID, key, input)
	if err != nil || resolved == nil || resolved.Code != receipt.Code {
		t.Fatalf("capacity rejection lost: %+v %v", resolved, err)
	}
	conn := f.dial(t)
	liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 3, "client_id": canvasfixture.ClientID, "last_revision": "0", "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
	p := liveError(t, conn)
	if p.Code != "canvas_capacity_exceeded" {
		t.Fatalf("oversized snapshot not explained: %+v", p)
	}
}

func TestCanvasAcceptedCapacityBoundaryReopensOverWebSocket(t *testing.T) {
	f := newLiveFixture(t)
	if err := f.db.Exec(`INSERT INTO canvas.node(id,document_id,title,node_type,node_action,config,x,y) SELECT gen_random_uuid(),?,'boundary','text','resource',jsonb_build_object('text',repeat('x',10000)),0,0 FROM generate_series(1,750)`, f.doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	svc := application.NewCollaborationService(canvasStore(f.db))
	revision := int64(1)
	rejected := false
	for range 20 {
		ops := make([]domain.Operation, 10)
		for i := range ops {
			n := resourceNode("text")
			n.Config.Text = ptr(strings.Repeat("中", 3300))
			ops[i] = domain.Operation{Type: "create_node", Node: &n}
		}
		r, err := svc.Submit(t.Context(), f.actor, f.doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: revision, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		if r.Status == "rejected" {
			if r.Code != "canvas_capacity_exceeded" {
				t.Fatalf("wrong budget rejection: %+v", r)
			}
			rejected = true
			break
		}
		revision++
		if r.Revision != revision {
			t.Fatal(r)
		}
	}
	if !rejected || revision == 1 {
		t.Fatal("did not exercise acceptance and rejection across the capacity boundary")
	}
	loaded, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
	if err != nil || loaded.Revision != revision {
		t.Fatal("rejected append changed the accepted document", err)
	}
	body, marshalErr := json.Marshal(application.Snapshot(loaded))
	if marshalErr != nil || len(body) > domain.MaxDocumentBytes {
		t.Fatal("accepted DTO exceeds shared budget", len(body), marshalErr)
	}
	conn := f.dial(t)
	conn.SetReadLimit(16 << 20)
	liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 3, "client_id": canvasfixture.ClientID, "last_revision": "0", "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
	p := liveRead(t, conn, "snapshot", revision)
	if p.Document == nil || len(p.Document.Nodes) != len(loaded.Nodes) || p.Revision != revision {
		t.Fatal("accepted boundary snapshot did not reopen intact")
	}
	liveRead(t, conn, "ready", revision)
}

// A concurrent project transaction already has a SHARE lock and then updates
// metadata. Socket authorization must request its strongest lock first.
func TestCanvasSocketAuthorityDoesNotUpgradeSharedProjectLocks(t *testing.T) {
	f := newLiveFixture(t)
	conn := f.dial(t)
	tx := f.db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	var row struct{ ID uuid.UUID }
	if err := tx.Raw(`SELECT id FROM workspace.project WHERE id=? FOR SHARE`, f.doc.ProjectID).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	var count atomic.Int32
	if err := f.db.Callback().Row().Before("gorm:row").Register("test:canvas-authority-barrier", func(query *gorm.DB) {
		sql := query.Statement.SQL.String()
		if strings.Contains(sql, "SELECT id AS project_id,org_id,revision,status FROM workspace.project") && strings.HasSuffix(sql, " FOR UPDATE") && count.Add(1) == 1 {
			arrived <- struct{}{}
			select {
			case <-release:
			case <-query.Statement.Context.Done():
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.db.Callback().Row().Remove("test:canvas-authority-barrier"); err != nil {
			t.Error(err)
		}
	})
	liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 3, "client_id": canvasfixture.ClientID, "last_revision": "0", "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-arrived:
	case <-ctx.Done():
		close(release)
		t.Fatal("authority did not reach project write lock")
	}
	updated := make(chan error, 1)
	go func() {
		if err := tx.WithContext(ctx).Raw(`SELECT id FROM workspace.project WHERE id=? FOR UPDATE`, f.doc.ProjectID).Scan(&row).Error; err != nil {
			updated <- err
			return
		}
		updated <- tx.Commit().Error
	}()
	close(release)
	select {
	case err := <-updated:
		if err != nil {
			t.Fatal("project update deadlocked with socket authority", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	liveRead(t, conn, "snapshot", 1)
	liveRead(t, conn, "ready", 1)
}
