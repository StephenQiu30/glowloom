package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gltf"
	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func packageDownloaded(t *testing.T, data []byte) *mediaapp.Downloaded {
	t.Helper()
	f, err := mediaapp.ReadPackageUpload(t.Context(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func ownPackageZIP(t *testing.T) []byte {
	t.Helper()
	image := uploadPNG(t)
	digest := sha256.Sum256(image)
	folder := uuid.New()
	text := "  完整正文\n第二行🌧️  "
	m := mediaapp.LibraryPackageManifest{App: "lanverse-media-library", Version: 1, ExportedAt: time.Now().UTC(), LibraryKind: domain.LibraryProject,
		Folders: []mediaapp.LibraryPackageFolder{{ID: folder, Name: "人物", Style: "cinema", Theme: "obsidian"}},
		Items: []mediaapp.LibraryPackageItem{
			{ID: "source-image", Kind: "image", Metadata: mediaapp.LibraryMetadata{FolderID: &folder, Title: "原件", Category: "character", Tags: []string{"人物"}, Note: "完整说明"}, State: "active", FilePath: packageString("files/source.png")},
			{ID: "source-text", Kind: "text", Metadata: mediaapp.LibraryMetadata{FolderID: &folder, PlainText: &text, Title: "正文", Category: "other", Tags: []string{}}, State: "active", Position: 1}},
		Files: []mediaapp.LibraryPackageFile{{Path: "files/source.png", FileName: "原件.png", MIMEType: "image/png", ByteSize: int64(len(image)), SHA256: hex.EncodeToString(digest[:])}}}
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return packageZIP(t, []string{"manifest.json", "files/source.png"}, [][]byte{body, image}, false)
}

func packageString(value string) *string { return &value }

func TestMediaPackageImportCompleteAtomicPrivateFilesFoldersAndExportRoundtrip(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	uploads := mediaapp.NewScopedUploadService(pgmedia.NewStore(db), pgmedia.NewStore(db), mediaflow.NewUploadProber(validator), mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	repo := pgmedia.NewPackageStore(db, libraryTestAccess, transferTestGuards, time.Now)
	service := mediaapp.NewLibraryPackageService(repo, uploads, mediaobjects.NewProjectCopyObjects(objects), time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	in := mediaapp.PackageImportRequest{Scope: scope, ExpectedRevision: 0, ExpectedProjectRevision: 1, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	file := packageDownloaded(t, ownPackageZIP(t))
	job, err := service.Import(t.Context(), actor, in, file)
	if err != nil || job.Status != "succeeded" || job.ItemCount != 2 || job.FolderCount != 1 || job.LibraryRevision != 1 || job.ProjectRevision != 2 {
		t.Fatal("whole import", job, err)
	}
	keys, err := repo.StorageUsageKeys(t.Context(), actor, scope)
	if err != nil || len(keys) != 4 {
		t.Fatal("retained archive/original/two actual thumbnails", len(keys), err)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			_ = objects.Remove(context.Background(), key)
		}
	})
	page, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).ListLibrary(t.Context(), actor, scope, mediaapp.LibraryQuery{Page: 1, PageSize: 120, State: "active", Order: "name_asc"})
	if err != nil || page.Total != 2 || len(page.Folders) != 1 {
		t.Fatal("formal full contents", page, err)
	}
	for _, item := range page.Items {
		if item.ID.String() == "source-image" || item.ID.String() == "source-text" || item.FolderID == nil || *item.FolderID != page.Folders[0].ID {
			t.Fatal("source identity or placement leaked", item)
		}
	}
	exported, err := service.Export(t.Context(), actor, scope)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = exported.Close() }()
	archive, err := mediaapp.ReadLibraryPackage(t.Context(), exported.File, exported.Size)
	if err != nil || len(archive.Manifest.Items) != 2 || len(archive.Manifest.Folders) != 1 || len(archive.Manifest.Files) != 1 {
		t.Fatal("complete exported package", err)
	}
	for _, entry := range archive.Manifest.Items {
		if entry.Kind == "text" && (entry.Metadata.PlainText == nil || *entry.Metadata.PlainText != "  完整正文\n第二行🌧️  ") {
			t.Fatal("editorial text was rewritten")
		}
	}
	personal := domain.LibraryScope{Kind: domain.LibraryPersonal}
	personalImport, err := service.Import(t.Context(), actor, mediaapp.PackageImportRequest{Scope: personal, ExpectedRevision: 0, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}, exported)
	if err != nil || personalImport.Status != "succeeded" || personalImport.ID == job.ID {
		t.Fatal("personal independent round trip", personalImport, err)
	}
	pkeys, err := repo.StorageUsageKeys(t.Context(), actor, personal)
	if err != nil || len(pkeys) != 4 {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, key := range pkeys {
			_ = objects.Remove(context.Background(), key)
		}
	})
	for _, key := range keys {
		if err := objects.Remove(t.Context(), key); err != nil {
			t.Fatal(err)
		}
	}
	independent, err := service.Export(t.Context(), actor, personal)
	if err != nil {
		t.Fatal("target depends on fixture source", err)
	}
	_ = independent.Close()
	replay, err := service.Import(t.Context(), actor, in, file)
	if err != nil || replay.ID != job.ID || replay.LibraryRevision != job.LibraryRevision {
		t.Fatal("permanent replay became a new import", replay, err)
	}
}

type packageUnknownWrite struct {
	mediaapp.PackageObjects
	once bool
}

func (o *packageUnknownWrite) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, mime, digest string) error {
	err := o.PackageObjects.PutIfAbsent(ctx, key, reader, size, mime, digest)
	if !o.once && err == nil {
		o.once = true
		return io.ErrUnexpectedEOF
	}
	return err
}

