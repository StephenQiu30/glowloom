package canvas_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	promptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/prompt"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	promptpg "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestResourceInputPostgresVersionsReplayAndSourceAuthorization(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	foreign, foreignProject := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	var role string
	if err := runtime.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "glowloom_app" {
		t.Fatal("input persistence must use restricted runtime role", role, err)
	}
	store := canvasStore(runtime)
	reader := application.NewService(store)
	writer := canvasfixture.NewEditService(store)
	inputReader := promptpg.NewDraftStore(runtime, promptwire.DraftAccess)
	for _, kind := range []string{"text", "image", "video", "audio"} {
		t.Run(kind, func(t *testing.T) {
			doc, err := reader.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Resource input " + kind})
			if err != nil {
				t.Fatal(err)
			}
			node := resourceInputNode(t, kind)
			draft := node.Config.Draft
			node.Config.Draft = nil
			create := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: []domain.Operation{{Type: "create_node", Node: &node}}}
			if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), create); err != nil || r.Status != "accepted" {
				t.Fatal("create resource", r, err)
			}
			before, err := reader.Get(t.Context(), actor, doc.ID)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(map[string]any{"draft": draft})
			first := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: []domain.Operation{{Type: "set", NodeID: node.ID, Path: []string{"config", "draft"}, ExpectedExists: ptr(true), Expected: raw(`{}`), Value: body}}}
			key := uuid.NewString()
			receipt, err := writer.Submit(t.Context(), actor, doc.ID, key, first)
			if err != nil || receipt.Status != "accepted" {
				t.Fatal("first input", receipt, err)
			}
			read, err := application.NewService(canvasStore(runtime)).Get(t.Context(), actor, doc.ID)
			if err != nil || len(read.Nodes) != 1 || read.Nodes[0].InputRevision != 1 || read.Nodes[0].NodeType != kind || read.Nodes[0].NodeAction != "resource" || !reflect.DeepEqual(read.Nodes[0].Config.Draft, draft) {
				t.Fatal("reopen lost same-node input", read, err)
			}
			if kind == "text" && (!reflect.DeepEqual(read.Nodes[0].Config.Text, before.Nodes[0].Config.Text) || !reflect.DeepEqual(read.Nodes[0].Config.RichText, before.Nodes[0].Config.RichText) || !reflect.DeepEqual(read.Nodes[0].Config.Composer, before.Nodes[0].Config.Composer)) {
				t.Fatal("generation input overwrote result text")
			}
			replay, err := canvasfixture.NewEditService(canvasStore(runtime)).Resolve(t.Context(), actor, doc.ID, key, first)
			if err != nil || replay == nil || *replay != receipt {
				t.Fatal("durable receipt replay", replay, err)
			}
			assertSnapshots := func(want int) {
				t.Helper()
				var count int
				if err := runtime.Raw(`SELECT count(*) FROM canvas.node_input WHERE node_id=?`, node.ID).Scan(&count).Error; err != nil || count != want {
					t.Fatal("unexpected immutable input history", count, want, err)
				}
			}
			assertSnapshots(1)
			source := promptdomain.InputSource{Kind: "canvas_input", CanvasID: doc.ID, NodeID: node.ID, CanvasRevision: read.Revision, InputRevision: 1}
			if got, err := inputReader.ReadInput(t.Context(), actor, project, source, false); err != nil || !reflect.DeepEqual(got, *draft.Composer) {
				t.Fatal("formal input reader cannot read resource", got, err)
			}
			if _, err := inputReader.ReadInput(t.Context(), foreign, project, source, false); err == nil {
				t.Fatal("foreign actor read input")
			}
			if _, err := inputReader.ReadInput(t.Context(), actor, foreignProject, source, false); err == nil {
				t.Fatal("cross-project input read")
			}
			layout := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: []domain.Operation{{Type: "set", NodeID: node.ID, Path: []string{"position"}, Value: raw(`{"x":55,"y":66}`)}}}
			if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), layout); err != nil || r.Status != "accepted" {
				t.Fatal("layout", r, err)
			}
			assertSnapshots(1)
			if _, err := inputReader.ReadInput(t.Context(), actor, project, source, false); !errors.Is(err, promptdomain.ErrInputStale) {
				t.Fatal("quote ignored stale canvas revision", err)
			}
			if _, err := inputReader.ReadInput(t.Context(), actor, project, source, true); err != nil {
				t.Fatal("confirm rejected unchanged generation input after layout", err)
			}
			next := composerNode(t).Config.Draft
			next.Composer.Params["temperature"] = raw(`0.4`)
			nextBody, _ := json.Marshal(map[string]any{"draft": next})
			change := first
			change.Operations = []domain.Operation{{Type: "set", NodeID: node.ID, Path: []string{"config", "draft"}, ExpectedExists: ptr(true), Expected: body, Value: nextBody}}
			if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), change); err != nil || r.Status != "accepted" {
				t.Fatal("change input", r, err)
			}
			assertSnapshots(2)
			if _, err := inputReader.ReadInput(t.Context(), actor, project, source, true); !errors.Is(err, promptdomain.ErrInputStale) {
				t.Fatal("confirm accepted changed input", err)
			}
			if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), change); err != nil || r.Status != "rejected" {
				t.Fatal("stale compare-and-set accepted", r, err)
			}
			deleteKey := uuid.NewString()
			deletion := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: []domain.Operation{{Type: "delete_node", NodeID: node.ID}}}
			if r, err := writer.Submit(t.Context(), actor, doc.ID, deleteKey, deletion); err != nil || r.Status != "accepted" {
				t.Fatal("delete input owner", r, err)
			}
			assertSnapshots(2)
			if _, err := inputReader.ReadInput(t.Context(), actor, project, source, true); err == nil {
				t.Fatal("deleted input remained executable")
			}
			if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, UndoOf: deleteKey}); err != nil || r.Status != "accepted" {
				t.Fatal("restore input owner", r, err)
			}
			assertSnapshots(3)
			read, err = reader.Get(t.Context(), actor, doc.ID)
			if err != nil || read.Nodes[0].ID != node.ID || read.Nodes[0].InputRevision != 3 {
				t.Fatal("restore reused input revision", read, err)
			}
		})
	}
}

