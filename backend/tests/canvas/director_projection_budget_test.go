package canvas_test

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

// Reproduce the real imported-motion document's 221 KiB shape without storing
// a user's recovery file, identifiers or pose in the repository.
func directorMotionBudgetNode(t *testing.T) domain.Node {
	t.Helper()
	node := directorWorkspaceNode(t)
	node.Config.Director.Mode = "dynamic"
	object := &node.Config.Director.Workspaces.Dynamic.Objects[0]
	object.Kind = "actor"
	object.Primitive = ""
	object.BuiltinActor = "male_standard"
	object.AssetID = nil
	for n := 1000; n <= 2048; n += 10 {
		object.MotionLibrary = nil
		for sourceIndex := 0; sourceIndex < 2; sourceIndex++ {
			track := domain.DirectorMotionTrack{Bone: "Hip", Property: "position"}
			for i := 0; i < n; i++ {
				track.Times = append(track.Times, float64(i)/60)
				track.Values = append(track.Values, math.Sin(float64(i+sourceIndex))*.01, math.Cos(float64(i+sourceIndex))*.01, math.Sin(float64(i+sourceIndex)*.3)*.01)
			}
			source := domain.DirectorMotionSource{ID: uuid.New(), Name: "Imported motion", Kind: "import", Duration: 60, Tracks: []domain.DirectorMotionTrack{track}, Rest: map[string]domain.DirectorTransform{"Hip": {Position: []float64{0, 0, 0}, Rotation: []float64{0, 0, 0}, Scale: []float64{1, 1, 1}}}}
			object.MotionLibrary = append(object.MotionLibrary, source)
		}
		object.MotionClips = []domain.DirectorMotionClip{{ID: uuid.New(), Name: "First motion", SourceAnimation: "user:" + object.MotionLibrary[0].ID.String(), Start: 0, Duration: 1, PlaybackRate: 1}}
		raw, err := json.Marshal(node.Config.Director)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) >= 220000 && len(raw) < 230000 {
			t.Logf("director JSON bytes: %d", len(raw))
			return node
		}
	}
	t.Fatal("could not construct representative bounded imported motion")
	return node
}

func TestDirectorImportedMotionYjsProjectionBudgetAndRewind(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	writer := canvasfixture.NewEditService(canvasStore(runtime))
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Imported director audit budget"})
	if err != nil {
		t.Fatal(err)
	}
	node := directorMotionBudgetNode(t)
	before, err := writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node})})
	if err != nil {
		t.Fatal("seed", err)
	}
	raw, _ := json.Marshal(before.Nodes[0].Config.Director)
	var next domain.DirectorConfig
	if err = json.Unmarshal(raw, &next); err != nil {
		t.Fatal(err)
	}
	object := &next.Workspaces.Dynamic.Objects[0]
	object.MotionClips = append(object.MotionClips, domain.DirectorMotionClip{ID: uuid.New(), Name: "Second motion", SourceAnimation: "user:" + object.MotionLibrary[1].ID.String(), Start: 1, Duration: 1, PlaybackRate: 1})
	expected, _ := json.Marshal(map[string]any{"director": before.Nodes[0].Config.Director})
	value, _ := json.Marshal(map[string]any{"director": next})
	operations := []domain.Operation{{Type: "set", NodeID: node.ID, Path: []string{"config", "director"}, ExpectedExists: ptr(true), Expected: expected, Value: value}}
	outcome, err := domain.ApplyOperations(before, operations)
	if err != nil {
		t.Fatal(err)
	}
	audit, _ := json.Marshal(map[string]any{"operations": operations, "effects": domain.DocumentEffects(before, outcome.Document), "touches": outcome.Changes})
	if len(audit) <= 1<<20 {
		t.Fatalf("regression does not cross old audit budget: %d", len(audit))
	}
	t.Logf("compact audit bytes before server normalization: %d", len(audit))
	after, err := writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: operations})
	if err != nil {
		t.Fatal("valid 221 KiB director rejected by amplified audit", err)
	}
	reopened, err := application.NewService(canvasStore(runtime)).Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Revision != after.Revision || len(reopened.Nodes[0].Config.Director.Workspaces.Dynamic.Objects[0].MotionClips) != 2 {
		t.Fatal("accepted motion was not durably reopened")
	}
	var stored struct {
		Commands []byte
		Bytes    int
	}
	if err = owner.Raw(`SELECT commands,octet_length(commands::text) AS bytes FROM canvas.command_log WHERE document_id=? AND revision=?`, doc.ID, after.Revision).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	t.Logf("persisted JSONB audit bytes: %d", stored.Bytes)
	if stored.Bytes <= 1<<20 || stored.Bytes > 4<<20 {
		t.Fatal("unexpected retained audit size", stored.Bytes)
	}
	var log struct {
		Effects    []domain.Effect      `json:"effects"`
		Operations []domain.Operation   `json:"operations"`
		Touches    []domain.FieldChange `json:"touches"`
	}
	if err = json.Unmarshal(stored.Commands, &log); err != nil {
		t.Fatal(err)
	}
	if len(log.Operations) == 0 || len(log.Touches) == 0 {
		t.Fatal("audit semantics were discarded")
	}
	rewound, err := domain.RewindEffects(after, log.Effects)
	if err != nil {
		partial := after
		for i := len(log.Effects) - 1; i >= 0; i-- {
			var stepErr error
			partial, stepErr = domain.RewindEffects(partial, log.Effects[i:i+1])
			if stepErr != nil {
				t.Fatalf("stored audit cannot rewind path %v: %v", log.Effects[i].Path, stepErr)
			}
		}
		t.Fatal("stored audit cannot rewind", err)
	}
	if !reflect.DeepEqual(rewound.Nodes, before.Nodes) {
		t.Fatal("stored audit lost preimage")
	}
	if found, err := canvasReferenceCheck(t, runtime, actor, project, uuid.New()); err != nil || found {
		t.Fatal("accepted audit breaks retained reference read", found, err)
	}
}

