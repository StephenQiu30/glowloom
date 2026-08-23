package scripts_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/database"
	"github.com/stephenqiu30/lanverse/backend/src/platform/objectstorage"
	"github.com/stephenqiu30/lanverse/backend/src/scripts"
)

type versionedMemoryStore struct {
	mu              sync.Mutex
	objects         map[string][]byte
	lastPutKey      string
	lastGetKey      string
	lastGetVersion  string
	putCount        int
	deleteCount     int
	receiptOverride *objectstorage.ObjectVersion
}

func (s *versionedMemoryStore) PutVersioned(_ context.Context, key string, content []byte, _ string) (objectstorage.ObjectVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	versionID := uuid.NewString()
	s.objects[key+"@"+versionID] = append([]byte(nil), content...)
	s.lastPutKey = key
	s.putCount++
	receipt := objectstorage.ObjectVersion{StorageProfile: "test-primary", Bucket: "test-media", Key: key, VersionID: versionID, ETag: "fixture-etag", Size: int64(len(content))}
	if s.receiptOverride != nil {
		receipt = *s.receiptOverride
	}
	return receipt, nil
}

func TestFixtureCandidateUsesOneOpaqueVersionedArtifact(t *testing.T) {
	orm, closeDatabase := integrationDatabase(t)
	t.Cleanup(closeDatabase)

	workspaceID, projectID, contentUnitID, shotID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	seedWorkspaceProject(t, orm, workspaceID, projectID)
	if err := orm.Table("prj_content_units").Create(map[string]any{"id": contentUnitID, "project_id": projectID, "kind": "episode", "title": "fixture", "status": "active", "ordinal": 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("sht_shots").Create(map[string]any{"id": shotID, "project_id": projectID, "content_unit_id": contentUnitID, "shot_key": "S001", "ordinal": 1, "status": "draft"}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deleteRecord := func(table, condition string, args ...any) {
			if result := orm.Table(table).Where(condition, args...).Delete(&struct{ ID uuid.UUID }{}); result.Error != nil {
				t.Logf("cleanup %s: %v", table, result.Error)
			}
		}
		var artifactIDs, jobIDs, planIDs, operationIDs []uuid.UUID
		orm.Table("media_artifacts").Where("project_id = ?", projectID).Pluck("id", &artifactIDs)
		orm.Table("exec_generation_jobs").Where("operation_id IN (?)", orm.Table("operations").Select("id").Where("project_id = ?", projectID)).Pluck("id", &jobIDs)
		orm.Table("gen_plans").Where("project_id = ?", projectID).Pluck("id", &planIDs)
		orm.Table("operations").Where("project_id = ?", projectID).Pluck("id", &operationIDs)
		deleteRecord("media_candidates", "project_id = ?", projectID)
		deleteRecord("media_artifact_locations", "artifact_id IN ?", artifactIDs)
		deleteRecord("media_artifacts", "id IN ?", artifactIDs)
		deleteRecord("exec_attempts", "job_id IN ?", jobIDs)
		deleteRecord("exec_generation_jobs", "id IN ?", jobIDs)
		deleteRecord("gen_plan_items", "plan_id IN ?", planIDs)
		deleteRecord("gen_plans", "id IN ?", planIDs)
		deleteRecord("operations", "id IN ?", operationIDs)
		deleteRecord("sht_shots", "id = ?", shotID)
		deleteRecord("prj_content_units", "id = ?", contentUnitID)
		deleteRecord("projects", "id = ?", projectID)
		deleteRecord("workspaces", "id = ?", workspaceID)
	})

	store := &versionedMemoryStore{}
	repository := scripts.NewScriptRepository(orm, store)
	tenantContext := database.WithWorkspaceID(context.Background(), workspaceID)
	first, err := repository.CreateFixtureCandidate(tenantContext, shotID, "storyboard")
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.CreateFixtureCandidate(tenantContext, shotID, "storyboard")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.ArtifactID != second.ArtifactID || store.putCount != 1 {
		t.Fatalf("duplicate fixture command produced a second side effect: first=%#v second=%#v puts=%d", first, second, store.putCount)
	}
	if strings.Contains(store.lastPutKey, workspaceID.String()) || strings.Contains(store.lastPutKey, projectID.String()) || strings.Contains(store.lastPutKey, shotID.String()) {
		t.Fatalf("fixture object key exposes a business identifier: %q", store.lastPutKey)
	}
	var location struct {
		ObjectVersionID string
		Status          string
	}
	if err := orm.Table("media_artifact_locations").Select("object_version_id", "status").Where("artifact_id = ?", first.ArtifactID).Take(&location).Error; err != nil {
		t.Fatal(err)
	}
	if location.ObjectVersionID == "" || location.Status != "active" {
		t.Fatalf("fixture location = %#v", location)
	}
}

func (s *versionedMemoryStore) GetVersioned(_ context.Context, key, versionID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastGetKey, s.lastGetVersion = key, versionID
	content, ok := s.objects[key+"@"+versionID]
	if !ok {
		return nil, fmt.Errorf("object version not found")
	}
	return append([]byte(nil), content...), nil
}

func (s *versionedMemoryStore) DeleteVersion(_ context.Context, key, versionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key+"@"+versionID)
	s.deleteCount++
	return nil
}

