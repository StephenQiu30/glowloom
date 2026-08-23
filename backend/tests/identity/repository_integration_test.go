package identity_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	. "github.com/stephenqiu30/lanverse/backend/src/identity"
	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
)

func TestAuthorizePathUsesCanonicalProjectAndSourceRevisionTables(t *testing.T) {
	if os.Getenv("LANVERSE_INTEGRATION") != "1" {
		t.Skip("set LANVERSE_INTEGRATION=1 to run PostgreSQL/GORM integration")
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	orm, err := database.OpenGORM(pool)
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}

	workspaceID, projectID, revisionID := uuid.New(), uuid.New(), uuid.New()
	t.Cleanup(func() {
		orm.Table("nar_source_revisions").Where("id = ?", revisionID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("projects").Where("id = ?", projectID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("workspaces").Where("id = ?", workspaceID).Delete(&struct{ ID uuid.UUID }{})
		pool.Close()
	})

	if err := orm.Table("workspaces").Create(&struct {
		ID   uuid.UUID
		Name string
	}{workspaceID, "identity integration workspace"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("projects").Create(&struct {
		ID          uuid.UUID
		WorkspaceID uuid.UUID
		Name        string
	}{projectID, workspaceID, "identity integration project"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("nar_source_revisions").Create(map[string]any{
		"id":             revisionID,
		"project_id":     projectID,
		"name":           "identity-integration.txt",
		"object_key":     "identity-integration/" + revisionID.String() + ".txt",
		"content_hash":   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"content_length": 1,
		"source_type":    "txt",
		"status":         "uploaded",
	}).Error; err != nil {
		t.Fatal(err)
	}

	repository := NewIdentityRepository(orm)
	authorizedContext := database.WithWorkspaceID(ctx, workspaceID)
	for _, path := range []string{
		"/api/projects/" + projectID.String() + "/script-revisions",
		"/api/script-revisions/" + revisionID.String() + "/analysis",
	} {
		if err := repository.AuthorizePath(authorizedContext, workspaceID, path); err != nil {
			t.Fatalf("AuthorizePath(%q) error = %v", path, err)
		}
	}
}
