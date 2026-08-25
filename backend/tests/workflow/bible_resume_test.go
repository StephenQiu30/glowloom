package workflow_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	biblegorm "github.com/StephenQiu30/lanverse/backend/internal/production/bible/adapter/gormdb"
	bibleapp "github.com/StephenQiu30/lanverse/backend/internal/production/bible/application"
)

func TestBibleResumePreservesClaimFenceAndRejectsLateResult(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL workflow journey")
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

	fixture := seedFailedBible(t, func(value any) error {
		return database.Create(value).Error
	})
	store := biblegorm.New(database)
	resumeAt := fixture.now.Add(time.Minute)
	service := bibleapp.NewService(store, bibleapp.Config{
		Now:   func() time.Time { return resumeAt },
		NewID: func() string { return uuid.NewString() },
	})
	actor := bibleapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}

	resumed, err := service.Resume(ctx, actor, bibleapp.ResumeCommand{
		BibleID: fixture.bibleID.String(), ExpectedRevision: fixture.bibleRevision,
		IdempotencyKey: "resume-failed-bible-1",
	})
	if err != nil {
		t.Fatalf("resume failed bible: %v", err)
	}
	if resumed.Status != "queued" || resumed.Revision != fixture.bibleRevision+1 {
		t.Fatalf("unexpected resumed bible: status=%s revision=%d", resumed.Status, resumed.Revision)
	}

	var queued model.AgentInvocation
	if err = database.First(&queued, "id = ?", fixture.invocationID).Error; err != nil {
		t.Fatalf("reload resumed invocation: %v", err)
	}
	if queued.Status != "queued" || queued.ClaimVersion != fixture.claimVersion || queued.LeaseExpiresAt != nil {
		t.Fatalf("resume changed the existing claim fence: %#v", queued)
	}

	claimAt := resumeAt.Add(time.Second)
	leaseExpiresAt := claimAt.Add(30 * time.Minute)
	current, found, err := store.ClaimNext(ctx, claimAt, leaseExpiresAt)
	if err != nil || !found {
		t.Fatalf("claim resumed invocation: found=%v err=%v", found, err)
	}
	if current.ID != fixture.invocationID.String() || current.ClaimVersion != fixture.claimVersion+1 {
		t.Fatalf("unexpected resumed claim: %#v", current)
	}

	lateApplied, err := store.FailInvocation(
		ctx, current.ID, fixture.claimVersion, "failed", "late_worker", "旧 Worker 的迟到结果", true, claimAt.Add(time.Second),
	)
	if err != nil || lateApplied {
		t.Fatalf("late pre-resume result applied: applied=%v err=%v", lateApplied, err)
	}

	var afterLate model.AgentInvocation
	if err = database.First(&afterLate, "id = ?", fixture.invocationID).Error; err != nil {
		t.Fatalf("reload invocation after late result: %v", err)
	}
	if afterLate.Status != "running" || afterLate.ClaimVersion != current.ClaimVersion || afterLate.LeaseExpiresAt == nil {
		t.Fatalf("late result changed the current claim: %#v", afterLate)
	}

	currentApplied, err := store.FailInvocation(
		ctx, current.ID, current.ClaimVersion, "failed", "current_worker", "当前 Worker 结果", true, claimAt.Add(2*time.Second),
	)
	if err != nil || !currentApplied {
		t.Fatalf("current claim result was not applied: applied=%v err=%v", currentApplied, err)
	}

	var finalized model.AgentInvocation
	if err = database.First(&finalized, "id = ?", fixture.invocationID).Error; err != nil {
		t.Fatalf("reload finalized invocation: %v", err)
	}
	if finalized.Status != "failed" || finalized.ClaimVersion != current.ClaimVersion || finalized.LeaseExpiresAt != nil {
		t.Fatalf("current result did not finalize the resumed claim: %#v", finalized)
	}
}

func TestBibleConfirmationReturnsPersistedOwnerReceipt(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the PostgreSQL workflow journey")
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

	fixture := seedFailedBible(t, func(value any) error { return database.Create(value).Error })
	if err = database.Model(&model.ProductionBible{}).Where("id = ?", fixture.bibleID).
		Updates(map[string]any{"status": "needs_review", "error": nil}).Error; err != nil {
		t.Fatalf("prepare reviewable production bible: %v", err)
	}
	confirmAt := fixture.now.Add(time.Minute)
	service := bibleapp.NewService(biblegorm.New(database), bibleapp.Config{
		Now: func() time.Time { return confirmAt }, NewID: uuid.NewString,
	})
	actor := bibleapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	command := bibleapp.ConfirmCommand{
		BibleID: fixture.bibleID.String(), ExpectedResultHash: strings.Repeat("b", 64),
		ExpectedRevision: fixture.bibleRevision, IdempotencyKey: "confirm-owner-receipt",
	}

	confirmed, err := service.Confirm(ctx, actor, command)
	if err != nil || confirmed.Bible.Status != "confirmed" || confirmed.Bible.Revision != fixture.bibleRevision+1 ||
		confirmed.Receipt.ID == "" || confirmed.Receipt.ResourceID != fixture.bibleID.String() {
		t.Fatalf("confirm production bible with receipt: result=%#v err=%v", confirmed, err)
	}
	var persisted model.CommandReceipt
	if err = database.First(&persisted, "id = ?", confirmed.Receipt.ID).Error; err != nil {
		t.Fatalf("load production owner receipt: %v", err)
	}
	if persisted.Operation != "production_bible.confirm" || persisted.ResourceID != fixture.bibleID ||
		persisted.InputHash != confirmed.Receipt.InputHash {
		t.Fatalf("persisted production owner receipt = %#v", persisted)
	}
	replayed, err := service.Confirm(ctx, actor, command)
	if err != nil || replayed.Bible.ID != confirmed.Bible.ID || replayed.Receipt.ID != confirmed.Receipt.ID {
		t.Fatalf("replay production owner receipt: result=%#v err=%v", replayed, err)
	}
	var receiptCount int64
	if err = database.Model(&model.CommandReceipt{}).
		Where("workspace_id = ? AND operation = ? AND idempotency_key = ?", persisted.WorkspaceID, persisted.Operation, command.IdempotencyKey).
		Count(&receiptCount).Error; err != nil || receiptCount != 1 {
		t.Fatalf("production owner receipt count = %d err=%v", receiptCount, err)
	}
}

