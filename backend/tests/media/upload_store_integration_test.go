package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
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

func TestUploadStorePersistsHumanEvidenceAndExactReplay(t *testing.T) {
	database := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, database)
	store := pgmedia.NewStore(database)
	objects := &uploadObjectsFake{}
	service := mediaapp.NewUploadService(store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = project
	first, err := service.Upload(t.Context(), in)
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	asset, err := store.FindAsset(t.Context(), actor, project, first.Asset.ID)
	if err != nil || !asset.CanReference() || asset.Origin != domain.OriginUpload || asset.SourceOperationID != nil || asset.ContainsRealPerson {
		t.Fatalf("uploaded asset=%+v err=%v", asset, err)
	}
	var review mediaapp.LocalUploadReview
	if err := json.Unmarshal(asset.ModerationDetail, &review); err != nil || review.Method != "local_workspace_owner_review" || review.PrincipalID != actor.ID || review.SHA256 != in.File.SHA256 || !review.RightsConfirmed || !review.NoAuthorizationRequiredRealPerson || review.ReviewedAt.IsZero() {
		t.Fatalf("human review=%+v err=%v", review, err)
	}
	replay, err := service.Upload(t.Context(), in)
	if err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatalf("exact replay=%+v err=%v first=%+v", replay, err, first)
	}
	in.Request.FileName = "不同名称.png"
	if _, err := service.Upload(t.Context(), in); !errors.Is(err, mediaapp.ErrUploadConflict) {
		t.Fatalf("key with different filename: %v", err)
	}
	in.Request.Key = uuid.New()
	reused, err := service.Upload(t.Context(), in)
	if err != nil || reused.DuplicateOf == nil || *reused.DuplicateOf != first.Asset.ID || reused.Asset.ID != first.Asset.ID {
		t.Fatalf("same hash reuse=%+v err=%v", reused, err)
	}
	var counts struct{ Assets, Receipts, Audits, Operations int64 }
	if err := database.Raw(`SELECT (SELECT count(*) FROM media.media_asset WHERE project_id=?) AS assets,(SELECT count(*) FROM media.upload_request WHERE project_id=?) AS receipts,(SELECT count(*) FROM audit.audit_log WHERE project_id=? AND action='media.uploaded') AS audits,(SELECT count(*) FROM operation.operation WHERE project_id=?) AS operations`, project, project, project, project).Scan(&counts).Error; err != nil || counts.Assets != 1 || counts.Receipts != 2 || counts.Audits != 2 || counts.Operations != 0 {
		t.Fatalf("transaction counts=%+v err=%v", counts, err)
	}
	query := mediaapp.NewAssetQuery(store, nil)
	if result, err := query.Reference(t.Context(), actor, project, first.Asset.ID); err != nil || result.ID != first.Asset.ID {
		t.Fatalf("canvas eligibility=%+v err=%v", result, err)
	}
	if len(objects.items) != 3 {
		t.Fatalf("duplicate upload leaked private objects: %d", len(objects.items))
	}
}

