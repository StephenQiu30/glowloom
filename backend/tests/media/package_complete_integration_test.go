package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type packageFixtureFile struct {
	kind, name, mime string
	data             []byte
}

func packageFilesZIP(t *testing.T, files []packageFixtureFile) []byte {
	t.Helper()
	m := mediaapp.LibraryPackageManifest{App: "lanverse-media-library", Version: 1, ExportedAt: time.Now().UTC(), LibraryKind: domain.LibraryPersonal, Folders: []mediaapp.LibraryPackageFolder{}, Items: []mediaapp.LibraryPackageItem{}, Files: []mediaapp.LibraryPackageFile{}}
	names, bodies := []string{"manifest.json"}, [][]byte{nil}
	for _, f := range files {
		path := "files/" + f.name
		digest := sha256.Sum256(f.data)
		m.Items = append(m.Items, mediaapp.LibraryPackageItem{ID: f.name, Kind: f.kind, State: "active", FilePath: &path, Metadata: mediaapp.LibraryMetadata{Title: f.name, Category: "material", Tags: []string{}}})
		m.Files = append(m.Files, mediaapp.LibraryPackageFile{Path: path, FileName: f.name, MIMEType: f.mime, ByteSize: int64(len(f.data)), SHA256: hex.EncodeToString(digest[:])})
		names, bodies = append(names, path), append(bodies, f.data)
	}
	var err error
	bodies[0], err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return packageZIP(t, names, bodies, false)
}

