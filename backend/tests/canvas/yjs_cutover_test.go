package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestYjsOnlyRejectsRetiredProtocolsWithoutInitializingOrWriting(t *testing.T) {
	f := newLiveFixture(t)
	for _, version := range []int{1, 2, 3, 5} {
		socket := f.dial(t)
		liveSend(t, socket, map[string]any{"type": "join", "protocol_version": version, "client_id": uuid.New(), "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
		var response struct {
			Type      string `json:"type"`
			Code      string `json:"code"`
			Supported []int  `json:"supported_versions"`
		}
		if err := wsjson.Read(t.Context(), socket, &response); err != nil {
			t.Fatal(err)
		}
		if response.Code != "unsupported_protocol_version" || len(response.Supported) != 1 || response.Supported[0] != 4 {
			t.Fatalf("protocol %d: %+v", version, response)
		}
	}
	var count int64
	if err := f.db.Raw(`SELECT count(*) FROM canvas.collab_document WHERE document_id=?`, f.doc.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("old protocol changed Yjs state", count, err)
	}
}
func TestYjsOnlyFirstOpenInitializesOnceAndRejectsOldSubmitMessages(t *testing.T) {
	f := newLiveFixture(t)
	for range 2 {
		socket := f.dial(t)
		liveSend(t, socket, map[string]any{"type": "join", "protocol_version": 4, "client_id": uuid.New(), "scope_id": f.actor.ScopeID, "csrf_token": f.auth.CSRFToken})
		packet := readYjsUntil(t, socket, func(p yjsPacket) bool { return p.Type == "yjs_snapshot" })
		if packet.State.Epoch == uuid.Nil || len(packet.State.State) == 0 || packet.State.CommitSeq != 0 || packet.State.Document.Revision != 1 {
			t.Fatal("invalid initial state", packet.State)
		}
		liveSend(t, socket, map[string]any{"type": "submit", "op_id": uuid.NewString(), "body": map[string]any{"operations": []any{}}})
		result := liveError(t, socket)
		if result.Code != "invalid_message" {
			t.Fatal("old editing message accepted", result)
		}
	}
	var count int64
	f.db.Raw(`SELECT count(*) FROM canvas.collab_document WHERE document_id=?`, f.doc.ID).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate initialization", count)
	}
	f.db.Raw(`SELECT count(*) FROM canvas.collab_receipt WHERE document_id=?`, f.doc.ID).Scan(&count)
	if count != 0 {
		t.Fatal("retired submit wrote receipt", count)
	}
}
func TestYjsNativeProjectionKeepsInputVersionsAndLockedTracks(t *testing.T) {
	for _, kind := range []string{"draft", "timeline"} {
		t.Run(kind, func(t *testing.T) {
			node := timelineNode()
			if kind == "draft" {
				node = draftNode(t, "image_generation")
				node.InputRevision = 1
			}
			before := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "projection", SchemaVersion: 1, Revision: 2, Nodes: []domain.Node{node}, Edges: []domain.Edge{}}
			projected := canvas.Snapshot(before)
			// Clone pointers so the authoritative baseline cannot be mutated by the test.
			body, _ := json.Marshal(projected)
			_ = json.Unmarshal(body, &projected)
			if kind == "draft" {
				projected.Nodes[0].Config.Draft.Prompt = "新输入"
			} else {
				before.Nodes[0].Config.Timeline.Tracks[0].Locked = true
				projected.Nodes[0].Config.Timeline.Tracks[0].Locked = true
				projected.Nodes[0].Config.Timeline.Clips[0].Text = "不能覆盖锁定轨道"
			}
			ops, err := canvas.YjsProjectionOperations(before, projected)
			if kind == "timeline" {
				if err == nil {
					t.Fatal("locked track bypassed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			after, err := domain.ApplyOperations(before, ops)
			if err != nil || after.Document.Nodes[0].InputRevision != 2 || len(after.Inputs) != 1 {
				t.Fatal("input version not derived", after, err)
			}
		})
	}
}

func TestYjsDeletionUndoRestoresDependentToolReferences(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := canvasfixture.NewService(canvasStore(db))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Yjs dependent restoration"})
	if err != nil {
		t.Fatal(err)
	}
	image := resourceNode("image")
	image.RefID = ptr(seedCanvasMedia(t, db, project, "image"))
	batch := batchNode(&image.ID)
	timeline := timelineNode()
	timeline.Config.Timeline.Tracks[0].Kind = "image"
	clip := &timeline.Config.Timeline.Clips[0]
	clip.Kind, clip.Text, clip.NodeID = "image", "", &image.ID
	director := directorNode()
	object := &director.Config.Director.Objects[0]
	object.Kind, object.BuiltinActor, object.SourceNodeID = "billboard", "", &image.ID
	if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{image, batch, timeline, director})}); err != nil {
		t.Fatal(err)
	}
	deletion := uuid.NewString()
	if _, err := service.Edit(t.Context(), actor, doc.ID, deletion, canvasfixture.EditInput{Operations: canvasfixture.Deletes([]uuid.UUID{image.ID})}); err != nil {
		t.Fatal(err)
	}
	// Unrelated later fields must survive restoration of deleted references.
	if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: []domain.Operation{canvasfixture.Field(batch.ID, []string{"title"}, "later title")}}); err != nil {
		t.Fatal(err)
	}
	restored, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{UndoOf: deletion})
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Nodes) != 4 {
		t.Fatal("lost stable identities")
	}
	for _, node := range restored.Nodes {
		switch node.ID {
		case batch.ID:
			if node.Title != "later title" || node.Config.BatchTable.Rows[0].InputNodeIDs[0] == nil || *node.Config.BatchTable.Rows[0].InputNodeIDs[0] != image.ID {
				t.Fatal("batch restoration lost later title or binding")
			}
		case timeline.ID:
			if len(node.Config.Timeline.Clips) != 1 || node.Config.Timeline.Clips[0].NodeID == nil || *node.Config.Timeline.Clips[0].NodeID != image.ID {
				t.Fatal("timeline reference not restored")
			}
		case director.ID:
			if len(node.Config.Director.Objects) != 1 || node.Config.Director.Objects[0].SourceNodeID == nil || *node.Config.Director.Objects[0].SourceNodeID != image.ID {
				t.Fatal("director reference not restored")
			}
		}
	}
}

func TestYjsUpdateIDIsScopedToSessionInProjectionAudit(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	store := canvasStore(db)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "scoped update identity"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	if _, err := canvasfixture.NewLegacySeeder(db, store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node})}); err != nil {
		t.Fatal(err)
	}
	shared := canvas.NewYjsService(store, yjsRuntime(t))
	state, err := shared.Initialize(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	left, right := yjsEdit(t, state, node.ID, "A", false), yjsEdit(t, state, node.ID, "B", false)
	right.UpdateID = left.UpdateID
	for _, update := range []canvas.YjsUpdate{left, right} {
		receipt, err := shared.Submit(t.Context(), actor, doc.ID, update)
		if err != nil || receipt.Status != "accepted" {
			t.Fatal("session-scoped identity collision", receipt, err)
		}
	}
	var logs int64
	if err := db.Raw(`SELECT count(*) FROM canvas.command_log WHERE document_id=?`, doc.ID).Scan(&logs).Error; err != nil || logs != 2 {
		t.Fatal(logs, err)
	}
}
