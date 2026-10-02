package operation_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	pgoperation "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
)

func operationReferenceFixture(t *testing.T) (*gorm.DB, identityapp.Principal, uuid.UUID) {
	t.Helper()
	database := operationStoreDB(t)
	var name string
	if err := database.Raw(`SELECT current_database()`).Scan(&name).Error; err != nil || name != "lanverse_reference" {
		t.Fatal("isolated reference database required", err)
	}
	actor, project := operationStoreProject(t, database)
	return database, actor, project
}

func operationReferenceCheck(t *testing.T, database *gorm.DB, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	t.Helper()
	var found bool
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		var err error
		found, err = pgoperation.NewMediaReferenceGuard(tx).HasMediaReferences(t.Context(), actor, project, asset)
		return err
	})
	return found, err
}

func TestOperationMediaReferenceGuardRetainsMediaMaskLegacyInputAndOutput(t *testing.T) {
	database, actor, project := operationReferenceFixture(t)
	_, task, input, _ := operationStoreRows(t, database, actor, project)
	assets := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	if err := database.Exec(`UPDATE operation.operation_input SET media_asset_id=?,mask_asset_id=? WHERE id=?`, assets[0], assets[1], input).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,ref_id)VALUES(?,?,2,'subject','media_asset',?)`, uuid.New(), task, assets[2]).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,sha256,moderation_status)VALUES(?,?,'image','upload','ready',?,'image/png',50,repeat('a',64),'passed')`, assets[3], project, "tests/reference/"+assets[3].String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO operation.operation_output(id,project_id,operation_id,seq_no,kind,media_asset_id,moderation_status)VALUES(?,?,?,0,'media',?,'passed')`, uuid.New(), project, task, assets[3]).Error; err != nil {
		t.Fatal(err)
	}
	for _, recycled := range []bool{false, true} {
		if recycled {
			for _, table := range []string{"operation_input", "operation_output", "operation"} {
				if err := database.Exec(`UPDATE operation.`+table+` SET is_delete=true WHERE `+map[string]string{"operation": "id", "operation_input": "operation_id", "operation_output": "operation_id"}[table]+`=?`, task).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
		for _, asset := range assets {
			if found, err := operationReferenceCheck(t, database, actor, project, asset); err != nil || !found {
				t.Fatal("lost retained task reference", recycled, asset, found, err)
			}
		}
		if found, err := operationReferenceCheck(t, database, actor, project, uuid.New()); err != nil || found {
			t.Fatal("unreferenced own asset", found, err)
		}
	}
}

func TestOperationMediaReferenceGuardRequiresCallerTransactionAndCurrentScope(t *testing.T) {
	database, actor, project := operationReferenceFixture(t)
	if _, err := pgoperation.NewMediaReferenceGuard(database).HasMediaReferences(t.Context(), actor, project, uuid.New()); err == nil {
		t.Fatal("pool accepted")
	}
	foreign, _ := operationStoreProject(t, database)
	if _, err := operationReferenceCheck(t, database, foreign, project, uuid.New()); !errors.Is(err, operationapp.ErrPublicNotFound) {
		t.Fatal("foreign scope", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := operationReferenceCheck(t, database, actor, project, uuid.New()); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("disabled actor", err)
	}
}

func TestOperationMediaReferenceGuardKeepsTaskProjectBoundary(t *testing.T) {
	database, actor, project := operationReferenceFixture(t)
	_, otherProject := operationStoreProject(t, database)
	_, task, input, _ := operationStoreRows(t, database, actor, otherProject)
	asset := uuid.New()
	if err := database.Exec(`UPDATE operation.operation_input SET media_asset_id=? WHERE id=?`, asset, input).Error; err != nil {
		t.Fatal(err)
	}
	if found, err := operationReferenceCheck(t, database, actor, project, asset); err != nil || found {
		t.Fatal("foreign task became own reference", task, found, err)
	}
}
