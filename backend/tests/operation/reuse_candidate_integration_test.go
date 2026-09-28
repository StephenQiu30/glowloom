package operation_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
)

func TestFindReusableCompletedRequiresSameProjectAndSafeOutputs(t *testing.T) {
	database := operationStoreDB(t)
	actor, projectID := operationStoreProject(t, database)
	otherActor, otherProjectID := operationStoreProject(t, database)
	store := pgoperation.NewStore(database)
	hash := uuid.NewString()

	seedCompletedOutput(t, database, projectID, hash, "rejected", "ready", "passed")
	if _, err := store.FindReusableCompleted(t.Context(), actor, projectID, hash); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("rejected output reuse = %v, want no candidate", err)
	}
	other := seedCompletedOutput(t, database, otherProjectID, hash, "passed", "ready", "passed")
	if _, err := store.FindReusableCompleted(t.Context(), actor, projectID, hash); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("other project reuse = %v, want no candidate", err)
	}
	if got, err := store.FindReusableCompleted(t.Context(), otherActor, otherProjectID, hash); err != nil || got != other {
		t.Fatalf("other actor own candidate = %s, %v; want %s", got, err, other)
	}

	approved := seedCompletedOutput(t, database, projectID, hash, "passed", "ready", "passed")
	if got, err := store.FindReusableCompleted(t.Context(), actor, projectID, hash); err != nil || got != approved {
		t.Fatalf("safe candidate = %s, %v; want %s", got, err, approved)
	}
	if err := database.Exec(`UPDATE operation.operation SET output_count = 2 WHERE id = ?::uuid`, approved.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.FindReusableCompleted(t.Context(), actor, projectID, hash); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("incomplete output set reuse = %v", err)
	}
	if err := database.Exec(`UPDATE operation.operation SET output_count = 1 WHERE id = ?::uuid`, approved.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET contains_real_person = true WHERE source_operation_id = ?::uuid`, approved.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.FindReusableCompleted(t.Context(), actor, projectID, hash); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("unverifiable real-person media reuse = %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET contains_real_person = false WHERE source_operation_id = ?::uuid`, approved.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		got, err := pgoperation.NewStore(tx).FindReusableCompleted(t.Context(), actor, projectID, hash)
		if err != nil {
			return err
		}
		if got != approved {
			t.Errorf("runtime role candidate = %s, want %s", got, approved)
		}
		return nil
	}); err != nil {
		t.Fatalf("find candidate with runtime role: %v", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET moderation_status = 'rejected' WHERE source_operation_id = ?::uuid`, approved.String()).Error; err != nil {
		t.Fatalf("reject source asset: %v", err)
	}
	if _, err := store.FindReusableCompleted(t.Context(), actor, projectID, hash); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("asset rejected after completion = %v, want no candidate", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET moderation_status = 'passed', status = 'processing' WHERE source_operation_id = ?::uuid`, approved.String()).Error; err != nil {
		t.Fatalf("mark source asset unready: %v", err)
	}
	if _, err := store.FindReusableCompleted(t.Context(), actor, projectID, hash); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("asset no longer ready = %v, want no candidate", err)
	}
	if err := database.Exec(`UPDATE media.media_asset SET status = 'ready' WHERE source_operation_id = ?::uuid`, approved.String()).Error; err != nil {
		t.Fatalf("restore source asset readiness: %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'disabled' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("disable actor: %v", err)
	}
	if _, err := store.FindReusableCompleted(t.Context(), actor, projectID, hash); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked actor reuse = %v, want forbidden", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status = 'active' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatalf("restore actor: %v", err)
	}
	if err := database.Exec(`UPDATE workspace.project SET is_delete = true, delete_time = now(), purge_after = now() + interval '30 days' WHERE id = ?::uuid`, projectID.String()).Error; err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if _, err := store.FindReusableCompleted(t.Context(), actor, projectID, hash); !errors.Is(err, pgoperation.ErrNotFound) {
		t.Fatalf("deleted project reuse = %v, want no candidate", err)
	}
}

func seedCompletedOutput(t *testing.T, database *gorm.DB, projectID uuid.UUID, hash, outputModeration, assetStatus, assetModeration string) uuid.UUID {
	t.Helper()
	opID, assetID := uuid.New(), uuid.New()
	if err := database.Exec(`
		INSERT INTO operation.operation
		  (id, project_id, target_type, capability, mode, model_profile_version_id,
		   price_rule_version_id, input_hash, origin, status, quote_micros,
		   quote_expires_at, region)
		VALUES (?::uuid, ?::uuid, 'free', 'image.generate', 'text_to_image',
		        ?::uuid, ?::uuid, ?, 'canvas', 'completed', 1,
		        now() + interval '15 minutes', 'domestic')
	`, opID.String(), projectID.String(), uuid.NewString(), uuid.NewString(), hash).Error; err != nil {
		t.Fatalf("create completed operation: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO media.media_asset
		  (id, project_id, kind, origin, status, object_key, mime_type,
		   byte_size, source_operation_id, moderation_status)
		VALUES (?::uuid, ?::uuid, 'image', 'generated', ?, ?, 'image/png',
		        1, ?::uuid, ?)
	`, assetID.String(), projectID.String(), assetStatus, "reuse-test/"+assetID.String(), opID.String(), assetModeration).Error; err != nil {
		t.Fatalf("create source media: %v", err)
	}
	if err := database.Exec(`
		INSERT INTO operation.operation_output
		  (id, project_id, operation_id, seq_no, kind, media_asset_id, moderation_status)
		VALUES (?::uuid, ?::uuid, ?::uuid, 0, 'media', ?::uuid, ?)
	`, uuid.NewString(), projectID.String(), opID.String(), assetID.String(), outputModeration).Error; err != nil {
		t.Fatalf("create source output: %v", err)
	}
	return opID
}
