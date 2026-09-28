package operation_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
)

func TestReadReuseSourceAvailableRechecksMutableOutputs(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	store := pgoperation.NewStore(database)
	hash := uuid.NewString()
	sourceID := seedCompletedOutput(t, database, projectID, hash, "passed", "ready", "passed")
	quoted := quotedSnapshotItem(projectID, 0).Operation
	quoted.InputHash = hash
	quoted.ReusedFromID = &sourceID

	assertAvailable := func(want bool) {
		t.Helper()
		available, err := store.ReadReuseSourceAvailable(t.Context(), actor, quoted)
		if err != nil || available != want {
			t.Fatalf("reuse source available = %v, %v; want %v", available, err, want)
		}
	}
	assertAvailable(true)
	if err := database.Exec(`
		INSERT INTO operation.operation_output
		  (id, project_id, operation_id, seq_no, kind, json_payload, moderation_status)
		VALUES (?::uuid, ?::uuid, ?::uuid, 1, 'json', '{}'::jsonb, 'passed')
	`, uuid.NewString(), projectID.String(), sourceID.String()).Error; err != nil {
		t.Fatalf("add reusable structured output: %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation SET output_count = 2 WHERE id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("update reusable output count: %v", err)
	}
	quoted.OutputCount = 2
	assertAvailable(true)
	if err := database.Exec(`UPDATE operation.operation_output SET moderation_status = 'rejected' WHERE operation_id = ?::uuid AND seq_no = 1`, sourceID.String()).Error; err != nil {
		t.Fatalf("reject structured output: %v", err)
	}
	assertAvailable(false)
	if err := database.Exec(`UPDATE operation.operation_output SET moderation_status = 'passed' WHERE operation_id = ?::uuid AND seq_no = 1`, sourceID.String()).Error; err != nil {
		t.Fatalf("restore structured output: %v", err)
	}
	quoted.InputHash = uuid.NewString()
	assertAvailable(false)
	quoted.InputHash = hash
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		available, err := pgoperation.NewStore(tx).ReadReuseSourceAvailable(t.Context(), actor, quoted)
		if err != nil {
			return err
		}
		if !available {
			t.Error("runtime role could not recheck reusable output")
		}
		return nil
	}); err != nil {
		t.Fatalf("recheck source as runtime role: %v", err)
	}

	if err := database.Exec(`UPDATE operation.operation_output SET moderation_status = 'rejected' WHERE operation_id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("reject output: %v", err)
	}
	assertAvailable(false)
	if err := database.Exec(`UPDATE operation.operation_output SET moderation_status = 'passed' WHERE operation_id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("restore output: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET status = 'processing' WHERE source_operation_id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("mark media unready: %v", err)
	}
	assertAvailable(false)
	if err := database.Exec(`UPDATE media.media_asset SET status = 'ready' WHERE source_operation_id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("restore media readiness: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET moderation_status = 'rejected' WHERE source_operation_id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("reject media: %v", err)
	}
	assertAvailable(false)
	if err := database.Exec(`UPDATE media.media_asset SET moderation_status = 'passed' WHERE source_operation_id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("restore media moderation: %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation SET status = 'failed' WHERE id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("invalidate source completion: %v", err)
	}
	assertAvailable(false)
	if err := database.Exec(`UPDATE operation.operation SET status = 'completed' WHERE id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("restore source completion: %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation_output SET is_delete = true WHERE operation_id = ?::uuid`, sourceID.String()).Error; err != nil {
		t.Fatalf("remove source output: %v", err)
	}
	assertAvailable(false)

	foreign, _ := operationStoreProject(t, database)
	if _, err := store.ReadReuseSourceAvailable(t.Context(), foreign, quoted); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("foreign project source read = %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("disable actor: %v", err)
	}
	if _, err := store.ReadReuseSourceAvailable(t.Context(), actor, quoted); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked actor source read = %v", err)
	}
}

func TestReadReuseSourceAvailableHoldsMediaLockThroughOuterTransaction(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	hash := uuid.NewString()
	sourceID := seedCompletedOutput(t, database, projectID, hash, "passed", "ready", "passed")
	quoted := quotedSnapshotItem(projectID, 0).Operation
	quoted.InputHash, quoted.ReusedFromID = hash, &sourceID
	var asset struct{ ID uuid.UUID }
	if err := database.Raw(`SELECT id FROM media.media_asset WHERE source_operation_id = ?::uuid`, sourceID.String()).Scan(&asset).Error; err != nil {
		t.Fatalf("read source asset id: %v", err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		available, err := pgoperation.NewStore(tx).ReadReuseSourceAvailable(t.Context(), actor, quoted)
		if err != nil || !available {
			t.Fatalf("lock available source = %v, %v", available, err)
		}
		blocked := make(chan error, 1)
		go func() {
			blocked <- database.Transaction(func(other *gorm.DB) error {
				if err := other.Exec(`SET LOCAL lock_timeout = '250ms'`).Error; err != nil {
					return err
				}
				return other.Exec(`UPDATE media.media_asset SET moderation_status = 'rejected' WHERE id = ?::uuid`, asset.ID.String()).Error
			})
		}()
		var pgError *pgconn.PgError
		if err := <-blocked; !errors.As(err, &pgError) || pgError.Code != "55P03" {
			t.Fatalf("concurrent asset update = %v, want lock timeout", err)
		}
		blockedInsert := make(chan error, 1)
		go func() {
			blockedInsert <- database.Transaction(func(other *gorm.DB) error {
				if err := other.Exec(`SET LOCAL lock_timeout = '250ms'`).Error; err != nil {
					return err
				}
				return other.Exec(`
					INSERT INTO operation.operation_output
					  (id, project_id, operation_id, seq_no, kind, media_asset_id, moderation_status)
					VALUES (?::uuid, ?::uuid, ?::uuid, 1, 'media', ?::uuid, 'pending')
				`, uuid.NewString(), projectID.String(), sourceID.String(), asset.ID.String()).Error
			})
		}()
		if err := <-blockedInsert; !errors.As(err, &pgError) || pgError.Code != "55P03" {
			t.Fatalf("concurrent output insert = %v, want lock timeout", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("keep reuse locks until commit: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET moderation_status = 'rejected' WHERE id = ?::uuid`, asset.ID.String()).Error; err != nil {
		t.Fatalf("update asset after confirmation transaction: %v", err)
	}
}
