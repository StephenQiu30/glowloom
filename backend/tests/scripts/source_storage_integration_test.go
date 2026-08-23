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
	mu             sync.Mutex
	objects        map[string][]byte
	lastPutKey     string
	lastGetKey     string
	lastGetVersion string
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
	return objectstorage.ObjectVersion{StorageProfile: "test-primary", Bucket: "test-media", Key: key, VersionID: versionID, ETag: "fixture-etag", Size: int64(len(content))}, nil
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
	return nil
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
