package microservices_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
	mediapg "github.com/StephenQiu30/glowloom-server/backend/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
)

func newMediaCoverFixture(t *testing.T) pinFixture {
	t.Helper()
	f := newPinFixture(t)
	asset, location := uuid.New(), uuid.New()
	key := "projects/" + f.acquire.Identity.ProjectID.String() + "/image/2026/11/" + asset.String() + ".png"
	if err := f.admin.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) SELECT ?,profile_id,bucket,?,'accepted',byte_size,sha256,mime_type FROM media.object_location WHERE id=?`, location, key, f.acquire.Identity.LocationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,mime_type,byte_size,sha256,moderation_status,moderation_detail,object_location_id) SELECT ?,project_id,kind,origin,status,?,mime_type,byte_size,sha256,moderation_status,moderation_detail,? FROM media.media_asset WHERE id=?`, asset, key, location, f.acquire.Identity.AssetID).Error; err != nil {
		t.Fatal(err)
	}
	f.acquire.Identity.AssetID, f.acquire.Identity.LocationID = asset, location
	return f
}

func mediaCoverRequest(t *testing.T, f pinFixture) mediaapp.ProjectCoverRequest {
	t.Helper()
	work := mediaWorkRequest(t, f)
	return mediaapp.ProjectCoverRequest{CloseID: work.CloseID, ProjectID: work.ProjectID, OrganizationID: work.OrganizationID, ScopeID: work.ScopeID, AssetID: f.acquire.Identity.AssetID, ProjectEpoch: 2, CoordinationID: uuid.NewSHA1(work.CloseID, []byte("creation.project.close.authority"))}
}

func installMediaCoverBarrier(t *testing.T, f pinFixture, request mediaapp.ProjectCoverRequest) {
	t.Helper()
	if err := pgfence.New(f.media).Install(t.Context(), writefence.Control{Authority: writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Project, ID: request.ProjectID, Epoch: request.ProjectEpoch}, Blocked: true, CoordinationID: request.CoordinationID}); err != nil {
		t.Fatal(err)
	}
}

func TestMediaProjectCoverRequiresOriginalClosedBarrier(t *testing.T) {
	f := newMediaCoverFixture(t)
	request := mediaCoverRequest(t, f)
	reader := mediapg.NewProjectCoverReader(f.media)
	if _, err := reader.ReadProjectCover(t.Context(), request); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("open project yielded a frozen cover observation: %v", err)
	}
	installMediaCoverBarrier(t, f, request)
	value, err := reader.ReadProjectCover(t.Context(), request)
	if err != nil || value.Request != request || !value.Available || value.AssetRevision != 1 {
		t.Fatalf("original image observation: %+v %v", value, err)
	}
	wrong := request
	wrong.CloseID = uuid.New()
	wrong.CoordinationID = uuid.NewSHA1(wrong.CloseID, []byte("creation.project.close.authority"))
	if _, err := reader.ReadProjectCover(t.Context(), wrong); !errors.Is(err, writefence.ErrFenced) {
		t.Fatalf("another closure received an observation: %v", err)
	}
	for _, owner := range []string{"identity", "creation", "execution"} {
		if _, err := mediapg.NewProjectCoverReader(authorityDatabase(t, owner)).ReadProjectCover(t.Context(), request); err == nil {
			t.Fatalf("%s read the Media cover participant", owner)
		}
	}
	tx := f.media.Begin()
	defer func() { _ = tx.Rollback().Error }()
	if _, err := mediapg.NewProjectCoverReader(tx).ReadProjectCover(t.Context(), request); !errors.Is(err, writefence.ErrTransaction) {
		t.Fatalf("outer transaction accepted: %v", err)
	}
	if _, err := pgfence.New(f.media).Load(t.Context(), request.CloseID); !errors.Is(err, writefence.ErrNotFound) {
		t.Fatalf("display read admitted a command: %v", err)
	}
}

func TestMediaProjectCoverDistinguishesUnavailableFromUnknown(t *testing.T) {
	for _, test := range []struct {
		name, mutation string
		unknown        bool
	}{
		{name: "missing", mutation: "missing"},
		{name: "recycled", mutation: "catalog"},
		{name: "processing", mutation: `UPDATE media.media_asset SET status='processing' WHERE id=?`},
		{name: "unknown_location", mutation: `UPDATE media.object_location SET state='unknown' WHERE id=(SELECT object_location_id FROM media.media_asset WHERE id=?)`, unknown: true},
		{name: "deleted_location", mutation: `UPDATE media.object_location SET state='deleted' WHERE id=(SELECT object_location_id FROM media.media_asset WHERE id=?)`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newMediaCoverFixture(t)
			request := mediaCoverRequest(t, f)
			installMediaCoverBarrier(t, f, request)
			if test.mutation == "catalog" {
				library := uuid.New()
				if err := f.admin.Exec(`INSERT INTO media.library(id,org_id,kind,project_id) VALUES (?,?,'project',?)`, library, request.OrganizationID, request.ProjectID).Error; err != nil {
					t.Fatal(err)
				}
				if err := f.admin.Exec(`INSERT INTO media.library_item(id,library_id,asset_id,title,category,catalog_state,trashed_at,position) VALUES (?,?,?,'recycled cover','material','trashed',statement_timestamp(),0)`, request.AssetID, library, request.AssetID).Error; err != nil {
					t.Fatal(err)
				}
			} else if test.name == "missing" {
				request.AssetID = uuid.New()
			} else if err := f.admin.Exec(test.mutation, request.AssetID).Error; err != nil {
				t.Fatal(err)
			}
			value, err := mediapg.NewProjectCoverReader(f.media).ReadProjectCover(t.Context(), request)
			if test.unknown {
				if err == nil {
					t.Fatalf("unknown location became a negative fact: %+v", value)
				}
			} else if err != nil || value.Available || value.Request != request {
				t.Fatalf("known unavailable cover: %+v %v", value, err)
			}
		})
	}
}
