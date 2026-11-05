package agentbusiness_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type imageObjects func(context.Context, mediadomain.ObjectRef) (io.ReadCloser, error)

func (f imageObjects) Get(ctx context.Context, r mediadomain.ObjectRef) (io.ReadCloser, error) {
	return f(ctx, r)
}
func imageFixture(t *testing.T, db *gorm.DB, g domain.Grant) (uuid.UUID, mediadomain.ObjectRef, []byte) {
	t.Helper()
	var data bytes.Buffer
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&data, im); err != nil {
		t.Fatal(err)
	}
	body := data.Bytes()
	digest := sha256.Sum256(body)
	sha := hex.EncodeToString(digest[:])
	id, location := uuid.New(), uuid.New()
	profile := "agent-image-" + uuid.NewString()[:8]
	ref := mediadomain.ObjectRef{ProfileID: profile, Bucket: "agent-images", Key: "projects/" + g.Scope.ProjectID.String() + "/image/2026/11/" + id.String() + ".png"}
	if err := db.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity)VALUES(?,'minio','http://127.0.0.1','http://127.0.0.1','us-east-1',?,'path',?)`, profile, ref.Bucket, domain.ContentHash(profile)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type)VALUES(?,?,?,?,'accepted',?,?,'image/png')`, location, profile, ref.Bucket, ref.Key, len(body), sha).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,byte_size,sha256,moderation_status)VALUES(?,?,'image','upload','ready',?,?,'fixture.png','image/png',?,?,'passed')`, id, g.Scope.ProjectID, ref.Key, location, len(body), sha).Error; err != nil {
		t.Fatal(err)
	}
	return id, ref, body
}
func TestMediaImageInspectRealPGOriginalBytesReplayAndRevoke(t *testing.T) {
	db, b, _, _, g := realCanvasBoundary(t)
	id, ref, body := imageFixture(t, db, g)
	reads := 0
	b.WithMediaObjects(imageObjects(func(ctx context.Context, got mediadomain.ObjectRef) (io.ReadCloser, error) {
		reads++
		if got != ref {
			t.Fatal("saved physical identity changed", got)
		}
		// Opening an object must never hold Agent authorization locks during I/O.
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Exec(`SELECT root_run_id FROM agent_business.run_authorization WHERE root_run_id=? FOR UPDATE NOWAIT`, g.RootRunID).Error
		}); err != nil {
			t.Fatal("network inside transaction", err)
		}
		return io.NopCloser(bytes.NewReader(body)), nil
	}))
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	status, receipt, message := businessToolRequest(t, router, g, "media_image_inspect", "image", map[string]any{"asset_id": id})
	if status != 200 {
		t.Fatal(status, message)
	}
	var result struct {
		Content  []struct{ Type, Data, MimeType string }
		Revision int64
	}
	if err := json.Unmarshal(receipt.Result, &result); err != nil || len(result.Content) != 1 || result.Content[0].Type != "image" || result.Content[0].MimeType != "image/png" {
		t.Fatal(string(receipt.Result), err)
	}
	got, err := base64.StdEncoding.DecodeString(result.Content[0].Data)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatal("original bytes changed", err)
	}
	status, replay, message := businessToolRequest(t, router, g, "media_image_inspect", "image", map[string]any{"asset_id": id})
	if status != 200 || replay.ReceiptID != receipt.ReceiptID || reads != 1 {
		t.Fatal("replay", reads, message)
	}
	if err := db.Exec(`UPDATE media.media_asset SET is_delete=true WHERE id=?`, id).Error; err != nil {
		t.Fatal(err)
	}
	status, _, _ = businessToolRequest(t, router, g, "media_image_inspect", "image", map[string]any{"asset_id": id})
	if status == 200 || reads != 1 {
		t.Fatal("revoked bytes replayed", status, reads)
	}
}
func TestMediaImageInspectStopDuringReadCreatesNoReceipt(t *testing.T) {
	db, b, _, actor, g := realCanvasBoundary(t)
	id, _, body := imageFixture(t, db, g)
	b.WithMediaObjects(imageObjects(func(ctx context.Context, _ mediadomain.ObjectRef) (io.ReadCloser, error) {
		if _, err := b.Store.Decide(ctx, actor, g.Scope.ProjectID, g.RootRunID, domain.DecisionInput{RequestID: uuid.New(), Action: "stop", ExpectedAuthorizationRevision: 1}); err != nil {
			t.Fatal(err)
		}
		return io.NopCloser(bytes.NewReader(body)), nil
	}))
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	status, _, message := businessToolRequest(t, router, g, "media_image_inspect", "stopped-image", map[string]any{"asset_id": id})
	if status != 409 {
		t.Fatal(status, message)
	}
	var n int64
	if err := db.Raw(`SELECT count(*) FROM agent_business.call_receipt WHERE root_run_id=?`, g.RootRunID).Scan(&n).Error; err != nil || n != 0 {
		t.Fatal("late image receipt", n, err)
	}
}

func TestMediaImageInspectOfficialSDKReadsSavedLocation(t *testing.T) {
	db, b, _, _, g := realCanvasBoundary(t)
	id, ref, body := imageFixture(t, db, g)
	requests := 0
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/"+ref.Bucket+"/"+ref.Key || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("unexpected unsigned physical request", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"fixture"`)
		_, _ = w.Write(body)
	}))
	t.Cleanup(endpoint.Close)
	t.Setenv("AGENT_IMAGE_FIXTURE_AK", "fixture-access")
	t.Setenv("AGENT_IMAGE_FIXTURE_SK", "fixture-secret")
	raw, _ := json.Marshal(map[string]any{"schema_version": 1, "revision": "1", "default_write_profile": ref.ProfileID, "upload": map[string]any{"mode": "relay", "signed_url_ttl": time.Minute, "session_ttl": time.Hour, "staging_retention": 24 * time.Hour, "multipart_threshold_bytes": 8 << 20, "part_size_bytes": 5 << 20, "part_concurrency": 1}, "access": map[string]any{"preview_ttl": time.Minute, "max_signed_url_ttl": 15 * time.Minute}, "limits": map[string]any{"max_image_bytes": 20 << 20, "max_video_bytes": 500 << 20, "max_audio_bytes": 100 << 20, "max_model_bytes": 64 << 20, "max_document_bytes": 20 << 20}, "profiles": map[string]any{ref.ProfileID: map[string]any{"provider": "minio", "mode": config.StorageProfileReadWrite, "endpoint": endpoint.URL, "signing_endpoint": endpoint.URL, "addressing_style": "path", "bucket": ref.Bucket, "region": "us-east-1", "credentials": map[string]any{"source": "env", "access_key_env": "AGENT_IMAGE_FIXTURE_AK", "secret_key_env": "AGENT_IMAGE_FIXTURE_SK"}}}})
	cfg, err := config.LoadObjectStorageEnvironment(string(raw), "local", os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	b.WithMediaObjects(mediaobjects.NewRoutedReadObjects(registry))
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	status, receipt, message := businessToolRequest(t, router, g, "media_image_inspect", "sdk-image", map[string]any{"asset_id": id})
	if status != 200 || requests != 1 {
		t.Fatal(status, requests, message)
	}
	var result struct{ Content []struct{ Data string } }
	if json.Unmarshal(receipt.Result, &result) != nil || len(result.Content) != 1 {
		t.Fatal(string(receipt.Result))
	}
	got, err := base64.StdEncoding.DecodeString(result.Content[0].Data)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatal("official saved object bytes differ", err)
	}
}

