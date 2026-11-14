package media_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

type uploadCommitRepository struct {
	entered, release chan struct{}
	unknown          bool
	mu               sync.Mutex
	rejected         error
	committed        bool
	authorizations   atomic.Int32
	checks           atomic.Int32
	cleanupLive      atomic.Bool
}

func (r *uploadCommitRepository) AuthorizeUpload(context.Context, identity.Principal, uuid.UUID) (string, error) {
	r.authorizations.Add(1)
	return "16:9", nil
}
func (*uploadCommitRepository) FindUpload(context.Context, identity.Principal, media.UploadRequest) (media.UploadResult, bool, error) {
	return media.UploadResult{}, false, nil
}
func (r *uploadCommitRepository) CommitUpload(ctx context.Context, _ identity.Principal, _ media.UploadRequest, asset domain.MediaAsset, _ []domain.Rendition) (media.UploadResult, error) {
	// In the unknown-receipt case the durable commit precedes revocation; only
	// its response is delayed. In rejection cases authorization changes first.
	if r.unknown {
		r.mu.Lock()
		r.committed = true
		r.mu.Unlock()
	}
	close(r.entered)
	select {
	case <-r.release:
	case <-ctx.Done():
		return media.UploadResult{}, ctx.Err()
	}
	if r.unknown {
		return media.UploadResult{}, context.Canceled
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rejected != nil {
		return media.UploadResult{}, r.rejected
	}
	r.committed = true
	return media.UploadResult{Asset: media.UploadSummary(asset)}, nil
}
func (r *uploadCommitRepository) UploadAssetExists(ctx context.Context, _, _ uuid.UUID) (bool, error) {
	r.checks.Add(1)
	_, bounded := ctx.Deadline()
	r.cleanupLive.Store(bounded && ctx.Err() == nil)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.committed, nil
}

type uploadCommitObjects struct{ created, removed atomic.Int32 }

func (o *uploadCommitObjects) CreateUploadObject(_ context.Context, target media.UploadTarget, key string, file *media.Downloaded) (domain.ObjectLocation, error) {
	o.created.Add(1)
	return domain.ObjectLocation{
		ID: uuid.New(), Ref: domain.ObjectRef{ProfileID: target.ProfileID, Bucket: target.Bucket, Key: key}, State: domain.ObjectLocationPresent,
		ByteSize: &file.Size, SHA256: &file.SHA256, MIMEType: &file.MIMEType,
	}, nil
}
func (o *uploadCommitObjects) RemoveUploadObject(context.Context, domain.ObjectLocation) error {
	o.removed.Add(1)
	return nil
}

type uploadDocumentPipeline struct{}

func (uploadDocumentPipeline) Probe(context.Context, *media.Downloaded) (media.ProbeResult, error) {
	codec := "txt"
	return media.ProbeResult{Kind: domain.KindDocument, Extension: "txt", Codec: &codec}, nil
}
func (uploadDocumentPipeline) Render(context.Context, *media.Downloaded, media.ProbeResult, string) ([]media.RenditionFile, error) {
	return nil, nil
}

// This is an application/port contract, not evidence of PostgreSQL locking or
// a distributed authorization fence. Those require isolated real-DB acceptance.
func TestProjectUploadCommitAuthorityContract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reject  error
		unknown bool
	}{
		{"session revoked before commit", identity.ErrSessionInvalid, false},
		{"membership revoked before commit", media.ErrNotFound, false},
		{"project closed before commit", workspace.ErrProjectStateConflict, false},
		{"committed then revoked and response lost", identity.ErrSessionInvalid, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			repo := &uploadCommitRepository{entered: make(chan struct{}), release: make(chan struct{}), unknown: tc.unknown}
			objects := &uploadCommitObjects{}
			service := unitRoutedUploadService(repo, nil, uploadDocumentPipeline{}, mediaflow.FFUploadNormalizer{}, uploadDocumentPipeline{}, objects)
			file, err := media.ReadUpload(ctx, strings.NewReader("A local document with confirmed rights."), "reference.txt")
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = file.Close() })
			input := media.UploadInput{
				Actor:   identity.Principal{ID: uuid.New(), OrgID: uuid.New()},
				Request: media.UploadRequest{ProjectID: uuid.New(), Key: uuid.New(), FileName: "reference.txt", RequestID: uuid.New()},
				File:    file, LocalReviewConfirmed: true,
			}
			finished := make(chan error, 1)
			done := make(chan struct{})
			var release sync.Once
			go func() {
				defer close(done)
				_, err := fixtureProjectUpload(ctx, service, input)
				finished <- err
			}()
			t.Cleanup(func() {
				cancel()
				release.Do(func() { close(repo.release) })
				<-done
			})
			select {
			case <-repo.entered:
			case err := <-finished:
				t.Fatalf("pipeline stopped before final commit: %v", err)
			case <-ctx.Done():
				t.Fatal("pipeline did not reach final commit")
			}
			if objects.created.Load() != 1 || repo.authorizations.Load() != 1 {
				t.Fatal("test did not cross admission and verified-object creation")
			}
			repo.mu.Lock()
			repo.rejected = tc.reject
			repo.mu.Unlock()
			wantError, wantRemoved := tc.reject, int32(1)
			if tc.unknown {
				wantError, wantRemoved = context.Canceled, 0
				cancel()
			}
			release.Do(func() { close(repo.release) })
			err = <-finished
			if !errors.Is(err, wantError) || objects.removed.Load() != wantRemoved || repo.checks.Load() != 1 || !repo.cleanupLive.Load() {
				t.Fatalf("commit/cleanup contract: error=%v removed=%d checks=%d live cleanup=%v", err, objects.removed.Load(), repo.checks.Load(), repo.cleanupLive.Load())
			}
			repo.mu.Lock()
			committed := repo.committed
			repo.mu.Unlock()
			if committed != tc.unknown || repo.authorizations.Load() != 1 {
				t.Fatal("admission was treated as a commit grant, or cleanup required new business authority")
			}
		})
	}
}
