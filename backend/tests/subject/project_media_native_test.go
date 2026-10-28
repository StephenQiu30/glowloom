package subject_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	operationwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/operation"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// SQL rows are fixture-proven frozen inputs; byte access/signing use the actual
// configured SDK. This is not a paid provider or real cloud acceptance claim.
func TestNativeProjectMediaSignerRequiresFrozenProjectLocationAndActualBytes(t *testing.T) {
	f := newActualSubjectGeneration(t)
	if _, err := f.confirm(t); err != nil {
		t.Fatal(err)
	}
	var reads atomic.Int64
	corrupt := atomic.Bool{}
	asset := uuid.New()
	bucket, key := "fixture-bucket", "projects/"+f.project.String()+"/image/2026/10/"+asset.String()+".png"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/"+key) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Length", fmt.Sprint(len(f.original)))
		w.Header().Set("X-Amz-Meta-Sha256", hexHash(f.original))
		if r.Method == http.MethodGet {
			reads.Add(1)
			if corrupt.Load() {
				_, _ = w.Write([]byte(strings.Repeat("x", len(f.original))))
			} else {
				_, _ = w.Write(f.original)
			}
		}
	}))
	defer server.Close()
	profile := "plain-" + uuid.NewString()
	t.Setenv("GLOWLOOM_ENV_FILE", "")
	t.Setenv("GLOWLOOM_OBJECT_STORAGE_CONFIG", "")
	t.Setenv("GLOWLOOM_ENV", "local")
	for k, v := range map[string]string{"PROVIDER": "minio", "ENDPOINT": server.URL, "BUCKET": bucket, "REGION": "us-east-1", "ACCESS_KEY": "fixture-access", "SECRET_KEY": "fixture-secret", "PROFILE_ID": profile, "REVISION": "plain-test-v1"} {
		t.Setenv("GLOWLOOM_OBJECT_STORAGE_"+k, v)
	}
	for _, k := range []string{"PROVIDER", "ENDPOINT", "BUCKET", "REGION", "ACCESS_KEY", "SECRET_KEY", "PROFILE_ID"} {
		t.Setenv("GLOWLOOM_OBJECT_STORAGE_LEGACY_"+k, "")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	registry, err := objectstorage.OpenProfiles(*cfg.ObjectStorage)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if err := pgmedia.NewStorageRoutes(f.owner).RegisterProfiles(t.Context(), []media.StorageProfile{{ID: profile, Provider: "minio", Endpoint: server.URL, SigningEndpoint: server.URL, Region: "us-east-1", Bucket: bucket, AddressingStyle: "path"}}); err != nil {
		t.Fatal(err)
	}
	location, input := uuid.New(), uuid.New()
	hash := hexHash(f.original)
	if err := f.owner.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type) VALUES(?,?,?,?,'accepted',?,?,'image/png')`, location, profile, bucket, key, len(f.original), hash).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.owner.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,'image','upload','ready',?,?,'reference.png','image/png',?,?,'passed')`, asset, f.project, key, location, len(f.original), hash).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.owner.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,ref_id,ref_version,media_asset_id) VALUES(?,?,2,'reference_image','media_asset',?,?,?)`, input, f.quote.OperationID, asset, hash, asset).Error; err != nil {
		t.Fatal(err)
	}
	f.store.WithProjectMedia(func(*gorm.DB) operation.ProjectMediaSigner { return operationwire.NewProjectMediaSigner(registry, nil) })
	ref, handled, err := f.store.ManagedProjectMediaReference(t.Context(), f.quote.OperationID, f.project, input)
	if err != nil || !handled || ref.URL == "" || ref.MediaKind != "image" || ref.Role != "reference_image" || reads.Load() != 1 {
		t.Fatal("formal ordinary input did not use exact source", handled, err)
	}
	corrupt.Store(true)
	if ref, _, err := f.store.ManagedProjectMediaReference(t.Context(), f.quote.OperationID, f.project, input); err == nil || ref.URL != "" {
		t.Fatal("corrupt source was signed", err)
	}
	corrupt.Store(false)

	if err := f.owner.Exec(`UPDATE media.media_asset SET object_location_id=NULL WHERE id=?`, asset).Error; err == nil {
		t.Fatal("immutable physical binding was weakened")
	}
	addInput := func(id, assetID, projectID uuid.UUID, seq int, key string) {
		t.Helper()
		if err := f.owner.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,moderation_status) VALUES(?,?,'image','upload','ready',?,'fixture.png','image/png',?,?,'passed')`, assetID, projectID, key, len(f.original), hash).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.owner.Exec(`INSERT INTO operation.operation_input(id,operation_id,seq_no,role,ref_type,ref_id,ref_version,media_asset_id) VALUES(?, ?,?,'reference_image','media_asset',?,?,?)`, id, f.quote.OperationID, seq, assetID, hash, assetID).Error; err != nil {
			t.Fatal(err)
		}
	}
	legacyInput := uuid.New()
	addInput(legacyInput, uuid.New(), f.project, 3, key+"-legacy")
	before := reads.Load()
	if _, handled, err := f.store.ManagedProjectMediaReference(t.Context(), f.quote.OperationID, f.project, legacyInput); err == nil || !handled || reads.Load() != before {
		t.Fatal("old bare key fell through to current target", err)
	}
	foreignInput := uuid.New()
	addInput(foreignInput, uuid.New(), subjectProject(t, f.owner, f.actor), 4, key+"-foreign")
	before = reads.Load()
	if _, _, err := f.store.ManagedProjectMediaReference(t.Context(), f.quote.OperationID, f.project, foreignInput); err == nil || reads.Load() != before {
		t.Fatal("project ownership widened by physical source", err)
	}
}
