package subject_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	canvaspg "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	subjectapp "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestCanvasSubjectNodePostgresFixedVersionAndRevocation(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	project := subjectProject(t, owner, actor)
	primary := subjectFixtureMedia(t, owner, actor, "image")
	if err := owner.Exec(`UPDATE media.media_asset SET object_key=? WHERE id=?`, "personal/"+actor.OrgID.String()+"/"+actor.ID.String()+"/image/2026/10/"+primary.AssetID.String()+".png", primary.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	media := &memoryMedia{facts: map[uuid.UUID]domain.MediaFact{primary.AssetID: primary}}
	subjects := subjectapp.NewService(subjectFixtureStore(runtime, media), time.Now)
	saved, err := subjects.Save(t.Context(), actor, uuid.Nil, uuid.New(), subjectapp.SaveInput{Name: "固定主体", Primary: subjectapp.MediaSelection{AssetID: primary.AssetID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Transaction(func(tx *gorm.DB) error {
		return canvasapp.ValidateSubjectNodes(t.Context(), actor, project, []canvasdomain.Node{{Config: canvasdomain.NodeConfig{Subject: &canvasdomain.SubjectConfig{Version: 1, SubjectID: &saved.ID, VersionID: &saved.VersionID}}}}, subjectwire.NewPromptSources(tx))
	}); err != nil {
		t.Fatal("source owner control", err)
	}
	store := canvaspg.NewScopedStore(runtime, nil, mediawire.NewCanvasProjectAccess).WithSubjectSources(subjectwire.NewPromptSources)
	writer := canvasfixture.NewEditService(store)
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), canvasapp.CreateInput{Name: "固定主体画布"})
	if err != nil {
		t.Fatal(err)
	}
	node := canvasdomain.Node{ID: uuid.New(), Title: "主体", NodeType: "subject", NodeAction: "resource", Config: canvasdomain.NodeConfig{Subject: &canvasdomain.SubjectConfig{Version: 1}}}
	apply := func(operations ...canvasdomain.Operation) canvasfixture.EditInput {
		return canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, Operations: operations}
	}
	if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), apply(canvasdomain.Operation{Type: "create_node", Node: &node})); err != nil || r.Status != "accepted" {
		t.Fatal("empty subject save", r, err)
	}
	empty := json.RawMessage(`{"subject":{"version":1}}`)
	pin, _ := json.Marshal(map[string]any{"subject": canvasdomain.SubjectConfig{Version: 1, SubjectID: &saved.ID, VersionID: &saved.VersionID}})
	exists := true
	update := canvasdomain.Operation{Type: "set", NodeID: node.ID, Path: []string{"config", "subject"}, ExpectedExists: &exists, Expected: empty, Value: pin}
	if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), apply(update)); err != nil || r.Status != "accepted" {
		t.Fatal("pin subject", r, err)
	}
	read, err := canvasapp.NewService(canvaspg.NewScopedStore(runtime, nil, mediawire.NewCanvasProjectAccess).WithSubjectSources(subjectwire.NewPromptSources)).Get(t.Context(), actor, doc.ID)
	if err != nil || len(read.Nodes) != 1 || read.Nodes[0].Config.Subject == nil || *read.Nodes[0].Config.Subject.VersionID != saved.VersionID {
		t.Fatal("reopen lost pin", read, err)
	}
	// A duplicate retains the same fixed identity, subject to the same owning proof.
	duplicate := read.Nodes[0]
	duplicate.ID = uuid.New()
	duplicate.ContentVersion = 0
	duplicate.InputRevision = 0
	if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), apply(canvasdomain.Operation{Type: "create_node", Node: &duplicate})); err != nil || r.Status != "accepted" {
		t.Fatal("duplicate", r, err)
	}
	// Reject a mixed batch atomically when its second subject version does not exist.
	wrong := duplicate
	wrong.ID = uuid.New()
	bad := uuid.New()
	wrong.Config.Subject = &canvasdomain.SubjectConfig{Version: 1, SubjectID: &saved.ID, VersionID: &bad}
	move := canvasdomain.Operation{Type: "set", NodeID: node.ID, Path: []string{"position"}, Value: json.RawMessage(`{"x":42,"y":0}`)}
	if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), apply(move, canvasdomain.Operation{Type: "create_node", Node: &wrong})); err != nil || r.Status != "rejected" {
		t.Fatal("invalid version batch", r, err)
	}
	current, err := writer.Get(t.Context(), actor, doc.ID)
	if err != nil || len(current.Nodes) != 2 {
		t.Fatal("batch leaked", current, err)
	}
	for _, n := range current.Nodes {
		if n.X == 42 {
			t.Fatal("failed batch leaked movement")
		}
	}
	// Advancing the library head must never retarget the canvas pin.
	newHead, err := subjects.Save(t.Context(), actor, saved.ID, uuid.New(), subjectapp.SaveInput{ExpectedRevision: saved.Revision, Name: "新版主体", Primary: subjectapp.MediaSelection{AssetID: primary.AssetID}})
	if err != nil || newHead.VersionID == saved.VersionID {
		t.Fatal("advance head", newHead, err)
	}
	if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), apply(move)); err != nil || r.Status != "accepted" {
		t.Fatal("fixed old version rejected", r, err)
	}
	fixed, err := writer.Get(t.Context(), actor, doc.ID)
	if err != nil || *fixed.Nodes[0].Config.Subject.VersionID != saved.VersionID {
		t.Fatal("head silently replaced fixed version", err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET status='rejected', moderation_status='rejected' WHERE id=?`, primary.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	move.Value = json.RawMessage(`{"x":43,"y":0}`)
	if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), apply(move)); err != nil || r.Status != "rejected" {
		t.Fatal("revoked media binding accepted", r, err)
	}
	if err := owner.Exec(`UPDATE media.media_asset SET status='ready', moderation_status='passed' WHERE id=?`, primary.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	// Permission loss must reject even an ordinary layout edit with an unchanged pin.
	if err := owner.Exec(`UPDATE subject.subject_head SET deleted_at=clock_timestamp() WHERE id=?`, saved.ID).Error; err != nil {
		t.Fatal(err)
	}
	if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), apply(move)); err != nil || r.Status != "rejected" {
		t.Fatal("unchanged deleted pin accepted", r, err)
	}
	// Clearing all invalid pins is a valid recovery, not a silent library deletion.
	clearPin := update
	clearPin.Expected = pin
	clearPin.Value = empty
	clearOther := clearPin
	clearOther.NodeID = duplicate.ID
	if r, err := writer.Submit(t.Context(), actor, doc.ID, uuid.NewString(), apply(clearPin, clearOther)); err != nil || r.Status != "accepted" {
		t.Fatal("clear invalid pins", r, err)
	}
}
