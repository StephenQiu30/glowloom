package gormdb_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
)

func TestSourceSpanReadsExactUnicodeEvidenceAndChecksCurrentAccess(t *testing.T) {
	dsn := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run source evidence reads")
	}
	ctx := context.Background()
	db, err := platformdatabase.Open(ctx, dsn, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(db) })
	if err = schema.Sync(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx := beginSourceAcceptanceTestTransaction(t, db)
	fixture := seedSourceAcceptanceProject(t, func(v any) error { return tx.Create(v).Error }, time.Now().UTC(), "甲😀乙\r\n第二场。")
	service := scriptapp.NewSourceService(scriptgorm.New(tx), scriptapp.SourceConfig{NewID: uuid.NewString})
	actor := scriptapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	if _, err = service.Accept(ctx, actor, scriptapp.AcceptSourceCommand{ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(), IdempotencyKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	query := scriptapp.SourceSpanQuery{ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(), Start: 1, End: 4, ExpectedHash: fixture.normalizedHash}
	span, err := service.ReadSpan(ctx, actor, query)
	if err != nil || span.Text != "😀乙\n" || span.TextHash != sourceHash("😀乙\n") || span.Identity.VersionID != fixture.revisionID.String() {
		t.Fatalf("wrong evidence: %+v %v", span, err)
	}
	for _, bounds := range [][2]int{{-1, 2}, {2, 2}, {2, 1}, {0, 100000}, {0, 16385}} {
		invalid := query
		invalid.Start, invalid.End = bounds[0], bounds[1]
		if _, err = service.ReadSpan(ctx, actor, invalid); scriptapp.ErrorCode(err) != "validation_failed" {
			t.Fatalf("invalid bounds %+v: %v", bounds, err)
		}
	}
	stale := query
	stale.ExpectedHash = strings.Repeat("f", 64)
	if _, err = service.ReadSpan(ctx, actor, stale); scriptapp.ErrorCode(err) != "source_hash_drift" {
		t.Fatalf("stale source accepted: %v", err)
	}
	foreign := seedSourceAcceptanceProject(t, func(v any) error { return tx.Create(v).Error }, time.Now().UTC(), "别人的原稿")
	foreignQuery := query
	foreignQuery.DocumentRevisionID = foreign.revisionID.String()
	if _, err = service.ReadSpan(ctx, actor, foreignQuery); err == nil {
		t.Fatal("cross-project source exposed")
	}
	// A new source head cannot change an existing citation.
	newID := seedSourceRevision(t, func(v any) error { return tx.Create(v).Error }, fixture, time.Now().UTC(), 2, "新版原文")
	old, err := service.GetExact(ctx, actor, query.ProjectID, query.DocumentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Accept(ctx, actor, scriptapp.AcceptSourceCommand{ProjectID: query.ProjectID, DocumentRevisionID: newID.String(), ExpectedHeadRevision: old.HeadRevision, ExpectedHeadHash: &old.HeadHash, IdempotencyKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	span, err = service.ReadSpan(ctx, actor, query)
	if err != nil || span.Text != "😀乙\n" {
		t.Fatalf("old citation drifted: %+v %v", span, err)
	}
	if err = tx.Model(&model.Membership{}).Where("user_id = ? AND workspace_id = ?", fixture.userID, fixture.workspaceID).Update("status", "removed").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.ReadSpan(ctx, actor, query); err == nil {
		t.Fatal("revoked user read source")
	}
}
