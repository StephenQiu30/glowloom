package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func packageLockDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LV_TEST_MEDIA_PACKAGE_LOCK_DB_DSN")
	if dsn == "" {
		t.Skip("set the independent migrated package lock database")
	}
	connection, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("open independent package lock database", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	var database, role, owner string
	if err := connection.DB.Raw(`SELECT current_database(),current_user,pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()`).Row().Scan(&database, &role, &owner); err != nil || !strings.HasPrefix(database, "lanverse_composition_package_lock_") || role != "lanverse_app" || role == owner {
		t.Fatal("package lock tests require their independent database and actual nonowner role")
	}
	return connection.DB
}

func packageLockPlan(t *testing.T, actor identityapp.Principal, project uuid.UUID) mediaapp.PackagePlan {
	t.Helper()
	key, request := uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	library, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	job := uuid.NewSHA1(key, []byte("media-package/"+actor.ID.String()))
	text := "真实 PostgreSQL 锁序回归"
	digest := strings.Repeat("a", 64)
	plan := mediaapp.PackagePlan{
		Version: 1, JobID: job, ActorID: actor.ID, OrgID: actor.OrgID, Key: key, RequestID: request, CreatedAt: now, AspectRatio: "16:9",
		Request: mediaapp.PackageImportRequest{Scope: scope, ExpectedProjectRevision: 1, LocalReviewConfirmed: true, Key: key, RequestID: request, ArchiveSHA256: digest, ArchiveBytes: 22},
		Folders: []domain.LibraryFolder{}, Assets: []domain.MediaAsset{}, Renditions: []domain.Rendition{}, Warnings: []mediaapp.PackageWarning{},
		Items:   []domain.LibraryItem{{ID: uuid.New(), LibraryID: library, PlainText: &text, Title: "正文", Category: "material", Tags: []string{}, State: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}},
		Objects: []mediaapp.PackageObject{{Key: "media-packages/" + actor.OrgID.String() + "/" + actor.ID.String() + "/" + job.String() + "/source.zip", MIMEType: "application/zip", SHA256: digest, ByteSize: 22}},
	}
	if err := plan.Validate(); err != nil {
		t.Fatal("valid complete SQL admission fixture", err)
	}
	return plan
}

// packageLockAccess pauses only after the real workspace reader holds its SQL
// project lock. The writer notification occurs after PackageStore's command
// lock, immediately before its real workspace FOR UPDATE authorization.
type packageLockAccess struct {
	mediaapp.LibraryProjectAccess
	readHeld, releaseRead chan struct{}
	writeEntered          chan struct{}
}

func (a packageLockAccess) Authorize(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool) (mediaapp.LibraryProjectFacts, error) {
	if write && a.writeEntered != nil {
		a.writeEntered <- struct{}{}
	}
	facts, err := a.LibraryProjectAccess.Authorize(ctx, actor, project, write)
	if err != nil || write || a.readHeld == nil {
		return facts, err
	}
	a.readHeld <- struct{}{}
	select {
	case <-a.releaseRead:
		return facts, nil
	case <-ctx.Done():
		return facts, ctx.Err()
	}
}

