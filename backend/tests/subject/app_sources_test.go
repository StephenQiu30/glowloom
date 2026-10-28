package subject_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"

	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	operationdomain "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	subjecthttp "github.com/StephenQiu30/glowloom-server/backend/internal/subject/adapter/http"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// The local HTTP server exercises the actual object SDK and integrity checks.
// Its synthetic media are not evidence of real storage/provider acceptance.
type subjectObjectServer struct {
	mu      sync.Mutex
	data    map[string][]byte
	reads   int
	objects subjectwire.Objects
}

func subjectStorage(t *testing.T) *subjectObjectServer {
	t.Helper()
	f := &subjectObjectServer{data: make(map[string][]byte)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data, ok := f.data[strings.TrimPrefix(r.URL.Path, "/subject-fixture/")]
		if !ok || r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"synthetic-object"`)
		if r.Method == http.MethodGet {
			f.reads++
			_, _ = w.Write(data)
		}
	}))
	t.Cleanup(server.Close)
	client, err := objectstorage.Open(server.URL, "subject-fixture", "synthetic-fixture-access", "synthetic-fixture-secret", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	f.objects = subjectwire.Objects{Legacy: client}
	return f
}

func (f *subjectObjectServer) put(key string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = bytes.Clone(data)
}

func (f *subjectObjectServer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

func subjectStoredPNG(t *testing.T, owner *gorm.DB, actor identity.Principal, storage *subjectObjectServer) (uuid.UUID, string, []byte) {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	digest := sha256.Sum256(data)
	id := uuid.New()
	key := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/2026/10/" + id.String() + ".png"
	if err := owner.Exec(`INSERT INTO media.media_asset(id,personal_org_id,personal_actor_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,moderation_status,width,height) VALUES(?,?,?,'image','upload','ready',?,'原图.png','image/png',?,?,'passed',2,2)`, id, actor.OrgID, actor.ID, key, len(data), hex.EncodeToString(digest[:])).Error; err != nil {
		t.Fatal(err)
	}
	storage.put(key, data)
	return id, key, bytes.Clone(data)
}

func subjectTeam(t *testing.T, owner *gorm.DB, a, b identity.Principal) (identity.Principal, identity.Principal) {
	t.Helper()
	teamID, scopeID := uuid.New(), uuid.New()
	if err := owner.Exec(`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'主体媒体集成团队',?)`, teamID, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{a.ID, b.ID} {
		if err := owner.Exec(`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'member','active',clock_timestamp(),'invited')`, teamID, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := owner.Exec(`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, scopeID, teamID, a.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	a.ScopeID, b.ScopeID, b.OrgID = scopeID, scopeID, a.OrgID
	return a, b
}

