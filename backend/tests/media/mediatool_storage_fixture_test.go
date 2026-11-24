package media_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	toolpg "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/adapter/postgres"
	toolflow "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/adapter/workflow"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/application"
)

// mediatoolFixtureDB activates the explicit task-owned storage configuration.
// Every business operation continues to execute under the runtime role.
func mediatoolFixtureDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := mediaStoreDB(t)
	_, cfg := mediaFixtureRegistry(t)
	routes := pgmedia.NewStorageRoutes(db)
	for id, p := range cfg.Profiles {
		if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{{ID: id, Provider: string(p.Provider), Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Region: p.Region, Bucket: p.Bucket, AddressingStyle: string(p.AddressingStyle)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := routes.ApplyEnvironment(t.Context(), storageEnvironmentFixture(t, db, "tool-fixture-"+uuid.NewString(), cfg.DefaultWriteProfile)); err != nil {
		t.Fatal(err)
	}
	return db
}

func mediatoolStorageLedger(tx *gorm.DB) toolapp.ToolStorageLedger {
	return pgmedia.NewStorageRoutes(tx)
}

func mediatoolExportStore(db *gorm.DB, timeline toolpg.TimelineFactory, media toolpg.MediaFactory) *toolpg.Store {
	return toolpg.NewStore(db, timeline, media).WithStorageAdmission(mediatoolStorageLedger)
}
func mediatoolDepthStore(db *gorm.DB, source toolpg.SavedMediaSourceFactory, media toolpg.DepthMediaFactory) *toolpg.DepthStore {
	return toolpg.NewDepthStore(db, source, media).WithStorageAdmission(mediatoolStorageLedger)
}
func mediatoolRoutedObjects(t *testing.T, db *gorm.DB) *toolflow.RoutedObjects {
	t.Helper()
	registry, _ := mediaFixtureRegistry(t)
	return toolflow.NewRoutedObjects(registry, pgmedia.NewStorageRoutes(db))
}

// mediatoolFixtureExec injects protected metadata as the isolated database owner.
// An outer owner transaction may impersonate the runtime role to keep lock tests
// rolled back; every injected statement restores that runtime role immediately.
func mediatoolFixtureExec(t *testing.T, db *gorm.DB, query string, values ...any) *gorm.DB {
	t.Helper()
	if _, inTransaction := db.Statement.ConnPool.(gorm.TxCommitter); !inTransaction {
		return libraryOwnerDB(t).Exec(query, values...)
	}
	if err := db.Exec(`RESET ROLE`).Error; err != nil {
		t.Fatal(err)
	}
	result := db.Exec(query, values...)
	if result.Error == nil {
		if err := db.Exec(`SET LOCAL ROLE glowloom_app`).Error; err != nil {
			t.Fatal(err)
		}
	}
	return result
}

// mediatoolFixtureLocation records a reserved identity only, without claiming
// provider bytes exist. Metadata retention and malformed-proof cases use it.
func mediatoolFixtureLocation(t *testing.T, db *gorm.DB, kind mediaapp.StorageWorkKind, project, job uuid.UUID, attempt int, key string) uuid.UUID {
	t.Helper()
	routes := pgmedia.NewStorageRoutes(db)
	intent, err := routes.PinStorageWork(t.Context(), toolapp.StorageOwner(kind, project, job, attempt))
	if err != nil {
		t.Fatal(err)
	}
	location, err := routes.ReserveWorkLocation(t.Context(), intent.ID, key)
	if err != nil {
		t.Fatal(err)
	}
	return location.ID
}