func packageWAV(t *testing.T) []byte {
	t.Helper()
	file := filepath.Join(t.TempDir(), "voice.wav")
	if out, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "pcm_s16le", file).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMediaPackageActualAllAcceptedKindsSourceZIPExactOriginalsAndIndependentRenditions(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, _ := mediaStoreProject(t, db)
	repo, service := packageActualService(t, db, objects, packageTestAccess)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	files := []packageFixtureFile{
		{"image", "animation.gif", "image/gif", uploadGIF(t, 3)},
		{"model", "scene.glb", "model/gltf-binary", glbBytes(t, glbDocument(), glbGeometry())},
		{"model", "scene.gltf", "model/gltf+json", gltfJSONBytes(t, gltfJSONDocument(t))},
		{"document", "script.txt", domain.MIMEText, []byte("  第一幕\n完整正文🌧️  ")},
		{"document", "script.docx", domain.MIMEDOCX, documentDOCX(t, nil)},
		{"audio", "voice.wav", "audio/wave", packageWAV(t)},
		{"video", "recording.webm", "video/webm", uploadWebMFixture(t, "libvpx", "0.3")},
	}
	data := packageFilesZIP(t, files)
	in := mediaapp.PackageImportRequest{Scope: scope, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	job, err := service.Import(t.Context(), actor, in, packageDownloaded(t, data))
	if err != nil || job.Status != "succeeded" || job.ItemCount != len(files) {
		t.Fatal("whole kinds package", job, err)
	}
	keys := packageCleanup(t, repo, objects, actor, scope)
	if len(keys) != 13 {
		t.Fatal("ZIP + seven originals + two GIF/two video/one audio renditions", len(keys))
	}
	var plan mediaapp.PackagePlan
	var frozen []byte
	if err := db.Raw(`SELECT frozen FROM media.package_job WHERE id=?`, job.ID).Row().Scan(&frozen); err != nil || json.Unmarshal(frozen, &plan) != nil {
		t.Fatal(err)
	}
	plan.Request.Key, plan.Request.RequestID = plan.Key, plan.RequestID
	plan.Request.ArchiveSHA256, plan.Request.ArchiveBytes = plan.Objects[0].SHA256, plan.Objects[0].ByteSize
	if err := plan.Validate(); err != nil {
		t.Fatal("valid closed plan", err)
	}
	source, err := objects.Get(t.Context(), plan.Objects[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(source)
	closeErr := source.Close()
	if err != nil || closeErr != nil || !bytes.Equal(actual, data) {
		t.Fatal("complete retained exact source ZIP", err, closeErr)
	}
	for _, asset := range plan.Assets {
		var source packageFixtureFile
		for _, f := range files {
			if f.name == asset.FileName {
				source = f
			}
		}
		if asset.MimeType == "video/mp4" {
			continue
		}
		if source.data == nil || asset.SHA256 == nil {
			t.Fatal("original format/name lost", asset.FileName)
		}
		input, err := objects.Get(t.Context(), asset.ObjectKey)
		if err != nil {
			t.Fatal(err)
		}
		bytesRead, err := io.ReadAll(input)
		closeErr := input.Close()
		if err != nil || closeErr != nil || !bytes.Equal(bytesRead, source.data) {
			t.Fatal("actual original differs", asset.FileName, err, closeErr)
		}
	}
	for _, mutate := range []func(*mediaapp.PackagePlan){
		func(p *mediaapp.PackagePlan) { p.Assets[0].ProjectID = uuid.New() },
		func(p *mediaapp.PackagePlan) { p.Objects[0].Key = "projects/foreign/source.zip" },
		func(p *mediaapp.PackagePlan) { p.Objects = p.Objects[:len(p.Objects)-1] },
		func(p *mediaapp.PackagePlan) { p.Renditions[0].MediaAssetID = uuid.New() },
		func(p *mediaapp.PackagePlan) { p.Items[0].FolderID = ptrUUID(uuid.New()) },
		func(p *mediaapp.PackagePlan) { p.Assets[0].ModerationDetail = []byte(`{"rights_confirmed":false}`) },
	} {
		var bad mediaapp.PackagePlan
		encoded, _ := json.Marshal(plan)
		if json.Unmarshal(encoded, &bad) != nil {
			t.Fatal("copy plan")
		}
		bad.Request = plan.Request
		mutate(&bad)
		if !errors.Is(bad.Validate(), mediaapp.ErrInvalidPackage) {
			t.Fatal("unclosed frozen identity accepted")
		}
	}
	exported, err := service.Export(t.Context(), actor, scope)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = exported.Close() }()
	archive, err := mediaapp.ReadLibraryPackage(t.Context(), exported.File, exported.Size)
	if err != nil || len(archive.Manifest.Items) != len(files) || len(archive.Manifest.Files) != len(files) {
		t.Fatal("complete accepted-kind export", err)
	}
	for _, f := range archive.Manifest.Files {
		if f.FileName == "scene.gltf" && f.MIMEType != "model/gltf+json" {
			t.Fatal("JSON model relabelled")
		}
	}
}

func TestMediaPackageSourceV1ActualImportNeverPromotesSourceConsentOrURLs(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, _ := mediaStoreProject(t, db)
	repo, service := packageActualService(t, db, objects, packageTestAccess)
	image := uploadPNG(t)
	data := packageZIP(t, []string{"assets.json", "files/original.png"}, [][]byte{sourcePackageManifest(t, image), image}, false)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	job, err := service.Import(t.Context(), actor, mediaapp.PackageImportRequest{Scope: scope, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}, packageDownloaded(t, data))
	if err != nil || job.Status != "succeeded" || len(job.Warnings) == 0 {
		t.Fatal(job, err)
	}
	packageCleanup(t, repo, objects, actor, scope)
	var rows []struct {
		ID                 uuid.UUID
		ContainsRealPerson bool
		ConsentRecordID    *uuid.UUID
		ModerationDetail   json.RawMessage
		ObjectKey          string
	}
	if err := db.Raw(`SELECT id,contains_real_person,consent_record_id,moderation_detail,object_key FROM media.media_asset WHERE personal_actor_id=?`, actor.ID).Scan(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	var review mediaapp.LocalUploadReview
	if rows[0].ContainsRealPerson || rows[0].ConsentRecordID != nil || json.Unmarshal(rows[0].ModerationDetail, &review) != nil || review.PrincipalID != actor.ID || !review.RightsConfirmed {
		t.Fatal("source allegations became target consent")
	}
	if bytes.Contains(rows[0].ModerationDetail, []byte("example.invalid")) {
		t.Fatal("source URL persisted as review")
	}
	input, err := objects.Get(t.Context(), rows[0].ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(input)
	_ = input.Close()
	if err != nil || !bytes.Equal(actual, image) {
		t.Fatal("source bytes not independently retained", err)
	}
}

func TestMediaPackageCompleteTrashExportOriginalMissingFailsWholeAndRetainsCatalogState(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, project := mediaStoreProject(t, db)
	repo, service := packageActualService(t, db, objects, packageTestAccess)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	job, err := service.Import(t.Context(), actor, mediaapp.PackageImportRequest{Scope: scope, ExpectedProjectRevision: 1, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}, packageDownloaded(t, ownPackageZIP(t)))
	if err != nil || job.Status != "succeeded" {
		t.Fatal(job, err)
	}
	packageCleanup(t, repo, objects, actor, scope)
	now := time.Now().UTC().Truncate(time.Microsecond)
	library, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE media.library_item SET catalog_state='trashed',trashed_at=?,revision=revision+1,update_time=? WHERE library_id=?`, now, now, library).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE media.media_asset SET is_delete=true,delete_time=?,purge_after=?,revision=revision+1,update_time=? WHERE project_id=?`, now, now.Add(30*24*time.Hour), now, project).Error
	}); err != nil {
		t.Fatal(err)
	}
	export, err := service.Export(t.Context(), actor, scope)
	if err != nil {
		t.Fatal("retained trash rejected", err)
	}
	archive, err := mediaapp.ReadLibraryPackage(t.Context(), export.File, export.Size)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Manifest.Items) != 2 {
		t.Fatal("trash omitted")
	}
	for _, item := range archive.Manifest.Items {
		if item.State != "trashed" || item.TrashedAt == nil || !item.TrashedAt.Equal(now) {
			t.Fatal("recycle state lost", item.State)
		}
	}
	personal := domain.LibraryScope{Kind: domain.LibraryPersonal}
	personalImport, err := service.Import(t.Context(), actor, mediaapp.PackageImportRequest{Scope: personal, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}, export)
	_ = export.Close()
	if err != nil || personalImport.Status != "succeeded" {
		t.Fatal("trash round trip", personalImport, err)
	}
	packageCleanup(t, repo, objects, actor, personal)
	var key string
	if err := db.Raw(`SELECT object_key FROM media.media_asset WHERE project_id=?`, project).Scan(&key).Error; err != nil {
		t.Fatal(err)
	}
	if err := objects.Remove(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	if failed, err := service.Export(t.Context(), actor, scope); err == nil || failed != nil {
		t.Fatal("missing trash original yielded partial export", err)
	}
}

type packageFailedTouch struct{ mediaapp.LibraryProjectAccess }

func (a packageFailedTouch) TouchContent(context.Context, identityapp.Principal, uuid.UUID, int64) (int64, error) {
	return 0, mediaapp.ErrUnavailable
}

func TestMediaPackageAtomicPublicationOutboxFailureLeavesNoHalfLibraryAndSameKeyRecovers(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, project := mediaStoreProject(t, db)
	failed := func(tx *gorm.DB) mediaapp.LibraryProjectAccess { return packageFailedTouch{packageTestAccess(tx)} }
	repo, service := packageActualService(t, db, objects, failed)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	in := mediaapp.PackageImportRequest{Scope: scope, ExpectedProjectRevision: 1, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	file := packageDownloaded(t, ownPackageZIP(t))
	if _, err := service.Import(t.Context(), actor, in, file); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("publication fault absent", err)
	}
	keys := packageCleanup(t, repo, objects, actor, scope)
	if len(keys) != 4 {
		t.Fatal("uncertain original intents erased", len(keys))
	}
	library, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"media.media_asset", "media.library_item", "media.library_folder"} {
		var count int64
		query := `SELECT count(*) FROM ` + table + ` WHERE library_id=?`
		arg := any(library)
		if table == "media.media_asset" {
			query = `SELECT count(*) FROM media.media_asset WHERE project_id=?`
			arg = project
		}
		if err := db.Raw(query, arg).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("publication half committed", table, count, err)
		}
	}
	_, recoveredService := packageActualService(t, db, objects, packageTestAccess)
	job, err := recoveredService.Import(t.Context(), actor, in, file)
	if err != nil || job.Status != "succeeded" || job.LibraryRevision != 1 || job.ProjectRevision != 2 {
		t.Fatal("original key recovery", job, err)
	}
	var logCount int64
	if err := db.Raw(`SELECT count(*) FROM audit.audit_log WHERE object_type='media.package' AND object_id=? AND action='media.package.imported'`, job.ID.String()).Scan(&logCount).Error; err != nil || logCount != 1 {
		t.Fatal("false or duplicate success audit", logCount, err)
	}
}
