package workflow_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	workflowauthoring "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/authoring"
	workflowgorm "github.com/StephenQiu30/lanverse/backend/internal/workflow/adapter/gormdb"
	workflowapp "github.com/StephenQiu30/lanverse/backend/internal/workflow/application"
	workflow "github.com/StephenQiu30/lanverse/backend/internal/workflow/domain"
)

func TestCompilerPersistsOneImmutableDefinitionAndRunInputSnapshot(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL workflow compiler journey")
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
		t.Fatal(err)
	}
	authoringStore := authoringgorm.New(database)
	now := time.Date(2026, time.August, 25, 19, 0, 0, 0, time.UTC)
	if _, err = authoringStore.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	authoringService := authoringapp.NewService(authoringStore, authoringapp.Config{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	authoringActor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	frozenInput := authoring.FrozenReference{
		Kind: "script_revision", ID: fixture.scriptRevisionID.String(), Version: "1", Hash: fixture.normalizedHash,
	}
	draft, err := authoringService.Create(ctx, authoringActor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "GUIDED", Graph: compilerJourneyGraph(),
		Layout: json.RawMessage(`{"guided":{"step":1}}`), FrozenInputs: []authoring.FrozenReference{frozenInput},
		CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "compiler-authoring-create-1",
	})
	if err != nil {
		t.Fatalf("create authoring draft: %v", err)
	}
	revision, err := authoringService.Publish(ctx, authoringActor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "compiler-authoring-publish-1",
	})
	if err != nil {
		t.Fatalf("publish authoring revision: %v", err)
	}

	workflowStore := workflowgorm.New(database)
	workflowNow := now.Add(time.Minute)
	service := workflowapp.NewService(
		workflowauthoring.New(authoringService), workflowStore, workflow.SystemCompilerContract(),
		workflowapp.Config{
			Now:   func() time.Time { return workflowNow },
			NewID: uuid.NewString,
		},
	)
	actor := workflowapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	const concurrentCompilers = 6
	start := make(chan struct{})
	results := make(chan workflow.CompiledFacts, concurrentCompilers)
	errorsFound := make(chan error, concurrentCompilers)
	var group sync.WaitGroup
	for index := range concurrentCompilers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			compiled, compileErr := service.Compile(ctx, actor, workflowapp.CompileCommand{
				AuthoringRevisionID: revision.ID, IdempotencyKey: fmt.Sprintf("workflow-compile-concurrent-%d", index),
			})
			if compileErr != nil {
				errorsFound <- compileErr
				return
			}
			results <- compiled
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsFound)
	for compileErr := range errorsFound {
		t.Fatalf("concurrent compile published revision: %v", compileErr)
	}
	var first workflow.CompiledFacts
	for compiled := range results {
		if first.DefinitionID == "" {
			first = compiled
			continue
		}
		if compiled.DefinitionID != first.DefinitionID || compiled.RunInputSnapshotID != first.RunInputSnapshotID {
			t.Fatalf("concurrent compilers diverged: first=%#v current=%#v", first, compiled)
		}
	}
	if _, err = uuid.Parse(first.DefinitionID); err != nil {
		t.Fatalf("invalid definition id: %q", first.DefinitionID)
	}
	if _, err = uuid.Parse(first.RunInputSnapshotID); err != nil {
		t.Fatalf("invalid input snapshot id: %q", first.RunInputSnapshotID)
	}
	if first.Definition.AuthoringRevisionID != revision.ID || first.Definition.ContentHash == "" || first.RunInputSnapshot.ContentHash == "" {
		t.Fatalf("incomplete persisted compilation: %#v", first)
	}

	replayed, err := service.Compile(ctx, actor, workflowapp.CompileCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-compile-concurrent-0",
	})
	if err != nil || replayed.DefinitionID != first.DefinitionID || replayed.RunInputSnapshotID != first.RunInputSnapshotID {
		t.Fatalf("compile command replay diverged: first=%#v replayed=%#v err=%v", first, replayed, err)
	}
	converged, err := service.Compile(ctx, actor, workflowapp.CompileCommand{
		AuthoringRevisionID: revision.ID, IdempotencyKey: "workflow-compile-2",
	})
	if err != nil || converged.DefinitionID != first.DefinitionID || converged.RunInputSnapshotID != first.RunInputSnapshotID {
		t.Fatalf("same immutable source created duplicate facts: first=%#v converged=%#v err=%v", first, converged, err)
	}
	if _, err = service.Compile(ctx, actor, workflowapp.CompileCommand{
		AuthoringRevisionID: draft.ID, IdempotencyKey: "workflow-compile-draft-1",
	}); err == nil {
		t.Fatal("workflow compiler accepted a mutable AuthoringDraft identity")
	}

	var definitionCount, snapshotCount int64
	if err = database.Model(&model.WorkflowDefinitionVersion{}).Count(&definitionCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.RunInputSnapshot{}).Count(&snapshotCount).Error; err != nil {
		t.Fatal(err)
	}
	if definitionCount != 1 || snapshotCount != 1 {
		t.Fatalf("unexpected workflow compiler facts: definitions=%d snapshots=%d", definitionCount, snapshotCount)
	}
	var persisted model.WorkflowDefinitionVersion
	if err = database.First(&persisted, "id = ?", first.DefinitionID).Error; err != nil {
		t.Fatalf("reload definition: %v", err)
	}
	if persisted.AuthoringRevisionID.String() != revision.ID || persisted.ContentHash != first.Definition.ContentHash ||
		strings.Contains(string(persisted.Definition), "layout") {
		t.Fatalf("persisted definition crossed immutable execution boundary: %#v", persisted)
	}
}

type compilerProjectFixture struct {
	userID, projectID, scriptRevisionID uuid.UUID
	normalizedHash                      string
}

func seedCompilerProject(t *testing.T, create func(any) error, now time.Time) compilerProjectFixture {
	t.Helper()
	userID, workspaceID, projectID := uuid.New(), uuid.New(), uuid.New()
	documentID, revisionID := uuid.New(), uuid.New()
	normalizedHash := strings.Repeat("2", 64)
	records := []any{
		&model.UserAccount{ID: userID, EmailNormalized: userID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "Compiler Test", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: workspaceID, Name: "Compiler Test", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "owner", Status: "active", JoinedAt: now},
		&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "Compiler Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 90_000, BudgetLimit: decimal.Zero, Currency: "CNY", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.ScriptDocument{ID: documentID, WorkspaceID: workspaceID, ProjectID: projectID, Title: "Compiler Script", SourceType: "text", Language: "zh-CN", RightsDeclaration: "原创测试文本", Status: "active", Revision: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.DocumentRevision{ID: revisionID, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: 1, SourceType: "text", RawText: "雨巷，夜", RawHash: strings.Repeat("1", 64), NormalizedText: "雨巷，夜", NormalizedHash: normalizedHash, NormalizerVersion: "test-v1", NormalizationMap: []byte(`{}`), CodepointCount: 4, AnalysisStatus: "deterministic", AnalyzerVersion: "test-v1", Blocks: []byte(`[]`), Issues: []byte(`[]`), CreatedBy: userID, CreatedAt: now},
	}
	for _, record := range records {
		if err := create(record); err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}
	return compilerProjectFixture{userID: userID, projectID: projectID, scriptRevisionID: revisionID, normalizedHash: normalizedHash}
}