func TestResourceInputPostgresReadOnlyGraphAdmission(t *testing.T) {
	for _, mutation := range []string{"unknown_config", "unknown_node", "unsupported_schema", "archived_project"} {
		t.Run(mutation, func(t *testing.T) {
			owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
			actor, project := seedCanvasActor(t, owner)
			runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
			store := canvasStore(runtime)
			writer := canvasfixture.NewEditService(store)
			doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Read-only input admission"})
			if err != nil {
				t.Fatal(err)
			}
			node, other := resourceInputNode(t, "text"), resourceNode("text")
			created, err := canvasfixture.NewLegacySeeder(owner, store).Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node, other})})
			if err != nil {
				t.Fatal(err)
			}
			source := promptdomain.InputSource{Kind: "canvas_input", CanvasID: doc.ID, NodeID: node.ID, CanvasRevision: created.Revision, InputRevision: 1}
			inputReader := promptpg.NewDraftStore(runtime, promptwire.DraftAccess)
			if _, err := inputReader.ReadInput(t.Context(), actor, project, source, false); err != nil {
				t.Fatal("valid control input rejected", err)
			}
			switch mutation {
			case "unknown_config":
				err = owner.Exec(`UPDATE canvas.node SET config=config || '{"future_input_contract":true}'::jsonb WHERE id=?`, node.ID).Error
			case "unknown_node":
				err = owner.Exec(`UPDATE canvas.node SET node_type='future' WHERE id=?`, other.ID).Error
			case "unsupported_schema":
				err = owner.Exec(`UPDATE canvas.document SET schema_version=99 WHERE id=?`, doc.ID).Error
			case "archived_project":
				err = owner.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, confirm := range []bool{false, true} {
				if _, err := inputReader.ReadInput(t.Context(), actor, project, source, confirm); err == nil {
					t.Fatalf("read-only graph admitted generation input: confirm=%t", confirm)
				} else if mutation != "archived_project" && !errors.Is(err, promptdomain.ErrInputStale) {
					t.Fatal("read-only graph must use the recognized source-conflict response", err)
				}
			}
		})
	}
}

func TestResourceInputPostgresAuditFailureRollsBackYjsAndInput(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	store := canvasStore(runtime)
	writer := canvasfixture.NewEditService(store)
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Input transaction rollback"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceInputNode(t, "image")
	created, err := writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node})})
	if err != nil {
		t.Fatal(err)
	}
	shared := application.NewYjsService(store, yjsRuntime(t))
	before, err := shared.Load(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	trigger := "resource_input_audit_" + uuid.NewString()[:8]
	if err := owner.Exec(`CREATE FUNCTION infra.` + trigger + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.topic='glowloom.audit.recorded.v1' AND NEW.partition_key='` + project.String() + `' THEN RAISE EXCEPTION 'synthetic resource input audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER ` + trigger + ` BEFORE INSERT ON infra.outbox FOR EACH ROW EXECUTE FUNCTION infra.` + trigger + `()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`DROP TRIGGER ` + trigger + ` ON infra.outbox; DROP FUNCTION infra.` + trigger + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	prior, _ := json.Marshal(map[string]any{"draft": created.Nodes[0].Config.Draft})
	next := composerNode(t).Config.Draft
	next.Composer.Params["temperature"] = raw(`0.7`)
	body, _ := json.Marshal(map[string]any{"draft": next})
	key := uuid.NewString()
	input := canvasfixture.EditInput{Operations: []domain.Operation{{Type: "set", NodeID: node.ID, Path: []string{"config", "draft"}, ExpectedExists: ptr(true), Expected: prior, Value: body}}}
	if _, err := writer.Submit(t.Context(), actor, doc.ID, key, input); err == nil {
		t.Fatal("audit failure did not abort accepted input transaction")
	}
	after, err := shared.Load(t.Context(), actor, doc.ID)
	if err != nil || !reflect.DeepEqual(after.State, before.State) || after.Document.Revision != before.Document.Revision {
		t.Fatal("failed audit leaked authoritative Yjs state", err)
	}
	read, err := writer.Get(t.Context(), actor, doc.ID)
	if err != nil || read.Nodes[0].InputRevision != 1 || !reflect.DeepEqual(read.Nodes[0].Config.Draft, created.Nodes[0].Config.Draft) {
		t.Fatal("failed audit leaked resource input", read, err)
	}
	var leaked int
	if err := runtime.Raw(`SELECT (SELECT count(*) FROM canvas.node_input WHERE node_id=? AND input_revision>1)+(SELECT count(*) FROM canvas.collab_receipt WHERE actor_id=? AND update_id=?)`, node.ID, actor.ID, key).Scan(&leaked).Error; err != nil || leaked != 0 {
		t.Fatal("failed transaction retained input or receipt", leaked, err)
	}
}
