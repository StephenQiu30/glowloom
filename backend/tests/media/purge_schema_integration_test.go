package media_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
)

func TestMediaPurgeRuntimeCannotRewriteOwnershipFrozenKeysOrPermanentReceipts(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _, in, _, _ := purgeBinaryFixture(t, db)
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE media.purge_job SET actor_id=actor_id WHERE id=?`,
		`UPDATE media.purge_job SET project_id=project_id WHERE id=?`,
		`UPDATE media.purge_job SET library_id=library_id WHERE id=?`,
		`UPDATE media.purge_item SET frozen=frozen WHERE job_id=?`,
		`UPDATE media.purge_item SET asset_id=asset_id WHERE job_id=?`,
		`UPDATE media.purge_object SET object_key=object_key WHERE job_id=?`,
		`UPDATE media.purge_command SET response_body=response_body WHERE job_id=?`,
		`DELETE FROM media.purge_job WHERE id=?`,
		`DELETE FROM media.purge_item WHERE job_id=?`,
		`DELETE FROM media.purge_object WHERE job_id=?`,
		`DELETE FROM media.purge_command WHERE job_id=?`,
	} {
		err := db.Exec(query, job.ID).Error
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.Code != "42501" {
			t.Fatal("runtime rewrote permanent physical ownership proof", query, err)
		}
	}
	owner := libraryOwnerDB(t)
	err = owner.Exec(`UPDATE media.purge_command SET response_body=response_body WHERE job_id=?`, job.ID).Error
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != "42501" {
		t.Fatal("permanent purge receipt was mutable even by fixture owner", err)
	}
	if _, err := repo.GetPurge(t.Context(), actor, job.ID); err != nil {
		t.Fatal("rejected writes changed current permanent job", err)
	}
}
