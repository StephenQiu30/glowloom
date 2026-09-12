package gormdb_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	app "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Reuse the existing committed Call fixture. This proves storage concurrency,
// not accepted business sources or actual PNG validity (the journey proves those).
func assertReferenceStagedMediaStorage(t *testing.T, ctx context.Context, store *generationgorm.Store, receipt domain.ReferenceCallReceipt) {
	t.Helper()
	pending, err := domain.NewReferenceStagedMedia(receipt, domain.ReferenceObjectStoreRef{Profile: "minio", Bucket: "lanverse", ObjectKey: receipt.Output.StagingObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	read := func() domain.ReferenceStagedMedia {
		t.Helper()
		var value domain.ReferenceStagedMedia
		if err := store.WithinReferenceStagedMedia(ctx, func(repo app.ReferenceStagedMediaRepository) error {
			var err error
			value, err = repo.FindReferenceStagedMedia(ctx, receipt.WorkspaceID, receipt.ProjectID, receipt.Call.CallKey)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if err := store.WithinReferenceStagedMedia(ctx, func(repo app.ReferenceStagedMediaRepository) error {
		return repo.InsertReferenceStagedMedia(ctx, pending)
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(read(), pending) {
		t.Fatal("quarantine was not committed")
	}
	if err := store.WithinReferenceStagedMedia(ctx, func(repo app.ReferenceStagedMediaRepository) error {
		return repo.InsertReferenceStagedMedia(ctx, pending)
	}); err == nil {
		t.Fatal("duplicate Call media accepted")
	}
	rejected, err := domain.RejectReferenceStagedMedia(pending, "checksum_mismatch", receipt.ObservedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("injected staged completion rollback")
	if err := store.WithinReferenceStagedMedia(ctx, func(repo app.ReferenceStagedMediaRepository) error {
		if err := repo.UpdateReferenceStagedMedia(ctx, pending, rejected); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("completion rollback: %v", err)
	}
	if !reflect.DeepEqual(read(), pending) {
		t.Fatal("failed completion changed quarantine")
	}
	var workers sync.WaitGroup
	var writes atomic.Int64
	start := make(chan struct{})
	for range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			err := store.WithinReferenceStagedMedia(ctx, func(repo app.ReferenceStagedMediaRepository) error {
				return repo.UpdateReferenceStagedMedia(ctx, pending, rejected)
			})
			if err == nil {
				writes.Add(1)
			} else {
				var stateErr interface{ SQLState() string }
				if !errors.Is(err, app.ErrReferenceStagedMediaConflict) && !(errors.As(err, &stateErr) && stateErr.SQLState() == "40001") {
					t.Error(err)
				}
			}
		}()
	}
	close(start)
	workers.Wait()
	if writes.Load() != 1 || !reflect.DeepEqual(read(), rejected) {
		t.Fatalf("media CAS had %d writers", writes.Load())
	}
	if err := store.WithinReferenceStagedMedia(ctx, func(repo app.ReferenceStagedMediaRepository) error {
		if _, err := repo.FindReferenceStagedMedia(ctx, receipt.WorkspaceID, uuid.NewString(), receipt.Call.CallKey); !errors.Is(err, app.ErrReferenceStagedMediaNotFound) {
			return errors.New("foreign project read succeeded")
		}
		return repo.UpdateReferenceStagedMedia(ctx, rejected, pending)
	}); err == nil {
		t.Fatal("rejected media was reset")
	}
}

func TestReferenceStagedMediaRequiresPhysicalCommit(t *testing.T) {
	for _, db := range []*gorm.DB{nil, {}, {Config: &gorm.Config{}, Statement: &gorm.Statement{ConnPool: &sql.Tx{}}}} {
		store := generationgorm.New(db)
		if err := store.WithinReferenceStagedMedia(context.Background(), func(app.ReferenceStagedMediaRepository) error {
			t.Fatal("staging accepted a savepoint as a commit")
			return nil
		}); err == nil {
			t.Fatal("non-standalone staging transaction accepted")
		}
	}
}
