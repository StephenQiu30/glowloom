package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestCollaborationPersistsTransformsAndTerminalRejections(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, db)
	store := canvasStore(db)
	legacy := application.NewService(store)
	svc := application.NewCollaborationService(store)
	doc, err := legacy.Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "OT"})
	if err != nil {
		t.Fatal(err)
	}
	n := resourceNode("text")
	n.Config.Text = ptr("AB")
	created, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}}})
	if err != nil || created.Status != "accepted" {
		t.Fatal(created, err)
	}
	a := application.CollaborationInput{OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "text", NodeID: n.ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "X"}}}}}}}
	b := application.CollaborationInput{OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "text", NodeID: n.ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Retain: 1}, {Delete: 1}}}}}
	ar, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), a)
	if err != nil || ar.Status != "accepted" {
		t.Fatal(ar, err)
	}
	key := uuid.NewString()
	br, err := svc.Submit(t.Context(), actor, doc.ID, key, b)
	if err != nil || br.Status != "accepted" {
		t.Fatal(br, err)
	}
	loaded, err := legacy.Get(t.Context(), actor, doc.ID)
	if err != nil || loaded.Revision != 4 || *loaded.Nodes[0].Config.Text != "XA" {
		t.Fatal(loaded, err)
	}
	replay, err := svc.Resolve(t.Context(), actor, doc.ID, key, b)
	if err != nil || replay == nil || replay.Revision != 4 {
		t.Fatal(replay, err)
	}
	bad := a
	bad.BaseRevision = 2
	bad.Operations = []domain.Operation{{Type: "text", NodeID: n.ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Retain: 3}}}}
	badKey := uuid.NewString()
	rejected, err := svc.Submit(t.Context(), actor, doc.ID, badKey, bad)
	if err != nil || rejected.Status != "rejected" {
		t.Fatal("invalid original must reject", rejected, err)
	}
	rejectedAgain, err := application.NewCollaborationService(canvasStore(db)).Resolve(t.Context(), actor, doc.ID, badKey, bad)
	if err != nil || rejectedAgain == nil || rejectedAgain.Status != "rejected" || rejectedAgain.Code != rejected.Code {
		t.Fatal("terminal rejection lost", rejectedAgain, err)
	}
	if found, err := canvasReferenceCheck(t, db, actor, p, uuid.New()); err != nil || found {
		t.Fatal("OT history broke media reference admission", found, err)
	}
	changes, err := svc.Changes(t.Context(), actor, doc.ID, 2)
	if err != nil || len(changes) != 2 || changes[0].Revision != 3 || changes[1].Revision != 4 {
		t.Fatal(changes, err)
	}
	if _, err = legacy.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 4, Commands: []domain.Command{{Type: "RenameNodes", Names: []domain.Name{{ID: n.ID, Title: "old"}}}}}); err == nil {
		t.Fatal("legacy write bypassed OT mode")
	}
}

func TestCollaborationHistoryGapAndDeletedIdentityStayTerminal(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, db)
	store := canvasStore(db)
	legacy := application.NewService(store)
	svc := application.NewCollaborationService(store)
	doc, err := legacy.Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "OT tombstones"})
	if err != nil {
		t.Fatal(err)
	}
	n := resourceNode("text")
	submit := func(base int64, ops ...domain.Operation) application.CollaborationReceipt {
		t.Helper()
		r, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{OperationVersion: 1, BaseRevision: base, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := submit(1, domain.Operation{Type: "create_node", Node: &n}); r.Status != "accepted" {
		t.Fatal(r)
	}
	if r := submit(2, domain.Operation{Type: "delete_node", NodeID: n.ID}); r.Status != "accepted" {
		t.Fatal(r)
	}
	if r := submit(3, domain.Operation{Type: "delete_node", NodeID: n.ID}); r.Status != "accepted" {
		t.Fatal("proven repeat deletion should noop", r)
	}
	if r := submit(4, domain.Operation{Type: "create_node", Node: &n}); r.Code != "id_conflict" {
		t.Fatal("deleted identity reused", r)
	}
	if r := submit(4, domain.Operation{Type: "delete_node", NodeID: uuid.New()}); r.Status != "rejected" {
		t.Fatal("unknown delete treated as noop", r)
	}
	if err := db.Exec(`DELETE FROM canvas.command_log WHERE document_id=? AND revision=3`, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	input := application.CollaborationInput{OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "delete_node", NodeID: n.ID}}}
	key := uuid.NewString()
	r, err := svc.Submit(t.Context(), actor, doc.ID, key, input)
	if err != nil || r.Code != "history_unavailable" {
		t.Fatal(r, err)
	}
	r2, err := svc.Resolve(t.Context(), actor, doc.ID, key, input)
	if err != nil || r2 == nil || r2.Code != r.Code {
		t.Fatal(r2, err)
	}
}

