package canvas_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestYjsSharedCheckpointsPreserveHistoryAndBoundCausalGrowth(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	store := canvasStore(database)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Shared checkpoints"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	_, err = canvasfixture.NewLegacySeeder(database, store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node})})
	if err != nil {
		t.Fatal(err)
	}
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts := func(checkpoints, versions int64) {
		t.Helper()
		var counts struct{ Checkpoints, Versions, Dangling int64 }
		err := database.Raw(`SELECT (SELECT count(*) FROM canvas.collab_checkpoint WHERE document_id=?) checkpoints,
(SELECT count(*) FROM canvas.collab_version WHERE document_id=?) versions,
(SELECT count(*) FROM canvas.collab_checkpoint c WHERE c.document_id=? AND NOT EXISTS(SELECT 1 FROM canvas.collab_document d WHERE d.document_id=c.document_id AND d.state_id=c.id) AND NOT EXISTS(SELECT 1 FROM canvas.collab_version v WHERE v.document_id=c.document_id AND v.state_id=c.id)) dangling`, doc.ID, doc.ID, doc.ID).Scan(&counts).Error
		if err != nil || counts.Checkpoints != checkpoints || counts.Versions != versions || counts.Dangling != 0 {
			t.Fatalf("checkpoint retention: %+v %v", counts, err)
		}
	}
	assertCounts(1, 1)
	body, err := json.Marshal(map[string]any{"state": initial.State, "nodeId": node.ID, "modes": []string{"causal", "causal", "text", "text"}})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "node", "../../../canvas/tests/sync-performance-fixture.mjs")
	command.Stdin = bytes.NewReader(body)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture: %v: %s", err, output)
	}
	var updates []canvas.YjsUpdate
	if err = json.Unmarshal(output, &updates); err != nil {
		t.Fatal(err)
	}
	if len(updates) != 4 {
		t.Fatalf("expected four updates: %s", output)
	}
	for i := range updates {
		updates[i].Epoch, updates[i].SessionID, updates[i].UpdateID = initial.Epoch, uuid.New(), uuid.New()
		receipt, err := service.Submit(t.Context(), actor, doc.ID, updates[i])
		if err != nil || receipt.Status != "accepted" || receipt.CommitSeq != canvas.Revision(i+1) {
			t.Fatalf("submit: %+v %v", receipt, err)
		}
		if i < 2 {
			if receipt.Effect != "CAUSAL_CHANGE" {
				t.Fatalf("expected causal change: %+v", receipt)
			}
			assertCounts(2, 1)
		} else {
			assertCounts(int64(i), int64(i))
		}
	}
	// A different identity carrying already integrated bytes is accepted NO_CHANGE.
	noop := updates[3]
	noop.UpdateID = uuid.New()
	receipt, err := service.Submit(t.Context(), actor, doc.ID, noop)
	if err != nil || receipt.Effect != "NO_CHANGE" || receipt.CommitSeq != 4 {
		t.Fatalf("no-change: %+v %v", receipt, err)
	}
	assertCounts(3, 3)
	var historical struct{ State []byte }
	if err = database.Raw(`SELECT c.state FROM canvas.collab_version v JOIN canvas.collab_checkpoint c ON c.document_id=v.document_id AND c.id=v.state_id WHERE v.document_id=? AND v.revision=?`, doc.ID, initial.Document.Revision).Scan(&historical).Error; err != nil || !bytes.Equal(historical.State, initial.State) {
		t.Fatal("historical binary changed", err)
	}
	var same bool
	if err = database.Raw(`SELECT d.state_id=v.state_id FROM canvas.collab_document d JOIN canvas.collab_version v ON v.document_id=d.document_id AND v.revision=d.content_revision WHERE d.document_id=?`, doc.ID).Scan(&same).Error; err != nil || !same {
		t.Fatal("head and history do not share state", err)
	}
	for _, test := range []struct {
		name, statement, code string
	}{
		{"immutable", `UPDATE canvas.collab_checkpoint SET state=state WHERE document_id=?`, "42501"},
		{"referenced", `DELETE FROM canvas.collab_checkpoint WHERE document_id=?`, "23503"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := database.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
					return err
				}
				return tx.Exec(test.statement, doc.ID).Error
			})
			var pgError *pgconn.PgError
			if !errors.As(err, &pgError) || pgError.Code != test.code {
				t.Fatalf("checkpoint %s: %v", test.name, err)
			}
		})
	}
	assertCounts(3, 3)
}
