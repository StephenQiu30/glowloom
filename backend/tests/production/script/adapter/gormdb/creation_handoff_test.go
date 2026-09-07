package gormdb_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	database "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	creationgorm "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/gormdb"
	creation "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/domain"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	script "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestSourceCreatesDurableCreationCommandAndFencesExpiredDelivery(t *testing.T) {
	db := creationDatabase(t)
	tx := beginSourceAcceptanceTestTransaction(t, db)
	ctx := context.Background()
	now := time.Now().UTC()
	fixture := seedSourceAcceptanceProject(t, func(v any) error { return tx.Create(v).Error }, now, "第一场\n林舟推开门😀。")
	store := creationgorm.New(tx)
	service := creation.NewService(store, creation.Config{Endpoint: "https://creation.example.invalid", Now: func() time.Time { return now }, NewID: uuid.NewString})
	actor := creation.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	input := creation.CreateCommand{ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(), SourceHash: fixture.normalizedHash, IdempotencyKey: uuid.NewString()}
	if _, err := service.Create(ctx, actor, input); !errors.Is(err, creation.ErrNotFound) {
		t.Fatalf("unaccepted source used: %v", err)
	}
	acceptCreationSource(t, tx, fixture)
	first, err := service.Create(ctx, actor, input)
	if err != nil {
		t.Fatal(err)
	}
	disabled := creation.NewService(store, creation.Config{Now: time.Now, NewID: uuid.NewString})
	replay, err := disabled.Create(ctx, actor, input)
	if err != nil || replay.Command.RunID != first.Command.RunID {
		t.Fatalf("configuration changed original run: %v", err)
	}
	changed := input
	changed.IdempotencyKey = "new-key"
	if _, err = disabled.Create(ctx, actor, changed); creationStatus(err) != 503 {
		t.Fatalf("disabled creation: %v", err)
	}
	changed = input
	changed.SourceHash = strings.Repeat("f", 64)
	if _, err = service.Create(ctx, actor, changed); creationStatus(err) != 409 {
		t.Fatalf("same key changed input: %v", err)
	}
	var count int64
	if err = tx.Model(&model.CreationCommandOutbox{}).Where("run_id = ?", first.Command.RunID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("outbox count=%d err=%v", count, err)
	}
	old, err := store.Claim(ctx, now, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Claim(ctx, now, time.Second); !errors.Is(err, creation.ErrNoDelivery) {
		t.Fatalf("active lease stolen: %v", err)
	}
	latest, err := store.Claim(ctx, now.Add(2*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Fence <= old.Fence || latest.Run.Command.RunID != old.Run.Command.RunID {
		t.Fatal("reclaim changed command or fence")
	}
	if err = store.Complete(ctx, old, domain.Unknown, "agent_outcome_unknown", nil, now.Add(3*time.Second), now.Add(time.Minute)); !errors.Is(err, creation.ErrLeaseLost) {
		t.Fatalf("stale attempt wrote result: %v", err)
	}
	if err = store.Complete(ctx, latest, domain.Blocked, "agent_rejected", nil, now.Add(3*time.Second), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	blocked, err := service.Get(ctx, actor, first.Command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := service.Retry(ctx, actor, first.Command.RunID, blocked.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Command != first.Command || retried.Endpoint != first.Endpoint {
		t.Fatal("retry changed fixed route or source")
	}
	if _, err = service.Retry(ctx, actor, first.Command.RunID, blocked.Revision); creationStatus(err) != 409 {
		t.Fatalf("stale retry: %v", err)
	}
	claim, err := store.Claim(ctx, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	receipt := domain.Acceptance{Schema: "creation-acceptance-production", CommandID: first.Command.CommandID, RunID: first.Command.RunID, PayloadHash: first.PayloadHash, FlowType: domain.FlowType, WorkflowID: first.Command.WorkflowID, ReceiptID: uuid.NewString(), AcceptedAt: now}
	if err = store.Complete(ctx, claim, domain.Accepted, "", &receipt, now, now); err != nil {
		t.Fatal(err)
	}
	read, err := service.Get(ctx, actor, first.Command.RunID)
	if err != nil || read.Acceptance == nil || read.Acceptance.ReceiptID != receipt.ReceiptID {
		t.Fatalf("durable receipt: %+v %v", read, err)
	}
	if err = tx.Model(&model.Membership{}).Where("workspace_id = ? AND user_id = ?", fixture.workspaceID, fixture.userID).Update("status", "removed").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Get(ctx, actor, first.Command.RunID); err == nil {
		t.Fatal("revoked actor read run")
	}
	if err = store.AuthorizeDelivery(ctx, first); err == nil {
		t.Fatal("revoked actor dispatched new work")
	}
}
func TestConcurrentCreationHasOneRunAndOneOutbox(t *testing.T) {
	db := creationDatabase(t)
	now := time.Now().UTC()
	ctx := context.Background()
	fixture := seedSourceAcceptanceProject(t, func(v any) error { return db.Create(v).Error }, now, "并发原创测试")
	// Cleanup is confined to this fixture; the database connection remains alive until afterwards.
	registerSourceAcceptanceFixtureCleanup(t, db, fixture, []string{"creation-source"})
	t.Cleanup(func() {
		ids := db.Model(&model.CreationRun{}).Select("id").Where("project_id = ?", fixture.projectID)
		if err := db.Where("run_id IN (?)", ids).Delete(&model.CreationCommandOutbox{}).Error; err != nil {
			t.Error(err)
		}
		if err := db.Where("project_id = ?", fixture.projectID).Delete(&model.CreationRun{}).Error; err != nil {
			t.Error(err)
		}
	})
	acceptCreationSource(t, db, fixture)
	service := creation.NewService(creationgorm.New(db), creation.Config{Endpoint: "https://creation.example.invalid", Now: time.Now, NewID: uuid.NewString})
	input := creation.CreateCommand{ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(), SourceHash: fixture.normalizedHash, IdempotencyKey: "one-run"}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			run, err := service.Create(ctx, creation.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, input)
			ids <- run.Command.RunID
			errs <- err
		})
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("duplicate creation under concurrency")
		}
	}
	var count int64
	if err := db.Model(&model.CreationRun{}).Where("project_id = ?", fixture.projectID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("run count=%d err=%v", count, err)
	}
}
func creationDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL for durable creation handoff")
	}
	db, err := database.Open(context.Background(), dsn, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err = schema.Sync(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}
func acceptCreationSource(t *testing.T, db *gorm.DB, fixture sourceAcceptanceFixture) {
	t.Helper()
	_, err := script.NewSourceService(scriptgorm.New(db), script.SourceConfig{Now: time.Now, NewID: uuid.NewString}).Accept(context.Background(), script.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, script.AcceptSourceCommand{ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(), IdempotencyKey: "creation-source"})
	if err != nil {
		t.Fatal(err)
	}
}
func creationStatus(err error) int {
	var problem *creation.Error
	if errors.As(err, &problem) {
		return problem.Status
	}
	return 0
}

func TestSourceAuthorizationPreservesDatabaseCancellation(t *testing.T) {
	db := creationDatabase(t)
	tx := beginSourceAcceptanceTestTransaction(t, db)
	fixture := seedSourceAcceptanceProject(t, func(v any) error { return tx.Create(v).Error }, time.Now().UTC(), "来源权限故障测试")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	err := scriptgorm.New(tx).WithinSourceTransaction(context.Background(), func(repo script.SourceRepository) error {
		_, queryErr := repo.ProjectWorkspace(cancelled, script.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, fixture.projectID.String(), true)
		if !errors.Is(queryErr, context.Canceled) {
			t.Fatalf("database failure was classified as revoked access: %v", queryErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreationOutboxFailureRollsBackRunAcceptance(t *testing.T) {
	db := creationDatabase(t)
	tx := beginSourceAcceptanceTestTransaction(t, db)
	now := time.Now().UTC()
	fixture := seedSourceAcceptanceProject(t, func(v any) error { return tx.Create(v).Error }, now, "事务回滚原创测试")
	acceptCreationSource(t, tx, fixture)
	injected := errors.New("synthetic outbox failure")
	callback := "test_fail_creation_outbox"
	if err := tx.Callback().Create().Before("gorm:create").Register(callback, func(query *gorm.DB) {
		if query.Statement.Table == "crn_command_outbox" {
			_ = query.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Callback().Create().Remove(callback) }()
	service := creation.NewService(creationgorm.New(tx), creation.Config{Endpoint: "https://creation.example.invalid", Now: time.Now, NewID: uuid.NewString})
	_, err := service.Create(context.Background(), creation.Actor{UserID: fixture.userID.String(), TokenVersion: 1}, creation.CreateCommand{ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(), SourceHash: fixture.normalizedHash, IdempotencyKey: "rollback-run"})
	if !errors.Is(err, injected) {
		t.Fatalf("injected failure lost: %v", err)
	}
	var count int64
	if err = tx.Model(&model.CreationRun{}).Where("project_id = ?", fixture.projectID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partially accepted run: %d %v", count, err)
	}
}
