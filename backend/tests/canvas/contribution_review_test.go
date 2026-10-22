package canvas_test

import (
	"encoding/json"
	"fmt"
	"testing"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestReviewConcurrentNodeDeleteUndo(t *testing.T) {
	d := otDoc()
	a, b := uuid.New(), uuid.New()
	aid, bid := uuid.NewString(), uuid.NewString()
	ops := []domain.Operation{{Type: "delete_node", NodeID: d.Nodes[0].ID}}
	_, bp, err := domain.TransformOperationPair(d, ops, ops)
	if err != nil {
		t.Fatal(err)
	}
	one, err := domain.ApplyOperations(d, ops)
	if err != nil {
		t.Fatal(err)
	}
	one.Document.Revision = 2
	two, err := domain.ApplyOperations(one.Document, bp)
	if err != nil {
		t.Fatal(err)
	}
	two.Document.Revision = 3
	history := []domain.ContributionBatch{
		{Revision: 2, ActorID: a, OpID: aid, OriginalBase: 1, Original: ops, Operations: ops, Effects: one.Effects, Touches: one.Changes},
		{Revision: 3, ActorID: b, OpID: bid, OriginalBase: 1, Original: ops, Operations: bp, Effects: two.Effects, Touches: two.Changes},
	}
	inv, err := domain.UndoContribution(d, history, aid, a)
	if err == nil {
		restored, e := domain.ApplyOperations(two.Document, inv)
		t.Fatalf("second editor deletion resurrected: inverse=%+v nodes=%d apply=%v", inv, len(restored.Document.Nodes), e)
	}
}
func TestReviewUndoCreatedNodeWithInsertedBatchRowConsumer(t *testing.T) {
	d := otDoc()
	a, b := uuid.New(), uuid.New()
	media := resourceNode("image")
	table := batchNode(nil)
	table.Config.BatchTable.ReferenceColumns = []domain.BatchReferenceColumn{{ID: uuid.New(), Label: "A"}}
	table.Config.BatchTable.Rows[0].InputNodeIDs = []*uuid.UUID{nil}
	d.Nodes = []domain.Node{table}
	row := table.Config.BatchTable.Rows[0]
	row.ID = uuid.New()
	row.InputNodeIDs = []*uuid.UUID{&media.ID}
	rowJSON, _ := json.Marshal(row)
	ops1 := []domain.Operation{{Type: "create_node", Node: &media}}
	ops2 := []domain.Operation{{Type: "insert", NodeID: table.ID, Path: []string{"config", "batch_table", "rows"}, ItemID: row.ID.String(), Value: rowJSON}}
	one, err := domain.ApplyOperations(d, ops1)
	if err != nil {
		t.Fatal(err)
	}
	one.Document.Revision = 2
	two, err := domain.ApplyOperations(one.Document, ops2)
	if err != nil {
		t.Fatal(err)
	}
	two.Document.Revision = 3
	history := []domain.ContributionBatch{
		{Revision: 2, ActorID: a, OpID: "a", OriginalBase: 1, Original: ops1, Operations: ops1, Effects: one.Effects, Touches: one.Changes},
		{Revision: 3, ActorID: b, OpID: "b", OriginalBase: 2, Original: ops2, Operations: ops2, Effects: two.Effects, Touches: two.Changes},
	}
	inv, err := domain.UndoContribution(d, history, "a", a)
	if err == nil {
		restored, e := domain.ApplyOperations(two.Document, inv)
		t.Fatalf("second editor source erased: nodes=%d rowSlot=%v apply=%v", len(restored.Document.Nodes), restored.Document.Nodes[0].Config.BatchTable.Rows[1].InputNodeIDs[0], e)
	}
}

// Covers both causal known-removal normalization and same-base transformation.
func TestCollaborationStructuralDeletionIntentSurvivesNoop(t *testing.T) {
	for _, kind := range []string{"node", "edge", "row"} {
		for _, known := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/known=%t", kind, known), func(t *testing.T) {
				db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
				actor, p := seedCanvasActor(t, db)
				store := canvasStore(db)
				svc := application.NewCollaborationService(store)
				doc, err := application.NewService(store).Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "tombstone"})
				if err != nil {
					t.Fatal(err)
				}
				send := func(base int64, key, undo string, ops ...domain.Operation) application.CollaborationReceipt {
					t.Helper()
					r, e := svc.Submit(t.Context(), actor, doc.ID, key, application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: base, UndoOf: undo, Operations: ops})
					if e != nil {
						t.Fatal(e)
					}
					return r
				}
				n := resourceNode("text")
				other := resourceNode("text")
				table := batchNode(nil)
				edge := domain.Edge{ID: uuid.New(), SourceNodeID: n.ID, TargetNodeID: other.ID, EdgeType: "annotation"}
				setup := []domain.Operation{{Type: "create_node", Node: &n}, {Type: "create_node", Node: &other}}
				deletion := domain.Operation{Type: "delete_node", NodeID: n.ID}
				if kind == "edge" {
					setup = append(setup, domain.Operation{Type: "connect", Edge: &edge})
					deletion = domain.Operation{Type: "disconnect", NodeID: edge.ID}
				}
				if kind == "row" {
					setup = []domain.Operation{{Type: "create_node", Node: &table}}
					deletion = domain.Operation{Type: "remove", NodeID: table.ID, Path: []string{"config", "batch_table", "rows"}, ItemID: table.Config.BatchTable.Rows[0].ID.String()}
				}
				if r := send(1, uuid.NewString(), "", setup...); r.Status != "accepted" {
					t.Fatal(r)
				}
				key := uuid.NewString()
				if r := send(2, key, "", deletion); r.Status != "accepted" {
					t.Fatal(r)
				}
				base := int64(2)
				if known {
					base = 3
				}
				if r := send(base, uuid.NewString(), "", deletion); r.Status != "accepted" {
					t.Fatal(r)
				}
				undoKey := uuid.NewString()
				input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 4, UndoOf: key, Operations: nil}
				if r := send(4, undoKey, key); r.Code != "undo_conflict" || r.Status != "rejected" {
					t.Fatal("tombstone resurrected", r)
				}
				r, e := application.NewCollaborationService(canvasStore(db)).Resolve(t.Context(), actor, doc.ID, undoKey, input)
				if e != nil || r == nil || r.Code != "undo_conflict" {
					t.Fatal("rejection not durable", r, e)
				}
			})
		}
	}
}