func subjectProject(t *testing.T, owner *gorm.DB, actor identity.Principal) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := owner.Exec(`INSERT INTO workspace.project(id,org_id,owner_scope_id,owner_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'主体引用测试','16:9','realistic')`, id, actor.OrgID, actor.ScopeID, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSubjectAppPersonalAdoptionTeamReuseAndExport(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	a, b := subjectSession(t, owner), subjectSession(t, owner)
	storage := subjectStorage(t)
	asset, key, original := subjectStoredPNG(t, owner, a, storage)
	store := subjectwire.NewStore(runtime, storage.objects)
	service := subject.NewService(store, time.Now)
	input := subject.SaveInput{Name: "个人原图主体", Description: "原图字节应完整保留", Primary: subject.MediaSelection{AssetID: asset}}
	storage.put(key, bytes.Repeat([]byte{'x'}, len(original)))
	if _, err := service.Save(t.Context(), a, uuid.Nil, uuid.New(), input); !errors.Is(err, media.ErrObjectMismatch) {
		t.Fatal("new adoption accepted corrupt original", err)
	}
	storage.put(key, original)
	personal, err := service.Save(t.Context(), a, uuid.Nil, uuid.New(), input)
	if err != nil {
		t.Fatal("real personal adoption used project-only CanReference gate", err)
	}
	if storage.count() < 2 {
		t.Fatal("adoption skipped real original SDK reads")
	}
	teamA, teamB := subjectTeam(t, owner, a, b)
	input.Name = "团队共享主体"
	shared, err := service.Save(t.Context(), teamA, uuid.Nil, uuid.New(), input)
	if err != nil {
		t.Fatal("personal source adoption into team", err)
	}
	before := storage.count()
	if _, err := service.Save(t.Context(), teamB, uuid.Nil, uuid.New(), input); !errors.Is(err, subject.ErrNotFound) || storage.count() != before {
		t.Fatal("teammate used bare asset ID to read another member's personal library", err)
	}
	if _, err := store.BoundMedia(t.Context(), a, shared.Primary.BindingID); !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("same physical org crossed logical space", err)
	}
	input.ExpectedRevision = shared.Revision
	input.Primary.BindingID = &shared.Primary.BindingID
	input.Name = "团队绑定复用"
	storage.put(key, bytes.Repeat([]byte{'x'}, len(original)))
	if _, err := service.Save(t.Context(), teamB, shared.ID, uuid.New(), input); !errors.Is(err, media.ErrObjectMismatch) {
		t.Fatal("binding reuse published a version from a corrupt original", err)
	}
	storage.put(key, original)
	updated, err := service.Save(t.Context(), teamB, shared.ID, uuid.New(), input)
	if err != nil || updated.Revision != 2 {
		t.Fatal("authorized teammate could not reuse exact adopted binding", err)
	}
	libraryID, err := (mediadomain.LibraryScope{Kind: mediadomain.LibraryPersonal}).Identity(a.OrgID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.library(id,kind,org_id,personal_actor_id) VALUES(?,'personal',?,?)`, libraryID, a.OrgID, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := owner.Exec(`INSERT INTO media.library_item(id,library_id,asset_id,title,category,catalog_state,trashed_at,position) VALUES(?,?,?,'原图','material','trashed',clock_timestamp(),0)`, asset, libraryID, asset).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(t.Context(), a, uuid.Nil, uuid.New(), subject.SaveInput{Name: "回收站不可新采用", Primary: subject.MediaSelection{AssetID: asset}}); !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("new adoption ignored catalog trash", err)
	}
	input.ExpectedRevision = updated.Revision
	updated, err = service.Save(t.Context(), teamB, shared.ID, uuid.New(), input)
	if err != nil || updated.Revision != 3 {
		t.Fatal("existing immutable binding lost retained catalog source", err)
	}
	invalid := input
	invalid.ExpectedRevision = updated.Revision
	invalid.Primary.AssetID = uuid.New()
	if _, err := service.Save(t.Context(), teamB, shared.ID, uuid.New(), invalid); err == nil {
		t.Fatal("binding ID authorized a different asset")
	}
	files := subjectwire.NewFiles(runtime, storage.objects)
	file, err := files.Media(t.Context(), teamB, shared.ID, updated.VersionID, updated.Primary.BindingID)
	if err != nil {
		t.Fatal("team bound preview", err)
	}
	data, readErr := io.ReadAll(file.Reader)
	closeErr := file.Reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(data, original) || file.MIMEType != "image/png" || file.Size != int64(len(original)) {
		t.Fatal("preview changed original bytes", readErr, closeErr)
	}
	if _, err := files.Media(t.Context(), teamB, personal.ID, updated.VersionID, updated.Primary.BindingID); !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("binding replayed under wrong subject tuple", err)
	}
	if _, err := files.Media(t.Context(), teamB, shared.ID, shared.VersionID, updated.Primary.BindingID); !errors.Is(err, subject.ErrNotFound) {
		t.Fatal("binding replayed under wrong immutable version", err)
	}
	selection := subject.ExportInput{Items: []subject.ExportItem{{ID: shared.ID, VersionID: shared.VersionID}}}
	assertSubjectExport(t, files, teamB, selection, original, shared.Primary.BindingID)
	before = storage.count()
	if _, err := files.Export(t.Context(), a, selection); !errors.Is(err, subject.ErrNotFound) || storage.count() != before {
		t.Fatal("export bypassed logical scope before object read", err)
	}
	project := subjectProject(t, owner, teamA)
	if err := runtime.Transaction(func(tx *gorm.DB) error {
		proof, err := subjectwire.NewGenerationAccess(tx, storage.objects).ResolveQuote(t.Context(), teamB, project, updated.Primary.BindingID)
		if err != nil {
			return err
		}
		in := operationdomain.OperationInput{RefType: "subject_media", RefID: &proof.BindingID, RefVersion: &proof.BindingSHA256, MediaAssetID: &proof.AssetID}
		if err := subjectwire.NewGenerationAccess(tx, storage.objects).VerifyInput(t.Context(), teamB, uuid.New(), project, in); err != nil {
			return err
		}
		wrongHash := strings.Repeat("a", 64)
		in.RefVersion = &wrongHash
		if err := subjectwire.NewGenerationAccess(tx, storage.objects).VerifyInput(t.Context(), teamB, uuid.New(), project, in); err == nil {
			t.Fatal("generation accepted another immutable binding digest")
		}
		return nil
	}); err != nil {
		t.Fatal("request generation authority", err)
	}
	assertSubjectPromptSource(t, runtime, storage.objects, teamB, project, updated)
	storage.put(key, bytes.Repeat([]byte{'x'}, len(original)))
	if _, err := files.Export(t.Context(), teamB, selection); !errors.Is(err, media.ErrObjectMismatch) {
		t.Fatal("export accepted changed original bytes", err)
	}
	storage.put(key, original)
	// Simulate an administrative owner reassignment with valid new owner path,
	// preserving asset/revision/hash. The original immutable grant cannot follow it.
	newKey := "personal/" + b.OrgID.String() + "/" + b.ID.String() + "/image/2026/10/" + asset.String() + ".png"
	if err := owner.Exec(`UPDATE media.media_asset SET personal_org_id=?,personal_actor_id=?,object_key=? WHERE id=?`, b.OrgID, b.ID, newKey, asset).Error; err != nil {
		t.Fatal(err)
	}
	storage.put(newKey, original)
	before = storage.count()
	if _, err := files.Export(t.Context(), teamB, selection); !errors.Is(err, media.ErrNotFound) || storage.count() != before {
		t.Fatal("frozen binding followed reassigned original ownership", err)
	}
	input.ExpectedRevision = updated.Revision
	input.Primary.BindingID = &updated.Primary.BindingID
	if _, err := service.Save(t.Context(), teamB, shared.ID, uuid.New(), input); !errors.Is(err, media.ErrNotFound) {
		t.Fatal("save reused reassigned source ownership", err)
	}
}

func assertSubjectExport(t *testing.T, files subject.Files, actor identity.Principal, in subject.ExportInput, original []byte, binding uuid.UUID) {
	t.Helper()
	file, err := files.Export(t.Context(), actor, in)
	if err != nil {
		t.Fatal("export", err)
	}
	data, readErr := io.ReadAll(file.Reader)
	name := file.Reader.(interface{ Name() string }).Name()
	closeErr := file.Reader.Close()
	if readErr != nil || closeErr != nil || file.Size != int64(len(data)) {
		t.Fatal("export stream", readErr, closeErr)
	}
	if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("export temporary file leaked", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.File) != 2 {
		t.Fatal("portable export archive", err)
	}
	var manifest struct {
		Subjects []subject.Detail `json:"subjects"`
		Media    []struct {
			BindingID uuid.UUID `json:"binding_id"`
			SHA256    string    `json:"sha256"`
		} `json:"media"`
	}
	for _, entry := range archive.File {
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			t.Fatal("archive entry", err, closeErr)
		}
		if entry.Name == "manifest.json" {
			if json.Unmarshal(content, &manifest) != nil || bytes.Contains(content, []byte("object_key")) || bytes.Contains(content, []byte("personal_actor_id")) {
				t.Fatal("manifest exposed private storage ownership")
			}
		} else if path.Base(entry.Name) != binding.String()+".png" || !bytes.Equal(content, original) {
			t.Fatal("export media path or original bytes changed")
		}
	}
	digest := sha256.Sum256(original)
	if len(manifest.Subjects) != 1 || len(manifest.Media) != 1 || manifest.Subjects[0].VersionID != in.Items[0].VersionID || manifest.Media[0].BindingID != binding || manifest.Media[0].SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("export did not pin selected historical version")
	}
}

func assertSubjectPromptSource(t *testing.T, runtime *gorm.DB, objects subjectwire.Objects, actor identity.Principal, project uuid.UUID, detail subject.Detail) {
	t.Helper()
	if err := runtime.Transaction(func(tx *gorm.DB) error {
		sources := subjectwire.NewPromptSources(tx)
		source, known, err := sources.ResolveSubject(t.Context(), actor, project, detail.ID.String(), detail.VersionID.String())
		if err != nil || !known || len(source.Media) != 1 || source.Media[0].BindingID != detail.Primary.BindingID.String() {
			t.Fatal("prompt did not preserve exact binding", known, err)
		}
		validator := subjectwire.NewGenerationAccess(tx, objects).(interface {
			ValidateBinding(context.Context, identity.Principal, uuid.UUID, promptdomain.SourceMedia, bool) error
		})
		if err := validator.ValidateBinding(t.Context(), actor, project, source.Media[0], false); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal("prompt source validation", err)
	}
}

func TestSubjectAppSourceLibraryKeepsTeamBrowsingPersonal(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	a, b := subjectSession(t, owner), subjectSession(t, owner)
	storage := subjectStorage(t)
	assetA, _, _ := subjectStoredPNG(t, owner, a, storage)
	assetB, _, originalB := subjectStoredPNG(t, owner, b, storage)
	_, teamB := subjectTeam(t, owner, a, b)
	library := subjectwire.NewSourceLibrary(runtime, storage.objects)
	query := media.SubjectCatalogueQuery{Kind: "image", Page: 1, PageSize: 12}
	page, err := library.List(t.Context(), teamB, query)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != assetB || page.CurrentActorID != b.ID || page.CurrentScopeID != teamB.ScopeID || page.Items[0].Source != "personal_asset" || page.Items[0].ProjectID != nil {
		t.Fatal("team source page did not retain caller personal ownership", page.Total, err)
	}
	if !page.Capabilities.UpscaledFilter {
		t.Fatal("real source upscale filter missing")
	}
	detail, err := library.Detail(t.Context(), teamB, assetB)
	if err != nil || detail.ID != assetB || detail.Media.MIMEType != "image/png" || detail.Media.Origin != "upload" {
		t.Fatal("own personal source detail", err)
	}
	if _, err := library.Detail(t.Context(), teamB, assetA); !errors.Is(err, media.ErrNotFound) {
		t.Fatal("team source picker exposed another member's source detail", err)
	}
	preview, err := library.Preview(t.Context(), teamB, assetB)
	if err != nil || preview.Asset.ID != assetB || preview.URL == "" {
		t.Fatal("own personal source preview", err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, preview.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal("actual source preview GET", err)
	}
	data, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if response.StatusCode != 200 || readErr != nil || closeErr != nil || !bytes.Equal(data, originalB) {
		t.Fatal("source preview signed a different member's original", readErr, closeErr)
	}
	if _, err := library.Preview(t.Context(), teamB, assetA); !errors.Is(err, media.ErrNotFound) {
		t.Fatal("team source picker signed another member's original", err)
	}
	derived, err := library.SourceActor(t.Context(), teamB, true)
	if err != nil || derived.ID != b.ID || derived.SessionID != b.SessionID || derived.OrgID != b.OrgID || derived.ScopeID != b.ScopeID {
		t.Fatal("upload authority did not derive original personal actor", err)
	}
	uploader := &subjectUploadCapture{t: t, target: teamB, source: b}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("principal", teamB); c.Next() })
	subjecthttp.NewSourceHandler(library, uploader).Register(router.Group("/api"))
	upload := httptest.NewRecorder()
	router.ServeHTTP(upload, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/subjects/sources/uploads", nil))
	if upload.Code != 201 || uploader.calls != 1 {
		t.Fatal("source handler did not pass server-derived actor to shared uploader", upload.Code)
	}
	if err := owner.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=(SELECT team_id FROM workspace.creation_scope WHERE id=?) AND user_id=?`, teamB.ScopeID, b.ID).Error; err != nil {
		t.Fatal(err)
	}
	before := storage.count()
	if _, err := library.List(t.Context(), teamB, query); err == nil {
		t.Fatal("revoked target membership retained source listing")
	}
	if _, err := library.Detail(t.Context(), teamB, assetB); err == nil {
		t.Fatal("revoked target membership retained source detail")
	}
	if _, err := library.Preview(t.Context(), teamB, assetB); err == nil {
		t.Fatal("revoked target membership retained source preview")
	}
	if _, err := library.SourceActor(t.Context(), teamB, true); err == nil || storage.count() != before {
		t.Fatal("revoked target admitted upload or read private objects", err)
	}
	upload = httptest.NewRecorder()
	router.ServeHTTP(upload, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/subjects/sources/uploads", nil))
	if upload.Code == 201 || uploader.calls != 1 {
		t.Fatal("revoked target reached multipart upload admission", upload.Code)
	}
}

// Only the HTTP delegation is replaced here; scope/session resolution is real.
// Multipart probing and storage acceptance belong to the existing media tests.
type subjectUploadCapture struct {
	t              *testing.T
	target, source identity.Principal
	calls          int
}

func (u *subjectUploadCapture) UploadPersonalFor(c *gin.Context, actor identity.Principal) {
	u.calls++
	current := identityhttp.Principal(c)
	if current != u.target || actor.ID != u.source.ID || actor.ScopeID != u.source.ScopeID || actor.OrgID != u.source.OrgID || actor.SessionID != u.source.SessionID {
		u.t.Error("upload mutated request principal or passed target as personal owner")
	}
	c.Status(201)
}
