package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type personalUploadFake struct {
	asset     domain.MediaAsset
	commits   int
	commitErr error
	exists    bool
	existsErr error
}

func (*personalUploadFake) AuthorizePersonalUpload(context.Context, identityapp.Principal) error {
	return nil
}
func (*personalUploadFake) FindPersonalUpload(context.Context, identityapp.Principal, mediaapp.UploadRequest) (mediaapp.PersonalUploadResult, bool, error) {
	return mediaapp.PersonalUploadResult{}, false, nil
}
func (r *personalUploadFake) CommitPersonalUpload(_ context.Context, _ identityapp.Principal, _ mediaapp.UploadRequest, asset domain.MediaAsset, _ []domain.Rendition) (mediaapp.PersonalUploadResult, error) {
	r.asset = asset
	r.commits++
	return mediaapp.PersonalUploadResult{Asset: mediaapp.PersonalUploadSummary(asset)}, r.commitErr
}
func (r *personalUploadFake) PersonalUploadAssetExists(context.Context, domain.PersonalOwnership, uuid.UUID) (bool, error) {
	return r.exists, r.existsErr
}

func TestPersonalUploadPublishesReviewedOwnedMediaThroughSharedPipeline(t *testing.T) {
	repo, objects := &personalUploadFake{}, &uploadObjectsFake{}
	service := mediaapp.NewScopedUploadService(&uploadRepoFake{}, repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	in := uploadInput(t)
	in.Request.ProjectID = uuid.Nil
	result, err := service.UploadPersonal(t.Context(), in)
	if err != nil || result.Asset.ID == uuid.Nil || repo.commits != 1 || repo.asset.Personal == nil || repo.asset.Personal.OrgID != in.Actor.OrgID || repo.asset.Personal.ActorID != in.Actor.ID || repo.asset.ProjectID != uuid.Nil || repo.asset.CanReference() || len(objects.items) != 3 {
		t.Fatal("personal actual probe/render/ownership failed", err)
	}
	data, err := json.Marshal(result)
	if err != nil || bytes.Contains(data, []byte("project_id")) || bytes.Contains(data, []byte("object_key")) || bytes.Contains(data, []byte("personal_actor")) || !strings.HasPrefix(repo.asset.ObjectKey, "personal/"+in.Actor.OrgID.String()+"/"+in.Actor.ID.String()+"/") {
		t.Fatal("personal response leaked internal scope", string(data), err)
	}
	if _, err := service.Upload(t.Context(), in); !errors.Is(err, mediaapp.ErrInvalidUpload) {
		t.Fatal("ordinary project upload accepted personal scope", err)
	}
}

func TestPersonalUploadRejectsCallerOwnershipAndRequiresExplicitReview(t *testing.T) {
	for _, kind := range []string{"project", "ownership", "review"} {
		t.Run(kind, func(t *testing.T) {
			repo, objects := &personalUploadFake{}, &uploadObjectsFake{}
			service := mediaapp.NewScopedUploadService(&uploadRepoFake{}, repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
			in := uploadInput(t)
			in.Request.ProjectID = uuid.Nil
			switch kind {
			case "project":
				in.Request.ProjectID = uuid.New()
			case "ownership":
				in.Request.Personal = &domain.PersonalOwnership{OrgID: in.Actor.OrgID, ActorID: uuid.New()}
			case "review":
				in.LocalReviewConfirmed = false
			}
			if _, err := service.UploadPersonal(t.Context(), in); !errors.Is(err, mediaapp.ErrInvalidUpload) || repo.commits != 0 || len(objects.items) != 0 {
				t.Fatal("unreviewed or caller scope was accepted", err)
			}
		})
	}
}
func TestPersonalUploadUnknownCommitNeverDestroysOwnedObjects(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exists   bool
		checkErr error
		objects  int
	}{{"rollback", false, nil, 0}, {"committed", true, nil, 3}, {"unknown", false, mediaapp.ErrUnavailable, 3}} {
		t.Run(tc.name, func(t *testing.T) {
			repo, objects := &personalUploadFake{commitErr: context.Canceled, exists: tc.exists, existsErr: tc.checkErr}, &uploadObjectsFake{}
			service := mediaapp.NewScopedUploadService(&uploadRepoFake{}, repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
			in := uploadInput(t)
			in.Request.ProjectID = uuid.Nil
			if _, err := service.UploadPersonal(t.Context(), in); !errors.Is(err, context.Canceled) || len(objects.items) != tc.objects {
				t.Fatal("unsafe unknown cleanup", err, len(objects.items))
			}
		})
	}
}
func TestPersonalUploadNilRequestJSONKeepsHistoricalProjectBytes(t *testing.T) {
	id := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	r := mediaapp.UploadRequest{ProjectID: id, Key: id, SHA256: strings.Repeat("a", 64), FileName: "x.png", ByteSize: 42, RequestID: id}
	got, err := json.Marshal(r)
	const want = `{"ProjectID":"11111111-1111-4111-8111-111111111111","Key":"11111111-1111-4111-8111-111111111111","SHA256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","FileName":"x.png","ByteSize":42,"RequestID":"11111111-1111-4111-8111-111111111111"}`
	if err != nil || string(got) != want {
		t.Fatal("project upload nil ownership bytes changed", string(got), err)
	}
}
