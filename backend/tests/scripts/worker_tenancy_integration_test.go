package scripts_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	"github.com/stephenqiu30/lanverse/backend/src/scripts"
)

func TestQueueAnalysisBindsWorkspaceAndProjectInOutbox(t *testing.T) {
	orm, cleanup := integrationDatabase(t)
	t.Cleanup(cleanup)

	workspaceID, projectID, revisionID := uuid.New(), uuid.New(), uuid.New()
	seedAnalysisTenant(t, orm, workspaceID, projectID, revisionID)
	t.Cleanup(func() { cleanupAnalysisTenant(t, orm, workspaceID, projectID, revisionID) })

	repository := scripts.NewScriptRepository(orm, nil)
	operation, err := repository.QueueAnalysis(database.WithWorkspaceID(context.Background(), workspaceID), revisionID)
	if err != nil {
		t.Fatalf("QueueAnalysis() error = %v", err)
	}
	events, err := repository.PendingOutbox(context.Background(), 10)
	if err != nil {
		t.Fatalf("PendingOutbox() error = %v", err)
	}
	if len(events) != 1 || events[0].OperationID != operation.ID {
		t.Fatalf("outbox events = %#v, want one event for operation %s", events, operation.ID)
	}

	var request scripts.AnalysisRequest
	if err := json.Unmarshal(events[0].Payload, &request); err != nil {
		t.Fatalf("decode analysis request: %v", err)
	}
	if request.WorkspaceID != workspaceID || request.ProjectID != projectID || request.RevisionID != revisionID {
		t.Fatalf("analysis request tenant binding = %#v", request)
	}
}

func TestProcessAnalysisRejectsCrossWorkspaceMessageBeforeStateChange(t *testing.T) {
	orm, cleanup := integrationDatabase(t)
	t.Cleanup(cleanup)

	workspaceA, projectA := uuid.New(), uuid.New()
	workspaceB, projectB, revisionB, operationB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	revisionB2 := uuid.New()
	seedWorkspaceProject(t, orm, workspaceA, projectA)
	seedAnalysisTenant(t, orm, workspaceB, projectB, revisionB)
	seedSourceRevision(t, orm, workspaceB, projectB, revisionB2, "other")
	if err := orm.Table("operations").Create(map[string]any{
		"id":         operationB,
		"project_id": projectB,
		"type":       "script_analysis",
		"status":     "queued",
		"progress":   0,
	}).Error; err != nil {
		t.Fatal(err)
	}
	boundRequest := scripts.AnalysisRequest{WorkspaceID: workspaceB, ProjectID: projectB, OperationID: operationB, RevisionID: revisionB}
	payload, err := json.Marshal(boundRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("outbox_events").Create(map[string]any{
		"id":           uuid.New(),
		"operation_id": operationB,
		"topic":        "lanverse.tasks.operation",
		"event_key":    revisionB.String(),
		"payload":      string(payload),
	}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupSourceRevision(t, orm, revisionB2)
		cleanupAnalysisTenant(t, orm, workspaceB, projectB, revisionB)
		cleanupAnalysisTenant(t, orm, workspaceA, projectA, uuid.Nil)
	})

	repository := scripts.NewScriptRepository(orm, nil)
	tests := []struct {
		name    string
		request scripts.AnalysisRequest
	}{
		{
			name: "foreign workspace",
			request: scripts.AnalysisRequest{
				WorkspaceID: workspaceA,
				ProjectID:   projectB,
				OperationID: operationB,
				RevisionID:  revisionB,
			},
		},
		{
			name: "foreign project",
			request: scripts.AnalysisRequest{
				WorkspaceID: workspaceB,
				ProjectID:   projectA,
				OperationID: operationB,
				RevisionID:  revisionB,
			},
		},
		{
			name: "missing tenant binding",
			request: scripts.AnalysisRequest{
				OperationID: operationB,
				RevisionID:  revisionB,
			},
		},
		{
			name: "different revision in same project",
			request: scripts.AnalysisRequest{
				WorkspaceID: workspaceB,
				ProjectID:   projectB,
				OperationID: operationB,
				RevisionID:  revisionB2,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := repository.ProcessAnalysis(context.Background(), test.request); err == nil {
				t.Fatal("ProcessAnalysis() accepted a tenant-mismatched message")
			}
			var operation struct {
				Status   string
				Progress int
			}
			if err := orm.Table("operations").Select("status", "progress").Where("id = ?", operationB).Take(&operation).Error; err != nil {
				t.Fatal(err)
			}
			if operation.Status != "queued" || operation.Progress != 0 {
				t.Fatalf("rejected message changed operation to %#v", operation)
			}
		})
	}
}

