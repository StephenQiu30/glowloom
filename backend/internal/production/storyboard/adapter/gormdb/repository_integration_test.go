package gormdb_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	planningdomain "github.com/StephenQiu30/lanverse/backend/internal/production/planning/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	"github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
	storyboarddomain "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/domain"
)

func TestPostgreSQLStoryboardingJourney(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL integration journey")
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
	assertSingleCatalog(t, database)

	fixture := seedStoryboardFixture(t, database)
	store := gormdb.New(database)
	now := fixture.now
	service := application.NewService(store, application.Config{
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
		NewID: func() string { return uuid.NewString() },
	})
	actor := application.Actor{UserID: fixture.userID.String(), TokenVersion: 1}

	batch, err := service.CreateBatch(ctx, actor, application.CreateBatchCommand{
		EpisodeID: fixture.episodeID.String(), IdempotencyKey: "storyboard-create-1",
	})
	if err != nil {
		t.Fatalf("create storyboard batch: %v", err)
	}
	if batch.Status != "queued" || batch.Revision != 1 {
		t.Fatalf("unexpected created batch: status=%s revision=%d", batch.Status, batch.Revision)
	}
	replayed, err := service.CreateBatch(ctx, actor, application.CreateBatchCommand{
		EpisodeID: fixture.episodeID.String(), IdempotencyKey: "storyboard-create-1",
	})
	if err != nil || replayed.ID != batch.ID {
		t.Fatalf("idempotent create replay failed: batch=%s err=%v", replayed.ID, err)
	}

	invocation, found, err := store.ClaimNext(ctx, now.Add(time.Second), now.Add(30*time.Minute))
	if err != nil || !found {
		t.Fatalf("claim storyboard invocation: found=%v err=%v", found, err)
	}
	var payload struct {
		Units []struct {
			ID       string `json:"unit_version_id"`
			Required bool   `json:"required_for_coverage"`
		} `json:"units"`
	}
	if err = json.Unmarshal(invocation.Payload, &payload); err != nil {
		t.Fatalf("decode invocation payload: %v", err)
	}
	unitIDs := make([]string, 0, len(payload.Units))
	for _, unit := range payload.Units {
		if unit.Required {
			unitIDs = append(unitIDs, unit.ID)
		}
	}
	if len(unitIDs) != 3 {
		t.Fatalf("expected scene, action and dialogue units, got %d", len(unitIDs))
	}
	candidate := storyboarddomain.Candidate{Shots: []storyboarddomain.DraftShot{{
		ProposalKey: "shot-001", Position: 1, Title: "雨巷相逢", NarrativeUnitVersionIDs: unitIDs,
		Spec:            map[string]any{"duration_ms": 5400, "visual": map[string]any{"shot_size": "wide", "camera_movement": "push_in"}},
		AssetReferences: []map[string]any{}, RiskCodes: []string{},
	}}}
	candidateJSON, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("encode candidate: %v", err)
	}
	validated, err := storyboarddomain.DecodeAndValidateCandidate(candidateJSON, invocation.Payload)
	if err != nil {
		t.Fatalf("validate candidate against invocation: %v", err)
	}
	resultHash, err := contract.CanonicalHash(candidateJSON)
	if err != nil {
		t.Fatalf("hash candidate: %v", err)
	}
	result := contract.Result{
		InvocationID: invocation.ID, Kind: invocation.Kind, InputHash: invocation.InputHash,
		Status: "succeeded", SchemaVersion: contract.SchemaVersion, Candidate: candidateJSON,
		ResultHash: &resultHash, Executor: contract.Executor{Name: "integration", Version: "1", Model: "deterministic"},
	}
	var executionPolicy contract.ExecutionPolicy
	if err = json.Unmarshal(invocation.ExecutionPolicy, &executionPolicy); err != nil {
		t.Fatalf("decode execution policy: %v", err)
	}
	contractInvocation := contract.Invocation{InvocationID: invocation.ID, Kind: invocation.Kind, InputHash: invocation.InputHash, SchemaVersion: contract.SchemaVersion, ExecutionPolicy: executionPolicy, Payload: invocation.Payload}
	if err = result.ValidateFor(contractInvocation); err != nil {
		t.Fatalf("validate agent result envelope: %v", err)
	}
	completionApplied, err := store.CompleteInvocation(ctx, invocation.ID, invocation.ClaimVersion, result, validated, now.Add(2*time.Second))
	if err != nil || !completionApplied {
		t.Fatalf("complete invocation: applied=%v err=%v", completionApplied, err)
	}

	batch, err = service.GetBatch(ctx, actor, batch.ID)
	if err != nil || batch.Status != "needs_review" || batch.Revision != 3 {
		t.Fatalf("unexpected review batch: status=%s revision=%d err=%v", batch.Status, batch.Revision, err)
	}
	batch, err = service.Decide(ctx, actor, application.DecisionCommand{
		BatchID: batch.ID, ProposalKey: "shot-001", Action: "accepted", ExpectedRevision: batch.Revision, IdempotencyKey: "shot-accept-1",
	})
	if err != nil {
		t.Fatalf("accept storyboard draft: %v", err)
	}
	batch, err = service.Approve(ctx, actor, application.RevisionCommand{BatchID: batch.ID, ExpectedRevision: batch.Revision, IdempotencyKey: "batch-approve-1"})
	if err != nil || batch.Status != "approved" {
		t.Fatalf("approve storyboard batch: status=%s err=%v", batch.Status, err)
	}
	preflight, err := service.PreflightApply(ctx, actor, batch.ID, batch.Revision)
	if err != nil || preflight.Created != 1 {
		t.Fatalf("preflight storyboard apply: created=%d err=%v", preflight.Created, err)
	}

	_, _, err = service.Apply(ctx, actor, application.ApplyCommand{
		RevisionCommand:   application.RevisionCommand{BatchID: batch.ID, ExpectedRevision: preflight.BatchRevision, IdempotencyKey: "batch-apply-stale"},
		ExpectedOrderHash: preflight.OrderHash, ImpactHash: strings.Repeat("0", 64),
	})
	if err == nil {
		t.Fatal("expected stale apply hash to be rejected")
	}
	assertShotCount(t, database, fixture.episodeID, 0)

	applied, shots, err := service.Apply(ctx, actor, application.ApplyCommand{
		RevisionCommand:   application.RevisionCommand{BatchID: batch.ID, ExpectedRevision: preflight.BatchRevision, IdempotencyKey: "batch-apply-1"},
		ExpectedOrderHash: preflight.OrderHash, ImpactHash: preflight.ImpactHash,
	})
	if err != nil || applied.Status != "applied" || len(shots) != 1 {
		t.Fatalf("apply storyboard batch: status=%s shots=%d err=%v", applied.Status, len(shots), err)
	}
	assertShotCount(t, database, fixture.episodeID, 1)

	exportPreflight, err := service.PreflightExport(ctx, actor, fixture.episodeID.String())
	if err != nil || !exportPreflight.Allowed || exportPreflight.ShotCount != 1 {
		t.Fatalf("preflight storyboard export: allowed=%v shots=%d err=%v", exportPreflight.Allowed, exportPreflight.ShotCount, err)
	}
	exported, err := service.CreateExport(ctx, actor, application.ExportCommand{
		EpisodeID: fixture.episodeID.String(), ExpectedOrderHash: exportPreflight.OrderHash, IdempotencyKey: "storyboard-export-1",
	})
	if err != nil {
		t.Fatalf("create storyboard export: %v", err)
	}
	hash := sha256.Sum256(exported.Package)
	if exported.ContentHash != hex.EncodeToString(hash[:]) || len(exported.Files) != 4 {
		t.Fatalf("unexpected deterministic export: hash=%s files=%d", exported.ContentHash, len(exported.Files))
	}
	exportReplay, err := service.CreateExport(ctx, actor, application.ExportCommand{
		EpisodeID: fixture.episodeID.String(), ExpectedOrderHash: exportPreflight.OrderHash, IdempotencyKey: "storyboard-export-1",
	})
	if err != nil || exportReplay.ID != exported.ID || string(exportReplay.Package) != string(exported.Package) {
		t.Fatalf("idempotent export replay failed: export=%s err=%v", exportReplay.ID, err)
	}
}