func TestCollaborationUndoKeepsOthersAndRejectsOverlappingDeletion(t *testing.T) {
	for _, overlap := range []bool{false, true} {
		t.Run(map[bool]string{false: "independent", true: "overlap"}[overlap], func(t *testing.T) {
			db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
			actor, p := seedCanvasActor(t, db)
			store := canvasStore(db)
			svc := application.NewCollaborationService(store)
			doc, err := application.NewService(store).Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "contribution undo"})
			if err != nil {
				t.Fatal(err)
			}
			n := resourceNode("text")
			n.Config.Text = ptr("ABCDE")
			create := application.CollaborationInput{OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}}}
			if r, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), create); err != nil || r.Status != "accepted" {
				t.Fatal(r, err)
			}
			target := uuid.NewString()
			input := application.CollaborationInput{OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "text", NodeID: n.ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Retain: 1}, {Delete: 2}}}}}
			if r, err := svc.Submit(t.Context(), actor, doc.ID, target, input); err != nil || r.Status != "accepted" {
				t.Fatal(r, err)
			}
			late := domain.SequenceEdit{{Retain: 3}, {Delete: 1}}
			if overlap {
				late = domain.SequenceEdit{{Retain: 2}, {Delete: 2}}
			}
			input.Operations[0].Text = late
			if r, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), input); err != nil || r.Status != "accepted" {
				t.Fatal(r, err)
			}
			undo := application.CollaborationInput{OperationVersion: 1, BaseRevision: 4, UndoOf: target, Operations: []domain.Operation{}}
			r, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), undo)
			if err != nil {
				t.Fatal(err)
			}
			if overlap {
				if r.Status != "rejected" || r.Code != "undo_conflict" {
					t.Fatal("overlapping deleted C was resurrected", r)
				}
			} else {
				if r.Status != "accepted" {
					t.Fatal("independent deletion made undo unavailable", r)
				}
				current, err := application.NewService(store).Get(t.Context(), actor, doc.ID)
				if err != nil || *current.Nodes[0].Config.Text != "ABCE" {
					t.Fatal("undo erased the other contribution", current, err)
				}
			}
		})
	}
}

func TestCollaborationUndoRestoresNodeRelationsAndStableCollections(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, db)
	store := canvasStore(db)
	svc := application.NewCollaborationService(store)
	doc, err := application.NewService(store).Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "structural undo"})
	if err != nil {
		t.Fatal(err)
	}
	group := resourceNode("group")
	child := resourceNode("text")
	child.ParentID = &group.ID
	table := batchNode(nil)
	submit := func(base int64, key string, ops []domain.Operation, undo string) application.CollaborationReceipt {
		t.Helper()
		r, err := svc.Submit(t.Context(), actor, doc.ID, key, application.CollaborationInput{OperationVersion: 1, BaseRevision: base, Operations: ops, UndoOf: undo})
		if err != nil || r.Status != "accepted" {
			t.Fatal(r, err)
		}
		return r
	}
	submit(1, uuid.NewString(), []domain.Operation{{Type: "create_node", Node: &group}, {Type: "create_node", Node: &child}, {Type: "create_node", Node: &table}}, "")
	deletion := uuid.NewString()
	submit(2, deletion, []domain.Operation{{Type: "delete_node", NodeID: group.ID}}, "")
	submit(3, uuid.NewString(), []domain.Operation{{Type: "set", NodeID: child.ID, Path: []string{"title"}, Value: raw(`"another title"`)}}, "")
	submit(4, uuid.NewString(), []domain.Operation{}, deletion)
	current, err := application.NewService(store).Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range current.Nodes {
		if n.ID == child.ID && (n.ParentID == nil || *n.ParentID != group.ID || n.Title != "another title") {
			t.Fatal("node undo erased independent edit", n)
		}
	}
	row := domain.BatchTableRow{ID: uuid.New(), Enabled: true, Prompt: "own row", InputNodeIDs: []*uuid.UUID{nil}}
	value, _ := json.Marshal(row)
	insert := uuid.NewString()
	submit(5, insert, []domain.Operation{{Type: "insert", NodeID: table.ID, Path: []string{"config", "batch_table", "rows"}, ItemID: row.ID.String(), Value: value}}, "")
	submit(6, uuid.NewString(), []domain.Operation{}, insert)
	current, err = application.NewService(store).Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range current.Nodes {
		if n.ID == table.ID && len(n.Config.BatchTable.Rows) != 1 {
			t.Fatal("row contribution remains", n)
		}
	}
}

