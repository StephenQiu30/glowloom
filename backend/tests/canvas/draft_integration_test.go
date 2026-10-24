package canvas_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestDraftFixtureBatchesPersistFinalInputs(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	a, p := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	store := canvasStore(runtime)
	s := canvasfixture.NewService(store)
	ot := canvasfixture.NewEditService(store)
	d, err := s.Create(t.Context(), a, p, uuid.NewString(), application.CreateInput{Name: "Concurrent draft"})
	if err != nil {
		t.Fatal(err)
	}
	n := draftNode(t, "image_generation")
	if _, err := s.Edit(t.Context(), a, d.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Nodes([]domain.Node{n})}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	receipts := make(chan canvasfixture.EditReceipt, 2)
	failures := make(chan error, 2)
	var workers sync.WaitGroup
	for _, text := range []string{"A", "B"} {
		workers.Go(func() {
			<-start
			r, err := ot.Submit(t.Context(), a, d.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "text", NodeID: n.ID, Path: []string{"config", "draft", "prompt"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: text}}}}}}})
			receipts <- r
			failures <- err
		})
	}
	close(start)
	workers.Wait()
	close(receipts)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for r := range receipts {
		if r.Status != "accepted" {
			t.Fatal(r)
		}
	}
	read, err := s.Get(t.Context(), a, d.ID)
	if err != nil || read.Revision != 4 || read.Nodes[0].InputRevision != 3 {
		t.Fatal("Yjs lost accepted input", read, err)
	}
	prompt := read.Nodes[0].Config.Draft.Prompt
	if prompt != "ABFirst" && prompt != "BAFirst" {
		t.Fatal(prompt)
	}
	var count int64
	if err := runtime.Raw(`SELECT count(*) FROM canvas.node_input WHERE node_id=?`, n.ID).Scan(&count).Error; err != nil || count != 3 {
		t.Fatal(count, err)
	}
}

func TestDraftPersistenceReceiptReplayAndAuditRollback(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	a, p := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	var role string
	if err := runtime.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("must use actual runtime role", role, err)
	}
	store := canvasStore(runtime)
	s := canvasfixture.NewService(store)
	ot := canvasfixture.NewEditService(store)
	d, err := s.Create(t.Context(), a, p, uuid.NewString(), application.CreateInput{Name: "Yjs draft"})
	if err != nil {
		t.Fatal(err)
	}
	n, v := draftNode(t, "image_generation"), draftNode(t, "video_generation")
	ops := canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{n, v}), canvasfixture.Text(n.ID, []string{"config", "draft", "prompt"}, "First", "Final in same batch"))
	in := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: ops}
	key := uuid.NewString()
	first, err := ot.Submit(t.Context(), a, d.ID, key, in)
	if err != nil || first.Status != "accepted" || first.Revision != 2 {
		t.Fatal(first, err)
	}
	var histories int64
	if err := runtime.Raw(`SELECT count(*) FROM canvas.node_input WHERE node_id IN (?,?)`, n.ID, v.ID).Scan(&histories).Error; err != nil || histories != 2 {
		t.Fatal("one final input per changed node/batch", histories, err)
	}
	var prompt string
	if err := runtime.Raw(`SELECT draft->>'prompt' FROM canvas.node_input WHERE node_id=? AND input_revision=1`, n.ID).Scan(&prompt).Error; err != nil || prompt != "Final in same batch" {
		t.Fatal(prompt, err)
	}
	for _, query := range []string{`UPDATE canvas.node_input SET draft='{}' WHERE node_id=?`, `DELETE FROM canvas.node_input WHERE node_id=?`} {
		if err := runtime.Exec(query, n.ID).Error; err == nil {
			t.Fatal("runtime changed immutable input")
		}
	}
	if _, err := s.Edit(t.Context(), a, d.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: canvasfixture.Positions([]domain.Move{{ID: n.ID, X: 180, Y: 90}})}); err != nil {
		t.Fatal(err)
	}
	replay, err := canvasfixture.NewEditService(canvasStore(runtime)).Resolve(t.Context(), a, d.ID, key, in)
	if err != nil || replay == nil || !reflect.DeepEqual(*replay, first) {
		t.Fatal("lost durable receipt", replay, err)
	}
	if err := owner.Exec(`UPDATE canvas.collab_receipt SET created_at=now()-interval '3 days' WHERE actor_id=? AND update_id=?`, a.ID, key).Error; err != nil {
		t.Fatal(err)
	}
	oldReplay, err := ot.Resolve(t.Context(), a, d.ID, key, in)
	if err != nil || oldReplay == nil || *oldReplay != first {
		t.Fatal(oldReplay, err)
	}
	for _, query := range []string{`UPDATE canvas.collab_receipt SET response_body='{}' WHERE actor_id=? AND update_id=?`, `DELETE FROM canvas.collab_receipt WHERE actor_id=? AND update_id=?`} {
		if err := runtime.Exec(query, a.ID, key).Error; err == nil {
			t.Fatal("runtime changed durable receipt")
		}
	}
	changed := in
	changed.BaseRevision = 3
	if _, err := ot.Submit(t.Context(), a, d.ID, key, changed); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	var audits int64
	if err := owner.Raw(`SELECT count(*) FROM infra.outbox WHERE topic='glowloom.audit.recorded.v1' AND payload->'data'->>'action'='canvas.saved' AND partition_key=?`, p.String()).Scan(&audits).Error; err != nil || audits != 1 {
		t.Fatal("audit/replay", audits, err)
	}
	trigger := "yjs_audit_" + uuid.NewString()[:8]
	query := `CREATE FUNCTION infra.` + trigger + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.topic='glowloom.audit.recorded.v1' AND NEW.partition_key='` + p.String() + `' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER ` + trigger + ` BEFORE INSERT ON infra.outbox FOR EACH ROW EXECUTE FUNCTION infra.` + trigger + `() `
	if err := owner.Exec(query).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`DROP TRIGGER ` + trigger + ` ON infra.outbox; DROP FUNCTION infra.` + trigger + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	failKey := uuid.NewString()
	failure := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 3, Operations: canvasfixture.Text(n.ID, []string{"config", "draft", "prompt"}, "Final in same batch", "Uncommitted")}
	if _, err := ot.Submit(t.Context(), a, d.ID, failKey, failure); err == nil {
		t.Fatal("audit failure ignored")
	}
	var leaked int64
	if err := runtime.Raw(`SELECT (SELECT count(*) FROM canvas.node_input WHERE node_id=? AND input_revision>1)+(SELECT count(*) FROM canvas.collab_receipt WHERE actor_id=? AND update_id=?)`, n.ID, a.ID, failKey).Scan(&leaked).Error; err != nil || leaked != 0 {
		t.Fatal(leaked, err)
	}
	read, err := s.Get(t.Context(), a, d.ID)
	if err != nil || read.Revision != 3 {
		t.Fatal(read, err)
	}
	for _, node := range read.Nodes {
		if node.ID == n.ID && (node.X != 180 || node.InputRevision != 1 || node.Config.Draft.Prompt != "Final in same batch") {
			body, _ := json.Marshal(node)
			t.Fatal(string(body))
		}
	}
}
