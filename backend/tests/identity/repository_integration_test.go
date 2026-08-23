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

	workspaceID, projectID, revisionID, artifactID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	t.Cleanup(func() {
		orm.Table("nar_source_revisions").Where("id = ?", revisionID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("media_artifact_locations").Where("artifact_id = ?", artifactID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("media_artifacts").Where("id = ?", artifactID).Delete(&struct{ ID uuid.UUID }{})
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
	contentHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := orm.Table("media_artifacts").Create(map[string]any{
		"id": artifactID, "workspace_id": workspaceID, "project_id": projectID, "content_hash": contentHash,
		"size_bytes": 1, "media_type": "text/plain", "purpose": "source", "retention_class": "standard", "status": "ready",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("media_artifact_locations").Create(map[string]any{
		"id": uuid.New(), "artifact_id": artifactID, "storage_profile": "test", "bucket": "test", "object_key": uuid.NewString(),
		"object_version_id": uuid.NewString(), "size_bytes": 1, "content_hash": contentHash, "status": "active",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("nar_source_revisions").Create(map[string]any{
		"id": revisionID, "project_id": projectID, "artifact_id": artifactID, "name": "identity-integration.txt",
		"source_type": "txt", "status": "uploaded",
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