func TestMediaImageInspectRejectsCorruptOrChangedBytesAndOtherProject(t *testing.T) {
	db, b, _, actor, g := realCanvasBoundary(t)
	id, _, body := imageFixture(t, db, g)
	reads := 0
	b.WithMediaObjects(imageObjects(func(_ context.Context, _ mediadomain.ObjectRef) (io.ReadCloser, error) {
		reads++
		bad := bytes.Clone(body)
		bad[len(bad)-1] ^= 1
		return io.NopCloser(bytes.NewReader(bad)), nil
	}))
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	status, _, message := businessToolRequest(t, router, g, "media_image_inspect", "corrupt", map[string]any{"asset_id": id})
	if status != 409 {
		t.Fatal("corrupt digest admitted", status, message)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM agent_business.call_receipt WHERE root_run_id=?`, g.RootRunID).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("corrupt image receipt", count, err)
	}
	other := g
	other.Scope.ProjectID = uuid.New()
	if err := db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type)VALUES(?,?,?,?,'Other image project','16:9','realistic')`, other.Scope.ProjectID, actor.OrgID, actor.ID, actor.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	foreign, _, _ := imageFixture(t, db, other)
	status, _, message = businessToolRequest(t, router, g, "media_image_inspect", "foreign", map[string]any{"asset_id": foreign})
	if status != 404 || reads != 1 {
		t.Fatal("image crossed admitted project", status, reads, message)
	}
	for _, tool := range b.Capabilities.Tools {
		if tool.Descriptor.Name == "media_image_inspect" {
			for _, role := range []string{"art", "qa"} {
				found := false
				for _, got := range tool.Descriptor.Roles {
					found = found || got == role
				}
				if !found {
					t.Fatal("image tool absent specialist role", role)
				}
			}
		}
	}
}
