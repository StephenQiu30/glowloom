package canvas_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

func TestDirectorRecoveryPrivateCASAndReopen(t *testing.T) {
	owner := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, owner)
	runtime := canvasDB(t, "GLOWLOOM_TEST_CANVAS_M1_RUNTIME_DSN")
	writer := canvasfixture.NewEditService(canvasStore(runtime))
	doc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "恢复稿"})
	if err != nil {
		t.Fatal(err)
	}
	node := directorWorkspaceNode(t)
	if _, err = writer.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{Operations: canvasfixture.Nodes([]domain.Node{node})}); err != nil {
		t.Fatal(err)
	}
	service := application.NewService(canvasStore(runtime))
	input := application.SaveDirectorRecoveryInput{Epoch: uuid.New(), BaseFingerprint: strings.Repeat("a", 64), Director: *node.Config.Director}
	first, err := service.SaveDirectorRecovery(t.Context(), actor, doc.ID, node.ID, input)
	if err != nil || first.Revision < 1 {
		t.Fatal(first, err)
	}
	read, err := application.NewService(canvasStore(runtime)).ReadDirectorRecovery(t.Context(), actor, doc.ID, node.ID)
	if err != nil || read.Draft == nil || read.Draft.Director.ID != node.Config.Director.ID {
		t.Fatal(read, err)
	}
	var conflict *application.RevisionConflict
	if _, err = service.SaveDirectorRecovery(t.Context(), actor, doc.ID, node.ID, input); !errors.As(err, &conflict) {
		t.Fatal("stale writer accepted", err)
	}
	input.ExpectedRevision = first.Revision
	input.Director.Title = "恢复稿第二版"
	second, err := service.SaveDirectorRecovery(t.Context(), actor, doc.ID, node.ID, input)
	if err != nil || second.Revision <= first.Revision {
		t.Fatal(second, err)
	}
	if err = service.DeleteDirectorRecovery(t.Context(), actor, doc.ID, node.ID, application.DeleteDirectorRecoveryInput{ExpectedRevision: first.Revision}); !errors.As(err, &conflict) {
		t.Fatal("stale delete accepted", err)
	}
	other, otherProject := seedCanvasActor(t, owner)
	if _, err = service.ReadDirectorRecovery(t.Context(), other, doc.ID, node.ID); err == nil {
		t.Fatal("cross account access accepted")
	}
	otherDoc, err := writer.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "另一个画布"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveDirectorRecovery(t.Context(), actor, otherDoc.ID, node.ID, input); err == nil {
		t.Fatal("cross canvas node accepted")
	}
	_ = otherProject
	input.ExpectedRevision = second.Revision
	input.Director.Workspaces.Dynamic = nil
	if _, err = service.SaveDirectorRecovery(t.Context(), actor, doc.ID, node.ID, input); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatal("malformed inactive workspace accepted", err)
	}
	if err = service.DeleteDirectorRecovery(t.Context(), actor, doc.ID, node.ID, application.DeleteDirectorRecoveryInput{ExpectedRevision: second.Revision}); err != nil {
		t.Fatal(err)
	}
	read, err = service.ReadDirectorRecovery(t.Context(), actor, doc.ID, node.ID)
	if err != nil || read.Draft != nil {
		t.Fatal(read, err)
	}
	input.ExpectedRevision = 0
	input.Director = *directorWorkspaceNode(t).Config.Director
	recreated, err := service.SaveDirectorRecovery(t.Context(), actor, doc.ID, node.ID, input)
	if err != nil || recreated.Revision <= second.Revision {
		t.Fatal("revision reused after recreation", recreated, err)
	}
	if err = service.DeleteDirectorRecovery(t.Context(), actor, doc.ID, node.ID, application.DeleteDirectorRecoveryInput{ExpectedRevision: second.Revision}); !errors.As(err, &conflict) {
		t.Fatal("ABA stale delete accepted", err)
	}
	input.ExpectedRevision = second.Revision
	if _, err = service.SaveDirectorRecovery(t.Context(), actor, doc.ID, node.ID, input); !errors.As(err, &conflict) {
		t.Fatal("ABA stale save accepted", err)
	}
}