func TestCollaborationEphemeralIdentityAndMetadataRemainExplainable(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, db)
	store := canvasStore(db)
	legacy := application.NewService(store)
	svc := application.NewCollaborationService(store)
	doc, err := legacy.Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "identity"})
	if err != nil {
		t.Fatal(err)
	}
	n := resourceNode("text")
	r, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}, {Type: "delete_node", NodeID: n.ID}}})
	if err != nil || r.Status != "accepted" {
		t.Fatal(r, err)
	}
	r, err = svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "create_node", Node: &n}}})
	if err != nil || r.Code != "id_conflict" {
		t.Fatal("ephemeral ID reused", r, err)
	}
	_, err = legacy.Rename(t.Context(), actor, doc.ID, uuid.NewString(), application.RenameInput{ExpectedRevision: 2, Name: "renamed"})
	if err != nil {
		t.Fatal(err)
	}
	changes, err := svc.Changes(t.Context(), actor, doc.ID, 2)
	if err != nil || len(changes) != 1 || len(changes[0].Effects) != 0 {
		t.Fatal("rename broke OT history", changes, err)
	}
}

func TestCollaborationRetainsMediaFromIntermediateOperations(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, db)
	store := canvasStore(db)
	doc, err := application.NewService(store).Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "intermediate media"})
	if err != nil {
		t.Fatal(err)
	}
	asset := seedCanvasMedia(t, db, p, "image")
	n := resourceNode("image")
	n.RefID = &asset
	r, err := application.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}, {Type: "delete_node", NodeID: n.ID}}})
	if err != nil || r.Status != "accepted" {
		t.Fatal(r, err)
	}
	referenced, err := canvasReferenceCheck(t, db, actor, p, asset)
	if err != nil || !referenced {
		t.Fatal("intermediate source escaped history retention", referenced, err)
	}
}

func TestCollaborationContributionOwnershipNoopAndRedo(t *testing.T) {
	for _, scenario := range []string{"same-value", "deleted-noop", "redo"} {
		t.Run(scenario, func(t *testing.T) {
			db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
			actor, p := seedCanvasActor(t, db)
			store := canvasStore(db)
			svc := application.NewCollaborationService(store)
			doc, err := application.NewService(store).Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "ownership"})
			if err != nil {
				t.Fatal(err)
			}
			n := resourceNode("text")
			n.Config.Text = ptr("AB")
			send := func(base int64, key, undo string, ops ...domain.Operation) application.CollaborationReceipt {
				t.Helper()
				r, err := svc.Submit(t.Context(), actor, doc.ID, key, application.CollaborationInput{OperationVersion: 1, BaseRevision: base, Operations: ops, UndoOf: undo})
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			if r := send(1, uuid.NewString(), "", domain.Operation{Type: "create_node", Node: &n}); r.Status != "accepted" {
				t.Fatal(r)
			}
			target := uuid.NewString()
			var op domain.Operation
			switch scenario {
			case "same-value":
				op = domain.Operation{Type: "set", NodeID: n.ID, Path: []string{"title"}, Value: raw(`"same"`)}
			case "deleted-noop":
				op = domain.Operation{Type: "text", NodeID: n.ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Retain: 1}, {Delete: 1}}}
			default:
				op = domain.Operation{Type: "text", NodeID: n.ID, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "X"}}}}}
			}
			if r := send(2, target, "", op); r.Status != "accepted" {
				t.Fatal(r)
			}
			if scenario != "redo" {
				if r := send(2, uuid.NewString(), "", op); r.Status != "accepted" {
					t.Fatal(r)
				}
				if r := send(4, uuid.NewString(), target); r.Code != "undo_conflict" {
					t.Fatal("lost later ownership", r)
				}
				return
			}
			undo := uuid.NewString()
			if r := send(3, undo, target); r.Status != "accepted" {
				t.Fatal(r)
			}
			if r := send(4, uuid.NewString(), undo); r.Status != "accepted" {
				t.Fatal("redo unavailable", r)
			}
			latest, err := application.NewService(store).Get(t.Context(), actor, doc.ID)
			if err != nil || *latest.Nodes[0].Config.Text != "XAB" {
				t.Fatal(latest, err)
			}
		})
	}
}

