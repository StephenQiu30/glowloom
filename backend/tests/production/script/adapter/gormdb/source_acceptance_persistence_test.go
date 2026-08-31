package gormdb_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
)

func TestAcceptSourcePublishesIndexHeadAndReceiptsAtomically(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Script Source acceptance journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	rootDatabase := database
	t.Cleanup(func() { _ = platformdatabase.Close(rootDatabase) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize GORM catalog: %v", err)
	}
	database = beginSourceAcceptanceTestTransaction(t, rootDatabase)

	now := time.Date(2026, time.August, 31, 6, 0, 0, 0, time.UTC)
	fixture := seedSourceAcceptanceProject(t, func(value any) error { return database.Create(value).Error }, now, "第一场 夜 内\r\n林舟😀握住门把。")
	service := scriptapp.NewSourceService(
		scriptgorm.New(database),
		scriptapp.SourceConfig{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)
	command := scriptapp.AcceptSourceCommand{
		ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(),
		ExpectedHeadRevision: 0, ExpectedHeadHash: nil, IdempotencyKey: "accept-source-" + fixture.projectID.String(),
	}
	actor := scriptapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	accepted, err := service.Accept(ctx, actor, command)
	if err != nil {
		t.Fatalf("accept Script Source: %v", err)
	}
	if accepted.Identity.OwnerKind != "production/script" ||
		accepted.Identity.LogicalID != fixture.documentID.String() ||
		accepted.Identity.VersionID != fixture.revisionID.String() || accepted.Identity.Revision != 1 ||
		accepted.Identity.ContentHash != fixture.normalizedHash || accepted.CodepointCount != fixture.codepointCount ||
		accepted.NewlineNormalization != "lf" || accepted.CodepointIndexRule != "unicode-code-point" ||
		accepted.HeadRevision != 1 || len(accepted.HeadHash) != 64 ||
		accepted.CollectionReceiptID == "" || accepted.CommandReceiptID == "" {
		t.Fatalf("accepted Script Source = %#v", accepted)
	}
	replayed, err := service.Accept(ctx, actor, command)
	if err != nil || replayed != accepted {
		t.Fatalf("idempotent Source replay = %#v err=%v", replayed, err)
	}
	conflictingCommand := command
	conflictingCommand.ExpectedHeadRevision = accepted.HeadRevision
	conflictingCommand.ExpectedHeadHash = &accepted.HeadHash
	if _, err = service.Accept(ctx, actor, conflictingCommand); scriptapp.ErrorCode(err) != "idempotency_conflict" {
		t.Fatalf("Source idempotency conflict error = %v", err)
	}
	read, err := service.GetExact(ctx, actor, fixture.projectID.String(), fixture.revisionID.String())
	if err != nil || read != accepted {
		t.Fatalf("exact Source query = %#v err=%v", read, err)
	}

	var indexCount, headCount, collectionReceiptCount int64
	for _, count := range []struct {
		model any
		value *int64
	}{
		{&model.SourceSpanIndexVersion{}, &indexCount},
		{&model.ScriptSourceScopeHead{}, &headCount},
		{&model.ScriptSourceCollectionReceipt{}, &collectionReceiptCount},
	} {
		if err = database.Model(count.model).Where("project_id = ?", fixture.projectID).Count(count.value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if indexCount != 1 || headCount != 1 || collectionReceiptCount != 1 {
		t.Fatalf("source facts = index:%d head:%d receipt:%d", indexCount, headCount, collectionReceiptCount)
	}
}

func TestAcceptSourceHeadCASAllowsOnlyOneConcurrentSuccess(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run the Script Source CAS journey")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 31, 6, 30, 0, 0, time.UTC)
	create := func(value any) error { return database.Create(value).Error }
	fixture := seedSourceAcceptanceProject(t, create, now, "第一场\n甲。")
	secondRevision := seedSourceRevision(t, create, fixture, now.Add(time.Second), 2, "第二场\n乙。")
	idempotencyKeys := []string{
		"source-first-" + fixture.projectID.String(),
		"source-second-" + fixture.projectID.String(),
	}
	registerSourceAcceptanceFixtureCleanup(t, database, fixture, idempotencyKeys)
	service := scriptapp.NewSourceService(scriptgorm.New(database), scriptapp.SourceConfig{
		Now: func() time.Time { return now.Add(2 * time.Second) }, NewID: uuid.NewString,
	})
	actor := scriptapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	commands := []scriptapp.AcceptSourceCommand{
		{ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(), ExpectedHeadRevision: 0, IdempotencyKey: idempotencyKeys[0]},
		{ProjectID: fixture.projectID.String(), DocumentRevisionID: secondRevision.String(), ExpectedHeadRevision: 0, IdempotencyKey: idempotencyKeys[1]},
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, command := range commands {
		workers.Add(1)
		go func(value scriptapp.AcceptSourceCommand) {
			defer workers.Done()
			<-start
			_, acceptErr := service.Accept(ctx, actor, value)
			results <- acceptErr
		}(command)
	}
	close(start)
	workers.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		if result == nil {
			successes++
			continue
		}
		if scriptapp.ErrorCode(result) == "head_conflict" {
			conflicts++
			continue
		}
		t.Fatalf("unexpected Source CAS result: %v", result)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("Source CAS outcomes: success=%d conflicts=%d", successes, conflicts)
	}
}

type sourceAcceptanceFixture struct {
	userID, workspaceID, projectID, documentID, revisionID uuid.UUID
	normalizedHash                                         string
	codepointCount                                         int
}

func seedSourceAcceptanceProject(t *testing.T, create func(any) error, now time.Time, rawText string) sourceAcceptanceFixture {
	t.Helper()
	userID, workspaceID, projectID, documentID, revisionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	normalizedText := strings.ReplaceAll(strings.ReplaceAll(rawText, "\r\n", "\n"), "\r", "\n")
	records := []any{
		&model.UserAccount{ID: userID, EmailNormalized: userID.String() + "@example.test", PasswordHash: "not-used", TokenVersion: 1, DisplayName: "Source Test", Status: "active", CreatedAt: now, UpdatedAt: now},
		&model.Workspace{ID: workspaceID, Name: "Source Test", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.Membership{ID: uuid.New(), WorkspaceID: workspaceID, UserID: userID, Role: "owner", Status: "active", JoinedAt: now},
		&model.Project{ID: projectID, WorkspaceID: workspaceID, Name: "Source Project", AspectRatio: "9:16", Language: "zh-CN", TargetDurationMS: 90_000, Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now},
		&model.ScriptDocument{ID: documentID, WorkspaceID: workspaceID, ProjectID: projectID, Title: "Source Script", SourceType: "text", Language: "zh-CN", RightsDeclaration: "原创测试文本", Status: "active", Revision: 1, CreatedBy: userID, CreatedAt: now, UpdatedAt: now},
		&model.DocumentRevision{ID: revisionID, WorkspaceID: workspaceID, DocumentID: documentID, VersionNo: 1, SourceType: "text", RawText: rawText, RawHash: sourceHash(rawText), NormalizedText: normalizedText, NormalizedHash: sourceHash(normalizedText), NormalizerVersion: "line-ending-lf", NormalizationMap: []byte(`{"newline":"lf"}`), CodepointCount: utf8.RuneCountInString(normalizedText), AnalysisStatus: "deterministic", AnalyzerVersion: "source-fixture", Blocks: []byte(`[]`), Issues: []byte(`[]`), CreatedBy: userID, CreatedAt: now},
	}
	for _, record := range records {
		if err := create(record); err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}
	return sourceAcceptanceFixture{
		userID: userID, workspaceID: workspaceID, projectID: projectID, documentID: documentID,
		revisionID: revisionID, normalizedHash: sourceHash(normalizedText), codepointCount: utf8.RuneCountInString(normalizedText),
	}
}

func seedSourceRevision(t *testing.T, create func(any) error, fixture sourceAcceptanceFixture, now time.Time, version int, rawText string) uuid.UUID {
	t.Helper()
	revisionID := uuid.New()
	record := model.DocumentRevision{
		ID: revisionID, WorkspaceID: fixture.workspaceID, DocumentID: fixture.documentID, VersionNo: version,
		SourceType: "text", RawText: rawText, RawHash: sourceHash(rawText), NormalizedText: rawText,
		NormalizedHash: sourceHash(rawText), NormalizerVersion: "line-ending-lf", NormalizationMap: []byte(`{"newline":"lf"}`),
		CodepointCount: utf8.RuneCountInString(rawText), AnalysisStatus: "deterministic", AnalyzerVersion: "source-fixture",
		Blocks: []byte(`[]`), Issues: []byte(`[]`), CreatedBy: fixture.userID, CreatedAt: now,
	}
	if err := create(&record); err != nil {
		t.Fatalf("seed Source revision: %v", err)
	}
	return revisionID
}

func sourceHash(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func beginSourceAcceptanceTestTransaction(t *testing.T, database *gorm.DB) *gorm.DB {
	t.Helper()
	transaction := database.Begin()
	if transaction.Error != nil {
		t.Fatalf("begin Source acceptance test transaction: %v", transaction.Error)
	}
	t.Cleanup(func() {
		if err := transaction.Rollback().Error; err != nil && !errors.Is(err, gorm.ErrInvalidTransaction) {
			t.Errorf("rollback Source acceptance test transaction: %v", err)
		}
	})
	return transaction
}

func registerSourceAcceptanceFixtureCleanup(
	t *testing.T,
	database *gorm.DB,
	fixture sourceAcceptanceFixture,
	idempotencyKeys []string,
) {
	t.Helper()
	t.Cleanup(func() {
		err := database.Transaction(func(transaction *gorm.DB) error {
			deletions := []struct {
				model any
				query string
				args  []any
			}{
				{&model.CommandReceipt{}, "workspace_id = ? AND idempotency_key IN ?", []any{fixture.workspaceID, idempotencyKeys}},
				{&model.ScriptSourceCollectionReceipt{}, "project_id = ?", []any{fixture.projectID}},
				{&model.ScriptSourceScopeHead{}, "project_id = ?", []any{fixture.projectID}},
				{&model.SourceSpanIndexVersion{}, "project_id = ?", []any{fixture.projectID}},
				{&model.DocumentRevision{}, "document_id = ?", []any{fixture.documentID}},
				{&model.ScriptDocument{}, "id = ?", []any{fixture.documentID}},
				{&model.Project{}, "id = ?", []any{fixture.projectID}},
				{&model.Membership{}, "workspace_id = ?", []any{fixture.workspaceID}},
				{&model.Workspace{}, "id = ?", []any{fixture.workspaceID}},
				{&model.UserAccount{}, "id = ?", []any{fixture.userID}},
			}
			for _, deletion := range deletions {
				if deleteErr := transaction.Unscoped().Where(deletion.query, deletion.args...).Delete(deletion.model).Error; deleteErr != nil {
					return deleteErr
				}
			}
			return nil
		})
		if err != nil {
			t.Errorf("clean Source acceptance fixture: %v", err)
		}
	})
}
