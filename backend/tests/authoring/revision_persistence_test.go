package authoring_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authoringgorm "github.com/StephenQiu30/lanverse/backend/internal/authoring/adapter/gormdb"
	authoringapp "github.com/StephenQiu30/lanverse/backend/internal/authoring/application"
	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
)

func TestAuthoringDraftPublishesImmutableRevisionsFromVerifiedInputs(t *testing.T) {
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
		t.Fatal(err)
	}
	store := authoringgorm.New(database)
	now := time.Date(2026, time.August, 25, 16, 0, 0, 0, time.UTC)
	if _, err = store.EnsureCatalog(ctx, catalog, now, uuid.NewString); err != nil {
		t.Fatalf("persist system catalog: %v", err)
	}
	fixture := seedAuthoringProject(t, func(value any) error { return database.Create(value).Error }, now)
	service := authoringapp.NewService(store, authoringapp.Config{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: uuid.NewString,
	})
	actor := authoringapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	input := authoring.FrozenReference{
		Kind: "script_revision", ID: fixture.revisionID.String(), Version: "1", Hash: fixture.normalizedHash,
	}

	draft, err := service.Create(ctx, actor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "guided", Graph: scriptToStoryboardGraph(),
		Layout: []byte(`{"nodes":{"script":{"x":10,"y":20}}}`), FrozenInputs: []authoring.FrozenReference{input},
		CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "authoring-create-1",
	})
	if err != nil {
		t.Fatalf("create authoring draft: %v", err)
	}
	if draft.Status != "active" || draft.Revision != 1 || draft.ProjectID != fixture.projectID.String() {
		t.Fatalf("unexpected draft: %#v", draft)
	}
	replayedDraft, err := service.Create(ctx, actor, authoringapp.CreateCommand{
		ProjectID: fixture.projectID.String(), AuthoringMode: "guided", Graph: scriptToStoryboardGraph(),
		Layout: []byte(`{"nodes":{"script":{"x":10,"y":20}}}`), FrozenInputs: []authoring.FrozenReference{input},
		CatalogKey: catalog.Key, CatalogVersion: catalog.Version, IdempotencyKey: "authoring-create-1",
	})
	if err != nil || replayedDraft.ID != draft.ID {
		t.Fatalf("idempotent draft replay failed: id=%s err=%v", replayedDraft.ID, err)
	}

	invalidInput := input
	invalidInput.Hash = strings.Repeat("f", 64)
	if _, err = service.Update(ctx, actor, authoringapp.UpdateCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, Graph: draft.Graph, Layout: draft.Layout,
		FrozenInputs: []authoring.FrozenReference{invalidInput}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version,
		IdempotencyKey: "authoring-update-invalid-1",
	}); err == nil {
		t.Fatal("draft accepted a frozen script hash that differs from PostgreSQL")
	}

	firstRevision, err := service.Publish(ctx, actor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "authoring-publish-1",
	})
	if err != nil {
		t.Fatalf("publish first authoring revision: %v", err)
	}
	if firstRevision.RevisionNo != 1 || len(firstRevision.ExecutionHash) != 64 || len(firstRevision.ContentHash) != 64 {
		t.Fatalf("unexpected first authoring revision: %#v", firstRevision)
	}
	replayedRevision, err := service.Publish(ctx, actor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "authoring-publish-1",
	})
	if err != nil || replayedRevision.ID != firstRevision.ID {
		t.Fatalf("idempotent publish replay failed: id=%s err=%v", replayedRevision.ID, err)
	}

	draft, err = service.Update(ctx, actor, authoringapp.UpdateCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, Graph: draft.Graph,
		Layout:       []byte(`{"nodes":{"script":{"x":900,"y":800}},"viewport":{"zoom":2}}`),
		FrozenInputs: []authoring.FrozenReference{input}, CatalogKey: catalog.Key, CatalogVersion: catalog.Version,
		IdempotencyKey: "authoring-update-layout-1",
	})
	if err != nil || draft.Revision != 2 {
		t.Fatalf("update authoring layout: revision=%d err=%v", draft.Revision, err)
	}
	secondRevision, err := service.Publish(ctx, actor, authoringapp.PublishCommand{
		DraftID: draft.ID, ExpectedRevision: draft.Revision, IdempotencyKey: "authoring-publish-2",
	})
	if err != nil {
		t.Fatalf("publish second authoring revision: %v", err)
	}
	if secondRevision.ID == firstRevision.ID || secondRevision.RevisionNo != 2 || secondRevision.ExecutionHash != firstRevision.ExecutionHash || secondRevision.ContentHash == firstRevision.ContentHash {
		t.Fatalf("layout-only revision changed execution semantics: first=%#v second=%#v", firstRevision, secondRevision)
	}

	var persistedFirst model.AuthoringRevision
	if err = database.First(&persistedFirst, "id = ?", firstRevision.ID).Error; err != nil {
		t.Fatalf("reload first authoring revision: %v", err)
	}
	if !sameJSON(persistedFirst.Layout, []byte(`{"nodes":{"script":{"x":10,"y":20}}}`)) || persistedFirst.ContentHash != firstRevision.ContentHash {
		t.Fatalf("later draft edit changed the first revision: %#v", persistedFirst)
	}
	var draftCount, revisionCount int64
	if err = database.Model(&model.AuthoringDraft{}).Count(&draftCount).Error; err != nil {
		t.Fatal(err)
	}
	if err = database.Model(&model.AuthoringRevision{}).Count(&revisionCount).Error; err != nil {
		t.Fatal(err)
	}
	if draftCount != 1 || revisionCount != 2 {
		t.Fatalf("unexpected authoring facts: drafts=%d revisions=%d", draftCount, revisionCount)
	}
}

func sameJSON(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

type authoringFixture struct {
	userID, projectID, revisionID uuid.UUID
	normalizedHash                string
}

func seedAuthoringProject(t *testing.T, create func(any) error, now time.Time) authoringFixture {
	t.Helper()
	userID, workspaceID, projectID := uuid.New(), uuid.New(), uuid.New()
	documentID, revisionID := uuid.New(), uuid.New()
	normalizedHash := strings.Repeat("2", 64)
	records := []any{
		&model.UserAccount{ID: userID, EmailNormalized: userID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "Authoring Test", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: workspaceID, Name: "Authoring Test", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "owner", Status: "active", JoinedAt: now},
		&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "Authoring Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 90_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.ScriptDocument{ID: documentID, WorkspaceID: workspaceID, ProjectID: projectID, Title: "Authoring Script", SourceType: "text", Language: "zh-CN", RightsDeclaration: "原创测试文本", Status: "active", Revision: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.DocumentRevision{ID: revisionID, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: 1, SourceType: "text", RawText: "雨巷，夜", RawHash: strings.Repeat("1", 64), NormalizedText: "雨巷，夜", NormalizedHash: normalizedHash, NormalizerVersion: "test-v1", NormalizationMap: []byte(`{}`), CodepointCount: 4, AnalysisStatus: "deterministic", AnalyzerVersion: "test-v1", Blocks: []byte(`[]`), Issues: []byte(`[]`), CreatedBy: userID, CreatedAt: now},
	}
	for _, record := range records {
		if err := create(record); err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}
	return authoringFixture{userID: userID, projectID: projectID, revisionID: revisionID, normalizedHash: normalizedHash}
}
