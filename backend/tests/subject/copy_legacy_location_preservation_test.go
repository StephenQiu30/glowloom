package subject_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// The final schema admits only copies whose original receipt includes physical
// ownership. A later route change must not rewrite that immutable receipt.
func TestCompletedCopyKeepsImmutableReceiptAfterRouteSwitch(t *testing.T) {
	f := newLocatedCopy(t)
	binding := media.ProjectCopyBinding{JobID: uuid.New(), OrgID: f.actor.OrgID, SourceProjectID: f.source, TargetProjectID: f.target}
	var snapshot media.ProjectCopySnapshot
	if err := f.runtime.Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = pgmedia.NewScopedProjectCopyStore(tx, app.NewMediaWorkerScopeAccess).WithRoutedStorage().Freeze(t.Context(), f.actor, binding, time.Now())
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,owner_scope_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,?,1,?,'receipt fixture','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, f.actor.OrgID, f.actor.ID, f.actor.ScopeID, f.source, f.target).Error
	}); err != nil {
		t.Fatal(err)
	}
	repo := &locatedCopyTransactions{ProjectCopyStore: pgmedia.NewScopedProjectCopyStore(f.runtime, app.NewMediaWorkerScopeAccess).WithRoutedStorage(), db: f.runtime}
	transfer := media.NewRoutedProjectCopyTransfer(repo, mediaobjects.NewRoutedCopyObjects(f.registry, pgmedia.NewStorageRoutes(f.runtime)), t.TempDir())
	if err := transfer.Transfer(t.Context(), f.actor, binding, snapshot); err != nil {
		t.Fatal(err)
	}
	before, err := repo.Register(t.Context(), f.actor, binding, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var immutableBefore string
	if err := f.owner.Raw(`SELECT content::text FROM media.project_copy_snapshot WHERE id=?`, snapshot.ID).Scan(&immutableBefore).Error; err != nil {
		t.Fatal(err)
	}
	objectsBefore, err := repo.Objects(t.Context(), f.actor, binding, snapshot)
	if err != nil || len(objectsBefore) != 1 {
		t.Fatal("completed copy lost physical receipt", err)
	}
	if err := f.owner.Exec(`UPDATE media.storage_route SET profile_id=?,revision='2' WHERE singleton=1`, f.sourceProfile).Error; err != nil {
		t.Fatal(err)
	}
	after, err := repo.Register(t.Context(), f.actor, binding, snapshot)
	if err != nil || after != before {
		t.Fatal("route change rewrote accepted copy receipt", err, before, after)
	}
	var immutableAfter string
	if err := f.owner.Raw(`SELECT content::text FROM media.project_copy_snapshot WHERE id=?`, snapshot.ID).Scan(&immutableAfter).Error; err != nil || immutableAfter != immutableBefore {
		t.Fatal("immutable content rewritten", err)
	}
	objectsAfter, err := repo.Objects(t.Context(), f.actor, binding, snapshot)
	if err != nil || len(objectsAfter) != 1 || objectsAfter[0].SourceLocation == nil || objectsAfter[0].TargetLocation == nil || objectsBefore[0].SourceLocation.ID != objectsAfter[0].SourceLocation.ID || objectsBefore[0].TargetLocation.ID != objectsAfter[0].TargetLocation.ID || objectsAfter[0].TargetLocation.Ref.ProfileID != f.targetProfile {
		t.Fatal("completed physical ownership followed current route", err)
	}
	err = f.owner.Exec(`UPDATE media.media_asset SET object_location_id=NULL WHERE id=?`, snapshot.AssetMapping[f.asset]).Error
	var bindingError *pgconn.PgError
	if !errors.As(err, &bindingError) || bindingError.Code != "23514" {
		t.Fatal("completed receipt lost the immutable binding guard", err)
	}
}
