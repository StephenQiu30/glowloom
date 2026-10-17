package media_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type locatedTransferMemory struct{ data map[domain.ObjectRef][]byte }

type routedWorkerAdmission struct {
	mediaapp.TransferWorkerRepository
	failure error
}

func (r routedWorkerAdmission) ClaimTransfer(context.Context, mediaapp.TransferWorkID) (mediaapp.TransferLease, error) {
	return mediaapp.TransferLease{}, r.failure
}

func TestRoutedTransferWorkerReachesPersistentAdmission(t *testing.T) {
	failure := errors.New("fixture admission unavailable")
	worker := mediaapp.NewRoutedTransferWorker(routedWorkerAdmission{failure: failure}, &locatedTransferMemory{}, t.TempDir())
	if _, err := worker.Execute(t.Context(), mediaapp.TransferWorkID{}); !errors.Is(err, failure) {
		t.Fatal("native physical port rejected before persistent admission", err)
	}
}

func (o *locatedTransferMemory) Get(_ context.Context, ref domain.ObjectRef) (io.ReadCloser, error) {
	b, ok := o.data[ref]
	if !ok {
		return nil, errors.New("missing exact object")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (o *locatedTransferMemory) Exists(_ context.Context, ref domain.ObjectRef) (bool, error) {
	_, ok := o.data[ref]
	return ok, nil
}
func (o *locatedTransferMemory) PutIfAbsent(_ context.Context, ref domain.ObjectRef, r io.Reader, _ int64, _, _ string) error {
	if _, ok := o.data[ref]; ok {
		return mediaapp.ErrObjectAlreadyExists
	}
	b, err := io.ReadAll(r)
	if err == nil {
		o.data[ref] = b
	}
	return err
}
func (o *locatedTransferMemory) Remove(_ context.Context, ref domain.ObjectRef) error {
	delete(o.data, ref)
	return nil
}

type routedCopyRepo struct{ object mediaapp.ProjectCopyObject }

func (r *routedCopyRepo) Objects(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, mediaapp.ProjectCopySnapshot) ([]mediaapp.ProjectCopyObject, error) {
	return []mediaapp.ProjectCopyObject{r.object}, nil
}
func (r *routedCopyRepo) RecordObjectDigest(_ context.Context, _ identityapp.Principal, _ mediaapp.ProjectCopyBinding, _ mediaapp.ProjectCopySnapshot, _ uuid.UUID, _, sha string, size int64) error {
	r.object.SHA256 = &sha
	r.object.ByteSize = &size
	r.object.SourceVerified = true
	return nil
}
func (r *routedCopyRepo) BeginObjectWrite(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, mediaapp.ProjectCopySnapshot, uuid.UUID, string) error {
	r.object.WriteStarted = true
	return nil
}
func (r *routedCopyRepo) ConfirmObject(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, mediaapp.ProjectCopySnapshot, uuid.UUID, string, string, int64) error {
	r.object.Status = "verified"
	return nil
}
func (r *routedCopyRepo) AuthorizeObjectRemoval(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, mediaapp.ProjectCopySnapshot, uuid.UUID, string) error {
	return nil
}
func (r *routedCopyRepo) ConfirmObjectRemoved(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, mediaapp.ProjectCopySnapshot, uuid.UUID, string) error {
	r.object.Status = "removed"
	return nil
}
func TestRoutedTransferCopiesSameKeyBetweenProfilesAndCleansOnlyTarget(t *testing.T) {
	data := []byte("original immutable bytes")
	facts := sourceDocumentAsset(data)
	source := readLocation("minio-main", "projects/source/image/same.png")
	target := readLocation("cos-main", source.Ref.Key)
	target.State = "reserved"
	repo := &routedCopyRepo{object: mediaapp.ProjectCopyObject{TargetAssetID: uuid.New(), SourceObjectKey: source.Ref.Key, TargetObjectKey: target.Ref.Key, SourceLocation: source, TargetLocation: target, ByteSize: &facts.ByteSize, SHA256: facts.SHA256, ContentType: "image/png", Status: "pending"}}
	objects := &locatedTransferMemory{data: map[domain.ObjectRef][]byte{source.Ref: data}}
	actor := identityapp.Principal{OrgID: uuid.New()}
	binding := mediaapp.ProjectCopyBinding{JobID: uuid.New(), OrgID: actor.OrgID, SourceProjectID: uuid.New(), TargetProjectID: uuid.New()}
	service := mediaapp.NewRoutedProjectCopyTransfer(repo, objects, t.TempDir())
	if err := service.Transfer(t.Context(), actor, binding, mediaapp.ProjectCopySnapshot{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(objects.data[target.Ref], data) || !bytes.Equal(objects.data[source.Ref], data) {
		t.Fatal("source or target physical bytes changed")
	}
	if err := service.Transfer(t.Context(), actor, binding, mediaapp.ProjectCopySnapshot{}); err != nil {
		t.Fatal("restarted readback", err)
	}
	if err := service.Cleanup(t.Context(), actor, binding, mediaapp.ProjectCopySnapshot{}); err != nil {
		t.Fatal(err)
	}
	if _, exists := objects.data[target.Ref]; exists || !bytes.Equal(objects.data[source.Ref], data) {
		t.Fatal("cleanup removed the source or retained target")
	}
}
