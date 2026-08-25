package authoring_test

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
)

func TestSystemNodeCatalogIsPersistedOnceAndRejectsVersionDrift(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL authoring journey")
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

	catalog, err := authoring.SystemCatalog()
	if err != nil {
		t.Fatalf("build system catalog: %v", err)
	}
	store := authoringgorm.New(database)
	now := time.Date(2026, time.August, 25, 15, 0, 0, 0, time.UTC)
	firstID, err := store.EnsureCatalog(ctx, catalog, now, uuid.NewString)
	if err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	secondID, err := store.EnsureCatalog(ctx, catalog, now.Add(time.Minute), uuid.NewString)
	if err != nil {
		t.Fatalf("reconcile identical system catalog: %v", err)
	}
	if firstID != secondID {
		t.Fatalf("catalog retry created a new identity: first=%s second=%s", firstID, secondID)
	}

	var definitionCount, catalogCount int64
	if err = database.Model(&model.NodeDefinitionVersion{}).Count(&definitionCount).Error; err != nil {
		t.Fatalf("count node definitions: %v", err)
	}
	if err = database.Model(&model.NodeCatalogVersion{}).Count(&catalogCount).Error; err != nil {
		t.Fatalf("count node catalogs: %v", err)
	}
	if definitionCount != int64(len(catalog.Definitions)) || catalogCount != 1 {
		t.Fatalf("unexpected catalog rows: definitions=%d catalogs=%d", definitionCount, catalogCount)
	}

	var persisted model.NodeCatalogVersion
	if err = database.First(&persisted, "id = ?", firstID).Error; err != nil {
		t.Fatalf("reload system catalog: %v", err)
	}
	if persisted.ContentHash != catalog.ContentHash || persisted.ExecutionHash != catalog.ExecutionHash || persisted.Status != "published" {
		t.Fatalf("persisted catalog differs from domain catalog: %#v", persisted)
	}

	driftedDefinitions := append([]authoring.NodeDefinition(nil), catalog.Definitions...)
	driftedDefinitions[0].Name += " Drifted"
	drifted, err := authoring.NewCatalog(catalog.Key, catalog.Version, driftedDefinitions)
	if err != nil {
		t.Fatalf("build drifted catalog: %v", err)
	}
	if _, err = store.EnsureCatalog(ctx, drifted, now.Add(2*time.Minute), uuid.NewString); err == nil {
		t.Fatal("same catalog version accepted changed node definitions")
	}
	if err = database.Model(&model.NodeCatalogVersion{}).Count(&catalogCount).Error; err != nil {
		t.Fatalf("recount node catalogs: %v", err)
	}
	if catalogCount != 1 {
		t.Fatalf("drift reconciliation created a second catalog: %d", catalogCount)
	}
}
