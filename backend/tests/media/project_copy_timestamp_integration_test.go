package media_test

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func TestProjectCopyMediaVerificationPreservesTimestampInstantsAcrossSessionTimezones(t *testing.T) {
	database := mediaStoreDB(t)
	objects := glbTestObjects(t)
	actor, binding, snapshot, _ := seedCopyMedia(t, database, objects)
	transfer := mediaapp.NewProjectCopyTransfer(pgmedia.NewProjectCopyStore(database), copyobjects.NewProjectCopyObjects(objects), t.TempDir())
	if err := transfer.Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal("transfer synthetic copy", err)
	}
	var receipt mediaapp.ProjectCopyReceipt
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL TIME ZONE 'UTC'`).Error; err != nil {
			return err
		}
		var err error
		receipt, err = pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal("register complete metadata under UTC", err)
	}
	for _, zone := range []string{"Asia/Shanghai", "America/New_York", "UTC"} {
		t.Run(zone, func(t *testing.T) {
			if err := database.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec(`SELECT set_config('TimeZone', ?, true)`, zone).Error; err != nil {
					return err
				}
				actual, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
				if err == nil && actual != receipt {
					t.Fatal("session timezone changed frozen content receipt")
				}
				return err
			}); err != nil {
				t.Fatal("same persisted timestamp instant must verify across timezone representations", err)
			}
		})
	}
	for _, target := range snapshot.AssetMapping {
		if err := database.Exec(`UPDATE media.media_asset SET update_time=update_time+interval '1 microsecond' WHERE id=? AND project_id=?`, target, binding.TargetProjectID).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := pgmedia.NewProjectCopyStore(tx).Register(t.Context(), actor, binding, snapshot)
		return err
	}); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("actual changed timestamp must remain a mismatched copy", err)
	}
}