type failedBibleFixture struct {
	userID, bibleID, invocationID uuid.UUID
	now                           time.Time
	bibleRevision, claimVersion   int
}

func seedFailedBible(t *testing.T, create func(any) error) failedBibleFixture {
	t.Helper()
	now := time.Date(2026, time.August, 25, 13, 0, 0, 0, time.UTC)
	userID, workspaceID, projectID := uuid.New(), uuid.New(), uuid.New()
	documentID, revisionID := uuid.New(), uuid.New()
	taskID, bibleID, invocationID := uuid.New(), uuid.New(), uuid.New()
	bibleRevision, claimVersion := 5, 7
	resultHash := strings.Repeat("b", 64)
	completedAt := now

	records := []any{
		&model.UserAccount{
			ID: userID, EmailNormalized: userID.String() + "@example.test", PasswordHash: "not-used",
			TokenVersion: 1, DisplayName: "Workflow Resume Test", Status: "active", CreatedAt: now, UpdatedAt: now,
		},
		&model.Workspace{
			ID: workspaceID, Name: "Workflow Resume Test", Status: "active", Revision: 1,
			CreatedAt: now, UpdatedAt: now,
		},
		&model.Membership{
			ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "owner", Status: "active", JoinedAt: now,
		},
		&model.Project{
			ID: projectID, WorkspaceID: workspaceID, Name: "Workflow Resume Project", AspectRatio: "9:16",
			Language: "zh-CN", TargetDurationMS: 90_000, BudgetLimit: decimal.Zero, Currency: "CNY",
			Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now,
		},
		&model.ScriptDocument{
			ID: documentID, WorkspaceID: workspaceID, ProjectID: projectID, Title: "Resume Script",
			SourceType: "text", Language: "zh-CN", RightsDeclaration: "原创测试文本", Status: "active",
			Revision: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		},
		&model.DocumentRevision{
			ID: revisionID, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: 1, SourceType: "text",
			RawText: "雨巷，夜", RawHash: strings.Repeat("1", 64), NormalizedText: "雨巷，夜",
			NormalizedHash: strings.Repeat("2", 64), NormalizerVersion: "test-v1", NormalizationMap: []byte(`{}`),
			CodepointCount: 4, AnalysisStatus: "deterministic", AnalyzerVersion: "test-v1",
			Blocks: []byte(`[]`), Issues: []byte(`[]`), CreatedBy: userID, CreatedAt: now,
		},
		&model.WorkflowTask{
			ID: taskID, WorkspaceID: workspaceID, TaskType: "production_bible", RequestType: "production_bible",
			RequestID: bibleID, Scope: []byte(`{}`), Status: "failed", ProgressStage: "agent_result",
			CancelStatus: "none", Revision: 4, CreatedAt: now, UpdatedAt: now,
		},
		&model.ProductionBible{
			ID: bibleID, WorkspaceID: workspaceID, ProjectID: projectID, DocumentRevisionID: revisionID, TaskID: taskID,
			Status: "failed", InputHash: strings.Repeat("2", 64), ResultHash: &resultHash,
			EngineVersion: "test-v1", ModelName: "deterministic", PromptVersion: "test-v1",
			SchemaVersion: "production-bible-schema-v1", HarnessVersion: "test-v1", CheckpointRevision: 1,
			Candidate:       []byte(`{"entities":[],"world_entries":[],"review_issues":[]}`),
			ReviewDecisions: []byte(`{}`), Error: []byte(`{"code":"agent_failed"}`), Revision: bibleRevision,
			CreatedBy: userID, CreatedAt: now, UpdatedAt: now,
		},
		&model.AgentInvocation{
			ID: invocationID, WorkspaceID: workspaceID, RequestType: "production_bible", RequestID: bibleID,
			Kind: "production_bible", InputHash: strings.Repeat("2", 64), Payload: []byte(`{}`), Status: "failed",
			Error: []byte(`{"code":"agent_failed"}`), Attempts: claimVersion, ClaimVersion: claimVersion,
			CompletedAt: &completedAt, CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, record := range records {
		if err := create(record); err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}

	return failedBibleFixture{
		userID: userID, bibleID: bibleID, invocationID: invocationID, now: now,
		bibleRevision: bibleRevision, claimVersion: claimVersion,
	}
}
