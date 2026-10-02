package media_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func TestPersonalUploadConcurrentAcceptanceHasOneOriginalAndLibraryRevision(t *testing.T) {
	for _, sameKey := range []bool{true, false} {
		t.Run(map[bool]string{true: "one permanent key", false: "same source different keys"}[sameKey], func(t *testing.T) {
			db := libraryRuntimeDB(t)
			actor, project := mediaStoreProject(t, db)
			store := pgmedia.NewStore(db)
			gate := uploadBlockingProber{entered: make(chan struct{}, 2), release: make(chan struct{})}
			objects := &uploadObjectsFake{}
			service := mediaapp.NewScopedUploadService(store, store, gate, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
			inputs := []mediaapp.UploadInput{uploadInput(t), uploadInput(t)}
			for i := range inputs {
				inputs[i].Actor = actor
				inputs[i].Request.ProjectID = uuid.Nil
			}
			if sameKey {
				inputs[1].Request.Key = inputs[0].Request.Key
			}
			results := make([]mediaapp.PersonalUploadResult, 2)
			errs := make([]error, 2)
			var wg sync.WaitGroup
			for i := range inputs {
				wg.Go(func() { results[i], errs[i] = service.UploadPersonal(t.Context(), inputs[i]) })
			}
			for range 2 {
				select {
				case <-gate.entered:
				case <-time.After(5 * time.Second):
					close(gate.release)
					wg.Wait()
					t.Fatal("both personal uploads did not enter probe")
				}
			}
			close(gate.release)
			wg.Wait()
			if errs[0] != nil || errs[1] != nil || results[0].Asset.ID != results[1].Asset.ID || sameKey && !reflect.DeepEqual(results[0], results[1]) {
				t.Fatal("concurrent personal acceptance", results, errs)
			}
			expected := int64(2)
			if sameKey {
				expected = 1
			}
			var counts struct{ Assets, Receipts, Audits, LibraryRevision, ProjectRevision int64 }
			err := db.Raw(`SELECT (SELECT count(*) FROM media.media_asset WHERE personal_actor_id=?) AS assets,(SELECT count(*) FROM media.upload_request WHERE principal_id=?) AS receipts,(SELECT count(*) FROM audit.audit_log WHERE actor_id=? AND action='media.uploaded') AS audits,(SELECT revision FROM media.library WHERE personal_actor_id=?) AS library_revision,(SELECT revision FROM workspace.project WHERE id=?) AS project_revision`, actor.ID, actor.ID, actor.ID, actor.ID, project).Scan(&counts).Error
			if err != nil || counts.Assets != 1 || counts.Receipts != expected || counts.Audits != expected || counts.LibraryRevision != 1 || counts.ProjectRevision != 1 || len(objects.items) != 3 {
				t.Fatal("personal dedup leaked target/project facts", counts, len(objects.items), err)
			}
		})
	}
}

func TestPersonalUploadRechecksRevocationBeforePublishing(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	actor, _ := mediaStoreProject(t, db)
	store := pgmedia.NewStore(db)
	objects := &uploadObjectsFake{}
	probe := archiveUploadProber{afterProbe: func() error {
		return owner.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error
	}}
	defer func() {
		if err := owner.Exec(`UPDATE identity."user" SET status='active' WHERE id=?`, actor.ID).Error; err != nil {
			t.Error(err)
		}
	}()
	service := mediaapp.NewScopedUploadService(store, store, probe, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = uuid.Nil
	if result, err := service.UploadPersonal(t.Context(), in); err == nil || result.Asset.ID != uuid.Nil || len(objects.items) != 0 {
		t.Fatal("mid upload revoked actor published", err, len(objects.items))
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM media.upload_request WHERE principal_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("revoked upload retained success receipt", count, err)
	}
}

func TestPersonalUploadReceiptMigrationKeepsOwnershipAndRuntimeACLClosed(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	actor, project := mediaStoreProject(t, db)
	foreign, _ := mediaStoreProject(t, db)
	original := uuid.New()
	sourceKey := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/2026/10/" + original.String() + ".png"
	if err := db.Exec(`INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,moderation_status) VALUES(?,?,?,'image','upload','ready',?,'scope.png','image/png',1,'passed')`, original, actor.OrgID, actor.ID, sourceKey).Error; err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO media.upload_request(project_id,personal_org_id,principal_id,request_key,sha256,file_name,byte_size,asset_id,response) VALUES(?,?,?,?,?,'scope.txt',1,?,'{}'::jsonb)`
	for _, candidate := range []struct {
		project, org any
		principal    uuid.UUID
	}{{project, actor.OrgID, actor.ID}, {nil, actor.OrgID, foreign.ID}, {project, nil, actor.ID}, {nil, nil, actor.ID}} {
		err := db.Exec(insert, candidate.project, candidate.org, candidate.principal, uuid.New(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", original).Error
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "23514" && pe.Code != "23503" {
			t.Fatal("crossscope upload receipt accepted", err)
		}
	}
	for _, query := range []string{`UPDATE media.upload_request SET file_name=file_name WHERE false`, `DELETE FROM media.upload_request WHERE false`, `UPDATE media.media_asset SET personal_actor_id=personal_actor_id WHERE false`, `UPDATE media.media_asset SET object_key=object_key WHERE false`} {
		err := db.Exec(query).Error
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "42501" {
			t.Fatal("runtime can rewrite permanent receipt or immutable source", err)
		}
	}
	// Exercise down/up restoration in a rollback-only owner transaction without
	// losing fixture receipts from previous real tests.
	down, err := os.ReadFile("../../db/migrations/202610020056_media_personal_upload.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("../../db/migrations/202610020056_media_personal_upload.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tx := owner.WithContext(ctx).Begin()
	defer func() { _ = tx.Rollback().Error }()
	if err := tx.Exec(`DELETE FROM media.upload_request WHERE project_id IS NULL`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(down)).Error; err != nil {
		t.Fatal("isolated down", err)
	}
	if err := tx.Exec(string(up)).Error; err != nil {
		t.Fatal("isolated up", err)
	}
	var nullAllowed bool
	if err := tx.Raw(`SELECT is_nullable='YES' FROM information_schema.columns WHERE table_schema='media' AND table_name='upload_request' AND column_name='project_id'`).Scan(&nullAllowed).Error; err != nil || !nullAllowed {
		t.Fatal("scope ddl not restored", err)
	}
}
