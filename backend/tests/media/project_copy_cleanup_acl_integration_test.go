package media_test

import (
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func TestProjectCopyCleanupColumnGrantsAndDownRemainClosed(t *testing.T) {
	runtime := mediaStoreDB(t)
	owner := libraryOwnerDB(t)
	var name, role string
	if err := runtime.Raw(`SELECT current_database(),current_user`).Row().Scan(&name, &role); err != nil || name != "lanverse_library" || role != "lanverse_app" {
		t.Fatal("cleanup ACL test requires isolated library database and runtime role", name, role, err)
	}
	for _, column := range []string{"project_id", "object_key"} {
		err := runtime.Exec(`UPDATE media.media_asset SET ` + column + `=` + column + ` WHERE false`).Error
		var databaseError *pgconn.PgError
		if !errors.As(err, &databaseError) || databaseError.Code != "42501" {
			t.Fatal("cleanup broadened asset ownership authority", column, err)
		}
	}
	up, err := os.ReadFile("../../db/migrations/202610020054_media_project_copy_cleanup.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("../../db/migrations/202610020054_media_project_copy_cleanup.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	// DDL is exercised in an outer rolled-back fixture transaction, preserving the
	// actual applied migration used by other nonowner integration tests.
	tx := owner.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	privileges := func(deletion bool) {
		t.Helper()
		for _, column := range []string{"is_delete", "delete_time", "purge_after", "revision", "sha256", "project_id", "object_key"} {
			want := column == "revision" || column == "sha256" || deletion && (column == "is_delete" || column == "delete_time" || column == "purge_after")
			var actual bool
			if err := tx.Raw(`SELECT has_column_privilege('lanverse_app','media.media_asset',?,'UPDATE')`, column).Scan(&actual).Error; err != nil || actual != want {
				t.Fatal("migration changed unrelated column authority", column, actual, want, err)
			}
		}
		var canDelete bool
		if err := tx.Raw(`SELECT has_table_privilege('lanverse_app','media.media_asset','DELETE')`).Scan(&canDelete).Error; err != nil || canDelete {
			t.Fatal("cleanup acquired physical asset DELETE", err)
		}
	}
	privileges(true)
	if err := tx.Exec(string(down)).Error; err != nil {
		t.Fatal(err)
	}
	privileges(false)
	if err := tx.Exec(string(up)).Error; err != nil {
		t.Fatal(err)
	}
	privileges(true)
}

func TestProjectCopyCleanupKeepsSourceMetadataAndObjects(t *testing.T) {
	database := mediaStoreDB(t)
	objects := glbTestObjects(t)
	actor, binding, snapshot, keys := seedCopyMedia(t, database, objects)
	var before struct {
		ID         string
		IsDelete   bool
		Revision   int64
		DeleteTime *time.Time
		PurgeAfter *time.Time
		ObjectKey  string
	}
	if err := database.Raw(`SELECT id,is_delete,revision,delete_time,purge_after,object_key FROM media.media_asset WHERE project_id=?`, binding.SourceProjectID).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	repo := pgmedia.NewProjectCopyStore(database)
	transfer := mediaapp.NewProjectCopyTransfer(repo, copyobjects.NewProjectCopyObjects(objects), t.TempDir())
	if err := transfer.Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := transfer.Cleanup(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		return pgmedia.NewProjectCopyStore(tx).FinishCleanup(t.Context(), actor, binding, snapshot)
	}); err != nil {
		t.Fatal("actual nonowner cleanup", err)
	}
	after := before
	if err := database.Raw(`SELECT id,is_delete,revision,delete_time,purge_after,object_key FROM media.media_asset WHERE project_id=?`, binding.SourceProjectID).Scan(&after).Error; err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("cleanup modified source lifecycle", err)
	}
	for _, key := range keys[:2] {
		present, err := objects.Exists(t.Context(), key)
		if err != nil || !present {
			t.Fatal("cleanup removed source bytes", err)
		}
	}
}
