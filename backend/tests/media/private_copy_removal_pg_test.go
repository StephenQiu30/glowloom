package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

func TestStorageAcceptedPrivateCopyRequiresCleanupAndPrivateTargetBeforeRealRemoval(t *testing.T) {
	owner, runtime := packageLocationDB(t)
	old, server := newCOSStageFixture(t)
	_, other := newCOSStageFixture(t)
	cfg := stageConfiguration(t, server.URL, other.URL)
	suffix := uuid.NewString()[:8]
	profileID := "private-copy-" + suffix
	cfg.Profiles[profileID] = cfg.Profiles["stage-old"]
	delete(cfg.Profiles, "stage-old")
	delete(cfg.Profiles, "stage-new")
	cfg.DefaultWriteProfile = profileID
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	p := cfg.Profiles[profileID]
	routes := pgmedia.NewStorageRoutes(owner)
	if err := routes.RegisterProfiles(t.Context(), []media.StorageProfile{{ID: profileID, Provider: p.Provider, Endpoint: p.Endpoint, SigningEndpoint: p.SigningEndpoint, Bucket: p.Bucket, Region: p.Region, AddressingStyle: p.AddressingStyle}}); err != nil {
		t.Fatal(err)
	}
	prior, _ := routes.ReadActive(t.Context())
	if _, err := routes.Activate(t.Context(), prior.Revision, "private-copy-"+uuid.NewString(), profileID); err != nil {
		t.Fatal(err)
	}
	actor, source := mediaStoreProject(t, owner)
	target, job, snapshot, asset := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := owner.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type,status) VALUES(?,?,?,?,?,'16:9','realistic','copying')`, target, actor.OrgID, actor.ID, actor.ScopeID, "private-target").Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO workspace.project_copy_job(id,org_id,actor_id,owner_scope_id,idem_key,request_sha256,admission_response,source_project_id,source_revision,target_project_id,target_name,status,stage,manifest,workspace_snapshot) VALUES(?,?,?,?,?,repeat('a',64),'{}'::jsonb,?,1,?,'private-target','running','media','{}'::jsonb,'{}'::jsonb)`, job, actor.OrgID, actor.ID, actor.ScopeID, uuid.New(), source, target).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.project_copy_snapshot(id,job_id,org_id,source_project_id,target_project_id,manifest_sha256,asset_count,rendition_count,content,asset_mapping) VALUES(?,?,?,?,?,repeat('b',64),1,0,'{}'::jsonb,'{}'::jsonb)`, snapshot, job, actor.OrgID, source, target).Error; err != nil {
		t.Fatal(err)
	}
	workerRoutes := pgmedia.NewStorageRoutes(runtime)
	work, err := workerRoutes.PinStorageWork(t.Context(), media.StorageWorkOwner{Kind: "project-copy", OwnerID: target, WorkID: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("projects/%s/document/2026/10/%s.txt", target, asset)
	l, err := workerRoutes.ReserveWorkLocation(t.Context(), work.ID, key)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("private copied bytes")
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	write, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := workerRoutes.BeginWorkWrite(write, work.ID, l); err != nil {
		t.Fatal(err)
	}
	ref := objectstorage.Ref{ProfileID: l.Ref.ProfileID, Bucket: l.Ref.Bucket, Key: l.Ref.Key}
	if err := registry.PutIfAbsent(write, ref, bytes.NewReader(body), int64(len(body)), "text/plain", digest); err != nil {
		t.Fatal(err)
	}
	if err := workerRoutes.ConfirmWorkLocation(t.Context(), work.ID, l, media.ObjectInfo{Size: int64(len(body)), SHA256: digest, ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,codec,moderation_status) VALUES(?,?,'document','upload','ready',?,?,'copy.txt','text/plain',?,?,'txt','passed')`, asset, target, key, l.ID, len(body), digest).Error; err != nil {
		t.Fatal(err)
	}
	if err := workerRoutes.AcceptStorageWork(t.Context(), work.ID, []uuid.UUID{l.ID}); err != nil {
		t.Fatal(err)
	}
	if ready, err := workerRoutes.BeginPrivateCopyRemoval(t.Context(), work.ID, l); err != nil || ready {
		t.Fatal("non-cleanup copy could remove accepted bytes", ready, err)
	}
	if err := owner.Exec(`UPDATE workspace.project_copy_job SET stage='cleanup' WHERE id=?`, job).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`UPDATE workspace.project SET status='active' WHERE id=?`, target).Error; err != nil {
		t.Fatal(err)
	}
	if ready, err := workerRoutes.BeginPrivateCopyRemoval(t.Context(), work.ID, l); err != nil || ready {
		t.Fatal("active target could be cleaned by copy worker", ready, err)
	}
	if err := owner.Exec(`UPDATE workspace.project SET status='copying' WHERE id=?`, target).Error; err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(1100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	foreign, err := routes.ReserveLocation(t.Context(), domain.ObjectRef{ProfileID: profileID, Bucket: p.Bucket, Key: "not-this-work"})
	if err != nil {
		t.Fatal(err)
	}
	if ready, err := workerRoutes.BeginPrivateCopyRemoval(t.Context(), work.ID, foreign); err != nil || ready {
		t.Fatal("foreign owner was removed", ready, err)
	}
	if ready, err := workerRoutes.BeginPrivateCopyRemoval(t.Context(), work.ID, l); err != nil || !ready {
		t.Fatal("proved unpublished copy could not retire", ready, err)
	}
	if _, err := workerRoutes.ReadRemovableWorkLocation(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	if err := registry.Remove(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Stat(t.Context(), ref); !errors.Is(err, objectstorage.ErrObjectMissing) {
		t.Fatal("physical absence not proved", err)
	}
	if err := workerRoutes.ConfirmWorkRemoval(t.Context(), work.ID, l); err != nil {
		t.Fatal(err)
	}
	state, err := workerRoutes.ReadLocation(t.Context(), l.ID)
	if err != nil || state.State != "deleted" || len(old.objects) != 0 {
		t.Fatal("absence receipt not durable", state.State, err)
	}
	var accepted bool
	if err := owner.Raw(`SELECT state='accepted' FROM media.storage_work_intent WHERE id=?`, work.ID).Scan(&accepted).Error; err != nil || !accepted {
		t.Fatal("immutable accepted work was rewritten", err)
	}
}