func TestMediaPackageUnknownWritePersistsWholeBatchAndReconcilesWithoutNewIdentities(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := glbTestObjects(t)
	actor, project := mediaStoreProject(t, db)
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	uploads := mediaapp.NewScopedUploadService(pgmedia.NewStore(db), pgmedia.NewStore(db), mediaflow.NewUploadProber(validator), mediaflow.FFUploadNormalizer{}, mediaflow.FFUploadRenderer{}, mediaflow.NewUploadObjects(objects), time.Now)
	repo := pgmedia.NewPackageStore(db, libraryTestAccess, transferTestGuards, time.Now)
	base := mediaobjects.NewProjectCopyObjects(objects)
	unknown := &packageUnknownWrite{PackageObjects: base}
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	input := mediaapp.PackageImportRequest{Scope: scope, ExpectedProjectRevision: 1, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	job, err := mediaapp.NewLibraryPackageService(repo, uploads, unknown, time.Now).Import(t.Context(), actor, input, packageDownloaded(t, ownPackageZIP(t)))
	if err != nil || job.Status != "needs_reconciliation" {
		t.Fatal("unknown write lost original job", job, err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM media.media_asset WHERE project_id=?`, project).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("partial package published", count, err)
	}
	busy, err := repo.HasInflightPackageWork(t.Context(), actor, project)
	if err != nil || !busy {
		t.Fatal("unknown batch did not block lifecycle", busy, err)
	}
	recovered, err := mediaapp.NewLibraryPackageService(repo, uploads, base, time.Now).Reconcile(t.Context(), actor, job.ID)
	if err != nil || recovered.ID != job.ID || recovered.Status != "succeeded" {
		t.Fatal("original durable recovery", recovered, err)
	}
	keys, err := repo.StorageUsageKeys(t.Context(), actor, scope)
	if err != nil || len(keys) != 4 {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			_ = objects.Remove(context.Background(), key)
		}
	})
	if busy, err := repo.HasInflightPackageWork(t.Context(), actor, project); err != nil || busy {
		t.Fatal("completed batch retained work reservation", busy, err)
	}
	input.ExpectedRevision = 99
	input.Key = uuid.New()
	if _, err := mediaapp.NewLibraryPackageService(repo, uploads, base, time.Now).Import(t.Context(), actor, input, packageDownloaded(t, ownPackageZIP(t))); !errors.Is(err, mediaapp.ErrPackageConflict) {
		t.Fatal("stale scope admitted", err)
	}
}
