package canvas_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
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
	shared := application.NewYjsService(canvasStore(f.db), yjsRuntime(t))
	_, err := shared.Initialize(t.Context(), f.actor, f.doc.ID, 1)
	var rejection *domain.OperationError
	if !errors.As(err, &rejection) || rejection.Code != "canvas_capacity_exceeded" {
		t.Fatalf("oversized migration: %v", err)
	}
	var changed int64
	if err := f.db.Raw(`SELECT (SELECT count(*) FROM canvas.collab_document WHERE document_id=?)+(SELECT count(*) FROM canvas.collab_receipt WHERE document_id=?)`, f.doc.ID, f.doc.ID).Scan(&changed).Error; err != nil || changed != 0 {
		t.Fatal("rejected migration wrote state", changed, err)
	}
	var revision int64
	f.db.Raw(`SELECT revision FROM canvas.document WHERE id=?`, f.doc.ID).Scan(&revision)
	if revision != 1 {
		t.Fatal("rejected migration changed revision", revision)
	}
	conn := f.dial(t)
	liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 4, "client_id": canvasfixture.ClientID, "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
	p := liveError(t, conn)
	if p.Code != "canvas_capacity_exceeded" {
		t.Fatalf("oversized snapshot not explained: %+v", p)
	}
}

func TestCanvasAcceptedCapacityBoundaryReopensOverWebSocket(t *testing.T) {
	f := newLiveFixture(t)
	if err := f.db.Exec(`INSERT INTO canvas.node(id,document_id,title,node_type,node_action,config,x,y) SELECT gen_random_uuid(),?,'boundary','text','resource',jsonb_build_object('text',repeat('x',10000)),0,0 FROM generate_series(1,625)`, f.doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	svc := canvasfixture.NewEditService(canvasStore(f.db))
	revision := int64(1)
	rejected := false
	for range 20 {
		ops := make([]domain.Operation, 10)
		for i := range ops {
			n := resourceNode("text")
			n.Config.Text = ptr(strings.Repeat("中", 3300))
			ops[i] = domain.Operation{Type: "create_node", Node: &n}
		}
		r, err := svc.Submit(t.Context(), f.actor, f.doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: revision, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		if r.Status == "rejected" {
			if r.Code != "STATE_TOO_LARGE" && r.Code != "canvas_capacity_exceeded" {
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
	liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 4, "client_id": canvasfixture.ClientID, "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
	packet := readYjsUntil(t, conn, func(p yjsPacket) bool { return p.Type == "yjs_snapshot" })
	if len(packet.State.Document.Nodes) != len(loaded.Nodes) || int64(packet.State.Document.Revision) != revision {
		t.Fatal("accepted Yjs boundary did not reopen intact")
	}
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
	// This callback belongs to the fixture's dedicated Gorm handle, which is
	// closed by canvasDB cleanup. Removing callbacks while a socket is finishing
	// an authorization read races Gorm's callback list; leave it with the handle.
	liveSend(t, conn, map[string]any{"type": "join", "protocol_version": 4, "client_id": canvasfixture.ClientID, "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
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
	readYjsUntil(t, conn, func(p yjsPacket) bool { return p.Type == "yjs_snapshot" })
}
