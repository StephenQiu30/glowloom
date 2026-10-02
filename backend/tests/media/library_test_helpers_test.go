package media_test

import (
	"os"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	platformdb "github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

// libraryOwnerDB is used only to create synthetic protected-column fixtures.
func libraryOwnerDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LV_TEST_LIBRARY_OWNER_DB_DSN")
	if dsn == "" {
		t.Skip("set the isolated library fixture owner DSN")
	}
	connection, err := platformdb.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal("open isolated library fixture database")
	}
	t.Cleanup(func() { _ = connection.Close() })
	var name, role, owner string
	if err := connection.DB.Raw(`SELECT current_database(),current_user,pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()`).Row().Scan(&name, &role, &owner); err != nil || name != "lanverse_library" || role == "lanverse_app" || role != owner {
		t.Fatal("owner fixture writes require isolated library database", name, role, err)
	}
	return connection.DB
}