func TestSourceRevisionRejectsIncompleteStorageReceipt(t *testing.T) {
	orm, closeDatabase := integrationDatabase(t)
	t.Cleanup(closeDatabase)

	workspaceID, projectID := uuid.New(), uuid.New()
	seedWorkspaceProject(t, orm, workspaceID, projectID)
	t.Cleanup(func() {
		orm.Table("projects").Where("id = ?", projectID).Delete(&struct{ ID uuid.UUID }{})
		orm.Table("workspaces").Where("id = ?", workspaceID).Delete(&struct{ ID uuid.UUID }{})
	})

	store := &versionedMemoryStore{receiptOverride: &objectstorage.ObjectVersion{StorageProfile: "test-primary", Bucket: "test-media", Size: 6}}
	repository := scripts.NewScriptRepository(orm, store)
	_, err := repository.CreateScriptRevision(database.WithWorkspaceID(context.Background(), workspaceID), projectID, scripts.SourceUpload{FileName: "invalid-receipt.txt", MediaType: "text/plain", Original: []byte("source")})
	if err == nil {
		t.Fatal("CreateScriptRevision() accepted a receipt without key and object version")
	}
	var sources, artifacts int64
	if err := orm.Table("nar_source_revisions").Where("project_id = ?", projectID).Count(&sources).Error; err != nil {
		t.Fatal(err)
	}
	if err := orm.Table("media_artifacts").Where("project_id = ?", projectID).Count(&artifacts).Error; err != nil {
		t.Fatal(err)
	}
	if sources != 0 || artifacts != 0 || store.deleteCount != 0 {
		t.Fatalf("incomplete receipt side effects: sources=%d artifacts=%d unsafe_deletes=%d", sources, artifacts, store.deleteCount)
	}
}

func TestSourceRevisionFreezesOpaqueExactObjectVersion(t *testing.T) {
	orm, closeDatabase := integrationDatabase(t)
	t.Cleanup(closeDatabase)

	workspaceID, projectID := uuid.New(), uuid.New()
	seedWorkspaceProject(t, orm, workspaceID, projectID)
	var revisionID, artifactID uuid.UUID
	t.Cleanup(func() {
		deleteRecord := func(table, condition string, args ...any) {
			if result := orm.Table(table).Where(condition, args...).Delete(&struct{ ID uuid.UUID }{}); result.Error != nil {
				t.Logf("cleanup %s: %v", table, result.Error)
			}
		}
		deleteRecord("nar_analysis_drafts", "source_revision_id = ?", revisionID)
		deleteRecord("outbox_events", "operation_id IN (?)", orm.Table("operations").Select("id").Where("project_id = ?", projectID))
		deleteRecord("operations", "project_id = ?", projectID)
		deleteRecord("nar_source_revisions", "id = ?", revisionID)
		deleteRecord("media_artifact_locations", "artifact_id = ?", artifactID)
		deleteRecord("media_artifacts", "id = ?", artifactID)
		deleteRecord("projects", "id = ?", projectID)
		deleteRecord("workspaces", "id = ?", workspaceID)
	})

	store := &versionedMemoryStore{}
	repository := scripts.NewScriptRepository(orm, store)
	content := []byte("第1集 归途\n场景：码头\n人物：林夏\n林夏：出发。\n")
	tenantContext := database.WithWorkspaceID(context.Background(), workspaceID)
	revision, err := repository.CreateScriptRevision(tenantContext, projectID, scripts.SourceUpload{FileName: "private-script.txt", MediaType: "text/plain", Original: content})
	if err != nil {
		t.Fatalf("CreateScriptRevision() error = %v", err)
	}
	revisionID = revision.ID
	if strings.Contains(store.lastPutKey, workspaceID.String()) || strings.Contains(store.lastPutKey, projectID.String()) || strings.Contains(store.lastPutKey, revisionID.String()) || strings.Contains(store.lastPutKey, "private-script") {
		t.Fatalf("object key exposes a business identifier: %q", store.lastPutKey)
	}

	var source struct{ ArtifactID uuid.UUID }
	if err := orm.Table("nar_source_revisions").Select("artifact_id").Where("id = ?", revisionID).Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	artifactID = source.ArtifactID
	if artifactID == uuid.Nil {
		t.Fatal("source revision has no logical artifact")
	}
	var location struct {
		ObjectKey       string
		ObjectVersionID string
		Status          string
	}
	if err := orm.Table("media_artifact_locations").Select("object_key", "object_version_id", "status").Where("artifact_id = ?", artifactID).Take(&location).Error; err != nil {
		t.Fatal(err)
	}
	if location.ObjectKey != store.lastPutKey || location.ObjectVersionID == "" || location.Status != "active" {
		t.Fatalf("stored exact location = %#v", location)
	}

	operation, err := repository.QueueAnalysis(tenantContext, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := repository.PendingOutbox(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var request scripts.AnalysisRequest
	for _, event := range events {
		if event.OperationID == operation.ID {
			if err := json.Unmarshal(event.Payload, &request); err != nil {
				t.Fatal(err)
			}
		}
	}
	if request.OperationID == uuid.Nil {
		t.Fatal("analysis outbox event not found")
	}
	if err := repository.ProcessAnalysis(context.Background(), request); err != nil {
		t.Fatalf("ProcessAnalysis() error = %v", err)
	}
	if store.lastGetKey != location.ObjectKey || store.lastGetVersion != location.ObjectVersionID {
		t.Fatalf("analysis read %q@%q, want %q@%q", store.lastGetKey, store.lastGetVersion, location.ObjectKey, location.ObjectVersionID)
	}
}
