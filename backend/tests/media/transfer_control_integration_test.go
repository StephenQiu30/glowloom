package media_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestMediaTransferQueuedCancellationIsPermanentAndNeverPublishesText(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, project := mediaStoreProject(t, db)
	source := domain.LibraryScope{Kind: domain.LibraryPersonal}
	target := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	text := "真实纯文本，不包含对象"
	created, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: source, Action: "create_text", Key: uuid.New(), Metadata: &mediaapp.LibraryMetadata{Title: "等待转移", Category: "other", PlainText: &text}})
	if err != nil || len(created.Items) != 1 {
		t.Fatal(err)
	}
	repo := pgmedia.NewTransferStore(db, libraryTestAccess, transferTestGuards, time.Now)
	job, err := repo.CreateTransfer(t.Context(), actor, mediaapp.TransferInput{Source: source, Target: target, Key: uuid.New(), ExpectedSourceRevision: 1, ExpectedProjectRevision: 1, Items: []mediaapp.LibraryItemRevision{{ID: created.Items[0].ID, Revision: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	cancelled, err := repo.ControlTransfer(t.Context(), actor, job.ID, key, job.Revision, "cancel")
	if err != nil || cancelled.Status != "cancelled" || cancelled.Items[0].Status != "cancelled" || cancelled.ExecutionUnconfirmed || cancelled.NeedsReconciliation {
		t.Fatal("queued SQL-only cancellation fabricated physical work", cancelled, err)
	}
	replay, err := repo.ControlTransfer(t.Context(), actor, job.ID, key, job.Revision, "cancel")
	if err != nil || !reflect.DeepEqual(replay, cancelled) {
		t.Fatal("permanent cancellation replay changed", replay, err)
	}
	if _, err := repo.ControlTransfer(t.Context(), actor, job.ID, key, job.Revision+1, "cancel"); !errors.Is(err, mediaapp.ErrLibraryKeyConflict) {
		t.Fatal("same cancellation key accepted another input", err)
	}
	page, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).ListLibrary(t.Context(), actor, target, libraryQuery())
	if err != nil || page.Total != 0 {
		t.Fatal("cancelled unpublished text leaked into target", page, err)
	}
	if _, err := repo.ControlTransfer(t.Context(), actor, job.ID, uuid.New(), cancelled.Revision, "retry"); !errors.Is(err, domain.ErrTransferConflict) {
		t.Fatal("cancelled intent silently restarted", err)
	}
	foreign, _ := mediaStoreProject(t, db)
	if _, err := repo.ControlTransfer(t.Context(), foreign, job.ID, key, job.Revision, "cancel"); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("foreign actor replayed creator's receipt", err)
	}
}