func TestUploadStoreConcurrentKeysHaveOneOwnedObjectSet(t *testing.T) {
	for _, sameKey := range []bool{true, false} {
		t.Run(map[bool]string{true: "same key", false: "same hash different keys"}[sameKey], func(t *testing.T) {
			database := mediaStoreDB(t)
			actor, project := mediaStoreProject(t, database)
			gate := uploadBlockingProber{entered: make(chan struct{}, 2), release: make(chan struct{})}
			objects := &uploadObjectsFake{}
			service := mediaapp.NewUploadService(pgmedia.NewStore(database), gate, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
			inputs := []mediaapp.UploadInput{uploadInput(t), uploadInput(t)}
			for i := range inputs {
				inputs[i].Actor = actor
				inputs[i].Request.ProjectID = project
			}
			if sameKey {
				inputs[1].Request.Key = inputs[0].Request.Key
			}
			results := make([]mediaapp.UploadResult, 2)
			errs := make([]error, 2)
			var wg sync.WaitGroup
			for i := range inputs {
				wg.Go(func() { results[i], errs[i] = service.Upload(t.Context(), inputs[i]) })
			}
			for range 2 {
				select {
				case <-gate.entered:
				case <-time.After(5 * time.Second):
					close(gate.release)
					wg.Wait()
					t.Fatal("concurrent uploads did not enter probe")
				}
			}
			close(gate.release)
			wg.Wait()
			if errs[0] != nil || errs[1] != nil || results[0].Asset.ID != results[1].Asset.ID {
				t.Fatalf("concurrent results=%+v errs=%v", results, errs)
			}
			if sameKey && !reflect.DeepEqual(results[0], results[1]) {
				t.Fatalf("same-key response differs: %+v", results)
			}
			wantReceipts := int64(2)
			if sameKey {
				wantReceipts = 1
			}
			var counts struct{ Assets, Receipts, Audits int64 }
			if err := database.Raw(`SELECT (SELECT count(*) FROM media.media_asset WHERE project_id=?) AS assets,(SELECT count(*) FROM media.upload_request WHERE project_id=?) AS receipts,(SELECT count(*) FROM audit.audit_log WHERE project_id=? AND action='media.uploaded') AS audits`, project, project, project).Scan(&counts).Error; err != nil || counts.Assets != 1 || counts.Receipts != wantReceipts || counts.Audits != wantReceipts || len(objects.items) != 3 {
				t.Fatalf("concurrent durable facts=%+v objects=%d err=%v", counts, len(objects.items), err)
			}
		})
	}
}

type archiveUploadProber struct{ afterProbe func() error }

func (p archiveUploadProber) Probe(ctx context.Context, file *mediaapp.Downloaded) (mediaapp.ProbeResult, error) {
	result, err := (mediaflow.FFUploadProber{}).Probe(ctx, file)
	if err != nil {
		return result, err
	}
	return result, p.afterProbe()
}
func TestUploadStoreRechecksArchiveAndRejectsForeignProject(t *testing.T) {
	database := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, database)
	foreign, _ := mediaStoreProject(t, database)
	store := pgmedia.NewStore(database)
	if _, err := store.AuthorizeUpload(t.Context(), foreign, project); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatalf("foreign project upload permission=%v", err)
	}
	objects := &uploadObjectsFake{}
	prober := archiveUploadProber{afterProbe: func() error {
		return database.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error
	}}
	service := mediaapp.NewUploadService(store, prober, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = project
	if _, err := service.Upload(t.Context(), in); !errors.Is(err, pgmedia.ErrProjectStateConflict) {
		t.Fatalf("mid-upload archive error=%v", err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id=?`, project).Scan(&count).Error; err != nil || count != 0 || len(objects.items) != 0 {
		t.Fatalf("archived upload persisted assets=%d objects=%d err=%v", count, len(objects.items), err)
	}
}

func TestUploadStoreDifferentHashCannotRewriteReceipt(t *testing.T) {
	database := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, database)
	service := mediaapp.NewUploadService(pgmedia.NewStore(database), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, &uploadObjectsFake{}, time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = project
	if _, err := service.Upload(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	data := append(uploadPNG(t), []byte("distinct immutable hash")...)
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), in.Request.FileName)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	in.File = file
	if _, err := service.Upload(t.Context(), in); !errors.Is(err, mediaapp.ErrUploadConflict) {
		t.Fatalf("same-key body change accepted=%v", err)
	}
}

type delayedUploadCommit struct {
	*pgmedia.Store
	database  *gorm.DB
	commitCtx context.Context
	release   chan struct{}
	pending   chan struct{}
	checking  chan struct{}
	finished  chan error
	done      chan struct{}
}

func (r *delayedUploadCommit) CommitUpload(_ context.Context, actor identityapp.Principal, request mediaapp.UploadRequest, asset domain.MediaAsset, rends []domain.Rendition) (mediaapp.UploadResult, error) {
	go func() {
		defer close(r.done)
		err := r.database.WithContext(r.commitCtx).Transaction(func(tx *gorm.DB) error {
			if _, err := pgmedia.NewStore(tx).CommitUpload(r.commitCtx, actor, request, asset, rends); err != nil {
				return err
			}
			close(r.pending)
			select {
			case <-r.release:
				return nil
			case <-r.commitCtx.Done():
				return r.commitCtx.Err()
			}
		})
		r.finished <- err
	}()
	select {
	case <-r.pending:
		return mediaapp.UploadResult{}, context.Canceled
	case err := <-r.finished:
		return mediaapp.UploadResult{}, err
	case <-r.commitCtx.Done():
		return mediaapp.UploadResult{}, r.commitCtx.Err()
	}
}
func (r *delayedUploadCommit) UploadAssetExists(ctx context.Context, project, id uuid.UUID) (bool, error) {
	close(r.checking)
	return r.Store.UploadAssetExists(ctx, project, id)
}

func TestUploadCleanupWaitsForPendingRealCommit(t *testing.T) {
	database := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, database)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	repo := &delayedUploadCommit{Store: pgmedia.NewStore(database), database: database, commitCtx: ctx, release: make(chan struct{}), pending: make(chan struct{}), checking: make(chan struct{}), finished: make(chan error, 1), done: make(chan struct{})}
	var releaseOnce sync.Once
	objects := &uploadObjectsFake{}
	service := mediaapp.NewUploadService(repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = project
	completed := make(chan error, 1)
	requestDone := make(chan struct{})
	go func() { defer close(requestDone); _, err := service.Upload(ctx, in); completed <- err }()
	defer func() { releaseOnce.Do(func() { close(repo.release) }); cancel(); <-repo.done; <-requestDone }()
	select {
	case <-repo.checking:
	case <-ctx.Done():
		t.Fatal("cleanup did not reach pending commit confirmation")
	}
	select {
	case err := <-completed:
		t.Fatalf("cleanup bypassed unresolved real COMMIT: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(repo.release) })
	if err := <-repo.finished; err != nil {
		t.Fatalf("delayed real commit: %v", err)
	}
	if err := <-completed; !errors.Is(err, context.Canceled) {
		t.Fatalf("ambiguous request error=%v", err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id=? AND status='ready'`, project).Scan(&count).Error; err != nil || count != 1 || len(objects.items) != 3 || len(objects.removed) != 0 {
		t.Fatalf("cleanup damaged committed asset: ready=%d objects=%d removed=%d err=%v", count, len(objects.items), len(objects.removed), err)
	}
}

func TestUploadStoreAuditFailureRollsBackPublication(t *testing.T) {
	database := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, database)
	fixtureDB := database
	if os.Getenv("LV_TEST_LIBRARY_OWNER_DB_DSN") != "" {
		fixtureDB = libraryOwnerDB(t)
	}
	function := "reject_upload_audit_" + strings.ReplaceAll(project.String(), "-", "")
	if err := fixtureDB.Exec(`CREATE FUNCTION audit.` + function + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture audit unavailable'; END $$`).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixtureDB.Exec(`CREATE TRIGGER ` + function + ` BEFORE INSERT ON audit.audit_log FOR EACH ROW WHEN (NEW.project_id='` + project.String() + `'::uuid) EXECUTE FUNCTION audit.` + function + `()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupDB := fixtureDB.WithContext(cleanCtx)
		if err := cleanupDB.Exec(`DROP TRIGGER ` + function + ` ON audit.audit_log`).Error; err != nil {
			t.Error(err)
		}
		if err := cleanupDB.Exec(`DROP FUNCTION audit.` + function + `()`).Error; err != nil {
			t.Error(err)
		}
	})
	objects := &uploadObjectsFake{}
	service := mediaapp.NewUploadService(pgmedia.NewStore(database), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = project
	if result, err := service.Upload(t.Context(), in); err == nil || result.Asset.ID != uuid.Nil {
		t.Fatalf("audit failure appeared successful: %+v err=%v", result, err)
	}
	var count int64
	if err := database.Raw(`SELECT (SELECT count(*) FROM media.media_asset WHERE project_id=?)+(SELECT count(*) FROM media.upload_request WHERE project_id=?)+(SELECT count(*) FROM audit.audit_log WHERE project_id=?)`, project, project, project).Scan(&count).Error; err != nil || count != 0 || len(objects.items) != 0 {
		t.Fatalf("audit failure left visible writes=%d objects=%d err=%v", count, len(objects.items), err)
	}
}
