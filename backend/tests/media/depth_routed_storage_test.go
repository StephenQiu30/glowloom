package media_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	tooldomain "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type routedDepthClaim struct {
	toolapp.DepthWorkerStore
	work     toolapp.DepthWork
	finished int
}

func (s *routedDepthClaim) Claim(context.Context, toolapp.DepthWorkID) (toolapp.DepthWork, error) {
	return s.work, nil
}

func (s *routedDepthClaim) Finish(context.Context, toolapp.DepthWorkID, bool, bool, string) (tooldomain.DepthJob, error) {
	s.finished++
	return s.work.Job, nil
}

type routedDepthObjectsFactory struct {
	owner  mediaapp.StorageWorkOwner
	inputs []mediadomain.MediaAsset
	err    error
}

func (f *routedDepthObjectsFactory) Open(_ context.Context, owner mediaapp.StorageWorkOwner, inputs []mediadomain.MediaAsset) (toolapp.DepthObjects, error) {
	f.owner, f.inputs = owner, inputs
	return nil, f.err
}

func TestDepthRoutedWorkerClosesClaimWhenExactStorageUnavailable(t *testing.T) {
	job, project := uuid.New(), uuid.New()
	asset := mediadomain.MediaAsset{ID: uuid.New(), ProjectID: project, ObjectKey: "private/source.mp4"}
	store := &routedDepthClaim{work: toolapp.DepthWork{
		Job:          tooldomain.DepthJob{ID: job, ProjectID: project, Status: tooldomain.DepthRunning, Attempt: 2},
		SourceAssets: []mediadomain.MediaAsset{asset},
	}}
	factory := &routedDepthObjectsFactory{err: mediaapp.ErrStorageNotReady}
	worker := toolapp.NewRoutedDepthWorker(store, factory, nil, nil, nil, nil)
	_, err := worker.Execute(t.Context(), toolapp.DepthWorkID{JobID: job, Attempt: 2, ExecutionID: uuid.New()})
	if !errors.Is(err, mediaapp.ErrStorageNotReady) || store.finished != 1 {
		t.Fatal("storage failure leaked the claimed execution", err, store.finished)
	}
	if factory.owner != toolapp.StorageOwner("depth", project, job, 2) || len(factory.inputs) != 1 || factory.inputs[0].ID != asset.ID {
		t.Fatal("attempt or authorized source identity lost")
	}
	body, err := json.Marshal(store.work)
	if err != nil || strings.Contains(string(body), "private/source.mp4") {
		t.Fatal("internal source locations exposed by work JSON", err)
	}
}

type routedDepthQueryStore struct {
	toolapp.DepthStore
	job   tooldomain.DepthJob
	asset mediadomain.MediaAsset
}

func (s routedDepthQueryStore) PreviewAsset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (tooldomain.DepthJob, mediadomain.MediaAsset, error) {
	return s.job, s.asset, nil
}

func TestDepthRoutedQueryUsesExactAcceptedLocationAndActualExpiry(t *testing.T) {
	sha := strings.Repeat("a", 64)
	asset := imageAsset(uuid.New(), uuid.New())
	asset.Kind, asset.MimeType, asset.FileName = mediadomain.KindVideo, "video/mp4", "depth.mp4"
	asset.ObjectKey = "projects/" + asset.ProjectID.String() + "/video/2026/10/" + asset.ID.String() + ".mp4"
	asset.SHA256, asset.ByteSize = &sha, 5
	asset.ObjectLocation = readLocation("tos-frozen", asset.ObjectKey)
	expiry := time.Now().UTC().Add(43 * time.Second).Truncate(time.Second)
	objects := &routedReadObjects{grants: map[mediadomain.ObjectRef]mediaapp.AccessGrant{asset.ObjectLocation.Ref: {URL: "https://fixture.invalid/depth", ExpiresAt: expiry}}, data: map[mediadomain.ObjectRef][]byte{asset.ObjectLocation.Ref: []byte("depth")}}
	store := routedDepthQueryStore{job: tooldomain.DepthJob{ID: uuid.New(), Status: tooldomain.DepthSucceeded, SHA256: &sha}, asset: asset}
	query := toolapp.NewRoutedDepthQuery(store, objects, objects, nil, nil)
	preview, err := query.Preview(t.Context(), identityapp.Principal{}, asset.ProjectID, store.job.ID)
	if err != nil || !preview.ExpiresAt.Equal(expiry) {
		t.Fatal("depth preview replaced physical grant expiry", err)
	}
	file, size, err := query.Download(t.Context(), identityapp.Principal{}, asset.ProjectID, store.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil || size != 5 || string(body) != "depth" {
		t.Fatal("depth download ignored saved physical location", err, closeErr)
	}
	store.asset.ObjectLocation.State = "present"
	if _, err := query.Preview(t.Context(), identityapp.Principal{}, asset.ProjectID, store.job.ID); !errors.Is(err, toolapp.ErrConflict) {
		t.Fatal("nonaccepted output was signed", err)
	}
}
