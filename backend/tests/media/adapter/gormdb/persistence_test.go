package media_test

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	adapter "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gormdb"
	app "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	database "github.com/StephenQiu30/lanverse/backend/internal/platform/database"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/schema"
	testgorm "github.com/StephenQiu30/lanverse/backend/tests/platform/adapter/gormdb"
	"github.com/google/uuid"
)

type signedStorage struct{ app.ObjectStore }

func (signedStorage) PresignPut(context.Context, string, time.Duration) (*url.URL, error) {
	return url.Parse("https://objects.example.test/synthetic")
}
func TestConcurrentUploadInitializationReturnsOneSession(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "identical declarations"
		if mixed {
			name = "conflicting declarations"
		}
		t.Run(name, func(t *testing.T) { verifyConcurrentUploadInitialization(t, mixed) })
	}
}

func verifyConcurrentUploadInitialization(t *testing.T, mixed bool) {
	t.Helper()
	dsn := os.Getenv("LANVERSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, dsn, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	if err = schema.Sync(ctx, db); err != nil {
		t.Fatal(err)
	}
	user, workspace := uuid.New(), uuid.New()
	now := time.Now().UTC()
	for _, record := range []any{&model.UserAccount{ID: user, EmailNormalized: user.String() + "@example.test", PasswordHash: "synthetic", TokenVersion: 1, DisplayName: "test", Status: "active", CreatedAt: now, UpdatedAt: now}, &model.Workspace{ID: workspace, Name: "upload concurrency", Status: "active", Revision: 1, CreatedAt: now, UpdatedAt: now}, &model.Membership{ID: uuid.New(), WorkspaceID: workspace, UserID: user, Role: "owner", Status: "active", JoinedAt: now}} {
		if err = db.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	testgorm.RegisterOwnedWorkspaceFixtureCleanup(t, db, testgorm.OwnedWorkspaceFixture{UserIDs: []string{user.String()}, WorkspaceID: workspace.String()})
	var delayed atomic.Bool
	if err = db.Callback().Query().After("gorm:query").Register("test:hold_first_missing_upload", func(tx *gorm.DB) {
		if tx.Statement.Table == "med_upload_sessions" && tx.Error == gorm.ErrRecordNotFound && delayed.CompareAndSwap(false, true) {
			time.Sleep(250 * time.Millisecond)
		}
	}); err != nil {
		t.Fatal(err)
	}
	service := app.NewService(adapter.New(db), signedStorage{}, app.Config{Now: time.Now, NewID: uuid.NewString})
	command := app.InitializeCommand{WorkspaceID: workspace.String(), Kind: "document", Filename: "source.txt", MIMEType: "text/plain", SHA256: strings.Repeat("a", 64), SizeBytes: 10, IdempotencyKey: "concurrent-upload"}
	const count = 12
	results := make([]app.Initialization, count)
	failures := make([]error, count)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			<-start
			request := command
			if mixed && i%2 == 1 {
				request.SHA256 = strings.Repeat("b", 64)
			}
			results[i], failures[i] = service.Initialize(ctx, app.Actor{UserID: user.String(), TokenVersion: 1}, request)
		})
	}
	close(start)
	wg.Wait()
	var sessionID string
	conflicts := 0
	for i, err := range failures {
		if err != nil {
			var failure *app.Error
			if mixed && errors.As(err, &failure) && failure.Code == "idempotency_conflict" {
				conflicts++
				continue
			}
			t.Errorf("request %d: %v", i, err)
			continue
		}
		if sessionID == "" {
			sessionID = results[i].Session.ID
		}
		if results[i].Session.ID != sessionID {
			t.Error("duplicate session identity")
		}
	}
	if sessionID == "" || (mixed && conflicts != count/2) {
		t.Fatalf("successful session=%q, conflicting declarations=%d", sessionID, conflicts)
	}
	var rows int64
	if err = db.Model(&model.UploadSession{}).Where("workspace_id = ?", workspace).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
}