func assertSingleCatalog(t *testing.T, database *gorm.DB) {
	t.Helper()
	tables, err := database.Migrator().GetTables()
	if err != nil {
		t.Fatalf("list synchronized tables: %v", err)
	}
	sort.Strings(tables)
	for _, table := range tables {
		if strings.Contains(strings.ToLower(table), "migration") {
			t.Fatalf("migration bookkeeping table must not exist: %s", table)
		}
	}
	expected := make([]string, 0, len(schema.Catalog()))
	for _, value := range schema.Catalog() {
		statement := &gorm.Statement{DB: database}
		if err = statement.Parse(value); err != nil {
			t.Fatalf("parse catalog model: %v", err)
		}
		expected = append(expected, statement.Schema.Table)
	}
	sort.Strings(expected)
	if strings.Join(tables, ",") != strings.Join(expected, ",") {
		t.Fatalf("database tables differ from the GORM catalog\nexpected: %v\nactual:   %v", expected, tables)
	}
}

type storyboardFixture struct {
	userID, episodeID uuid.UUID
	now               time.Time
}

func seedStoryboardFixture(t *testing.T, database *gorm.DB) storyboardFixture {
	t.Helper()
	now := time.Date(2026, time.August, 25, 6, 0, 0, 0, time.UTC)
	userID, workspaceID, projectID := uuid.New(), uuid.New(), uuid.New()
	documentID, revisionID, episodeID := uuid.New(), uuid.New(), uuid.New()
	versionID, structureID, bibleID, bibleTaskID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	visualStyle := "水墨电影感"
	description := "GORM 集成测试项目"
	confirmedAt := now
	scenes, err := json.Marshal([]planningdomain.Scene{{
		ID: uuid.NewString(), Heading: "雨巷，夜", Position: 1, SourceStart: 0, SourceEnd: 24,
		NarrativeUnits: []planningdomain.NarrativeUnit{{ID: uuid.NewString(), Kind: "action", Text: "顾清禾撑伞停下。", SourceStart: 0, SourceEnd: 9}},
		Dialogues:      []planningdomain.Dialogue{{ID: uuid.NewString(), Speaker: "顾清禾", Text: "你终于来了。", SourceStart: 10, SourceEnd: 18}},
		Tasks:          []planningdomain.ProductionTask{{ID: uuid.NewString(), Kind: "shot_breakdown", Label: "拆解场景分镜", Status: "accepted", Required: true}},
	}})
	if err != nil {
		t.Fatalf("encode scenes: %v", err)
	}
	bibleCandidate := datatypes.JSON([]byte(`{"world_entries":[{"name":"长安雨巷","description":"潮湿的夜巷"}]}`))
	resultHash := strings.Repeat("b", 64)
	records := []any{
		&model.UserAccount{ID: userID, EmailNormalized: userID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "集成测试", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: workspaceID, Name: "集成测试空间", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "owner", Status: "active", JoinedAt: now},
		&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "镜中长安", Description: &description, AspectRatio: "9:16", Language: "zh-CN", VisualStyle: &visualStyle, TargetDurationMS: 90_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.ScriptDocument{ID: documentID, WorkspaceID: workspaceID, ProjectID: projectID, Title: "镜中长安", SourceType: "text", Language: "zh-CN", RightsDeclaration: "原创测试文本", Status: "active", Revision: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.DocumentRevision{ID: revisionID, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: 1, SourceType: "text", RawText: "第一集\n雨巷，夜\n顾清禾：你终于来了。", RawHash: strings.Repeat("1", 64), NormalizedText: "第一集\n雨巷，夜\n顾清禾：你终于来了。", NormalizedHash: strings.Repeat("2", 64), NormalizerVersion: "test-v1", NormalizationMap: datatypes.JSON([]byte(`{}`)), CodepointCount: 24, AnalysisStatus: "deterministic", AnalyzerVersion: "test-v1", Blocks: datatypes.JSON([]byte(`[]`)), Issues: datatypes.JSON([]byte(`[]`)), CreatedBy: userID, CreatedAt: now},
		&model.Episode{ID: episodeID, WorkspaceID: workspaceID, ProjectID: projectID, Name: "雨巷相逢", Position: 1, TargetDurationMS: 90_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.EpisodeScriptVersion{ID: versionID, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, VersionNo: 1, DocumentRevisionID: revisionID, SourceStart: 0, SourceEnd: 24, Content: "雨巷，夜\n顾清禾：你终于来了。", ContentHash: strings.Repeat("3", 64), Status: "published", CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.EpisodeStructure{ID: structureID, WorkspaceID: workspaceID, ProjectID: projectID, EpisodeID: episodeID, ScriptVersionID: versionID, Status: "confirmed", Scenes: datatypes.JSON(scenes), ResultHash: strings.Repeat("4", 64), Revision: 2, ConfirmedBy: &userID, ConfirmedAt: &confirmedAt, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.WorkflowTask{ID: bibleTaskID, WorkspaceID: workspaceID, TaskType: "production_bible", RequestType: "production_bible", RequestID: bibleID, Scope: datatypes.JSON([]byte(`{}`)), Status: "succeeded", ProgressStage: "confirmed", CancelStatus: "none", Revision: 2, CreatedAt: now, UpdatedAt: now},
		&model.ProductionBible{ID: bibleID, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: revisionID, TaskID: bibleTaskID, Status: "confirmed", InputHash: strings.Repeat("5", 64), ResultHash: &resultHash, EngineVersion: "test-v1", ModelName: "deterministic", PromptVersion: "test-v1", SchemaVersion: contract.SchemaVersion, HarnessVersion: "test-v1", CheckpointRevision: 0, Candidate: bibleCandidate, Revision: 3, ConfirmedAt: &confirmedAt, ConfirmedBy: &userID, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
	}
	for _, record := range records {
		if err = database.Omit(clause.Associations).Create(record).Error; err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}
	if err = database.Model(&model.Episode{}).Where("id = ?", episodeID).Update("current_script_version_id", versionID).Error; err != nil {
		t.Fatalf("set current script version: %v", err)
	}
	return storyboardFixture{userID: userID, episodeID: episodeID, now: now}
}

func assertShotCount(t *testing.T, database *gorm.DB, episodeID uuid.UUID, expected int64) {
	t.Helper()
	var count int64
	if err := database.Model(&model.StoryboardShot{}).Where("episode_id = ? AND status = ?", episodeID, "active").Count(&count).Error; err != nil {
		t.Fatalf("count active shots: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d active shots, got %d", expected, count)
	}
}