// The larger per-record allowance remains bounded; reference checks never turn
// oversized retained history into a false absence result.
func TestDirectorRetainedAuditBudgetRejectsOversize(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	doc, err := application.NewService(canvasStore(owner)).Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Retained director limit"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"name": strings.Repeat("x", 4<<20)})
	if err != nil {
		t.Fatal(err)
	}
	tx := owner.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error(err)
		}
	}()
	if err = tx.Exec(`INSERT INTO canvas.command_log(id,document_id,revision,commands) VALUES(?,?,2,?::jsonb)`, uuid.New(), doc.ID, string(body)).Error; err != nil {
		t.Fatal(err)
	}
	found, err := canvasReferenceCheck(t, tx, actor, project, uuid.New())
	if err == nil || found || !strings.Contains(err.Error(), "exceeds reference read budget") {
		t.Fatal("oversized retained audit became absent", found, err)
	}
}

func TestDirectorOversizedAtomicAuditRejectsWithoutPartialSave(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	writer := canvasfixture.NewEditService(canvasStore(runtime))
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Atomic director limit"})
	if err != nil {
		t.Fatal(err)
	}
	var nodes []domain.Node
	for range 4 {
		nodes = append(nodes, directorMotionBudgetNode(t))
	}
	before, err := writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes(nodes)})
	if err != nil {
		t.Fatal("seed", err)
	}
	var operations []domain.Operation
	for _, node := range before.Nodes {
		data, _ := json.Marshal(node.Config.Director)
		var next domain.DirectorConfig
		if err = json.Unmarshal(data, &next); err != nil {
			t.Fatal(err)
		}
		object := &next.Workspaces.Dynamic.Objects[0]
		object.MotionClips = append(object.MotionClips, domain.DirectorMotionClip{ID: uuid.New(), Name: "Second motion", SourceAnimation: "user:" + object.MotionLibrary[1].ID.String(), Start: 1, Duration: 1, PlaybackRate: 1})
		expected, _ := json.Marshal(map[string]any{"director": node.Config.Director})
		value, _ := json.Marshal(map[string]any{"director": next})
		operations = append(operations, domain.Operation{Type: "set", NodeID: node.ID, Path: []string{"config", "director"}, ExpectedExists: ptr(true), Expected: expected, Value: value})
	}
	var logsBefore int64
	if err = owner.Raw(`SELECT count(*) FROM canvas.command_log WHERE document_id=?`, doc.ID).Scan(&logsBefore).Error; err != nil {
		t.Fatal(err)
	}
	receipt, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: operations})
	if err != nil || receipt.Status != "rejected" || receipt.Code != "operation_too_large" {
		t.Fatal("oversized atomic audit not rejected at persistence boundary", receipt, err)
	}
	after, err := writer.Get(t.Context(), actor, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || !reflect.DeepEqual(after.Nodes, before.Nodes) {
		t.Fatal("rejected audit partially changed document")
	}
	var logsAfter int64
	if err = owner.Raw(`SELECT count(*) FROM canvas.command_log WHERE document_id=?`, doc.ID).Scan(&logsAfter).Error; err != nil || logsAfter != logsBefore {
		t.Fatal("rejected audit leaked command history", logsBefore, logsAfter, err)
	}
}
