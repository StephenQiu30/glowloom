package canvas_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/yjsruntime"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func yjsRuntime(t *testing.T) *yjsruntime.Client {
	t.Helper()
	return yjsruntime.New(yjsRuntimeAddress(t), "isolated-test-runtime-token-32-bytes")
}
func yjsRuntimeAddress(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../../canvas-collab")
	if err != nil {
		t.Fatal(err)
	}
	script := `import {runtimeServer} from './src/server.mjs'; const s=runtimeServer({token:'isolated-test-runtime-token-32-bytes'});s.listen(0,'127.0.0.1',()=>console.log(s.address().port));`
	command := exec.CommandContext(t.Context(), "node", "--input-type=module", "-e", script)
	command.Dir = root
	pipe, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	scanner := bufio.NewScanner(pipe)
	if !scanner.Scan() {
		t.Fatalf("runtime did not start: %s", stderr.String())
	}
	return "http://127.0.0.1:" + scanner.Text()
}
func yjsEdit(t *testing.T, state canvas.YjsState, node uuid.UUID, text string, evil bool) canvas.YjsUpdate {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"state": state.State, "nodeId": node, "text": text, "evil": evil})
	command := exec.CommandContext(t.Context(), "node", "../../../canvas-collab/tests/service-fixture.mjs")
	command.Stdin = bytes.NewReader(body)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	input := canvas.YjsUpdate{Epoch: state.Epoch, SessionID: uuid.New(), UpdateID: uuid.New()}
	if err := json.Unmarshal(output, &input); err != nil {
		t.Fatal(err)
	}
	return input
}
func TestYjsRuntimeAtomicAcceptanceConcurrentTextAndRecovery(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	store := canvasStore(database)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Yjs acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	node.Config.Text = ptr("AB中文😀")
	_, err = canvas.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &node}}})
	if err != nil {
		t.Fatal(err)
	}
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	a, b := yjsEdit(t, initial, node.ID, "甲", false), yjsEdit(t, initial, node.ID, "乙", false)
	first, err := service.Submit(t.Context(), actor, doc.ID, a)
	if err != nil || first.Status != "accepted" {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := service.Submit(t.Context(), actor, doc.ID, b)
	if err != nil || second.Status != "accepted" {
		t.Fatalf("second: %+v %v", second, err)
	}
	if first.CommitSeq != 1 || second.CommitSeq != 2 {
		t.Fatal("noncontiguous commit sequence")
	}
	loaded, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(loaded.Document.Nodes[0].Config)
	if !strings.Contains(string(body), "甲") || !strings.Contains(string(body), "乙") {
		t.Fatalf("lost rich text: %s", body)
	}
	replay, err := service.Submit(t.Context(), actor, doc.ID, a)
	if err != nil || replay.CommitSeq != first.CommitSeq {
		t.Fatal("replayed edit changed durable sequence", err)
	}
	modified := a
	modified.ReviewSeq = ptr(canvas.Revision(2))
	if _, err := service.Submit(t.Context(), actor, doc.ID, modified); !errors.Is(err, canvas.ErrIdempotencyConflict) {
		t.Fatal("identity mutation accepted", err)
	}
	bad := yjsEdit(t, loaded, node.ID, "", true)
	rejected, err := service.Submit(t.Context(), actor, doc.ID, bad)
	if err != nil || rejected.Status != "rejected" {
		t.Fatalf("bad root: %+v %v", rejected, err)
	}
	after, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil || !bytes.Equal(after.State, loaded.State) || after.CommitSeq != loaded.CommitSeq {
		t.Fatal("rejection contaminated accepted state", err)
	}
	var count int64
	database.Raw(`SELECT count(*) FROM infra.outbox WHERE topic='glowloom.canvas.collaboration_committed.v1' AND partition_key=?`, doc.ID.String()).Scan(&count)
	if count != 2 {
		t.Fatalf("atomic events %d", count)
	}
	_, err = canvas.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: int64(after.Document.Revision), Operations: []domain.Operation{{Type: "set", NodeID: node.ID, Path: []string{"title"}, Value: json.RawMessage(`"legacy"`)}}})
	if err == nil {
		t.Fatal("legacy writer bypassed fence")
	}
	var receipts int64
	database.Raw(`SELECT count(*) FROM canvas.collab_receipt WHERE document_id=?`, doc.ID).Scan(&receipts)
	if receipts != 3 {
		t.Fatalf("receipt count %d", receipts)
	}
}

