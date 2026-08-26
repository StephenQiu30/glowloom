package workflow_test

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	platformdatabase "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	storyboardgorm "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/adapter/gormdb"
	storyboardapp "github.com/StephenQiu30/lanverse/backend/internal/production/storyboard/application"
)

func TestShotImageBindingTargetFreezesReplacementAndRejectsConcurrentStaleWriter(t *testing.T) {
	databaseURL := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LANVERSE_TEST_DATABASE_URL to run Shot image binding target integration")
	}
	ctx := context.Background()
	database, err := platformdatabase.Open(ctx, databaseURL, io.Discard)
	if err != nil {
		t.Fatalf("open Shot image binding target database: %v", err)
	}
	t.Cleanup(func() { _ = platformdatabase.Close(database) })
	if err = schema.Sync(ctx, database); err != nil {
		t.Fatalf("synchronize Shot image binding target GORM catalog: %v", err)
	}
	now := time.Date(2026, time.August, 27, 9, 0, 0, 0, time.UTC)
	fixture := seedCompilerProject(t, func(value any) error { return database.Create(value).Error }, now)
	shotID := seedFormalStoryboardShot(t, func(value any) error { return database.Create(value).Error }, fixture, now)
	actor := storyboardapp.Actor{UserID: fixture.userID.String(), TokenVersion: 1}
	first, second, third := selectedImage(fixture, "a"), selectedImage(fixture, "b"), selectedImage(fixture, "c")
	service := storyboardapp.NewShotImageBindingService(
		storyboardgorm.New(database),
		&selectedImageSource{selections: map[string]storyboardapp.SelectedImageSnapshot{
			first.ID: first, second.ID: second, third.ID: third,
		}},
		storyboardapp.Config{Now: func() time.Time { return now }, NewID: uuid.NewString},
	)

	initial, err := service.RequireShotImageBindingTarget(ctx, actor, shotID.String())
	if err != nil || initial.Shot.ID != shotID.String() || initial.ExpectedCurrentRevision != 0 ||
		initial.CurrentBindingID != "" || len(initial.ContentHash) != 64 {
		t.Fatalf("freeze initial Shot image binding target: target=%#v err=%v", initial, err)
	}
	firstResult, err := service.BindSelectedImageAtTarget(ctx, actor, storyboardapp.BindSelectedImageAtTargetCommand{
		ShotID: shotID.String(), CandidateSelectionID: first.ID,
		ExpectedShotRevision: initial.Shot.Revision, ExpectedShotContentHash: initial.Shot.ContentHash,
		ExpectedCurrentRevision: initial.ExpectedCurrentRevision, ExpectedBindingTargetHash: initial.ContentHash,
		IdempotencyKey: "shot-target-first",
	})
	if err != nil || firstResult.Binding.Revision != 1 {
		t.Fatalf("bind first image at frozen target: result=%#v err=%v", firstResult, err)
	}
	replayed, err := service.BindSelectedImageAtTarget(ctx, actor, storyboardapp.BindSelectedImageAtTargetCommand{
		ShotID: shotID.String(), CandidateSelectionID: first.ID,
		ExpectedShotRevision: initial.Shot.Revision, ExpectedShotContentHash: initial.Shot.ContentHash,
		ExpectedCurrentRevision: initial.ExpectedCurrentRevision, ExpectedBindingTargetHash: initial.ContentHash,
		IdempotencyKey: "shot-target-first",
	})
	if err != nil || replayed.Binding.ID != firstResult.Binding.ID || replayed.Receipt.ID != firstResult.Receipt.ID {
		t.Fatalf("replay first binding after target advanced: result=%#v err=%v", replayed, err)
	}
	if _, err = service.BindSelectedImageAtTarget(ctx, actor, storyboardapp.BindSelectedImageAtTargetCommand{
		ShotID: shotID.String(), CandidateSelectionID: second.ID,
		ExpectedShotRevision: initial.Shot.Revision, ExpectedShotContentHash: initial.Shot.ContentHash,
		ExpectedCurrentRevision: initial.ExpectedCurrentRevision, ExpectedBindingTargetHash: initial.ContentHash,
		IdempotencyKey: "shot-target-stale",
	}); err == nil {
		t.Fatal("accepted a stale Shot image binding target")
	}

	replacement, err := service.RequireShotImageBindingTarget(ctx, actor, shotID.String())
	if err != nil || replacement.ExpectedCurrentRevision != 1 ||
		replacement.CurrentBindingID != firstResult.Binding.ID || replacement.CurrentBindingContentHash != firstResult.Binding.ContentHash ||
		replacement.ContentHash == initial.ContentHash {
		t.Fatalf("freeze replacement Shot image binding target: target=%#v err=%v", replacement, err)
	}
	tampered := replacement
	tampered.ContentHash = strings.Repeat("0", 64)
	if _, err = service.BindSelectedImageAtTarget(ctx, actor, storyboardapp.BindSelectedImageAtTargetCommand{
		ShotID: shotID.String(), CandidateSelectionID: second.ID,
		ExpectedShotRevision: tampered.Shot.Revision, ExpectedShotContentHash: tampered.Shot.ContentHash,
		ExpectedCurrentRevision: tampered.ExpectedCurrentRevision, ExpectedBindingTargetHash: tampered.ContentHash,
		IdempotencyKey: "shot-target-tampered",
	}); err == nil {
		t.Fatal("accepted a tampered Shot image binding target hash")
	}

	type attempt struct {
		selectionID string
		key         string
	}
	attempts := []attempt{{second.ID, "shot-target-second"}, {third.ID, "shot-target-third"}}
	results := make(chan storyboardapp.BindSelectedImageResult, len(attempts))
	errorsFound := make(chan error, len(attempts))
	var workers sync.WaitGroup
	for _, value := range attempts {
		workers.Add(1)
		go func(value attempt) {
			defer workers.Done()
			result, bindErr := service.BindSelectedImageAtTarget(ctx, actor, storyboardapp.BindSelectedImageAtTargetCommand{
				ShotID: shotID.String(), CandidateSelectionID: value.selectionID,
				ExpectedShotRevision: replacement.Shot.Revision, ExpectedShotContentHash: replacement.Shot.ContentHash,
				ExpectedCurrentRevision:   replacement.ExpectedCurrentRevision,
				ExpectedBindingTargetHash: replacement.ContentHash, IdempotencyKey: value.key,
			})
			if bindErr != nil {
				errorsFound <- bindErr
				return
			}
			results <- result
		}(value)
	}
	workers.Wait()
	close(results)
	close(errorsFound)
	if len(results) != 1 || len(errorsFound) != 1 {
		t.Fatalf("concurrent frozen target writers did not yield one winner: successes=%d errors=%d", len(results), len(errorsFound))
	}
	winner := <-results
	if winner.Binding.Revision != 2 {
		t.Fatalf("frozen target winner revision=%d, want 2", winner.Binding.Revision)
	}
}