func TestCollaborationInsertedRowConsumerRejectsCreateUndo(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, db)
	store := canvasStore(db)
	svc := application.NewCollaborationService(store)
	doc, err := application.NewService(store).Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "row consumer"})
	if err != nil {
		t.Fatal(err)
	}
	send := func(base int64, key, undo string, ops ...domain.Operation) application.CollaborationReceipt {
		t.Helper()
		r, e := svc.Submit(t.Context(), actor, doc.ID, key, application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: base, UndoOf: undo, Operations: ops})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	table := batchNode(nil)
	table.Config.BatchTable.ReferenceColumns = []domain.BatchReferenceColumn{{ID: uuid.New(), Label: "A"}}
	table.Config.BatchTable.Rows[0].InputNodeIDs = []*uuid.UUID{nil}
	media := resourceNode("image")
	media.RefID = ptr(seedCanvasMedia(t, db, p, "image"))
	if r := send(1, uuid.NewString(), "", domain.Operation{Type: "create_node", Node: &table}); r.Status != "accepted" {
		t.Fatal(r)
	}
	key := uuid.NewString()
	if r := send(2, key, "", domain.Operation{Type: "create_node", Node: &media}); r.Status != "accepted" {
		t.Fatal(r)
	}
	row := table.Config.BatchTable.Rows[0]
	row.ID = uuid.New()
	row.InputNodeIDs = []*uuid.UUID{&media.ID}
	value, _ := json.Marshal(row)
	if r := send(3, uuid.NewString(), "", domain.Operation{Type: "insert", NodeID: table.ID, Path: []string{"config", "batch_table", "rows"}, ItemID: row.ID.String(), Value: value}); r.Status != "accepted" {
		t.Fatal(r)
	}
	undoKey := uuid.NewString()
	if r := send(4, undoKey, key); r.Code != "undo_conflict" {
		t.Fatal(r)
	}
	r, e := application.NewCollaborationService(canvasStore(db)).Resolve(t.Context(), actor, doc.ID, undoKey, application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 4, UndoOf: key, Operations: nil})
	if e != nil || r == nil || r.Code != "undo_conflict" {
		t.Fatal(r, e)
	}
}
