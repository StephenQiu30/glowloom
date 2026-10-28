package subject_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

// This is the old single-bucket object port fixture. The native SDK path has
// independent coverage; this fixture proves immutable legacy SQL receipts survive
// addition of physical ownership facts, without rewriting their semantic hashes.
type legacyCopyBytes struct{ objects map[string][]byte }

func (o legacyCopyBytes) Get(_ context.Context, k string) (io.ReadCloser, error) {
	b, ok := o.objects[k]
	if !ok {
		return nil, media.ErrObjectMissing
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (o legacyCopyBytes) Exists(_ context.Context, k string) (bool, error) {
	_, ok := o.objects[k]
	return ok, nil
}
func (o legacyCopyBytes) PutIfAbsent(_ context.Context, k string, r io.Reader, _ int64, _, _ string) error {
	if _, ok := o.objects[k]; ok {
		return media.ErrObjectAlreadyExists
	}
	b, e := io.ReadAll(r)
	if e != nil {
		return e
	}
	o.objects[k] = b
	return nil
}
func (o legacyCopyBytes) Remove(_ context.Context, k string) error { delete(o.objects, k); return nil }

func TestCompletedLegacyCopyKeepsImmutableReceiptAfterLocationBackfill(t *testing.T) {
	f := newLocatedCopy(t)
	source := subjectProject(t, f.owner, f.actor)
	asset := uuid.New()
	key := "projects/" + source.String() + "/image/2026/10/" + asset.String() + ".png"
	if err := f.owner.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,'image','upload','ready',?,'legacy.png','image/png',?,?,'passed')`, asset, source, key, len(f.data), hexHash(f.data)).Error; err != nil {
		t.Fatal(err)
	}
	binding := media.ProjectCopyBinding{JobID: uuid.New(), OrgID: f.actor.OrgID, SourceProjectID: source, TargetProjectID: f.target}
	var snapshot media.ProjectCopySnapshot
	if err := f.runtime.Transaction(func(tx *gorm.DB) error {
		var e error
		snapshot, e = pgmedia.NewScopedProjectCopyStore(tx, app.NewMediaWorkerScopeAccess).Freeze(t.Context(), f.actor, binding, time.Now())
		if e != nil {
			return e
		}
		return tx.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,owner_scope_id,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot,idem_key,request_sha256,admission_response) VALUES(?,?,?,?,?,1,?,'legacy fixture','queued','media','{}','{}',gen_random_uuid(),repeat('a',64),'{}')`, binding.JobID, f.actor.OrgID, f.actor.ID, f.actor.ScopeID, source, f.target).Error
	}); err != nil {
		t.Fatal(err)
	}
	repo := &locatedCopyTransactions{ProjectCopyStore: pgmedia.NewScopedProjectCopyStore(f.runtime, app.NewMediaWorkerScopeAccess), db: f.runtime}
	objects := legacyCopyBytes{map[string][]byte{key: bytes.Clone(f.data)}}
	if err := media.NewProjectCopyTransfer(repo, objects, t.TempDir()).Transfer(t.Context(), f.actor, binding, snapshot); err != nil {
		t.Fatal(err)
	}
	before, err := repo.Register(t.Context(), f.actor, binding, snapshot)
	if err != nil {
		t.Fatal("legacy registration", err)
	}
	var immutableBefore string
	if err = f.owner.Raw(`SELECT content::text FROM media.project_copy_snapshot WHERE id=?`, snapshot.ID).Scan(&immutableBefore).Error; err != nil {
		t.Fatal(err)
	}
	target := snapshot.AssetMapping[asset]
	for _, id := range []uuid.UUID{asset, target} {
		var k string
		if err = f.owner.Raw(`SELECT object_key FROM media.media_asset WHERE id=?`, id).Scan(&k).Error; err != nil {
			t.Fatal(err)
		}
		location := uuid.New()
		if err = f.owner.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) VALUES(?,?,?,?,'accepted',?,?,'image/png')`, location, f.sourceProfile, "source-bucket", k, len(objects.objects[k]), hexHash(objects.objects[k])).Error; err != nil {
			t.Fatal(err)
		}
		if err = f.owner.Exec(`UPDATE media.media_asset SET object_location_id=? WHERE id=?`, location, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	after, err := repo.Register(t.Context(), f.actor, binding, snapshot)
	if err != nil || after != before {
		t.Fatal("physical backfill changed accepted legacy receipt", err, before, after)
	}
	var immutableAfter string
	if err = f.owner.Raw(`SELECT content::text FROM media.project_copy_snapshot WHERE id=?`, snapshot.ID).Scan(&immutableAfter).Error; err != nil || immutableAfter != immutableBefore {
		t.Fatal("legacy immutable content rewritten", err)
	}
}
