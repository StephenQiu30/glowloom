package media_test

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type videoLinkReceiptFixture struct {
	personalUploadFake
	name   string
	key    uuid.UUID
	result mediaapp.PersonalUploadResult
}

func (r *videoLinkReceiptFixture) LookupPersonalVideoLink(_ context.Context, _ identity.Principal, key uuid.UUID, name string) (mediaapp.PersonalUploadResult, bool, error) {
	if r.result.Asset.ID == uuid.Nil {
		return mediaapp.PersonalUploadResult{}, false, nil
	}
	if r.name != name || r.key != key {
		return mediaapp.PersonalUploadResult{}, false, mediaapp.ErrUploadConflict
	}
	return r.result, true, nil
}

func (r *videoLinkReceiptFixture) CommitPersonalUpload(ctx context.Context, actor identity.Principal, request mediaapp.UploadRequest, asset domain.MediaAsset, renditions []domain.Rendition) (mediaapp.PersonalUploadResult, error) {
	result, err := r.personalUploadFake.CommitPersonalUpload(ctx, actor, request, asset, renditions)
	if err == nil {
		r.name, r.key, r.result = request.FileName, request.Key, result
	}
	return result, err
}

// Only the fixture transport substitutes the real platform origin. Production
// always uses the closed platform allowlist and public, DNS-pinned HTTPS.
type videoLinkTLSFixture struct {
	resolver *mediaflow.PublicVideoLinkResolver
	url      string
}

func (r videoLinkTLSFixture) Resolve(ctx context.Context, _ string, maxBytes int64) (*mediaapp.Downloaded, error) {
	return r.resolver.Resolve(ctx, r.url, maxBytes)
}