func integrationDatabase(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	if os.Getenv("LANVERSE_INTEGRATION") != "1" {
		t.Skip("set LANVERSE_INTEGRATION=1 to run PostgreSQL/GORM integration")
	}
	pool, err := database.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	orm, err := database.OpenGORM(pool)
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return orm, pool.Close
}

func seedAnalysisTenant(t *testing.T, orm *gorm.DB, workspaceID, projectID, revisionID uuid.UUID) {
	t.Helper()
	seedWorkspaceProject(t, orm, workspaceID, projectID)
	seedSourceRevision(t, orm, workspaceID, projectID, revisionID, "source")
}

func seedSourceRevision(t *testing.T, orm *gorm.DB, workspaceID, projectID, revisionID uuid.UUID, content string) {
	t.Helper()
	artifactID := uuid.New()
	contentHash := scripts.HashContent(content)
	if err := orm.Table("media_artifacts").Create(map[string]any{
		"id": artifactID, "workspace_id": workspaceID, "project_id": projectID, "content_hash": contentHash,
		"size_bytes": len(content), "media_type": "text/plain", "purpose": "source", "retention_class": "standard", "status": "ready",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("media_artifact_locations").Create(map[string]any{
		"id": uuid.New(), "artifact_id": artifactID, "storage_profile": "test", "bucket": "test", "object_key": uuid.NewString(),
		"object_version_id": uuid.NewString(), "size_bytes": len(content), "content_hash": contentHash, "status": "active",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("nar_source_revisions").Create(map[string]any{
		"id": revisionID, "project_id": projectID, "artifact_id": artifactID, "name": "worker-tenancy.txt",
		"source_type": "txt", "status": "uploaded",
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func seedWorkspaceProject(t *testing.T, orm *gorm.DB, workspaceID, projectID uuid.UUID) {
	t.Helper()
	if err := orm.Table("workspaces").Create(map[string]any{"id": workspaceID, "name": "worker tenancy workspace"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("projects").Create(map[string]any{"id": projectID, "workspace_id": workspaceID, "name": "worker tenancy project"}).Error; err != nil {
		t.Fatal(err)
	}
}

func cleanupAnalysisTenant(t *testing.T, orm *gorm.DB, workspaceID, projectID, revisionID uuid.UUID) {
	t.Helper()
	type cleanupRecord struct{ ID uuid.UUID }
	deleteBy := func(table, condition string, args ...any) {
		if result := orm.Table(table).Where(condition, args...).Delete(&cleanupRecord{}); result.Error != nil {
			t.Logf("cleanup %s: %v", table, result.Error)
		}
	}
	deleteBy("outbox_events", "operation_id IN (?)", orm.Table("operations").Select("id").Where("project_id = ?", projectID))
	deleteBy("operations", "project_id = ?", projectID)
	if revisionID != uuid.Nil {
		cleanupSourceRevision(t, orm, revisionID)
	}
	deleteBy("projects", "id = ?", projectID)
	deleteBy("workspaces", "id = ?", workspaceID)
}

func cleanupSourceRevision(t *testing.T, orm *gorm.DB, revisionID uuid.UUID) {
	t.Helper()
	var source struct{ ArtifactID uuid.UUID }
	if err := orm.Table("nar_source_revisions").Select("artifact_id").Where("id = ?", revisionID).Take(&source).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Logf("load source artifact for cleanup: %v", err)
		}
		return
	}
	deleteRecord := func(table, condition string, args ...any) {
		if result := orm.Table(table).Where(condition, args...).Delete(&struct{ ID uuid.UUID }{}); result.Error != nil {
			t.Logf("cleanup %s: %v", table, result.Error)
		}
	}
	deleteRecord("nar_source_revisions", "id = ?", revisionID)
	deleteRecord("media_artifact_locations", "artifact_id = ?", source.ArtifactID)
	deleteRecord("media_artifacts", "id = ?", source.ArtifactID)
}
