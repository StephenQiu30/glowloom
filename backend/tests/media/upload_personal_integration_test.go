package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestPersonalUploadActualPrivateObjectsClosedScopeAndPermanentReplay(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	foreign, _ := mediaStoreProject(t, db)
	store := pgmedia.NewStore(db)
	service := mediaapp.NewScopedUploadService(store, store, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	in := uploadInput(t)
	in.Actor = actor
	in.Request.ProjectID = uuid.Nil
	first, err := service.UploadPersonal(t.Context(), in)
	if err != nil || first.Asset.ID == uuid.Nil {
		t.Fatal("actual personal upload", err)
	}
	replay, err := service.UploadPersonal(t.Context(), in)
	if err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatal("personal receipt changed", replay, err)
	}
	in.Request.FileName = "renamed.png"
	if _, err := service.UploadPersonal(t.Context(), in); !errors.Is(err, mediaapp.ErrUploadConflict) {
		t.Fatal("different immutable body under original key", err)
	}
	in.Request.Key = uuid.New()
	reused, err := service.UploadPersonal(t.Context(), in)
	if err != nil || reused.DuplicateOf == nil || *reused.DuplicateOf != first.Asset.ID {
		t.Fatal("personal same hash reuse", reused, err)
	}
	var row struct {
		ProjectID                      *uuid.UUID
		PersonalOrgID, PersonalActorID uuid.UUID
		ObjectKey                      string
		ModerationDetail               []byte
	}
	if err := db.Raw(`SELECT project_id,personal_org_id,personal_actor_id,object_key,moderation_detail FROM media.media_asset WHERE id=?`, first.Asset.ID).Scan(&row).Error; err != nil || row.ProjectID != nil || row.PersonalOrgID != actor.OrgID || row.PersonalActorID != actor.ID {
		t.Fatal("personal owning row", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = objects.Remove(ctx, row.ObjectKey)
	})
	file, err := objects.Get(t.Context(), row.ObjectKey)
	if err != nil {
		t.Fatal("actual personal bytes", err)
	}
	data, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || !bytes.Equal(data, uploadPNG(t)) {
		t.Fatal("personal original bytes differ", err)
	}
	var rends []struct{ ObjectKey string }
	if err := db.Raw(`SELECT object_key FROM media.rendition WHERE media_asset_id=? ORDER BY kind`, first.Asset.ID).Scan(&rends).Error; err != nil || len(rends) != 2 {
		t.Fatal("actual personal thumbnails missing", err)
	}
	for _, rend := range rends {
		rend := rend
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = objects.Remove(ctx, rend.ObjectKey)
		})
		info, err := objects.Stat(t.Context(), rend.ObjectKey)
		if err != nil || info.Size <= 0 || len(info.SHA256) != 64 {
			t.Fatal("private rendition unavailable", err)
		}
	}
	var review mediaapp.LocalUploadReview
	if json.Unmarshal(row.ModerationDetail, &review) != nil || review.PrincipalID != actor.ID || review.SHA256 != in.File.SHA256 || !review.RightsConfirmed || !review.NoAuthorizationRequiredRealPerson {
		t.Fatal("personal review not exact", review)
	}
	if _, err := store.FindAsset(t.Context(), actor, project, first.Asset.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("project ordinary media read accepted personal", err)
	}
	if _, err := mediaapp.NewAssetQuery(store, nil).Reference(t.Context(), actor, project, first.Asset.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("personal became formal project reference", err)
	}
	catalog := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	page, err := catalog.ListLibrary(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryPersonal}, libraryQuery())
	if err != nil || page.Total != 1 || page.Items[0].AssetID == nil || *page.Items[0].AssetID != first.Asset.ID || page.Revision != 1 {
		t.Fatal("actual personal library missing upload", page, err)
	}
	if _, err := catalog.LibraryDetail(t.Context(), foreign, domain.LibraryScope{Kind: domain.LibraryPersonal}, first.Asset.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatal("foreign user can read personal catalog", err)
	}
	req := in.Request
	req.Personal = &domain.PersonalOwnership{OrgID: actor.OrgID, ActorID: actor.ID}
	req.SHA256 = in.File.SHA256
	req.ByteSize = in.File.Size
	if _, _, err := store.FindPersonalUpload(t.Context(), foreign, req); !errors.Is(err, mediaapp.ErrInvalidUpload) {
		t.Fatal("foreign identity can replay personal receipt", err)
	}
	payload, _ := json.Marshal(first)
	if strings.Contains(string(payload), "project_id") || strings.Contains(string(payload), "object_key") {
		t.Fatal("public personal response leaked private internals")
	}
}
