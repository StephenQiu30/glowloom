package gormdb_test

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	creationgorm "github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/gormdb"
	"github.com/StephenQiu30/lanverse/backend/internal/production/creation/adapter/scriptreader"
	creation "github.com/StephenQiu30/lanverse/backend/internal/production/creation/application"
	scriptgorm "github.com/StephenQiu30/lanverse/backend/internal/production/script/adapter/gormdb"
	script "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
	"github.com/google/uuid"
)

func TestCreationSourceBridgeReadsFrozenNormalizedSourceAndRejectsRevocation(t *testing.T) {
	db := creationDatabase(t)
	tx := beginSourceAcceptanceTestTransaction(t, db)
	ctx := context.Background()
	now := time.Now().UTC()
	fixture := seedSourceAcceptanceProject(t, func(v any) error { return tx.Create(v).Error }, now, "甲😀e\u0301\r\n乙")
	acceptCreationSource(t, tx, fixture)
	store := creationgorm.New(tx)
	actor := creation.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	run, err := creation.NewService(store, creation.Config{Endpoint: "https://creation.example.invalid", Now: time.Now, NewID: uuid.NewString}).Create(ctx, actor, creation.CreateCommand{ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(), SourceHash: fixture.normalizedHash, IdempotencyKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	sources := script.NewSourceService(scriptgorm.New(tx), script.SourceConfig{NewID: uuid.NewString})
	bridge := creation.NewSourceService(store, scriptreader.New(sources))
	old, err := sources.GetExact(ctx, script.Actor{UserID: actor.UserID, TokenVersion: 1}, run.Command.ProjectID, run.Command.Source.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	newRevision := seedSourceRevision(t, func(v any) error { return tx.Create(v).Error }, fixture, now, 2, "new head")
	if _, err = sources.Accept(ctx, script.Actor{UserID: actor.UserID, TokenVersion: 1}, script.AcceptSourceCommand{ProjectID: run.Command.ProjectID, DocumentRevisionID: newRevision.String(), ExpectedHeadRevision: old.HeadRevision, ExpectedHeadHash: &old.HeadHash, IdempotencyKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, err := bridge.ReadSource(ctx, run.Command.RunID, run.PayloadHash)
		if err != nil || got.Text != "甲😀e\u0301\n乙" || got.ContentHash != fixture.normalizedHash || got.RevisionID != fixture.revisionID.String() {
			t.Fatalf("frozen text drifted: %+v %v", got, err)
		}
	}
	if err = tx.Model(&model.UserAccount{}).Where("id = ?", fixture.userID).Update("token_version", 2).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.ReadSource(ctx, run.Command.RunID, run.PayloadHash); err == nil {
		t.Fatal("old token version read source")
	}
}

func TestCreationSourceBridgeReleasesProjectLockBeforeOwnerRead(t *testing.T) {
	db := creationDatabase(t)
	fixture := seedSourceAcceptanceProject(t, func(v any) error { return db.Create(v).Error }, time.Now().UTC(), "甲😀e\u0301\r\n乙")
	registerSourceAcceptanceFixtureCleanup(t, db, fixture, []string{"creation-source"})
	acceptCreationSource(t, db, fixture)
	store := creationgorm.New(db)
	actor := creation.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	run, err := creation.NewService(store, creation.Config{Endpoint: "https://creation.example.invalid", Now: time.Now, NewID: uuid.NewString}).Create(context.Background(), actor, creation.CreateCommand{ProjectID: fixture.projectID.String(), DocumentRevisionID: fixture.revisionID.String(), SourceHash: fixture.normalizedHash, IdempotencyKey: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, row := range []any{&model.CreationCommandOutbox{}, &model.CreationRun{}} {
			key := "run_id"
			if _, ok := row.(*model.CreationRun); ok {
				key = "id"
			}
			if err := db.Where(key+" = ?", run.Command.RunID).Delete(row).Error; err != nil {
				t.Error(err)
			}
		}
	})
	// Use independent transactions from the connection pool, as bootstrap does.
	// An enclosing test transaction turns them into savepoints and hides self-locks.
	sources := script.NewSourceService(scriptgorm.New(db), script.SourceConfig{NewID: uuid.NewString})
	bridge := creation.NewSourceService(store, scriptreader.New(sources))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := bridge.ReadSource(ctx, run.Command.RunID, run.PayloadHash)
	if err != nil || got.Text != "甲😀e\u0301\n乙" || got.ContentHash != fixture.normalizedHash {
		t.Fatalf("independent owner read blocked or drifted: %+v %v", got, err)
	}
}
