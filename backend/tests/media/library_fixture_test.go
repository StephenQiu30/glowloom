package media_test

import (
	"os"
	"testing"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func libraryRuntimeDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("LV_TEST_LIBRARY_OWNER_DB_DSN") == "" {
		t.Skip("set the isolated library fixture scope before running library integration tests")
	}
	db := mediaStoreDB(t)
	var name, role string
	if err := db.Raw(`SELECT current_database(),current_user`).Row().Scan(&name, &role); err != nil || name != "lanverse_library" || role != "lanverse_app" {
		t.Fatal("library tests require isolated database and actual nonowner role", name, role, err)
	}
	return db
}

func libraryTestAccess(tx *gorm.DB) mediaapp.LibraryProjectAccess {
	return app.NewMediaLibraryProjectAccess(tx)
}
