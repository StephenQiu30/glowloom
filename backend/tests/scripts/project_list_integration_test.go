package scripts_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	"github.com/stephenqiu30/lanverse/backend/src/platform/messaging"
	. "github.com/stephenqiu30/lanverse/backend/src/scripts"
)

func TestListWorkspaceProjectsReturnsOnlyTenantAndLatestRestorableWorkflow(t *testing.T) {
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

	workspaceID, foreignWorkspaceID := uuid.New(), uuid.New()
	projectID, emptyProjectID, foreignProjectID := uuid.New(), uuid.New(), uuid.New()
	revisionID, operationID, artifactID := uuid.New(), uuid.New(), uuid.New()
	createdAt := time.Now().UTC()
	t.Cleanup(func() {
		orm.Table("outbox_events").Where("operation_id = ?", operationID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("operations").Where("id = ?", operationID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("nar_source_revisions").Where("id = ?", revisionID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("media_artifacts").Where("id = ?", artifactID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("projects").Where("id IN ?", []uuid.UUID{projectID, emptyProjectID, foreignProjectID}).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("workspaces").Where("id IN ?", []uuid.UUID{workspaceID, foreignWorkspaceID}).Delete(&struct{ ID uuid.UUID }{})
		pool.Close()
	})

	for _, workspace := range []map[string]any{
		{"id": workspaceID, "name": "project list workspace"},
		{"id": foreignWorkspaceID, "name": "foreign workspace"},
	} {
		if err := orm.Table("workspaces").Create(workspace).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, project := range []map[string]any{
		{"id": projectID, "workspace_id": workspaceID, "name": "可恢复项目", "created_at": createdAt.Add(-time.Minute)},
		{"id": emptyProjectID, "workspace_id": workspaceID, "name": "空项目", "created_at": createdAt},
		{"id": foreignProjectID, "workspace_id": foreignWorkspaceID, "name": "其他租户项目", "created_at": createdAt.Add(time.Minute)},
	} {
		if err := orm.Table("projects").Create(project).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := orm.Table("media_artifacts").Create(map[string]any{
		"id": artifactID, "workspace_id": workspaceID, "project_id": projectID,
		"content_hash": strings.Repeat("a", 64), "size_bytes": 1, "media_type": "text/plain",
		"purpose": "source", "retention_class": "standard", "status": "ready",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("nar_source_revisions").Create(map[string]any{
		"id": revisionID, "project_id": projectID, "artifact_id": artifactID,
		"name": "cross-device.txt", "source_type": "txt", "status": "approved",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("operations").Create(map[string]any{
		"id": operationID, "project_id": projectID, "type": "script_analysis",
		"status": "succeeded", "progress": 100, "created_at": createdAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(AnalysisRequest{
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		OperationID: operationID,
		RevisionID:  revisionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("outbox_events").Create(map[string]any{
		"id": uuid.New(), "operation_id": operationID, "topic": messaging.OperationTaskTopic,
		"event_key": revisionID.String(), "payload": payload,
	}).Error; err != nil {
		t.Fatal(err)
	}

	repository := NewScriptRepository(orm, nil)
	page, err := repository.ListProjects(
		database.WithWorkspaceID(ctx, workspaceID),
		workspaceID,
		ProjectQuery{Page: 1, PageSize: 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("project page = %#v, want two current-tenant projects", page)
	}
	if page.Items[0].ID != emptyProjectID || page.Items[0].LatestWorkflow != nil {
		t.Fatalf("newest empty project = %#v", page.Items[0])
	}
	project := page.Items[1]
	if project.ID != projectID || project.LatestWorkflow == nil {
		t.Fatalf("restorable project = %#v", project)
	}
	if project.LatestWorkflow.ProjectID != projectID ||
		project.LatestWorkflow.SourceRevisionID != revisionID ||
		project.LatestWorkflow.OperationID != operationID ||
		project.LatestWorkflow.OperationStatus != "succeeded" ||
		project.LatestWorkflow.SourceStatus != "approved" {
		t.Fatalf("latest workflow = %#v", project.LatestWorkflow)
	}
}
