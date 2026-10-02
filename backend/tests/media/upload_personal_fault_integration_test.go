package media_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type pendingPersonalCommit struct {
	*pgmedia.Store
	db                               *gorm.DB
	ctx                              context.Context
	pending, release, checking, done chan struct{}
	finished                         chan error
	keys                             []string
}

func (r *pendingPersonalCommit) CommitPersonalUpload(_ context.Context, actor identityapp.Principal, request mediaapp.UploadRequest, a domain.MediaAsset, rends []domain.Rendition) (mediaapp.PersonalUploadResult, error) {
	r.keys = []string{a.ObjectKey}
	for _, rend := range rends {
		r.keys = append(r.keys, rend.ObjectKey)
	}
	go func() {
		defer close(r.done)
		err := r.db.WithContext(r.ctx).Transaction(func(tx *gorm.DB) error {
			if _, err := pgmedia.NewStore(tx).CommitPersonalUpload(r.ctx, actor, request, a, rends); err != nil {
				return err
			}
			close(r.pending)
			select {
			case <-r.release:
				return nil
			case <-r.ctx.Done():
				return r.ctx.Err()
			}
		})
		r.finished <- err
	}()
	select {
	case <-r.pending:
		return mediaapp.PersonalUploadResult{}, context.Canceled
	case err := <-r.finished:
		return mediaapp.PersonalUploadResult{}, err
	case <-r.ctx.Done():
		return mediaapp.PersonalUploadResult{}, r.ctx.Err()
	}
}
func (r *pendingPersonalCommit) PersonalUploadAssetExists(ctx context.Context, owner domain.PersonalOwnership, id uuid.UUID) (bool, error) {
	close(r.checking)
	return r.Store.PersonalUploadAssetExists(ctx, owner, id)
}

func TestPersonalUploadCleanupJoinsUnknownRealCommitBeforeObjectRemoval(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _ := mediaStoreProject(t, db)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	repo := &pendingPersonalCommit{Store: pgmedia.NewStore(db), db: db, ctx: ctx, pending: make(chan struct{}), release: make(chan struct{}), checking: make(chan struct{}), done: make(chan struct{}), finished: make(chan error, 1)}
	var release sync.Once
	objects := glbTestObjects(t)
	service := mediaapp.NewScopedUploadService(repo.Store, repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = uuid.Nil
	result := make(chan error, 1)
	joined := make(chan struct{})
	go func() { defer close(joined); _, err := service.UploadPersonal(ctx, in); result <- err }()
	defer func() { release.Do(func() { close(repo.release) }); cancel(); <-repo.done; <-joined }()
	select {
	case <-repo.checking:
	case <-ctx.Done():
		t.Fatal("unknown cleanup did not wait for commit")
	}
	select {
	case err := <-result:
		t.Fatal("pending commit was bypassed", err)
	case <-time.After(100 * time.Millisecond):
	}
	release.Do(func() { close(repo.release) })
	if err := <-repo.finished; err != nil {
		t.Fatal("held actual commit", err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal("unknown result changed", err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id IS NULL AND personal_org_id=? AND personal_actor_id=? AND status='ready'`, actor.OrgID, actor.ID).Scan(&count).Error; err != nil || count != 1 || len(repo.keys) != 3 {
		t.Fatal("unknown committed original was destroyed", count, err)
	}
	for _, key := range repo.keys {
		if info, err := objects.Stat(t.Context(), key); err != nil || info.Size < 1 || len(info.SHA256) != 64 {
			t.Fatal("unknown actual private original/rendition was removed", err)
		}
		key := key
		t.Cleanup(func() {
			clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := objects.Remove(clean, key); err != nil {
				t.Error("cleanup actual personal fixture")
			}
		})
	}
}

func TestPersonalUploadZeroRowReceiptRollsBackObjectsAndAudit(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	actor, _ := mediaStoreProject(t, db)
	name := "drop_personal_receipt_" + strings.ReplaceAll(actor.ID.String(), "-", "")
	if err := owner.Exec(`CREATE FUNCTION media.` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`CREATE TRIGGER ` + name + ` BEFORE INSERT ON media.upload_request FOR EACH ROW WHEN (NEW.principal_id='` + actor.ID.String() + `'::uuid) EXECUTE FUNCTION media.` + name + `()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`DROP TRIGGER ` + name + ` ON media.upload_request`).Error; err != nil {
			t.Error(err)
		}
		if err := owner.Exec(`DROP FUNCTION media.` + name + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	objects := &uploadObjectsFake{}
	store := pgmedia.NewStore(db)
	service := mediaapp.NewScopedUploadService(store, store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = uuid.Nil
	if result, err := service.UploadPersonal(t.Context(), in); err == nil || result.Asset.ID != uuid.Nil {
		t.Fatal("zero row receipt returned success", err)
	}
	var count int64
	if err := db.Raw(`SELECT (SELECT count(*) FROM media.media_asset WHERE personal_actor_id=?)+(SELECT count(*) FROM media.upload_request WHERE principal_id=?)+(SELECT count(*) FROM media.library WHERE personal_actor_id=?)+(SELECT count(*) FROM audit.audit_log WHERE actor_id=? AND action='media.uploaded')`, actor.ID, actor.ID, actor.ID, actor.ID).Scan(&count).Error; err != nil || count != 0 || len(objects.items) != 0 {
		t.Fatal("zero row acceptance left partial effects", count, len(objects.items), err)
	}
}
