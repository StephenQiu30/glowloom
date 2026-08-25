package workflow_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestNodeCachePersistsOneImmutableWorkspaceScopedFactUnderConcurrency(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL node cache journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	now := time.Date(2026, time.August, 26, 3, 0, 0, 0, time.UTC)
	workspaceID := uuid.New()
	if err = database.Create(&model.Workspace{
		ID: workspaceID, Name: "Node Cache Test", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed cache workspace: %v", err)
	}

	material, cacheKey, err := workflow.BuildNodeCacheKey(workflow.NodeCacheKeyMaterial{
		SchemaVersion: workflow.NodeCacheKeySchemaVersion, NodeDefinitionContentHash: strings.Repeat("1", 64),
		ConfigHash: strings.Repeat("2", 64), NormalizedInputHash: strings.Repeat("3", 64),
		InputArtifactHashes: []string{strings.Repeat("4", 64)}, RuntimeContractVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("build cache key: %v", err)
	}
	output, outputHash, err := workflow.CanonicalNodeOutput(json.RawMessage(`{"artifact_ids":["artifact-1"],"binding":"production_bible"}`))
	if err != nil {
		t.Fatalf("build cache output: %v", err)
	}
	base := workflow.NodeCacheEntry{
		WorkspaceID: workspaceID.String(), CacheKey: cacheKey, KeyMaterial: material,
		Output: output, OutputHash: outputHash, SourceWorkflowRunID: uuid.NewString(),
		SourceNodeRunID: uuid.NewString(), CreatedAt: now,
	}
	store := workflowgorm.New(database)
	const callers = 8
	results := make(chan workflow.NodeCacheEntry, callers)
	errorsFound := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			desired := base
			desired.ID = uuid.NewString()
			persisted, ensureErr := store.EnsureNodeCache(ctx, desired)
			if ensureErr != nil {
				errorsFound <- ensureErr
				return
			}
			results <- persisted
		}()
	}
	wait.Wait()
	close(results)
	close(errorsFound)
	for ensureErr := range errorsFound {
		t.Fatalf("concurrent node cache ensure: %v", ensureErr)
	}
	var persistedID string
	for persisted := range results {
		if persistedID == "" {
			persistedID = persisted.ID
		}
		if persisted.ID != persistedID || persisted.CacheKey != cacheKey || persisted.OutputHash != outputHash ||
			string(persisted.Output) != string(output) {
			t.Fatalf("concurrent cache result drifted: %#v", persisted)
		}
	}
	var count int64
	if err = database.Model(&model.NodeCacheEntry{}).
		Where("workspace_id = ? AND cache_key = ?", workspaceID, cacheKey).Count(&count).Error; err != nil {
		t.Fatalf("count node cache entries: %v", err)
	}
	if count != 1 {
		t.Fatalf("node cache entry count = %d, want 1", count)
	}
	found, err := store.FindNodeCache(ctx, workspaceID.String(), cacheKey)
	if err != nil || found.ID != persistedID || found.OutputHash != outputHash || string(found.Output) != string(output) {
		t.Fatalf("find workspace node cache: entry=%#v err=%v", found, err)
	}
	if _, err = store.FindNodeCache(ctx, uuid.NewString(), cacheKey); err == nil {
		t.Fatal("node cache lookup crossed the workspace boundary")
	}

	drifted := base
	drifted.ID = uuid.NewString()
	drifted.Output, drifted.OutputHash, err = workflow.CanonicalNodeOutput(json.RawMessage(`{"artifact_ids":["artifact-2"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsureNodeCache(ctx, drifted); err == nil {
		t.Fatal("node cache accepted a different immutable output for the same key")
	}
	secondWorkspaceID := uuid.New()
	if err = database.Create(&model.Workspace{
		ID: secondWorkspaceID, Name: "Second Node Cache Test", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed second cache workspace: %v", err)
	}
	drifted.WorkspaceID = secondWorkspaceID.String()
	if crossWorkspace, ensureErr := store.EnsureNodeCache(ctx, drifted); ensureErr != nil || crossWorkspace.OutputHash != drifted.OutputHash {
		t.Fatalf("workspace-scoped cache identity leaked across tenants: entry=%#v err=%v", crossWorkspace, ensureErr)
	}
	if err = database.Model(&model.NodeCacheEntry{}).
		Where("cache_key = ? AND workspace_id IN ?", cacheKey, []uuid.UUID{workspaceID, secondWorkspaceID}).
		Count(&count).Error; err != nil {
		t.Fatalf("count cross-workspace cache entries: %v", err)
	}
	if count != 2 {
		t.Fatalf("cross-workspace node cache entry count = %d, want 2", count)
	}
}