func TestYjsTwoActorsConcurrencyStaleReviewWriterFenceAndRuntimeReload(t *testing.T) {
	a, b, team, node := newTeamLiveFixtures(t)
	store := canvasStore(a.db)
	runtime := yjsRuntime(t)
	service := canvas.NewYjsService(store, runtime)
	initial, err := service.Initialize(t.Context(), a.actor, a.doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	left, right := yjsEdit(t, initial, node, "甲并发", false), yjsEdit(t, initial, node, "乙并发", false)
	results := make(chan error, 2)
	go func() {
		r, e := service.Submit(t.Context(), a.actor, a.doc.ID, left)
		if e == nil && r.Status != "accepted" {
			e = fmt.Errorf("left: %s", r.Code)
		}
		results <- e
	}()
	go func() {
		r, e := service.Submit(t.Context(), b.actor, a.doc.ID, right)
		if e == nil && r.Status != "accepted" {
			e = fmt.Errorf("right: %s", r.Code)
		}
		results <- e
	}()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	head, err := service.Load(t.Context(), a.actor, a.doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if head.CommitSeq != 2 {
		t.Fatal("lost concurrent commit", head.CommitSeq)
	}
	for _, name := range []string{"甲并发", "乙并发"} {
		body, _ := json.Marshal(head.Document)
		if !strings.Contains(string(body), name) {
			t.Fatal("lost actor contribution", name)
		}
	}
	stale := yjsEdit(t, head, node, "过期审阅", false)
	stale.ReviewSeq = ptr(canvas.Revision(1))
	receipt, err := service.Submit(t.Context(), a.actor, a.doc.ID, stale)
	if err != nil || receipt.Status != "rejected" || receipt.Code != "REVIEW_STALE" {
		t.Fatal(receipt, err)
	}
	repeated, err := service.Resolve(t.Context(), a.actor, a.doc.ID, stale)
	if err != nil || repeated == nil || repeated.Code != "REVIEW_STALE" {
		t.Fatal(repeated, err)
	}
	pending := yjsEdit(t, head, node, "接管", false)
	candidate, err := runtime.Prepare(t.Context(), head, pending)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.Exec(`UPDATE canvas.collab_document SET writer_term=writer_term+1 WHERE document_id=?`, a.doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitYjs(t.Context(), a.actor, head, pending, &candidate, ""); !errors.Is(err, canvas.ErrYjsContention) {
		t.Fatal("old writer committed", err)
	}
	// A new stateless runtime process loads durable binary state without room memory.
	restarted := canvas.NewYjsService(store, yjsRuntime(t))
	fresh, err := restarted.Load(t.Context(), b.actor, a.doc.ID)
	if err != nil || !bytes.Equal(head.State, fresh.State) {
		t.Fatal("runtime reload lost state", err)
	}
	accepted, err := restarted.Submit(t.Context(), a.actor, a.doc.ID, pending)
	if err != nil || accepted.Status != "accepted" || accepted.CommitSeq != 3 {
		t.Fatal(accepted, err)
	}
	resolved, err := restarted.Resolve(t.Context(), a.actor, a.doc.ID, pending)
	if err != nil || resolved == nil || resolved.CommitSeq != accepted.CommitSeq {
		t.Fatal("lost ACK did not resolve", err)
	}
	if err := a.db.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, b.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Load(t.Context(), b.actor, a.doc.ID); err == nil {
		t.Fatal("removed actor read shared state")
	}
	if _, err := restarted.Resolve(t.Context(), b.actor, a.doc.ID, right); err == nil {
		t.Fatal("removed actor resolved private receipt")
	}
}

func TestYjsReceiptFailureRollsBackProjectionBinaryLogAndOutbox(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	store := canvasStore(database)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "atomic failure"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	node.Config.Text = ptr("AB")
	_, err = canvas.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &node}}})
	if err != nil {
		t.Fatal(err)
	}
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	input := yjsEdit(t, initial, node.ID, "回滚", false)
	name := "qa_yjs_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ddl := fmt.Sprintf(`CREATE FUNCTION canvas.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.document_id='%s'::uuid THEN RAISE EXCEPTION 'isolated receipt fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON canvas.collab_receipt FOR EACH ROW EXECUTE FUNCTION canvas.%s()`, name, doc.ID, name, name)
	if err := database.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		database.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON canvas.collab_receipt; DROP FUNCTION IF EXISTS canvas.%s()`, name, name))
	}
	t.Cleanup(cleanup)
	if _, err := service.Submit(t.Context(), actor, doc.ID, input); err == nil {
		t.Fatal("injected receipt failure accepted")
	}
	current, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil || !bytes.Equal(current.State, initial.State) || current.Document.Revision != initial.Document.Revision {
		t.Fatal("partial accepted state survived", err)
	}
	var logs, events int64
	database.Raw(`SELECT count(*) FROM canvas.collab_update WHERE document_id=?`, doc.ID).Scan(&logs)
	database.Raw(`SELECT count(*) FROM infra.outbox WHERE topic='glowloom.canvas.collaboration_committed.v1' AND partition_key=?`, doc.ID.String()).Scan(&events)
	if logs != 0 || events != 0 {
		t.Fatal("partial publication survived", logs, events)
	}
	cleanup()
	receipt, err := service.Submit(t.Context(), actor, doc.ID, input)
	if err != nil || receipt.Status != "accepted" || receipt.CommitSeq != 1 {
		t.Fatal(receipt, err)
	}
}

func TestYjsConcurrentDuplicateEdgesRejectWithoutBinaryContamination(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	store := canvasStore(db)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Yjs graph conflict"})
	if err != nil {
		t.Fatal(err)
	}
	a, b := resourceNode("text"), resourceNode("text")
	a.Config.Text = ptr("A")
	b.Config.Text = ptr("B")
	_, err = canvas.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &a}, {Type: "create_node", Node: &b}}})
	if err != nil {
		t.Fatal(err)
	}
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	edit := func(edge domain.Edge) canvas.YjsUpdate {
		body, _ := json.Marshal(map[string]any{"state": initial.State, "edge": edge})
		command := exec.CommandContext(t.Context(), "node", "../../../canvas-collab/tests/service-fixture.mjs")
		command.Stdin = bytes.NewReader(body)
		output, err := command.Output()
		if err != nil {
			t.Fatal(err)
		}
		input := canvas.YjsUpdate{Epoch: initial.Epoch, SessionID: uuid.New(), UpdateID: uuid.New()}
		if err := json.Unmarshal(output, &input); err != nil {
			t.Fatal(err)
		}
		return input
	}
	left := edit(domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: a.ID, TargetNodeID: b.ID})
	right := edit(domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: a.ID, TargetNodeID: b.ID})
	first, err := service.Submit(t.Context(), actor, doc.ID, left)
	if err != nil || first.Status != "accepted" {
		t.Fatal(first, err)
	}
	before, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Submit(t.Context(), actor, doc.ID, right)
	if err != nil || second.Status != "rejected" {
		t.Fatal(second, err)
	}
	after, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil || !bytes.Equal(before.State, after.State) || len(after.Document.Edges) != 1 || after.CommitSeq != 1 {
		t.Fatal("duplicate relation polluted durable state", err)
	}
}

func TestYjsStableIdentityRestorationAuthorAndOldLifetimeFence(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	store := canvasStore(database)
	document, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	node.Config.Text = ptr("AB")
	if _, err = canvas.NewCollaborationService(store).Submit(t.Context(), actor, document.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &node}}}); err != nil {
		t.Fatal(err)
	}
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), actor, document.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	rich, err := service.Submit(t.Context(), actor, document.ID, yjsEdit(t, initial, node.ID, "持久富文本", false))
	if err != nil || rich.Status != "accepted" {
		t.Fatal(rich, err)
	}
	initial, err = service.Load(t.Context(), actor, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	late := yjsEdit(t, initial, node.ID, "late", false)
	body, _ := json.Marshal(initial)
	cmd := exec.CommandContext(t.Context(), "node", "../../../canvas-collab/tests/service-lifecycle-fixture.mjs")
	cmd.Stdin = bytes.NewReader(body)
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture %v %s", err, data)
	}
	var updates struct {
		Deletion    canvas.YjsUpdate
		Restoration canvas.YjsUpdate
	}
	if err = json.Unmarshal(data, &updates); err != nil {
		t.Fatal(err)
	}
	for _, input := range []*canvas.YjsUpdate{&updates.Deletion, &updates.Restoration} {
		input.Epoch = initial.Epoch
		input.SessionID = canvasfixture.ClientID
		input.UpdateID = uuid.New()
		receipt, err := service.Submit(t.Context(), actor, document.ID, *input)
		if err != nil || receipt.Status != "accepted" {
			t.Fatalf("lifecycle %+v %v", receipt, err)
		}
	}
	restored, err := service.Load(t.Context(), actor, document.ID)
	if err != nil || len(restored.Document.Nodes) != 1 || restored.Document.Nodes[0].ID != node.ID {
		t.Fatalf("restoration %+v %v", restored.Document, err)
	}
	if restored.Document.Nodes[0].ContentVersion <= initial.Document.Nodes[0].ContentVersion {
		t.Fatal("restored content version must advance")
	}
	rejected, err := service.Submit(t.Context(), actor, document.ID, late)
	if err != nil || rejected.Status != "rejected" || rejected.Code != "TARGET_DELETED" {
		t.Fatalf("late lifetime %+v %v", rejected, err)
	}
}

func TestYjsHistoryRestorationNewEpochReceiptAndOldUpdateFence(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	store := canvasStore(database)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "History"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	node.Config.Text = ptr("AB")
	if _, err = canvas.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &node}}}); err != nil {
		t.Fatal(err)
	}
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := service.Versions(t.Context(), actor, doc.ID, 0)
	if err != nil || len(versions) != 1 {
		t.Fatalf("initial history %v %v", versions, err)
	}
	var checkpoint struct {
		State     []byte
		CommitSeq int64
		GraphHash string
	}
	if err = database.Raw(`SELECT state,commit_seq,graph_hash FROM canvas.collab_version WHERE id=?`, versions[0].ID).Scan(&checkpoint).Error; err != nil || !bytes.Equal(checkpoint.State, initial.State) || checkpoint.CommitSeq != int64(initial.CommitSeq) || checkpoint.GraphHash != initial.GraphHash {
		t.Fatal("history lost binary checkpoint", err)
	}
	edit := yjsEdit(t, initial, node.ID, "saved", false)
	accepted, err := service.Submit(t.Context(), actor, doc.ID, edit)
	if err != nil || accepted.Status != "accepted" {
		t.Fatal(accepted, err)
	}
	input := canvas.YjsRestore{ID: uuid.New(), SessionID: canvasfixture.ClientID, VersionID: versions[0].ID, Epoch: initial.Epoch, CommitSeq: accepted.CommitSeq}
	beforeRestore, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	name := "qa_restore_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ddl := fmt.Sprintf(`CREATE FUNCTION canvas.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.document_id='%s'::uuid THEN RAISE EXCEPTION 'isolated restore receipt fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON canvas.collab_control_receipt FOR EACH ROW EXECUTE FUNCTION canvas.%s()`, name, doc.ID, name, name)
	if err = database.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		database.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON canvas.collab_control_receipt; DROP FUNCTION IF EXISTS canvas.%s()`, name, name))
	}
	t.Cleanup(cleanup)
	if _, err = service.RestoreVersion(t.Context(), actor, doc.ID, input); err == nil {
		t.Fatal("restore receipt fault accepted")
	}
	failed, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil || failed.Epoch != beforeRestore.Epoch || failed.Document.Revision != beforeRestore.Document.Revision || !bytes.Equal(failed.State, beforeRestore.State) {
		t.Fatal("partial restore survived", err)
	}
	var count int64
	if err = database.Raw(`SELECT count(*) FROM canvas.command_log WHERE document_id=? AND op_id=?`, doc.ID, input.ID.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("partial restore log", count, err)
	}
	failedVersions, err := service.Versions(t.Context(), actor, doc.ID, 0)
	if err != nil || len(failedVersions) != 2 {
		t.Fatal("partial restore history", len(failedVersions), err)
	}
	cleanup()
	restored, err := service.RestoreVersion(t.Context(), actor, doc.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Epoch == initial.Epoch || restored.Revision <= accepted.ContentRevision {
		t.Fatal("history must create a new accepted epoch/version")
	}
	replay, err := service.RestoreVersion(t.Context(), actor, doc.ID, input)
	if err != nil || replay != restored {
		t.Fatal("restore replay", replay, err)
	}
	current, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil || current.Document.Nodes[0].Config.Text == nil || *current.Document.Nodes[0].Config.Text != "AB" {
		t.Fatal("restored content", current, err)
	}
	receipt, err := service.Resolve(t.Context(), actor, doc.ID, edit)
	if err != nil || receipt == nil || receipt.CommitSeq != accepted.CommitSeq {
		t.Fatal("old receipt lost", receipt, err)
	}
	late := yjsEdit(t, initial, node.ID, "late", false)
	rejected, err := service.Submit(t.Context(), actor, doc.ID, late)
	if err != nil || rejected.Status != "rejected" || rejected.Code != "EPOCH_CHANGED" {
		t.Fatal("old epoch", rejected, err)
	}
	input.ID = uuid.New()
	if _, err = service.RestoreVersion(t.Context(), actor, doc.ID, input); !errors.Is(err, canvas.ErrYjsContention) {
		t.Fatal("stale restore", err)
	}
	versions, err = service.Versions(t.Context(), actor, doc.ID, 0)
	if err != nil || len(versions) != 3 {
		t.Fatal("retained versions", len(versions), err)
	}
}

func TestYjsConcurrentReferenceAllocationsSurviveHistoryRestore(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	store := canvasStore(database)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "References"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	node.Config.Text = ptr("AB")
	if _, err = canvas.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &node}}}); err != nil {
		t.Fatal(err)
	}
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := service.Versions(t.Context(), actor, doc.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ref-a", "ref-b"} {
		asset := seedCanvasMedia(t, database, project, "image")
		body, _ := json.Marshal(map[string]any{"state": initial.State, "nodeId": node.ID, "reference": map[string]any{"id": id, "source": map[string]any{"kind": "media", "sourceId": asset, "sourceVersion": "1"}, "bindingGeneration": 1, "mediaKind": "image", "role": "reference_image", "order": 0}})
		cmd := exec.CommandContext(t.Context(), "node", "../../../canvas-collab/tests/service-fixture.mjs")
		cmd.Stdin = bytes.NewReader(body)
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture %v %s", err, data)
		}
		update := canvas.YjsUpdate{Epoch: initial.Epoch, SessionID: uuid.New(), UpdateID: uuid.New()}
		if err = json.Unmarshal(data, &update); err != nil {
			t.Fatal(err)
		}
		receipt, err := service.Submit(t.Context(), actor, doc.ID, update)
		if err != nil || receipt.Status != "accepted" {
			t.Fatalf("reference %+v %v", receipt, err)
		}
	}
	current, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	composer := current.Document.Nodes[0].Config.Composer
	if composer == nil || len(composer.References) != 2 || composer.LabelState.Allocations["ref-a"].Ordinal != 1 || composer.LabelState.Allocations["ref-b"].Ordinal != 2 {
		t.Fatalf("concurrent allocations %+v", composer)
	}
	_, err = service.RestoreVersion(t.Context(), actor, doc.ID, canvas.YjsRestore{ID: uuid.New(), SessionID: uuid.New(), VersionID: versions[0].ID, Epoch: initial.Epoch, CommitSeq: current.CommitSeq})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	labels := restored.Document.Nodes[0].Config.Composer.LabelState
	if labels.HighWater["image"] != 2 || labels.Allocations["ref-b"].Ordinal != 2 {
		t.Fatal("history lost allocation tombstones", labels)
	}
}

func TestYjsRestorationCannotReplayAnotherActorsDeletion(t *testing.T) {
	first, second, _, sourceID := newTeamLiveFixtures(t)
	target := resourceNode("text")
	target.Config.Text = ptr("target")
	edge := domain.Edge{ID: uuid.New(), EdgeType: "annotation", SourceNodeID: sourceID, TargetNodeID: target.ID, Binding: json.RawMessage(`{"schemaVersion":1,"fromPortId":"text","toPortId":"prompt","purpose":"PROMPT","order":0,"binding":{"kind":"NODE_VALUE"}}`)}
	linked, err := canvas.NewCollaborationService(canvasStore(first.db)).Submit(t.Context(), first.actor, first.doc.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "create_node", Node: &target}, {Type: "connect", Edge: &edge}}})
	if err != nil || linked.Status != "accepted" {
		t.Fatal(linked, err)
	}
	service := canvas.NewYjsService(canvasStore(first.db), yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), first.actor, first.doc.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(initial)
	cmd := exec.CommandContext(t.Context(), "node", "../../../canvas-collab/tests/service-lifecycle-fixture.mjs")
	cmd.Stdin = bytes.NewReader(body)
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture %v %s", err, data)
	}
	var updates struct {
		Deletion    canvas.YjsUpdate
		Restoration canvas.YjsUpdate
	}
	if err = json.Unmarshal(data, &updates); err != nil {
		t.Fatal(err)
	}
	for _, input := range []*canvas.YjsUpdate{&updates.Deletion, &updates.Restoration} {
		input.Epoch = initial.Epoch
		input.SessionID = canvasfixture.ClientID
		input.UpdateID = uuid.New()
	}
	deleted, err := service.Submit(t.Context(), first.actor, first.doc.ID, updates.Deletion)
	if err != nil || deleted.Status != "accepted" {
		t.Fatal(deleted, err)
	}
	forged, err := service.Submit(t.Context(), second.actor, first.doc.ID, updates.Restoration)
	if err != nil || forged.Status != "rejected" || forged.Code != "restoration_unavailable" {
		t.Fatal("another actor restored deletion", forged, err)
	}
	current, err := service.Load(t.Context(), first.actor, first.doc.ID)
	if err != nil || len(current.Document.Nodes) != 0 || current.CommitSeq != deleted.CommitSeq {
		t.Fatal("rejected restoration contaminated state", err)
	}
	accepted, err := service.Submit(t.Context(), first.actor, first.doc.ID, updates.Restoration)
	if err != nil || accepted.Status != "accepted" {
		t.Fatal("original author restoration", accepted, err)
	}
	restoredState, err := service.Load(t.Context(), first.actor, first.doc.ID)
	if err != nil || len(restoredState.Document.Edges) != 1 || restoredState.Document.Edges[0].ID != edge.ID {
		t.Fatal("bound edge not restored", err)
	}
	versions, err := service.Versions(t.Context(), first.actor, first.doc.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(restoredState)
	cmd = exec.CommandContext(t.Context(), "node", "../../../canvas-collab/tests/service-lifecycle-fixture.mjs")
	cmd.Stdin = bytes.NewReader(body)
	data, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(data))
	}
	if err = json.Unmarshal(data, &updates); err != nil {
		t.Fatal(err)
	}
	updates.Deletion.Epoch = restoredState.Epoch
	updates.Deletion.UpdateID = uuid.New()
	deleted, err = service.Submit(t.Context(), first.actor, first.doc.ID, updates.Deletion)
	if err != nil || deleted.Status != "accepted" {
		t.Fatal(deleted, err)
	}
	history, err := service.RestoreVersion(t.Context(), first.actor, first.doc.ID, canvas.YjsRestore{ID: uuid.New(), SessionID: canvasfixture.ClientID, VersionID: versions[0].ID, Epoch: restoredState.Epoch, CommitSeq: deleted.CommitSeq})
	if err != nil || history.Epoch == restoredState.Epoch {
		t.Fatal("history failed to restore bound edge", history, err)
	}
	restoredState, err = service.Load(t.Context(), first.actor, first.doc.ID)
	if err != nil || len(restoredState.Document.Edges) != 1 || restoredState.Document.Edges[0].ID != edge.ID {
		t.Fatal("history lost edge identity", err)
	}
}

// Experimental documents had no parameters Map until their first parameter edit.
// Rebuild only through an authorized history restore, never mutate live identities.
func TestYjsExperimentalStateUpgradeAndConcurrentFirstParameters(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	store := canvasStore(db)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "旧实验中文画布"})
	if err != nil {
		t.Fatal(err)
	}
	n := resourceNode("text")
	n.Config.Text = ptr("保留中文🌏")
	if _, err = canvas.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &n}}}); err != nil {
		t.Fatal(err)
	}
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	run := func(input map[string]any) []byte {
		t.Helper()
		body, _ := json.Marshal(input)
		cmd := exec.CommandContext(t.Context(), "node", "../../../canvas-collab/tests/legacy-fixture.mjs")
		cmd.Stdin = bytes.NewReader(body)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("legacy fixture: %s %v", out, err)
		}
		return out
	}
	var legacy struct {
		State     []byte
		GraphHash string
	}
	if err = json.Unmarshal(run(map[string]any{"state": initial.State, "legacy": true}), &legacy); err != nil {
		t.Fatal(err)
	}
	// Only the isolated fixture installs an earlier persisted shape, without changing its projection.
	if err = db.Exec(`UPDATE canvas.collab_document SET state=?,graph_hash=? WHERE document_id=?`, legacy.State, legacy.GraphHash, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`UPDATE canvas.collab_version SET state=?,graph_hash=? WHERE document_id=?`, legacy.State, legacy.GraphHash, doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	old, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil || !bytes.Equal(old.State, legacy.State) {
		t.Fatal("old binary not loaded", err)
	}
	versions, err := service.Versions(t.Context(), actor, doc.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.RestoreVersion(t.Context(), actor, doc.ID, canvas.YjsRestore{ID: uuid.New(), SessionID: uuid.New(), VersionID: versions[0].ID, Epoch: old.Epoch, CommitSeq: old.CommitSeq})
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil || result.Epoch == old.Epoch || upgraded.WriterTerm <= old.WriterTerm || upgraded.Document.Nodes[0].ID != n.ID || *upgraded.Document.Nodes[0].Config.Text != "保留中文🌏" {
		t.Fatal("upgrade lost identity/content/fence", err)
	}
	edit := func(state canvas.YjsState, key string) canvas.YjsUpdate {
		t.Helper()
		input := canvas.YjsUpdate{Epoch: state.Epoch, SessionID: uuid.New(), UpdateID: uuid.New()}
		if err := json.Unmarshal(run(map[string]any{"state": state.State, "nodeId": n.ID, "key": key, "value": key}), &input); err != nil {
			t.Fatal(err)
		}
		return input
	}
	a, b := edit(upgraded, "alpha"), edit(upgraded, "beta")
	for _, input := range []canvas.YjsUpdate{a, b} {
		receipt, err := service.Submit(t.Context(), actor, doc.ID, input)
		if err != nil || receipt.Status != "accepted" {
			t.Fatal("first concurrent parameters rejected", receipt, err)
		}
	}
	current, err := service.Load(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	value, _ := json.Marshal(current.Document.Nodes[0].Config)
	if !strings.Contains(string(value), "alpha") || !strings.Contains(string(value), "beta") {
		t.Fatalf("lost concurrent parameters: %s", value)
	}
	receipt, err := service.Submit(t.Context(), actor, doc.ID, edit(old, "stale"))
	if err != nil || receipt.Code != "EPOCH_CHANGED" {
		t.Fatal("old epoch accepted", receipt, err)
	}
}
