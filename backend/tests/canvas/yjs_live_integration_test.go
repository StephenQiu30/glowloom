package canvas_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	live "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/websocket"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

type yjsPacket struct {
	Type     string          `json:"type"`
	Status   int             `json:"status"`
	State    canvas.YjsState `json:"yjs_state"`
	Presence []live.Presence `json:"presence"`
}

func readYjsUntil(t *testing.T, socket *websocket.Conn, accept func(yjsPacket) bool) yjsPacket {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	for range 100 {
		var p yjsPacket
		if err := wsjson.Read(ctx, socket, &p); err != nil {
			t.Fatal(err)
		}
		if accept(p) {
			return p
		}
	}
	t.Fatal("expected Yjs packet not observed")
	return yjsPacket{}
}
func TestYjsTwoAccountsThreeSocketsPresenceRevocationAndNoBusinessWrites(t *testing.T) {
	address := yjsRuntimeAddress(t)
	first, second, team, _ := newTeamLiveFixtures(t, func(cfg *config.Config) {
		cfg.CanvasRuntimeURL = address
		cfg.CanvasRuntimeToken = "isolated-test-runtime-token-32-bytes"
	})
	store := canvasStore(first.db)
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), first.actor, first.doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	sockets := []*websocket.Conn{first.dial(t), second.dial(t), first.dial(t)}
	clients := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for i, socket := range sockets {
		f := first
		if i == 1 {
			f = second
		}
		liveSend(t, socket, map[string]any{"type": "join", "protocol_version": 4, "client_id": clients[i], "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
		readYjsUntil(t, socket, func(p yjsPacket) bool { return p.Type == "yjs_snapshot" })
	}
	readYjsUntil(t, sockets[0], func(p yjsPacket) bool { return p.Type == "yjs_presence" && len(p.Presence) == 3 })
	liveSend(t, sockets[1], map[string]any{"type": "yjs_presence", "body": map[string]any{"cursor": map[string]any{"x": 125, "y": 240}}})
	observed := readYjsUntil(t, sockets[2], func(p yjsPacket) bool {
		for _, peer := range p.Presence {
			if peer.ClientID == clients[1] && peer.Cursor != nil {
				return true
			}
		}
		return false
	})
	for _, peer := range observed.Presence {
		if peer.ClientID == clients[1] && (peer.ActorID != second.actor.ID || peer.Cursor.X != 125 || peer.Cursor.Y != 240) {
			t.Fatal("presence spoofed identity or wrong coordinates")
		}
	}
	current, err := service.Load(t.Context(), first.actor, first.doc.ID)
	if err != nil || current.CommitSeq != initial.CommitSeq || current.Document.Revision != initial.Document.Revision {
		t.Fatal("presence wrote business state", err)
	}
	if err = first.db.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, second.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	liveSend(t, sockets[1], map[string]any{"type": "sync"})
	denied := readYjsUntil(t, sockets[1], func(p yjsPacket) bool { return p.Type == "error" })
	if denied.Status != 404 || len(denied.Presence) > 0 {
		t.Fatal("revoked private delivery", denied)
	}
	readYjsUntil(t, sockets[0], func(p yjsPacket) bool { return p.Type == "yjs_presence" && len(p.Presence) == 2 })
	_ = sockets[2].Close(websocket.StatusNormalClosure, "test page closed")
	readYjsUntil(t, sockets[0], func(p yjsPacket) bool { return p.Type == "yjs_presence" && len(p.Presence) == 1 })
}

func TestYjsTeamHistoryRestoreThroughAuthenticatedSocket(t *testing.T) {
	address := yjsRuntimeAddress(t)
	first, _, _, node := newTeamLiveFixtures(t, func(cfg *config.Config) {
		cfg.CanvasRuntimeURL = address
		cfg.CanvasRuntimeToken = "isolated-test-runtime-token-32-bytes"
	})
	service := canvas.NewYjsService(canvasStore(first.db), yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), first.actor, first.doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := service.Versions(t.Context(), first.actor, first.doc.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	edited, err := service.Submit(t.Context(), first.actor, first.doc.ID, yjsEdit(t, initial, node, "历史恢复", false))
	if err != nil || edited.Status != "accepted" {
		t.Fatal(edited, err)
	}
	socket := first.dial(t)
	client := uuid.New()
	liveSend(t, socket, map[string]any{"type": "join", "protocol_version": 4, "client_id": client, "scope_id": first.actor.ScopeID, "csrf_token": first.auth.CSRFToken})
	readYjsUntil(t, socket, func(p yjsPacket) bool { return p.Type == "yjs_snapshot" })
	command := canvas.YjsRestore{ID: uuid.New(), SessionID: client, VersionID: versions[0].ID, Epoch: initial.Epoch, CommitSeq: edited.CommitSeq}
	liveSend(t, socket, map[string]any{"type": "yjs_restore", "op_id": command.ID.String(), "body": command})
	response := readYjsUntil(t, socket, func(p yjsPacket) bool { return p.Type == "yjs_restore_ack" || p.Type == "error" })
	if response.Type != "yjs_restore_ack" {
		t.Fatal(response)
	}
	current, err := service.Load(t.Context(), first.actor, first.doc.ID)
	if err != nil || current.Epoch == initial.Epoch {
		t.Fatal("epoch not restored", err)
	}
}

func TestYjsPrivateCanvasResponsesAreNotCacheable(t *testing.T) {
	first, _, _, _ := newTeamLiveFixtures(t)
	for _, authenticated := range []bool{true, false} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, first.server.URL+"/api/canvases/"+first.doc.ID.String(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if authenticated {
			req.Header.Set("Cookie", "glowloom_session="+first.auth.Token)
			req.Header.Set("X-Creation-Scope", first.actor.ScopeID.String())
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		directives := strings.Split(res.Header.Get("Cache-Control"), ",")
		for i := range directives {
			directives[i] = strings.TrimSpace(directives[i])
		}
		if !slices.Contains(directives, "no-store") {
			t.Fatal("private API may be cached", res.StatusCode, res.Header)
		}
		if authenticated && res.StatusCode != 200 {
			t.Fatal("authorized read failed", res.StatusCode)
		}
		if !authenticated && res.StatusCode != 401 {
			t.Fatal("anonymous read allowed", res.StatusCode)
		}
	}
}
