package media_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestStorageLegacyBindingRequiresExactOriginalResource(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	if err := db.Exec(`TRUNCATE media.storage_legacy_binding`).Error; err != nil {
		t.Fatal(err)
	}
	routes := pgmedia.NewStorageRoutes(db)
	profiles := storageProfiles(t, routes)
	if _, err := routes.ResolveLegacyRef(t.Context(), "projects/old/image/file.png"); !errors.Is(err, mediaapp.ErrStorageNotReady) {
		t.Fatal("unproved history acquired a default", err)
	}
	if err := routes.BindLegacyStorage(t.Context(), profiles[1], strings.Repeat("a", 64)); !errors.Is(err, mediaapp.ErrStorageIdentityConflict) {
		t.Fatal("COS inferred as original MinIO", err)
	}
	if err := routes.BindLegacyStorage(t.Context(), profiles[0], strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := routes.BindLegacyStorage(t.Context(), profiles[0], strings.Repeat("a", 64)); err != nil {
		t.Fatal("proof replay", err)
	}
	changed := profiles[0]
	changed.Endpoint = "https://different.example.test"
	if err := routes.BindLegacyStorage(t.Context(), changed, strings.Repeat("a", 64)); !errors.Is(err, mediaapp.ErrStorageIdentityConflict) {
		t.Fatal("old proof rebound", err)
	}
	ref, err := routes.ResolveLegacyRef(t.Context(), "projects/old/image/file.png")
	if err != nil || ref.ProfileID != "minio-main" || ref.Bucket != "storage-test" {
		t.Fatal("legacy resolution ignored proof", err)
	}
}

func TestStorageLegacyBackfillVerifiesFactsAndFencesConcurrentChanges(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	profiles := storageProfiles(t, routes)
	if err := routes.BindLegacyStorage(t.Context(), profiles[0], strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	_, project := mediaStoreProject(t, db)
	data := []byte("unchanged original")
	a := sourceDocumentAsset(data)
	a.ProjectID = project
	a.ObjectKey = "projects/" + project.String() + "/document/2026/10/" + a.ID.String() + ".txt"
	if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,codec,moderation_status,revision) VALUES(?,?,'document','upload','ready',?,?,?,?,?,'txt','passed',1)`, a.ID, project, a.ObjectKey, a.FileName, a.MimeType, a.ByteSize, *a.SHA256).Error; err != nil {
		t.Fatal(err)
	}
	candidates, err := routes.LegacyCandidates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var candidate mediaapp.LegacyObjectCandidate
	for _, c := range candidates {
		if c.Ref.Key == a.ObjectKey {
			candidate = c
		}
	}
	if candidate.Ref.Key == "" {
		t.Fatal("unresolved original omitted")
	}
	bad := mediaapp.ObjectInfo{Size: a.ByteSize + 1, ContentType: a.MimeType, SHA256: *a.SHA256}
	if _, err := routes.BackfillLegacyObject(t.Context(), candidate, bad); !errors.Is(err, mediaapp.ErrObjectMismatch) {
		t.Fatal("wrong bytes accepted", err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET file_name='changed.txt' WHERE id=?`, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	actual := mediaapp.ObjectInfo{Size: a.ByteSize, ContentType: a.MimeType, SHA256: *a.SHA256}
	if _, err := routes.BackfillLegacyObject(t.Context(), candidate, actual); !errors.Is(err, mediaapp.ErrStorageRouteConflict) {
		t.Fatal("stale migration snapshot accepted", err)
	}
	candidates, err = routes.LegacyCandidates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		if c.Ref.Key == a.ObjectKey {
			candidate = c
		}
	}
	location, err := routes.BackfillLegacyObject(t.Context(), candidate, actual)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct{ ObjectLocationID uuid.UUID }
	if err := db.Raw(`SELECT object_location_id FROM media.media_asset WHERE id=?`, a.ID).Scan(&saved).Error; err != nil || saved.ObjectLocationID != location.ID {
		t.Fatal("original not bound atomically", err)
	}
	if location.Ref != (domain.ObjectRef{ProfileID: "minio-main", Bucket: "storage-test", Key: a.ObjectKey}) || location.State != "accepted" {
		t.Fatal("wrong legacy identity")
	}
	replay, err := routes.BackfillLegacyObject(t.Context(), candidate, actual)
	if err != nil || replay.ID != location.ID {
		t.Fatal("backfill not idempotent", err)
	}
}