func TestCollaborationReferenceLabelsMentionsAndStaleBinding(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, db)
	store := canvasStore(db)
	svc := application.NewCollaborationService(store)
	doc, err := application.NewService(store).Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "references"})
	if err != nil {
		t.Fatal(err)
	}
	n := composerNode(t)
	n.InputRevision = 0
	send := func(base int64, key, undo string, ops ...domain.Operation) application.CollaborationReceipt {
		t.Helper()
		r, err := svc.Submit(t.Context(), actor, doc.ID, key, application.CollaborationInput{OperationVersion: 1, BaseRevision: base, Operations: ops, UndoOf: undo})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := send(1, uuid.NewString(), "", domain.Operation{Type: "create_node", Node: &n}); r.Status != "accepted" {
		t.Fatal(r)
	}
	binding := func(id string, asset uuid.UUID) json.RawMessage {
		body, _ := json.Marshal(map[string]any{"id": id, "source": map[string]any{"kind": "media", "sourceId": asset, "sourceVersion": "1"}, "bindingGeneration": 1, "mediaKind": "image", "role": "reference_image", "order": 0})
		return body
	}
	a := binding("a", seedCanvasMedia(t, db, p, "image"))
	b := binding("b", seedCanvasMedia(t, db, p, "image"))
	path := []string{"config", "draft", "composer", "references"}
	text := []string{"config", "draft", "composer", "document"}
	for i, v := range []json.RawMessage{a, b} {
		id := []string{"a", "b"}[i]
		atoms := []domain.Atom{{Type: "reference", ReferenceID: id}}
		if i == 0 {
			atoms = append(atoms, atoms[0])
		}
		if r := send(2, uuid.NewString(), "", domain.Operation{Type: "insert", NodeID: n.ID, Path: path, ItemID: id, Value: v}, domain.Operation{Type: "text", NodeID: n.ID, Path: text, Text: domain.SequenceEdit{{Insert: atoms}}}); r.Status != "accepted" {
			t.Fatal(r)
		}
	}
	loaded, err := application.NewService(store).Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	d := loaded.Nodes[0].Config.Draft.Composer
	if d.LabelState.Allocations["a"].Ordinal != 1 || d.LabelState.Allocations["b"].Ordinal != 2 || loaded.Nodes[0].InputRevision != 3 {
		t.Fatal("server normalization", d, loaded.Nodes[0].InputRevision)
	}
	remove := uuid.NewString()
	if r := send(4, remove, "", domain.Operation{Type: "remove", NodeID: n.ID, Path: path, ItemID: "a"}); r.Status != "accepted" {
		t.Fatal(r)
	}
	if r := send(5, uuid.NewString(), remove); r.Status != "accepted" {
		t.Fatal("reference restore", r)
	}
	loaded, err = application.NewService(store).Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	d = loaded.Nodes[0].Config.Draft.Composer
	if d.References[0].ID != "a" || d.References[0].BindingGeneration != 2 || d.LabelState.Allocations["a"].Ordinal != 1 {
		t.Fatal("restore lost label or generation", d)
	}
	atoms, err := domain.PromptSequence(d.Document)
	if err != nil || len(atoms) != 3 {
		t.Fatal("duplicate mentions not restored", atoms, err)
	}
	var old map[string]any
	_ = json.Unmarshal(a, &old)
	delete(old, "id")
	delete(old, "order")
	expected, _ := json.Marshal(old)
	old["bindingGeneration"] = 2
	value, _ := json.Marshal(old)
	present := true
	if r := send(4, uuid.NewString(), "", domain.Operation{Type: "set", NodeID: n.ID, Path: append(path, "@a", "binding"), Expected: expected, ExpectedExists: &present, Value: value}); r.Status != "rejected" {
		t.Fatal("old callback rebound restored reference", r)
	}
}
