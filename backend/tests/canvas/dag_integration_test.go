package canvas_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestDAGPersistenceCompleteBatchReloadCleanupAndReplay(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := canvasfixture.NewService(canvasStore(db))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "DAG持久化"})
	if err != nil {
		t.Fatal(err)
	}
	image := resourceNode("image")
	image.RefID = ptr(seedCanvasMedia(t, db, project, "image"))
	batch := batchNode(&image.ID)
	// Repeated source bindings are distinct slots, not duplicate visual edges.
	batch.Config.BatchTable.ReferenceColumns = append(batch.Config.BatchTable.ReferenceColumns, domain.BatchReferenceColumn{ID: uuid.New(), Label: "第二列"})
	batch.Config.BatchTable.Rows[0].InputNodeIDs = append(batch.Config.BatchTable.Rows[0].InputNodeIDs, &image.ID)
	director := directorNode()
	obj := &director.Config.Director.Objects[0]
	obj.Kind, obj.BuiltinActor, obj.SourceNodeID = "billboard", "", &image.ID
	timeline := timelineNode()
	timeline.Config.Timeline.Tracks[0].Kind = "image"
	clip := &timeline.Config.Timeline.Clips[0]
	clip.Kind, clip.Text, clip.NodeID = "image", "", &image.ID
	key := uuid.NewString()
	input := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{batch, director, timeline}), canvasfixture.Nodes([]domain.Node{image}))}
	saved, err := service.Edit(t.Context(), actor, doc.ID, key, input)
	if err != nil || saved.Revision != 2 {
		t.Fatal("valid final graph failed to persist", err)
	}
	fresh := canvasfixture.NewService(canvasStore(db))
	again, err := fresh.Edit(t.Context(), actor, doc.ID, key, input)
	if err != nil || again.Revision != 2 || len(again.Nodes) != 4 {
		t.Fatal("durable replay", err)
	}
	loaded, err := fresh.Get(t.Context(), actor, doc.ID)
	if err != nil || loaded.Revision != 2 || len(loaded.Nodes) != 4 {
		t.Fatal("reload", err)
	}
	failedKey := uuid.NewString()
	_, err = fresh.Edit(t.Context(), actor, doc.ID, failedKey, canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: canvasfixture.Concat(canvasfixture.Positions([]domain.Move{{ID: batch.ID, X: 90}}), canvasfixture.Edges([]domain.Edge{{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: image.ID, TargetNodeID: uuid.New()}}))})
	if !operationRejected(err, "invalid_operation", "invalid_graph") {
		t.Fatal("invalid endpoint accepted", err)
	}
	loaded, err = fresh.Get(t.Context(), actor, doc.ID)
	if err != nil || loaded.Revision != 2 {
		t.Fatal("failed batch advanced version", err)
	}
	for _, n := range loaded.Nodes {
		if n.ID == batch.ID && n.X != 0 {
			t.Fatal("partial layout persisted")
		}
	}
	deleteKey := uuid.NewString()
	deletion := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: canvasfixture.Concat(canvasfixture.Deletes([]uuid.UUID{image.ID}))}
	if _, err := fresh.Edit(t.Context(), actor, doc.ID, deleteKey, deletion); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Edit(t.Context(), actor, doc.ID, deleteKey, deletion); err != nil {
		t.Fatal("delete replay", err)
	}
	loaded, err = canvasfixture.NewService(canvasStore(db)).Get(t.Context(), actor, doc.ID)
	if err != nil || loaded.Revision != 3 || len(loaded.Nodes) != 3 {
		t.Fatal("delete reload", err)
	}
	for _, n := range loaded.Nodes {
		if c := n.Config.BatchTable; c != nil {
			if len(c.Rows[0].InputNodeIDs) != 2 || c.Rows[0].InputNodeIDs[0] != nil || c.Rows[0].InputNodeIDs[1] != nil {
				t.Fatal("not all slots cleared")
			}
		}
		if c := n.Config.Director; c != nil {
			if c.Objects[0].SourceNodeID != nil || c.Objects[0].AssetID == nil || *c.Objects[0].AssetID != *image.RefID {
				t.Fatal("director lost pinned asset")
			}
		}
		if c := n.Config.Timeline; c != nil {
			if c.Clips[0].NodeID != nil || c.Clips[0].AssetID == nil || *c.Clips[0].AssetID != *image.RefID {
				t.Fatal("timeline lost pinned asset")
			}
		}
	}
	var assets, logs, receipts, operations int64
	if err := db.Raw(`SELECT count(*) FROM media.media_asset WHERE id=? AND NOT is_delete`, *image.RefID).Scan(&assets).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT count(*) FROM canvas.command_log WHERE document_id=?`, doc.ID).Scan(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT count(*) FROM canvas.collab_receipt WHERE document_id=?`, doc.ID).Scan(&receipts).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT count(*) FROM operation.operation WHERE project_id=?`, project).Scan(&operations).Error; err != nil {
		t.Fatal(err)
	}
	if assets != 1 || logs != 2 || receipts != 2 || operations != 0 {
		t.Fatalf("unexpected side effects asset=%d log=%d receipt=%d operation=%d", assets, logs, receipts, operations)
	}
}

func TestDAGPersistenceUnknownHistoryRemainsReadableAndReadOnly(t *testing.T) {
	for _, kind := range []string{"input", "unknown_config", "nested_unknown", "unknown_type", "future_version", "invalid_endpoint"} {
		t.Run(kind, func(t *testing.T) {
			db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
			actor, project := seedCanvasActor(t, db)
			service := canvasfixture.NewService(canvasStore(db))
			doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "旧图保护"})
			if err != nil {
				t.Fatal(err)
			}
			a, b := resourceNode("text"), batchNode(nil)
			_, err = canvasfixture.NewLegacySeeder(db, canvasStore(db)).Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{a, b}))})
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "input":
				err = db.Exec(`INSERT INTO canvas.edge(id,document_id,edge_type,source_node_id,target_node_id) VALUES(?,?,'input',?,?)`, uuid.New(), doc.ID, a.ID, b.ID).Error
			case "unknown_config":
				err = db.Exec(`UPDATE canvas.node SET config=config || '{"future":{"keep":true}}'::jsonb WHERE id=?`, a.ID).Error
			case "nested_unknown":
				err = db.Exec(`UPDATE canvas.node SET config=jsonb_set(config,'{batch_table,rows,0,future_binding}','"keep"'::jsonb) WHERE id=?`, b.ID).Error
			case "unknown_type":
				err = db.Exec(`UPDATE canvas.node SET node_type='future_tool' WHERE id=?`, a.ID).Error
			case "future_version":
				err = db.Exec(`UPDATE canvas.node SET config=jsonb_set(config,'{batch_table,version}','99'::jsonb) WHERE id=?`, b.ID).Error
			case "invalid_endpoint":
				err = db.Exec(`UPDATE canvas.node SET config=jsonb_set(config,'{batch_table,rows,0,input_node_ids}',?::jsonb) WHERE id=?`, `["`+uuid.NewString()+`"]`, b.ID).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := service.Get(t.Context(), actor, doc.ID)
			if err != nil {
				t.Fatal("legacy graph unreadable", err)
			}
			if loaded.ReadOnlyReason == "" {
				t.Fatal("unsafe graph was not identified as read-only")
			}
			if kind == "unknown_config" || kind == "nested_unknown" {
				body, err := json.Marshal(loaded)
				if err != nil {
					t.Fatal(err)
				}
				var response map[string]any
				if err := json.Unmarshal(body, &response); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, raw := range response["nodes"].([]any) {
					n := raw.(map[string]any)
					c := n["config"].(map[string]any)
					if kind == "unknown_config" && n["id"] == a.ID.String() {
						_, found = c["future"]
					}
					if kind == "nested_unknown" && n["id"] == b.ID.String() {
						row := c["batch_table"].(map[string]any)["rows"].([]any)[0].(map[string]any)
						_, found = row["future_binding"]
					}
				}
				if !found {
					t.Fatal("unknown field lost in API representation")
				}
			}
			router := directorOutputRouter(db, actor)
			read := directorOutputRequest(t.Context(), t, router, "GET", "/api/canvases/"+doc.ID.String(), "", nil)
			if read.Code != 200 {
				t.Fatal("read-only graph GET failed", read.Code)
			}
			shared := application.NewYjsService(canvasStore(db), yjsRuntime(t))
			if _, err := shared.Initialize(t.Context(), actor, doc.ID, 2); !errors.Is(err, domain.ErrGraphReadOnly) {
				t.Fatal("protected legacy graph migrated", err)
			}
			var revision int64
			var receipts int64
			if err := db.Raw(`SELECT revision FROM canvas.document WHERE id=?`, doc.ID).Scan(&revision).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Raw(`SELECT count(*) FROM canvas.collab_receipt WHERE document_id=?`, doc.ID).Scan(&receipts).Error; err != nil {
				t.Fatal(err)
			}
			if revision != 2 || receipts != 0 {
				t.Fatal("read-only graph mutated", revision, receipts)
			}
		})
	}
}

func TestDAGPersistenceConcurrentDuplicateAnnotationHasSingleWinner(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := canvasfixture.NewService(canvasStore(db))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "并发图"})
	if err != nil {
		t.Fatal(err)
	}
	a, b := resourceNode("text"), resourceNode("text")
	if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{a, b}))}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	result := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			_, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: canvasfixture.Concat(canvasfixture.Edges([]domain.Edge{{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: a.ID, TargetNodeID: b.ID}}))})
			result <- err
		})
	}
	close(start)
	wg.Wait()
	close(result)
	wins, conflicts := 0, 0
	for err := range result {
		var conflict *domain.OperationError
		switch {
		case err == nil:
			wins++
		case errors.As(err, &conflict) && conflict.Code == "invalid_operation":
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	got, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil || wins != 1 || conflicts != 1 || got.Revision != 3 || len(got.Edges) != 1 {
		t.Fatal("concurrent graph diverged", wins, conflicts, got, err)
	}
}