func TestStorageBackfilledBindingCannotBeRepointed(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	storageProfiles(t, routes)
	_, project := mediaStoreProject(t, db)
	a := sourceDocumentAsset([]byte("history"))
	a.ProjectID = project
	a.ObjectKey = "projects/" + project.String() + "/document/2026/10/" + a.ID.String() + ".txt"
	original, err := routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: "minio-main", Bucket: "storage-test", Key: a.ObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: "cos-main", Bucket: "storage-test-123", Key: a.ObjectKey})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,codec,moderation_status,revision,object_location_id) VALUES(?,?,'document','upload','ready',?,?,?,?,?,'txt','passed',1,?)`, a.ID, project, a.ObjectKey, a.FileName, a.MimeType, a.ByteSize, *a.SHA256, original.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET object_location_id=? WHERE id=?`, replacement.ID, a.ID).Error; err == nil {
		t.Fatal("a historical file was rebound to another platform")
	}
}

func TestStorageLegacyMissingObjectNeedsRetiredOwnerProof(t *testing.T) {
	db := storageRouteDB(t).Begin()
	t.Cleanup(func() { _ = db.Rollback().Error })
	routes := pgmedia.NewStorageRoutes(db)
	profiles := storageProfiles(t, routes)
	if err := routes.BindLegacyStorage(t.Context(), profiles[0], strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	actor, project := mediaStoreProject(t, db)
	insertObject := func(key string, removed bool) {
		t.Helper()
		request := uuid.New()
		sha := strings.Repeat("a", 64)
		if err := db.Exec(`INSERT INTO script.request(actor_id,request_id,org_id,project_id,action,request_hash,created_at) VALUES(?,?,?,?,'create',?,now())`, actor.ID, request, actor.OrgID, project, sha).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO script.command(actor_id,request_id,org_id,project_id,action,request_hash,plan,created_at) VALUES(?,?,?,?,'create',?,'{}'::jsonb,now())`, actor.ID, request, actor.OrgID, project, sha).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO script.object_intent(actor_id,request_id,org_id,project_id,object_key,sha256,byte_size,mime) VALUES(?,?,?,?,?,?,1,'application/json')`, actor.ID, request, actor.OrgID, project, key, sha).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO script.object_state(object_key,put_started,confirmed,removed,updated_at) VALUES(?,true,true,?,now())`, key, removed).Error; err != nil {
			t.Fatal(err)
		}
	}
	key := "projects/" + uuid.NewString() + "/script/retired.json"
	insertObject(key, true)
	candidates, err := routes.LegacyCandidates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var candidate mediaapp.LegacyObjectCandidate
	for _, c := range candidates {
		if c.Ref.Key == key {
			candidate = c
		}
	}
	if !candidate.Retired {
		t.Fatal("retired owning state not inventoried")
	}
	location, err := routes.BackfillMissingLegacyObject(t.Context(), candidate)
	if err != nil || location.State != "deleted" {
		t.Fatal("proved retired object blocked migration", err)
	}
	if _, err := routes.BackfillMissingLegacyObject(t.Context(), candidate); err != nil {
		t.Fatal("tombstone retry", err)
	}
	liveKey := "projects/" + uuid.NewString() + "/script/live.json"
	insertObject(liveKey, false)
	candidates, err = routes.LegacyCandidates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		if c.Ref.Key == liveKey {
			if _, err := routes.BackfillMissingLegacyObject(t.Context(), c); !errors.Is(err, mediaapp.ErrObjectMismatch) {
				t.Fatal("missing live object treated as intentionally removed", err)
			}
		}
	}
}
