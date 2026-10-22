package canvas_test

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func mediaReplacement(t *testing.T, node, expected, next uuid.UUID) domain.Operation {
	t.Helper()
	body, err := json.Marshal(map[string]any{"type": "replace_media", "node_id": node, "expected_media_ref": map[string]any{"asset_id": expected}, "new_media_ref": map[string]any{"asset_id": next}})
	if err != nil {
		t.Fatal(err)
	}
	var op domain.Operation
	if err := json.Unmarshal(body, &op); err != nil {
		t.Fatal(err)
	}
	return op
}

func contentVersion(t *testing.T, n domain.Node) int64 {
	t.Helper()
	body, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	var v int64
	if raw := fields["content_version"]; len(raw) > 0 {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		var err error
		v, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
	}
	return v
}

func TestCanvasReplaceMediaPreservesIdentityVersionEdgesAndUndo(t *testing.T) {
	for _, kind := range []string{"image", "video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
			actor, project := seedCanvasActor(t, db)
			store := canvasStore(db)
			svc := application.NewCollaborationService(store)
			read := application.NewService(store)
			doc, err := read.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "replace media"})
			if err != nil {
				t.Fatal(err)
			}
			old, next := seedCanvasMedia(t, db, project, kind), seedCanvasMedia(t, db, project, kind)
			n, other := resourceNode(kind), resourceNode("text")
			n.RefID = &old
			edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: n.ID, TargetNodeID: other.ID}
			submit := func(base int64, key string, ops []domain.Operation, undo string) application.CollaborationReceipt {
				t.Helper()
				r, e := svc.Submit(t.Context(), actor, doc.ID, key, application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: base, Operations: ops, UndoOf: undo})
				if e != nil {
					t.Fatal(e)
				}
				return r
			}
			if r := submit(1, uuid.NewString(), []domain.Operation{{Type: "create_node", Node: &n}, {Type: "create_node", Node: &other}, {Type: "connect", Edge: &edge}}, ""); r.Status != "accepted" {
				t.Fatal(r)
			}
			key := uuid.NewString()
			op := mediaReplacement(t, n.ID, old, next)
			if r := submit(2, key, []domain.Operation{op}, ""); r.Status != "accepted" {
				t.Fatalf("media replacement: %+v", r)
			}
			assertGraph := func(revision, version int64, asset uuid.UUID) {
				t.Helper()
				d, e := read.Get(t.Context(), actor, doc.ID)
				if e != nil || d.Revision != revision || len(d.Nodes) != 2 || len(d.Edges) != 1 || d.Edges[0].ID != edge.ID {
					t.Fatalf("identity/edge lost: %+v %v", d, e)
				}
				for _, node := range d.Nodes {
					if node.ID == n.ID && (node.RefID == nil || *node.RefID != asset || contentVersion(t, node) != version) {
						t.Fatalf("media/version: %+v", node)
					}
				}
			}
			assertGraph(3, 2, next)
			if r := submit(2, key, []domain.Operation{op}, ""); r.Status != "accepted" || r.Revision != 3 {
				t.Fatalf("replace replay: %+v", r)
			}
			if r := submit(3, uuid.NewString(), []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"position"}, Value: json.RawMessage(`{"x":10,"y":20}`)}}, ""); r.Status != "accepted" {
				t.Fatal(r)
			}
			assertGraph(4, 2, next)
			if r := submit(4, uuid.NewString(), []domain.Operation{mediaReplacement(t, n.ID, old, next)}, ""); r.Status != "rejected" || r.Code != "media_conflict" {
				t.Fatalf("stale old binding: %+v", r)
			}
			if r := submit(4, uuid.NewString(), nil, key); r.Status != "accepted" {
				t.Fatalf("replacement undo: %+v", r)
			}
			assertGraph(5, 3, old)
			for _, asset := range []uuid.UUID{old, next} {
				if found, e := canvasReferenceCheck(t, db, actor, project, asset); e != nil || !found {
					t.Fatalf("media history retention lost %s: %v %v", asset, found, e)
				}
			}
		})
	}
}

func TestCanvasReplaceMediaRejectsUnauthorizedAndWrongKindAtomically(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, p := seedCanvasActor(t, db)
	store := canvasStore(db)
	svc := application.NewCollaborationService(store)
	read := application.NewService(store)
	doc, err := read.Create(t.Context(), actor, p, uuid.NewString(), application.CreateInput{Name: "replace rejection"})
	if err != nil {
		t.Fatal(err)
	}
	old := seedCanvasMedia(t, db, p, "image")
	n := resourceNode("image")
	n.RefID = &old
	r, err := svc.Submit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}}})
	if err != nil || r.Status != "accepted" {
		t.Fatal(r, err)
	}
	_, foreign := seedCanvasActor(t, db)
	for _, test := range []struct {
		asset uuid.UUID
		code  string
	}{
		{seedCanvasMedia(t, db, foreign, "image"), "source_unavailable"},
		{seedCanvasMedia(t, db, p, "video"), "invalid_operation"},
		{uuid.New(), "source_unavailable"},
	} {
		asset := test.asset
		input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"title"}, Value: json.RawMessage(`"must roll back"`)}, mediaReplacement(t, n.ID, old, asset)}}
		key := uuid.NewString()
		r, err := svc.Submit(t.Context(), actor, doc.ID, key, input)
		if err != nil || r.Status != "rejected" || r.Code != test.code {
			t.Fatal(r, err)
		}
		receipt, e := svc.Resolve(t.Context(), actor, doc.ID, key, input)
		if e != nil || receipt == nil || receipt.Status != "rejected" {
			t.Fatal(receipt, e)
		}
		d, e := read.Get(t.Context(), actor, doc.ID)
		if e != nil || d.Revision != 2 || d.Nodes[0].Title != n.Title || *d.Nodes[0].RefID != old {
			t.Fatal("partial rejected replacement", d, e)
		}
	}
}

