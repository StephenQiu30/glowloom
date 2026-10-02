package media_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type purgePreviewSigner struct{ calls int }

func (s *purgePreviewSigner) PresignGet(context.Context, string, time.Duration) (string, error) {
	s.calls++
	return "https://example.invalid/new-preview", nil
}

func TestMediaGenericReferenceRejectsCatalogTrashRemovalAndCancelledPurge(t *testing.T) {
	for _, phase := range []string{"trashed", "removed", "cancelled"} {
		t.Run(phase, func(t *testing.T) {
			db := libraryRuntimeDB(t)
			actor, project, input, _, _ := purgeBinaryFixture(t, db)
			library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
			if phase == "removed" {
				restored, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: input.Scope, Key: uuid.New(), Action: "restore_items", ExpectedRevision: 2, Items: input.Items})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: input.Scope, Key: uuid.New(), Action: "remove_items", ExpectedRevision: restored.Revision, Items: []mediaapp.LibraryItemRevision{{ID: input.Items[0].ID, Revision: 3}}}); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "cancelled" {
				repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
				job, err := repo.CreatePurge(t.Context(), actor, input)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := repo.ControlPurge(t.Context(), actor, job.ID, uuid.New(), job.Revision, "cancel"); err != nil {
					t.Fatal(err)
				}
			}
			var deleted bool
			if err := db.Raw(`SELECT is_delete FROM media.media_asset WHERE id=?`, input.Items[0].ID).Scan(&deleted).Error; err != nil || deleted {
				t.Fatal("catalog-only hiding must retain the reviewed original", deleted, err)
			}
			store := pgmedia.NewStore(db)
			if _, err := store.FindAsset(t.Context(), actor, project, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatal("hidden retained original formed a new ordinary binding", err)
			}
			if _, err := store.FindRenditions(t.Context(), actor, project, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatal("hidden retained original exposed ordinary renditions", err)
			}
			signer := &purgePreviewSigner{}
			query := mediaapp.NewAssetQuery(store, signer)
			if _, err := query.Reference(t.Context(), actor, project, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatal("hidden retained original formed a new canvas/generation/cover binding", err)
			}
			if _, err := query.Preview(t.Context(), actor, project, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) || signer.calls != 0 {
				t.Fatal("ordinary preview leased hidden retained bytes", err, signer.calls)
			}
			page, err := query.List(t.Context(), actor, project, "image", "", 20)
			if err != nil || len(page.Items) != 0 {
				t.Fatal("formal picker offered catalog-hidden media", page, err)
			}
			if phase != "removed" {
				if _, err := library.FindLibraryMedia(t.Context(), actor, input.Scope, input.Items[0].ID); err != nil {
					t.Fatal("trash viewer lost eligible original after cancel", err)
				}
				detail, err := library.LibraryDetail(t.Context(), actor, input.Scope, input.Items[0].ID)
				if err != nil {
					t.Fatal(err)
				}
				p, err := library.ListLibrary(t.Context(), actor, input.Scope, libraryQuery())
				if err != nil {
					t.Fatal(err)
				}
				if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: input.Scope, Key: uuid.New(), Action: "restore_items", ExpectedRevision: p.Revision, Items: []mediaapp.LibraryItemRevision{{ID: detail.ID, Revision: detail.Revision}}}); err != nil {
					t.Fatal(err)
				}
				if _, err := query.Reference(t.Context(), actor, project, input.Items[0].ID); err != nil {
					t.Fatal("actual restore did not release the new-binding gate", err)
				}
			}
		})
	}
}

