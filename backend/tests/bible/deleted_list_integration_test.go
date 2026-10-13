package bible_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
)

func TestDeletedSubjectsRemainRecoverableAfterReload(t *testing.T) {
	db, owner := deletionDatabase(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(deletionStore(db), time.Now)
	receipt, err := service.Change(t.Context(), actor, createCharacter(project))
	if err != nil {
		t.Fatal(err)
	}
	command := app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "delete", EntryID: receipt.EntryID, ExpectedRevision: receipt.Revision, Key: uuid.New(), RequestID: uuid.New()}
	_, err = service.Change(t.Context(), actor, command)
	var impact *app.ImpactConflict
	if !errors.As(err, &impact) {
		t.Fatalf("real deletion must return owning impact proof: %v", err)
	}
	command.AcknowledgedImpact = &impact.Proof
	receipt, err = service.Change(t.Context(), actor, command)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := app.NewService(deletionStore(db), time.Now)
	active, err := reloaded.List(t.Context(), actor, app.ListInput{ProjectID: project, Kind: domain.KindCharacter})
	if err != nil || len(active.Entries) != 0 {
		t.Fatalf("active contains deleted: %v", err)
	}
	trash, err := reloaded.List(t.Context(), actor, app.ListInput{ProjectID: project, Kind: domain.KindCharacter, Deleted: true})
	if err != nil || len(trash.Entries) != 1 || trash.Entries[0].Head.ID != receipt.EntryID {
		t.Fatalf("deleted entry lost: %+v %v", trash, err)
	}
	_, err = reloaded.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "restore", EntryID: receipt.EntryID, ExpectedRevision: receipt.Revision, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	trash, err = reloaded.List(t.Context(), actor, app.ListInput{ProjectID: project, Kind: domain.KindCharacter, Deleted: true})
	if err != nil || len(trash.Entries) != 0 {
		t.Fatalf("restored entry remains deleted: %v", err)
	}
}