func TestCanvasCollaborationHashIgnoresNestedJSONKeyOrder(t *testing.T) {
	f := newLiveFixture(t)
	svc := application.NewCollaborationService(canvasStore(f.db))
	n := resourceNode("text")
	r, err := svc.Submit(t.Context(), f.actor, f.doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}}})
	if err != nil || r.Status != "accepted" {
		t.Fatal(r, err)
	}
	input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "set", NodeID: n.ID, Path: []string{"position"}, Value: json.RawMessage(`{"x":10,"y":20}`)}}}
	key := uuid.NewString()
	r, err = svc.Submit(t.Context(), f.actor, f.doc.ID, key, input)
	if err != nil || r.Status != "accepted" {
		t.Fatal(r, err)
	}
	input.Operations[0].Value = json.RawMessage(`{"y":20,"x":10}`)
	resolved, err := svc.Resolve(t.Context(), f.actor, f.doc.ID, key, input)
	if err != nil || resolved == nil || resolved.Revision != 3 {
		t.Fatalf("semantic JSON replay differed: %+v %v", resolved, err)
	}
}

func TestCanvasReplaceMediaReceiptFailureRollsBackAndOriginalRequestRetries(t *testing.T) {
	f := newLiveFixture(t)
	svc := application.NewCollaborationService(canvasStore(f.db))
	old, next := seedCanvasMedia(t, f.db, f.doc.ProjectID, "image"), seedCanvasMedia(t, f.db, f.doc.ProjectID, "image")
	n := resourceNode("image")
	n.RefID = &old
	r, err := svc.Submit(t.Context(), f.actor, f.doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}}})
	if err != nil || r.Status != "accepted" {
		t.Fatal(r, err)
	}
	key := uuid.NewString()
	input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{mediaReplacement(t, n.ID, old, next)}}
	trigger := "replace_receipt_" + uuid.NewString()[:8]
	ddl := `CREATE FUNCTION canvas.` + trigger + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.document_id='` + f.doc.ID.String() + `' AND NEW.idem_key='` + key + `' THEN RAISE EXCEPTION 'injected receipt write failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER ` + trigger + ` BEFORE INSERT ON canvas.save_receipt FOR EACH ROW EXECUTE FUNCTION canvas.` + trigger + `()`
	if err := f.db.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	cleanup := func() error {
		return f.db.Exec(`DROP TRIGGER IF EXISTS ` + trigger + ` ON canvas.save_receipt; DROP FUNCTION IF EXISTS canvas.` + trigger + `()`).Error
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	})
	if _, err := svc.Submit(t.Context(), f.actor, f.doc.ID, key, input); err == nil {
		t.Fatal("receipt failure was reported as a successful save")
	}
	if receipt, err := svc.Resolve(t.Context(), f.actor, f.doc.ID, key, input); err != nil || receipt != nil {
		t.Fatal("failed transaction left terminal receipt", receipt, err)
	}
	loaded, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
	if err != nil || loaded.Revision != 2 || *loaded.Nodes[0].RefID != old || contentVersion(t, loaded.Nodes[0]) != 1 {
		t.Fatal("receipt failure left partial graph", loaded, err)
	}
	var count int64
	if err := f.db.Raw(`SELECT count(*) FROM canvas.command_log WHERE document_id=?`, f.doc.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("receipt failure left canonical log", count, err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	r, err = svc.Submit(t.Context(), f.actor, f.doc.ID, key, input)
	if err != nil || r.Status != "accepted" || r.Revision != 3 {
		t.Fatal("exact original request could not retry", r, err)
	}
}

func TestCanvasConcurrentMediaReplacementHasOneWinner(t *testing.T) {
	f := newLiveFixture(t)
	svc := application.NewCollaborationService(canvasStore(f.db))
	old := seedCanvasMedia(t, f.db, f.doc.ProjectID, "image")
	next := []uuid.UUID{seedCanvasMedia(t, f.db, f.doc.ProjectID, "image"), seedCanvasMedia(t, f.db, f.doc.ProjectID, "image")}
	n := resourceNode("image")
	n.RefID = &old
	r, err := svc.Submit(t.Context(), f.actor, f.doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}}})
	if err != nil || r.Status != "accepted" {
		t.Fatal(r, err)
	}
	type outcome struct {
		receipt application.CollaborationReceipt
		err     error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, asset := range next {
		input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{mediaReplacement(t, n.ID, old, asset)}}
		workers.Go(func() {
			<-start
			r, err := svc.Submit(t.Context(), f.actor, f.doc.ID, uuid.NewString(), input)
			results <- outcome{r, err}
		})
	}
	close(start)
	workers.Wait()
	close(results)
	accepted, rejected := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		switch {
		case result.receipt.Status == "accepted":
			accepted++
		case result.receipt.Status == "rejected" && result.receipt.Code == "media_conflict":
			rejected++
		default:
			t.Fatal(result.receipt)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("concurrent replacements: accepted=%d rejected=%d", accepted, rejected)
	}
	d, err := f.service.Get(t.Context(), f.actor, f.doc.ID)
	if err != nil || d.Revision != 3 || contentVersion(t, d.Nodes[0]) != 2 || (*d.Nodes[0].RefID != next[0] && *d.Nodes[0].RefID != next[1]) {
		t.Fatal("replacement lost accepted binding", d, err)
	}
}