func videoLinkActualTLSFixture(t *testing.T) (videoLinkTLSFixture, *atomic.Int32, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.mp4")
	if out, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-t", "1", "-pix_fmt", "yuv420p", "-y", path).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s: %v", out, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var origin string
	var reads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.URL.Path == "/share" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<meta property="og:video" content="` + origin + `/video">`))
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	origin = server.URL
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	resolver, err := mediaflow.NewPublicVideoLinkResolver(mediaflow.VideoLinkResolverOptions{SourceOrigins: []string{origin}, TLSRootCAs: roots, AllowTestLoopbackTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	return videoLinkTLSFixture{resolver: resolver, url: origin + "/share"}, &reads, server.Close
}

func TestVideoLinkFormalUploadPipelineAndOfflineReceiptReplay(t *testing.T) {
	fixture, reads, shutdown := videoLinkActualTLSFixture(t)
	repo, objects := &videoLinkReceiptFixture{}, &uploadObjectsFake{}
	uploads := unitRoutedUploadService(&uploadRepoFake{}, repo, mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects)
	service := mediaapp.NewVideoLinkService(uploads, fixture)
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer}
	key := uuid.New()
	input := mediaapp.VideoLinkRequest{URL: "https://v.douyin.com/fixture/", LocalReviewConfirmed: true}
	result, err := service.Import(t.Context(), actor, key, uuid.New(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Asset.Kind != "video" || result.Asset.ID == uuid.Nil || repo.commits != 1 || repo.asset.Personal == nil || repo.asset.Personal.ActorID != actor.ID || repo.asset.Personal.OrgID != actor.OrgID || len(objects.items) != 3 {
		t.Fatal("formal probe/render/object publication not completed", result, repo.commits, len(objects.items))
	}
	readCount := reads.Load()
	shutdown()
	replayed, err := service.Import(t.Context(), actor, key, uuid.New(), input)
	if err != nil || replayed.Asset.ID != result.Asset.ID || repo.commits != 1 || reads.Load() != readCount {
		t.Fatal("receipt replay downloaded again", replayed, err, reads.Load())
	}
}

func TestVideoLinkActualPGUploadUsesCanonicalProjectionForReplay(t *testing.T) {
	db := mediaStoreDB(t)
	actor, _ := mediaStoreProject(t, db)
	store := mediaTestStore(db)
	routes := pgmedia.NewStorageRoutes(db)
	profile := "link-pipeline-" + uuid.NewString()[:8]
	bucket := "link-pipeline-" + uuid.NewString()[:8] + "-123456789"
	endpoint := "https://" + bucket + ".cos.ap-shanghai.myqcloud.com"
	if err := routes.RegisterProfiles(t.Context(), []mediaapp.StorageProfile{{ID: profile, Provider: "cos", Endpoint: endpoint, SigningEndpoint: endpoint, Region: "ap-shanghai", Bucket: bucket}}); err != nil {
		t.Fatal(err)
	}
	revision := storageEnvironmentRevision(db, "link-pipeline-"+uuid.NewString())
	policy, err := json.Marshal(uploadRoutingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	// Seed non-secret deployment facts only. Admission, source download, ffprobe,
	// rendering, physical locations and CommitPersonalUpload all run normally.
	if err := db.Exec(`INSERT INTO media.storage_config_revision(revision,config_sha256,profile_id,upload_policy,session_ttl_seconds)VALUES(?,?,?,?,3600)`, revision, strings.ReplaceAll(uuid.NewString(), "-", "")+strings.Repeat("d", 32), profile, string(policy)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.storage_route(singleton,revision,profile_id)VALUES(1,?,?) ON CONFLICT(singleton)DO UPDATE SET revision=EXCLUDED.revision,profile_id=EXCLUDED.profile_id`, revision, profile).Error; err != nil {
		t.Fatal(err)
	}
	objects := &uploadObjectsFake{locations: routes}
	uploads := mediaapp.NewRoutedUploadService(store, store, pgmedia.NewUploadAdmission(store), mediaflow.FFUploadProber{}, mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, objects, time.Now)
	fixture, reads, shutdown := videoLinkActualTLSFixture(t)
	service := mediaapp.NewVideoLinkService(uploads, fixture)
	key := uuid.New()
	input := mediaapp.VideoLinkRequest{URL: "https://v.douyin.com/actual-pg/", LocalReviewConfirmed: true}
	result, err := service.Import(t.Context(), actor, key, uuid.New(), input)
	if err != nil {
		t.Fatal("actual PostgreSQL publication", err)
	}
	if result.Asset.ID == uuid.Nil || result.Asset.Kind != "video" || len(objects.items) != 3 {
		t.Fatal("actual verified publication missing", result, len(objects.items))
	}
	var count int64
	if err := db.Table("media.library_item").Where("asset_id=?", result.Asset.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("fixture or upload fabricated catalog row", count, err)
	}
	detail, err := mediaLibraryStore(db, libraryTestAccess, time.Now).LibraryDetail(t.Context(), actor, domain.LibraryScope{Kind: domain.LibraryPersonal}, result.Asset.ID)
	if err != nil || detail.ID != result.Asset.ID || detail.State != domain.LibraryItemStateActive || detail.Revision != domain.LibraryItemInitialRevision {
		t.Fatal("canonical library detail missing", detail, err)
	}
	var source struct {
		SHA256, SourceKind, FileName string
		ByteSize                     int64
	}
	if err := db.Table("media.upload_request").Select("sha256,source_kind,file_name,byte_size").Where("personal_org_id=? AND principal_id=? AND request_key=?", actor.OrgID, actor.ID, key).Take(&source).Error; err != nil || source.SourceKind != "video_link" {
		t.Fatal("trusted source provenance missing", source, err)
	}
	localRequest := mediaapp.UploadRequest{Key: key, RequestID: uuid.New(), Personal: &domain.PersonalOwnership{OrgID: actor.OrgID, ActorID: actor.ID}, SHA256: source.SHA256, FileName: source.FileName, ByteSize: source.ByteSize}
	if _, _, err := store.FindPersonalUpload(t.Context(), actor, localRequest); !errors.Is(err, mediaapp.ErrUploadConflict) {
		t.Fatal("local replay impersonated link request", err)
	}
	readCount := reads.Load()
	shutdown()
	replay, err := service.Import(t.Context(), actor, key, uuid.New(), input)
	if err != nil || replay.Asset.ID != result.Asset.ID || reads.Load() != readCount {
		t.Fatal("canonical upload receipt failed replay", replay, err, reads.Load())
	}
	libraryID, err := (domain.LibraryScope{Kind: domain.LibraryPersonal}).Identity(actor.OrgID, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.library_item(id,library_id,asset_id,title,category,position,catalog_state)VALUES(?,?,?,'removed reference','material',0,'removed')`, result.Asset.ID, libraryID, result.Asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Import(t.Context(), actor, key, uuid.New(), input); !errors.Is(err, mediaapp.ErrUploadConflict) {
		t.Fatal("removed canonical entry replayed", err)
	}
}

func TestVideoLinkProvenanceIsServerOnly(t *testing.T) {
	var request mediaapp.UploadRequest
	if err := json.Unmarshal([]byte(`{"VideoLink":true,"video_link":true,"source_kind":"video_link"}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.VideoLink || request.SourceKind() != "local" {
		t.Fatal("HTTP JSON assigned trusted provenance")
	}
	request.VideoLink = true
	raw, err := json.Marshal(request)
	if err != nil || strings.Contains(string(raw), "VideoLink") || strings.Contains(string(raw), "source_kind") {
		t.Fatal("trusted field entered serialized HTTP input", string(raw), err)
	}
}
