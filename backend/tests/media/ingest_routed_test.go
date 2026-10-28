package media_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/google/uuid"

	flow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type routedIngestFixture struct {
	operation  media.SourceOperation
	target     media.StorageWorkIntent
	asset      domain.MediaAsset
	renditions []domain.Rendition
	output     media.IngestOutput
	writes     int
	bad        bool
}

func (f *routedIngestFixture) FindOutput(context.Context, uuid.UUID, int) (media.IngestOutput, bool, error) {
	return f.output, f.output.OutputID != "", nil
}
func (f *routedIngestFixture) LoadOperation(context.Context, uuid.UUID) (media.SourceOperation, error) {
	return f.operation, nil
}
func (f *routedIngestFixture) SaveOutput(_ context.Context, a domain.MediaAsset, rs []domain.Rendition, _ int, id uuid.UUID) (media.IngestOutput, error) {
	f.asset, f.renditions = a, rs
	f.output = media.IngestOutput{OutputID: id.String(), MediaAssetID: a.ID.String(), ProjectID: a.ProjectID, ObjectKey: a.ObjectKey, ObjectLocation: a.ObjectLocation, Kind: string(a.Kind), MIMEType: a.MimeType, ByteSize: a.ByteSize, SHA256: *a.SHA256}
	return f.output, nil
}
func (f *routedIngestFixture) FindRenditions(context.Context, uuid.UUID) ([]domain.Rendition, error) {
	return f.renditions, nil
}
func (f *routedIngestFixture) ReadStorageWork(_ context.Context, o media.StorageWorkOwner) (media.StorageWorkIntent, error) {
	if o != f.target.Owner {
		return media.StorageWorkIntent{}, media.ErrStorageRouteConflict
	}
	return f.target, nil
}
func (*routedIngestFixture) Download(ctx context.Context, _ string) (*media.Downloaded, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 4))); err != nil {
		return nil, err
	}
	return media.ReadUpload(ctx, &b, "generated.png")
}
func (f *routedIngestFixture) CreateOperationObject(_ context.Context, in media.StorageWorkIntent, key string, file *media.Downloaded) (domain.ObjectLocation, error) {
	f.writes++
	ref := domain.ObjectRef{ProfileID: in.ProfileID, Bucket: in.Bucket, Key: key}
	if f.bad {
		ref.ProfileID = "unproved-new-default"
	}
	return domain.ObjectLocation{ID: uuid.New(), Ref: ref, State: "present", ByteSize: &file.Size, MIMEType: &file.MIMEType, SHA256: &file.SHA256}, nil
}
func (*routedIngestFixture) StatOperationObject(_ context.Context, loc domain.ObjectLocation) (media.ObjectInfo, error) {
	return media.ObjectInfo{Size: *loc.ByteSize, ContentType: *loc.MIMEType, SHA256: *loc.SHA256}, nil
}
func newRoutedIngestFixture() *routedIngestFixture {
	op, project := uuid.New(), uuid.New()
	return &routedIngestFixture{operation: media.SourceOperation{ID: op, ProjectID: project, ProviderKey: "controlled", ModelKey: "controlled", Region: "domestic", AspectRatio: "16:9", OutputKind: domain.KindImage, CreateTime: time.Now().UTC()}, target: media.StorageWorkIntent{ID: uuid.New(), Owner: media.StorageWorkOwner{Kind: "operation", OwnerID: project, WorkID: op}, ProfileID: "tos-frozen", Bucket: "controlled-fixture", ConfigRevision: "frozen-v1", State: "open"}}
}
func TestRoutedIngestPersistsPhysicalIdentityAndReplaysWithoutNewWrites(t *testing.T) {
	f := newRoutedIngestFixture()
	s := media.NewRoutedIngestService(f, f, flow.FFProber{}, flow.FFRenderer{}, f, f)
	in := media.IngestInput{OperationID: f.operation.ID.String(), SeqNo: 1, URL: "controlled staged bytes"}
	out, err := s.Ingest(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if out.ObjectLocation == nil || f.asset.ObjectLocation == nil || f.asset.ObjectLocation.Ref.ProfileID != "tos-frozen" || len(f.renditions) != 2 || f.writes != 3 {
		t.Fatal("formal location was not retained")
	}
	for _, r := range f.renditions {
		if r.ObjectLocation == nil || r.ObjectLocation.Ref.ProfileID != "tos-frozen" {
			t.Fatal("rendition lost fixed profile")
		}
	}
	replay, err := s.Ingest(t.Context(), in)
	if err != nil || replay.OutputID != out.OutputID || f.writes != 3 {
		t.Fatal("replay rewrote output", err)
	}
	f.output.ObjectLocation.Ref.ProfileID = "different-default"
	if _, err := s.Ingest(t.Context(), in); err == nil {
		t.Fatal("replay accepted foreign location")
	}
}
func TestRoutedIngestRejectsUnprovedLocationBeforePublishing(t *testing.T) {
	f := newRoutedIngestFixture()
	f.bad = true
	s := media.NewRoutedIngestService(f, f, flow.FFProber{}, flow.FFRenderer{}, f, f)
	if _, err := s.Ingest(t.Context(), media.IngestInput{OperationID: f.operation.ID.String(), SeqNo: 1, URL: "controlled"}); err == nil || f.output.OutputID != "" {
		t.Fatal("published a foreign location")
	}
}
