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
)

func TestM1DraftTwoWindowsHaveOneCASWinner(t *testing.T) {
	owner := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	a, p := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "LV_TEST_CANVAS_M1_RUNTIME_DSN")
	s := application.NewService(canvasStore(runtime))
	d, err := s.Create(t.Context(), a, p, uuid.NewString(), application.CreateInput{Name: "Concurrent draft"})
	if err != nil {
		t.Fatal(err)
	}
	n := draftNode(t, "image_generation")
	if _, err := s.Execute(t.Context(), a, d.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}}}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	var workers sync.WaitGroup
	for _, prompt := range []string{"Window A", "Window B"} {
		workers.Go(func() {
			<-start
			var config domain.NodeConfig
			_ = json.Unmarshal([]byte(`{"draft":{"prompt":"`+prompt+`","negative_prompt":"","aspect_ratio":"1:1","candidate_count":1}}`), &config)
			_, err := s.Execute(t.Context(), a, d.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: n.ID, Config: &config}}})
			errorsCh <- err
		})
	}
	close(start)
	workers.Wait()
	close(errorsCh)
	wins, conflicts := 0, 0
	for err := range errorsCh {
		var conflict *application.RevisionConflict
		switch {
		case err == nil:
			wins++
		case errors.As(err, &conflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	read, err := s.Get(t.Context(), a, d.ID)
	if err != nil || wins != 1 || conflicts != 1 || read.Revision != 3 || len(read.Nodes) != 1 || read.Nodes[0].InputRevision != 2 {
		t.Fatal("concurrent CAS lost updates", wins, conflicts, err)
	}
	var count int64
	if err := runtime.Raw(`SELECT count(*) FROM canvas.node_input WHERE node_id=?`, n.ID).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("losing draft escaped transaction", count, err)
	}
}

func TestM1DraftPersistenceHistoryReplayAndRollback(t *testing.T) {
	owner := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	a, p := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "LV_TEST_CANVAS_M1_RUNTIME_DSN")
	var role string
	if err := runtime.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("must use actual runtime role", role, err)
	}
	s := application.NewService(canvasStore(runtime))
	d, err := s.Create(t.Context(), a, p, uuid.NewString(), application.CreateInput{Name: "M1 draft"})
	if err != nil {
		t.Fatal(err)
	}
	n := draftNode(t, "image_generation")
	v := draftNode(t, "video_generation")
	var config domain.NodeConfig
	_ = json.Unmarshal([]byte(`{"draft":{"prompt":"Final in same batch","negative_prompt":"","aspect_ratio":"16:9","candidate_count":2}}`), &config)
	in := application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n, v}}, {Type: "UpdateNodeConfig", ID: n.ID, Config: &config}}}
	key := uuid.NewString()
	first, err := s.Execute(t.Context(), a, d.ID, key, in)
	if err != nil || first.Revision != 2 {
		t.Fatal("draft atomic save", err)
	}
	var histories int64
	if err := runtime.Raw(`SELECT count(*) FROM canvas.node_input WHERE node_id IN (?,?)`, n.ID, v.ID).Scan(&histories).Error; err != nil || histories != 3 {
		t.Fatal("lost intermediate immutable input", histories, err)
	}
	var prompt string
	if err := runtime.Raw(`SELECT draft->>'prompt' FROM canvas.node_input WHERE node_id=? AND input_revision=1`, n.ID).Scan(&prompt).Error; err != nil || prompt != "First" {
		t.Fatal("initial input overwritten", prompt, err)
	}
	for _, query := range []string{`UPDATE canvas.node_input SET draft='{}' WHERE node_id=?`, `DELETE FROM canvas.node_input WHERE node_id=?`} {
		if err := runtime.Exec(query, n.ID).Error; err == nil {
			t.Fatal("runtime changed immutable history")
		}
	}
	moved, err := s.Execute(t.Context(), a, d.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: n.ID, X: 180, Y: 90}}}}})
	if err != nil || moved.Revision != 3 {
		t.Fatal("move", err)
	}
	replay, err := application.NewService(canvasStore(runtime)).Execute(t.Context(), a, d.ID, key, in)
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatal("lost receipt replay", err)
	}
	if err := owner.Exec(`UPDATE canvas.save_receipt SET created_at=now()-interval '3 days' WHERE actor_id=? AND idem_key=?`, a.ID, key).Error; err != nil {
		t.Fatal(err)
	}
	oldReplay, err := s.Execute(t.Context(), a, d.ID, key, in)
	if err != nil || !reflect.DeepEqual(oldReplay, first) {
		t.Fatal("receipt expired with shared cache", err)
	}
	for _, query := range []string{`UPDATE canvas.save_receipt SET response_body='{}' WHERE actor_id=? AND idem_key=?`, `DELETE FROM canvas.save_receipt WHERE actor_id=? AND idem_key=?`} {
		if err := runtime.Exec(query, a.ID, key).Error; err == nil {
			t.Fatal("runtime changed durable save receipt")
		}
	}
	changed := in
	changed.ExpectedRevision = 3
	if _, err := s.Execute(t.Context(), a, d.ID, key, changed); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatal("save key reused", err)
	}
	if _, err := s.Execute(t.Context(), a, d.ID, uuid.NewString(), in); err == nil {
		t.Fatal("stale window saved")
	}
	bad := application.CommandsInput{ExpectedRevision: 3, Commands: []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: n.ID, X: 999}}}, {Type: "DeleteNodes", IDs: []uuid.UUID{v.ID}}}}
	if _, err := s.Execute(t.Context(), a, d.ID, uuid.NewString(), bad); err == nil {
		t.Fatal("unsupported deletion accepted")
	}
	read, err := s.Get(t.Context(), a, d.ID)
	if err != nil || read.Revision != 3 {
		t.Fatal("failure changed revision", err)
	}
	for _, node := range read.Nodes {
		if node.ID == n.ID && (node.X != 180 || inputRevision(t, node) != 2) {
			t.Fatal("move changed input or failed batch leaked")
		}
	}
	var audits int64
	if err := owner.Raw(`SELECT count(*) FROM infra.outbox WHERE topic='glowloom.audit.recorded.v1' AND payload->'data'->>'action'='canvas.saved' AND partition_key=?`, p.String()).Scan(&audits).Error; err != nil || audits != 2 {
		t.Fatal("audit count/replay", audits, err)
	}
	// A database-triggered audit failure must roll back the graph, history and receipt.
	trigger := "m1_audit_" + uuid.NewString()[:8]
	query := `CREATE FUNCTION infra.` + trigger + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.topic='glowloom.audit.recorded.v1' AND NEW.partition_key='` + p.String() + `' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER ` + trigger + ` BEFORE INSERT ON infra.outbox FOR EACH ROW EXECUTE FUNCTION infra.` + trigger + `()`
	if err := owner.Exec(query).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`DROP TRIGGER ` + trigger + ` ON infra.outbox; DROP FUNCTION infra.` + trigger + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	failKey := uuid.NewString()
	configRaw, _ := json.Marshal(config)
	_ = json.Unmarshal(configRaw, &config)
	config.Draft.Prompt = "Uncommitted"
	if _, err := s.Execute(t.Context(), a, d.ID, failKey, application.CommandsInput{ExpectedRevision: 3, Commands: []domain.Command{{Type: "UpdateNodeConfig", ID: n.ID, Config: &config}}}); err == nil {
		t.Fatal("audit failure ignored")
	}
	var failures int64
	if err := runtime.Raw(`SELECT (SELECT count(*) FROM canvas.node_input WHERE node_id=? AND input_revision>2)+(SELECT count(*) FROM canvas.save_receipt WHERE actor_id=? AND idem_key=?)`, n.ID, a.ID, failKey).Scan(&failures).Error; err != nil || failures != 0 {
		t.Fatal("history/receipt leaked", failures, err)
	}
}
