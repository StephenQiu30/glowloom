package media_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// This repository test seeds physical metadata only. The companion TLS pipeline
// test verifies actual source bytes, ffprobe, renditions and object writes.
func TestVideoLinkReceiptActualPGScopeFingerprintAndRemoval(t *testing.T) {
	db := mediaStoreDB(t)
	actor, _ := mediaStoreProject(t, db)
	foreign, _ := mediaStoreProject(t, db)
	store := mediaTestStore(db)
	key := uuid.New()
	name := "video-link-" + strings.Repeat("a", 64) + ".mp4"
	if _, found, err := store.LookupPersonalVideoLink(t.Context(), actor, key, name); err != nil || found {
		t.Fatal("new personal library cannot start", found, err)
	}
	id, location := uuid.New(), uuid.New()
	library, err := (domain.LibraryScope{Kind: domain.LibraryPersonal}).Identity(actor.OrgID, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	profile := "link-receipt-" + uuid.NewString()[:8]
	objectKey := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/video/2026/11/" + id.String() + ".mp4"
	sha := strings.Repeat("b", 64)
	for _, query := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity)VALUES(?,'cos','http://127.0.0.1','http://127.0.0.1','us-east-1','link-fixture','',?)`, []any{profile, strings.ReplaceAll(uuid.NewString(), "-", "") + strings.Repeat("c", 32)}},
		{`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type)VALUES(?,?,'link-fixture',?,'accepted',1,?,'video/mp4')`, []any{location, profile, objectKey, sha}},
		{`INSERT INTO media.media_asset(id,project_id,personal_org_id,personal_actor_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,moderation_status)VALUES(?,NULL,?,?,'video','upload','ready',?,?,'reference.mp4','video/mp4',1,?,'passed')`, []any{id, actor.OrgID, actor.ID, objectKey, location, sha}},
		{`INSERT INTO media.library(id,kind,org_id,personal_actor_id)VALUES(?,'personal',?,?)`, []any{library, actor.OrgID, actor.ID}},
		{`INSERT INTO media.library_item(id,library_id,asset_id,title,category,position)VALUES(?,?,?,'reference','material',0)`, []any{id, library, id}},
	} {
		if err := db.Exec(query.sql, query.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	body, err := json.Marshal(mediaapp.PersonalUploadResult{Asset: mediaapp.LibraryAssetSummary{ID: id, Kind: "video", Revision: 1, ByteSize: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.upload_request(project_id,personal_org_id,principal_id,request_key,sha256,file_name,byte_size,asset_id,response,source_kind)VALUES(NULL,?,?,?,?,?,1,?,?::jsonb,'video_link')`, actor.OrgID, actor.ID, key, sha, name, id, string(body)).Error; err != nil {
		t.Fatal(err)
	}
	forgedKey := uuid.New()
	if err := db.Exec(`INSERT INTO media.upload_request(project_id,personal_org_id,principal_id,request_key,sha256,file_name,byte_size,asset_id,response)VALUES(NULL,?,?,?,?,?,1,?,?::jsonb)`, actor.OrgID, actor.ID, forgedKey, sha, name, id, string(body)).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LookupPersonalVideoLink(t.Context(), actor, forgedKey, name); !errors.Is(err, mediaapp.ErrUploadConflict) {
		t.Fatal("local upload forged successful link provenance", err)
	}
	result, found, err := store.LookupPersonalVideoLink(t.Context(), actor, key, name)
	if err != nil || !found || result.Asset.ID != id {
		t.Fatal("receipt not recovered", found, err)
	}
	if _, _, err := store.LookupPersonalVideoLink(t.Context(), actor, key, "different-source.mp4"); !errors.Is(err, mediaapp.ErrUploadConflict) {
		t.Fatal("changed share page replayed", err)
	}
	if _, found, err := store.LookupPersonalVideoLink(t.Context(), foreign, key, name); err != nil || found {
		t.Fatal("foreign receipt exposed", found, err)
	}
	if err := db.Table("media.library_item").Where("id=?", id).Update("catalog_state", "removed").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LookupPersonalVideoLink(t.Context(), actor, key, name); !errors.Is(err, mediaapp.ErrUploadConflict) {
		t.Fatal("removed library item replayed", err)
	}
	if err := db.Table("identity.user").Where("id=?", actor.ID).Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LookupPersonalVideoLink(t.Context(), actor, key, name); err == nil {
		t.Fatal("revoked actor replayed receipt")
	}
}
