package media_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestMediaTransferRuntimeCannotRewriteOwnershipFrozenBytesKeysOrReceipts(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	for _, query := range []string{
		`UPDATE media.transfer_job SET actor_id=actor_id WHERE id=?`,
		`UPDATE media.transfer_job SET source_library_id=source_library_id WHERE id=?`,
		`UPDATE media.transfer_job SET manifest_sha256=manifest_sha256 WHERE id=?`,
		`UPDATE media.transfer_item SET frozen=frozen WHERE job_id=?`,
		`UPDATE media.transfer_object SET target_object_key=target_object_key WHERE job_id=?`,
		`UPDATE media.transfer_command SET response_body=response_body WHERE job_id=?`,
		`DELETE FROM media.transfer_job WHERE id=?`,
		`DELETE FROM media.transfer_item WHERE job_id=?`,
		`DELETE FROM media.transfer_object WHERE job_id=?`,
		`DELETE FROM media.transfer_command WHERE job_id=?`,
	} {
		err := f.db.Exec(query, f.job.ID).Error
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.Code != "42501" {
			t.Fatal("runtime rewrote retained private ownership proof", err)
		}
	}
	owner := libraryOwnerDB(t)
	err := owner.Exec(`UPDATE media.transfer_command SET response_body=response_body WHERE job_id=?`, f.job.ID).Error
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Code != "42501" {
		t.Fatal("permanent receipts were mutable even by fixture owner", err)
	}
	if _, err := f.repo.GetTransfer(t.Context(), f.actor, f.job.ID); err != nil {
		t.Fatal("rejected writes changed current ownership proof", err)
	}
}