func TestMediaPackageReplayAndAdmissionUseCommandBeforeProjectLock(t *testing.T) {
	db := packageLockDB(t)
	actor, project := mediaStoreProject(t, db)
	plan := packageLockPlan(t, actor, project)
	readHeld, releaseRead := make(chan struct{}, 1), make(chan struct{})
	writeEntered := make(chan struct{}, 1)
	reader := pgmedia.NewPackageStore(db, func(tx *gorm.DB) mediaapp.LibraryProjectAccess {
		return packageLockAccess{LibraryProjectAccess: packageTestAccess(tx), readHeld: readHeld, releaseRead: releaseRead}
	}, transferTestGuards, time.Now)
	writer := pgmedia.NewPackageStore(db, func(tx *gorm.DB) mediaapp.LibraryProjectAccess {
		return packageLockAccess{LibraryProjectAccess: packageTestAccess(tx), writeEntered: writeEntered}
	}, transferTestGuards, time.Now)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	readResult, writeResult := make(chan error, 1), make(chan error, 1)
	go func() {
		_, _, found, err := reader.FindPackageImport(ctx, actor, plan.Request)
		if err == nil && found {
			err = errors.New("initial replay discovered an admission before releasing its project read lock")
		}
		readResult <- err
	}()
	select {
	case <-readHeld:
	case <-ctx.Done():
		t.Fatal("real project read authorization did not complete", ctx.Err())
	}
	go func() {
		job, err := writer.AdmitPackageImport(ctx, actor, plan.Request, plan)
		if err == nil && (job.ID != plan.JobID || job.Status != "needs_reconciliation") {
			err = errors.New("concurrent admission did not retain the complete original job")
		}
		writeResult <- err
	}()
	// The old reader holds project SHARE before command; the old writer reaches
	// project UPDATE holding command. The fixed writer waits at command instead.
	select {
	case <-writeEntered:
	case <-time.After(250 * time.Millisecond):
	case <-ctx.Done():
	}
	close(releaseRead)
	for _, result := range []<-chan error{readResult, writeResult} {
		select {
		case err := <-result:
			if err != nil {
				var databaseError *pgconn.PgError
				if errors.As(err, &databaseError) {
					t.Errorf("same-key replay/admission SQLSTATE %s: %v", databaseError.Code, err)
				} else {
					t.Error("same-key replay/admission", err)
				}
			}
		case <-ctx.Done():
			t.Error("same-key replay/admission did not terminate", ctx.Err())
		}
	}
}

func TestMediaPackageReplayKeepsOriginalReceiptAndCurrentAuthority(t *testing.T) {
	db := packageLockDB(t)
	actor, project := mediaStoreProject(t, db)
	plan := packageLockPlan(t, actor, project)
	repo := pgmedia.NewPackageStore(db, packageTestAccess, transferTestGuards, time.Now)
	if _, err := repo.AdmitPackageImport(t.Context(), actor, plan.Request, plan); err != nil {
		t.Fatal("complete durable SQL admission", err)
	}
	// No private object is written in this SQL protocol test. Unknown verification
	// records the unchanged job, reserving every declared key for later recovery.
	original, err := repo.WithPackageImport(t.Context(), actor, plan.JobID, func(mediaapp.PackagePlan) error { return io.ErrUnexpectedEOF })
	if err != nil || original.Status != "needs_reconciliation" {
		t.Fatal("original permanent SQL receipt", err)
	}
	text := "后续目录修改"
	if _, err := pgmedia.NewLibraryStore(db, packageTestAccess, time.Now).ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: plan.Request.Scope, Key: uuid.New(), Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, Title: "新增正文", Category: "material", Tags: []string{}}}); err != nil {
		t.Fatal("later authorized catalog change", err)
	}
	replay, frozen, found, err := repo.FindPackageImport(t.Context(), actor, plan.Request)
	want, _ := json.Marshal(original)
	got, _ := json.Marshal(replay)
	if err != nil || !found || frozen == nil || !bytes.Equal(want, got) {
		t.Fatal("later project/library revisions rewrote the permanent receipt", err)
	}
	changed := plan.Request
	changed.ArchiveSHA256 = strings.Repeat("b", 64)
	if _, _, _, err := repo.FindPackageImport(t.Context(), actor, changed); !errors.Is(err, mediaapp.ErrPackageConflict) {
		t.Fatal("same command key accepted a changed archive digest", err)
	}
	foreign, _ := mediaStoreProject(t, db)
	if _, _, _, err := repo.FindPackageImport(t.Context(), foreign, plan.Request); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("foreign actor discovered a project import", err)
	}
	if err := db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal("disable only the isolated fixture actor", err)
	}
	if _, _, _, err := repo.FindPackageImport(t.Context(), actor, plan.Request); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("permanent receipt bypassed current actor revocation", err)
	}
}