func TestMediaDocumentNewFreezeRejectsHiddenCatalogButKeepsFrozenOriginal(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	body := []byte("只能由既有不可变剧本证据读回的原文")
	upload := documentUpload(t, db, objects, actor, project, "frozen-document.txt", body)
	reader := mediaapp.NewDocumentSources(pgmedia.NewDocumentSourceStore(db), objects)
	frozen, err := reader.Freeze(t.Context(), actor, project, []uuid.UUID{upload.Asset.ID})
	if err != nil {
		t.Fatal(err)
	}
	library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	for i, action := range []string{"recycle_items", "restore_items", "remove_items"} {
		if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: action, ExpectedRevision: int64(i), Items: []mediaapp.LibraryItemRevision{{ID: upload.Asset.ID, Revision: int64(i)}}}); err != nil {
			t.Fatal(action, err)
		}
		original, err := reader.Open(t.Context(), actor, project, frozen[0])
		if err != nil {
			t.Fatal("catalog-only state invalidated frozen source bytes", action, err)
		}
		actual, readErr := io.ReadAll(original.File)
		closeErr := original.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(actual, body) {
			t.Fatal("frozen original changed", readErr, closeErr)
		}
		_, err = reader.Freeze(t.Context(), actor, project, []uuid.UUID{upload.Asset.ID})
		if action == "restore_items" {
			if err != nil {
				t.Fatal("restored source cannot form a new admission", err)
			}
		} else if !errors.Is(err, mediaapp.ErrNotFound) {
			t.Fatal("hidden original formed a new script admission", action, err)
		}
	}
}

func TestMediaPurgeReservationRejectsInconsistentCatalogAndOriginalFlags(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	actor, project, input, _, _ := purgeBinaryFixture(t, db)
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	if _, err := repo.CreatePurge(t.Context(), actor, input); err != nil {
		t.Fatal(err)
	}
	// This owner-only synthetic fault proves that reservation is independently
	// authoritative. Normal admission sets is_delete and does not make this state.
	if err := owner.Exec(`UPDATE media.media_asset SET is_delete=false,delete_time=NULL,purge_after=NULL WHERE id=?`, input.Items[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE media.library_item SET catalog_state='active',trashed_at=NULL WHERE id=?`, input.Items[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := pgmedia.NewStore(db).FindAsset(t.Context(), actor, project, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("physical reservation was replaced by mutable visibility flags", err)
	}
}

func TestMediaPurgeGenericReferencePreviewAndRenditionsRejectReservedOriginals(t *testing.T) {
	for _, phase := range []string{"queued", "unknown"} {
		t.Run(phase, func(t *testing.T) {
			db := libraryRuntimeDB(t)
			actor, project, input, _, objects := purgeBinaryFixture(t, db)
			library := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
			if _, err := library.FindLibraryMedia(t.Context(), actor, input.Scope, input.Items[0].ID); err != nil {
				t.Fatal("trash preview before physical admission must remain available", err)
			}
			repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
			job, err := repo.CreatePurge(t.Context(), actor, input)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "unknown" {
				unknown := &purgeUnknownRemove{PurgeObjects: objects, once: true}
				job, err = mediaapp.NewPurgeWorker(repo, unknown).Execute(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
				if err != nil || !job.NeedsReconciliation || job.Status != "needs_reconciliation" {
					t.Fatal("actual unknown removal did not retain admission", job, err)
				}
			}
			base := pgmedia.NewStore(db)
			if _, err := base.FindAsset(t.Context(), actor, project, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatal("accepted physical purge still allowed ordinary new reference", err)
			}
			if _, err := base.FindRenditions(t.Context(), actor, project, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatal("new rendition read bypassed reserved parent", err)
			}
			signer := &purgePreviewSigner{}
			query := mediaapp.NewAssetQuery(base, signer)
			if _, err := query.Reference(t.Context(), actor, project, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatal("new canvas/generation/cover reference bypassed physical reservation", err)
			}
			if _, err := query.Preview(t.Context(), actor, project, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) || signer.calls != 0 {
				t.Fatal("reserved bytes acquired a new lease", err, signer.calls)
			}
			page, err := query.List(t.Context(), actor, project, "image", "", 20)
			if err != nil || len(page.Items) != 0 {
				t.Fatal("formal picker offered reserved original", page, err)
			}
			if _, err := library.FindLibraryMedia(t.Context(), actor, input.Scope, input.Items[0].ID); !errors.Is(err, mediaapp.ErrNotFound) {
				t.Fatal("library trash preview formed a new lease during purge", err)
			}
		})
	}
}
