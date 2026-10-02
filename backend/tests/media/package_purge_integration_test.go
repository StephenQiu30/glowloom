package media_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func packagePurgeDB(t *testing.T, owner bool) *gorm.DB {
	t.Helper()
	name := "LV_TEST_MEDIA_PACKAGE_GUARD_DB_DSN"
	if owner {
		name = "LV_TEST_MEDIA_PACKAGE_GUARD_OWNER_DB_DSN"
	}
	dsn := os.Getenv(name)
	if dsn == "" {
		t.Skip("set the dedicated isolated package guard database")
	}
	connection, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("open isolated package guard database")
	}
	t.Cleanup(func() { _ = connection.Close() })
	var database, role, databaseOwner string
	if err := connection.DB.Raw(`SELECT current_database(),current_user,pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()`).Row().Scan(&database, &role, &databaseOwner); err != nil || !strings.HasPrefix(database, "lanverse_composition_package_") || !owner && role != "lanverse_app" || owner && (role == "lanverse_app" || role != databaseOwner) {
		t.Fatal("package guards require their isolated database and actual owner/runtime roles")
	}
	return connection.DB
}

func TestMediaPackagePersonalUnknownBlocksPurgeUntilActualCancellation(t *testing.T) {
	db := packagePurgeDB(t, false)
	actor, _ := mediaStoreProject(t, db)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	library := pgmedia.NewLibraryStore(db, packageTestAccess, time.Now)
	text := "待清理的实际个人正文"
	created, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "create_text", Metadata: &mediaapp.LibraryMetadata{Title: "个人正文", Category: "material", Tags: []string{}, PlainText: &text}})
	if err != nil {
		t.Fatal(err)
	}
	item := created.Items[0].ID
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 1, Action: "recycle_items", Items: []mediaapp.LibraryItemRevision{{ID: item, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	repo, service := packageActualService(t, db, &packageUnknownWrite{PackageObjects: objects}, packageTestAccess)
	job, err := service.Import(t.Context(), actor, mediaapp.PackageImportRequest{Scope: scope, ExpectedRevision: 2, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}, packageDownloaded(t, ownPackageZIP(t)))
	if err != nil || job.Status != "needs_reconciliation" {
		t.Fatal("actual retained personal package", job, err)
	}
	packageCleanup(t, repo, objects, actor, scope)
	if busy, err := repo.HasInflightLibraryPackageWork(t.Context(), actor, scope); err != nil || !busy {
		t.Fatal("owning personal guard lost the actual unknown batch", busy, err)
	}
	foreign, _ := mediaStoreProject(t, db)
	if busy, err := repo.HasInflightLibraryPackageWork(t.Context(), foreign, scope); err != nil || busy {
		t.Fatal("foreign personal library discovered another actor's batch", busy, err)
	}
	spoof := actor
	spoof.OrgID = foreign.OrgID
	if _, err := repo.HasInflightLibraryPackageWork(t.Context(), spoof, scope); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("guard trusted a stale actor/organization declaration", err)
	}
	purges := pgmedia.NewPurgeStore(db, packageTestAccess, transferTestGuards, nil, time.Now)
	in := mediaapp.PurgeInput{Scope: scope, Items: []mediaapp.LibraryItemRevision{{ID: item, Revision: 2}}, ExpectedRevision: 2, PermanentDeleteConfirmed: true, Key: uuid.New()}
	if _, err := purges.CreatePurge(t.Context(), actor, in); !errors.Is(err, domain.ErrPurgeConflict) {
		t.Fatal("unknown personal package did not block purge", err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM media.purge_job WHERE actor_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("blocked admission left a partial purge", count, err)
	}
	_, recovered := packageActualService(t, db, objects, packageTestAccess)
	cancelled, err := recovered.Cancel(t.Context(), actor, job.ID, mediaapp.PackageControl{ExpectedRevision: 1, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatal("complete actual absent proof did not close unknown package", cancelled, err)
	}
	if busy, err := repo.HasInflightLibraryPackageWork(t.Context(), actor, scope); err != nil || busy {
		t.Fatal("owning personal guard retained a closed batch", busy, err)
	}
	in.Key = uuid.New()
	accepted, err := purges.CreatePurge(t.Context(), actor, in)
	if err != nil || accepted.Status != "queued" || accepted.Scope.ProjectID != nil {
		t.Fatal("terminal personal package blocked authorized cleanup", accepted, err)
	}
}

func TestMediaPackagePersonalUnreadableOwnerAndRevokedActorNeverAdmitPurge(t *testing.T) {
	db, owner := packagePurgeDB(t, false), packagePurgeDB(t, true)
	actor, _ := mediaStoreProject(t, db)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	library := pgmedia.NewLibraryStore(db, packageTestAccess, time.Now)
	text := "仍保留的个人正文"
	created, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "create_text", Metadata: &mediaapp.LibraryMetadata{Title: "正文", Category: "material", Tags: []string{}, PlainText: &text}})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Items[0].ID
	if _, err := library.ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), ExpectedRevision: 1, Action: "recycle_items", Items: []mediaapp.LibraryItemRevision{{ID: id, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	var database string
	if err := owner.Raw(`SELECT current_database()`).Scan(&database).Error; err != nil || !strings.HasPrefix(database, "lanverse_composition_package_") {
		t.Fatal("permission failure fixture requires its independent package composition database")
	}
	if err := owner.Exec(`REVOKE SELECT ON media.package_job FROM lanverse_app`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(`GRANT SELECT ON media.package_job TO lanverse_app`).Error; err != nil {
			t.Error("restore only this isolated database package read permission", err)
		}
	})
	purges := pgmedia.NewPurgeStore(db, packageTestAccess, transferTestGuards, nil, time.Now)
	in := mediaapp.PurgeInput{Scope: scope, Items: []mediaapp.LibraryItemRevision{{ID: id, Revision: 2}}, ExpectedRevision: 2, PermanentDeleteConfirmed: true, Key: uuid.New()}
	if _, err := purges.CreatePurge(t.Context(), actor, in); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("unreadable personal package owner was treated as idle", err)
	}
	if err := owner.Exec(`GRANT SELECT ON media.package_job TO lanverse_app`).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE identity."user" SET is_delete=true WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	in.Key = uuid.New()
	if _, err := purges.CreatePurge(t.Context(), actor, in); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("revoked current actor admitted permanent cleanup")
	}
	if _, err := pgmedia.NewPackageStore(db, packageTestAccess, nil, time.Now).HasInflightLibraryPackageWork(t.Context(), actor, scope); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("package reader skipped current actor revocation", err)
	}
	var count int64
	if err := owner.Raw(`SELECT count(*) FROM media.purge_job WHERE actor_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("unavailable or revoked authorization partly admitted purge", count, err)
	}
}
