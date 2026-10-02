package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type packageBeforeArchiveFailure struct {
	mediaapp.PackageObjects
	failed bool
}

func (o *packageBeforeArchiveFailure) PutIfAbsent(ctx context.Context, key string, r io.Reader, size int64, mime, digest string) error {
	if !o.failed {
		o.failed = true
		return io.ErrUnexpectedEOF
	}
	return o.PackageObjects.PutIfAbsent(ctx, key, r, size, mime, digest)
}

func TestMediaPackageUnknownBeforeArchiveSameOriginalKeyRecoversAndPriorReceiptDoesNotDrift(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, project := mediaStoreProject(t, db)
	repo, failedService := packageActualService(t, db, &packageBeforeArchiveFailure{PackageObjects: objects}, packageTestAccess)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	in := mediaapp.PackageImportRequest{Scope: scope, ExpectedProjectRevision: 1, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	input := packageDownloaded(t, ownPackageZIP(t))
	first, err := failedService.Import(t.Context(), actor, in, input)
	if err != nil || first.Status != "needs_reconciliation" {
		t.Fatal(first, err)
	}
	packageCleanup(t, repo, objects, actor, scope)
	_, service := packageActualService(t, db, objects, packageTestAccess)
	head, err := service.Reconcile(t.Context(), actor, first.ID, mediaapp.PackageControl{ExpectedRevision: 1, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil || head.Status != "needs_reconciliation" {
		t.Fatal("missing archive misreported success", head, err)
	}
	replayed, err := service.Import(t.Context(), actor, in, input)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(replayed)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("original sealed import response changed", replayed, err)
	}
	current, err := service.Get(t.Context(), actor, first.ID)
	if err != nil || current.Status != "succeeded" || current.ID != first.ID || current.Revision != 2 {
		t.Fatal("resubmitted exact source did not recover original batch", current, err)
	}
	var jobs int64
	if err := db.Raw(`SELECT count(*) FROM media.package_job WHERE actor_id=?`, actor.ID).Scan(&jobs).Error; err != nil || jobs != 1 {
		t.Fatal("recovery created new identities", jobs, err)
	}
}

func TestMediaPackageRejectedCompleteBatchNeverWritesAnArchiveOrPartialAsset(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, project := mediaStoreProject(t, db)
	_, service := packageActualService(t, db, objects, packageTestAccess)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	remote := gltfJSONDocument(t)
	remote["buffers"].([]any)[0].(map[string]any)["uri"] = "https://example.invalid/never-fetch.bin"
	for _, bad := range []packageFixtureFile{
		{"image", "spoof.png", "image/png", []byte("wrong image bytes")},
		{"model", "remote.gltf", "model/gltf+json", gltfJSONBytes(t, remote)},
		{"model", "misnamed.glb", "model/gltf-binary", gltfJSONBytes(t, gltfJSONDocument(t))},
		{"document", "active.docx", domain.MIMEDOCX, documentDOCX(t, map[string]string{"word/vbaProject.bin": "active"})},
	} {
		files := []packageFixtureFile{{"image", "valid.png", "image/png", uploadPNG(t)}, bad}
		if _, err := service.Import(t.Context(), actor, mediaapp.PackageImportRequest{Scope: scope, ExpectedProjectRevision: 1, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}, packageDownloaded(t, packageFilesZIP(t, files))); err == nil {
			t.Fatal("unsupported byte closure accepted", bad.name)
		}
	}
	for _, query := range []string{`SELECT count(*) FROM media.package_job WHERE actor_id=?`, `SELECT count(*) FROM media.media_asset WHERE project_id=?`} {
		var count int64
		target := actor.ID
		if strings.Contains(query, "project_id") {
			target = project
		}
		if err := db.Raw(query, target).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("rejected batch persisted partial target", count, err)
		}
	}
}

func TestMediaPackageFrozenSQLAndTerminalCommandAreImmutableAndRollbackIsGuarded(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, _ := mediaStoreProject(t, db)
	repo, service := packageActualService(t, db, objects, packageTestAccess)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	in := mediaapp.PackageImportRequest{Scope: scope, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}
	job, err := service.Import(t.Context(), actor, in, packageDownloaded(t, ownPackageZIP(t)))
	if err != nil || job.Status != "succeeded" {
		t.Fatal(job, err)
	}
	packageCleanup(t, repo, objects, actor, scope)
	for _, statement := range []string{`UPDATE media.package_job SET frozen='{}'::bytea WHERE id=?`, `DELETE FROM media.package_job WHERE id=?`, `UPDATE media.package_job SET revision=revision+1 WHERE id=?`, `UPDATE media.package_object SET object_key='foreign' WHERE job_id=?`} {
		if err := db.Exec(statement, job.ID).Error; err == nil {
			t.Fatal("nonowner mutated frozen/terminal facts", statement)
		}
	}
	if err := owner.Exec(`UPDATE media.package_job SET frozen='{}'::bytea WHERE id=?`, job.ID).Error; err == nil {
		t.Fatal("owning immutable trigger did not retain frozen bytes")
	}
	if err := owner.Exec(`UPDATE media.package_command SET response='{}'::bytea WHERE actor_id=? AND idem_key=?`, actor.ID, in.Key).Error; err == nil {
		t.Fatal("permanent command trigger absent")
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "202610020059_media_package.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx := owner.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := tx.Exec(string(down)).Error; err == nil || !strings.Contains(err.Error(), "cannot remove retained media package batches") {
		_ = tx.Rollback().Error
		t.Fatal("retained private source and intents downgrade guard was not reached", err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if current, err := service.Get(t.Context(), actor, job.ID); err != nil || current.ID != job.ID {
		t.Fatal("guarded downgrade damaged package owner", current, err)
	}
	if _, err := service.Import(t.Context(), actor, in, packageDownloaded(t, ownPackageZIP(t))); !errors.Is(err, mediaapp.ErrPackageConflict) {
		t.Fatal("different retained source ZIP shared same command", err)
	}
}

func TestMediaPackageProjectFolderEightLevelsCompleteMappingAndPersonalDepthRejectsWhole(t *testing.T) {
	db := libraryRuntimeDB(t)
	objects := mediaobjects.NewProjectCopyObjects(glbTestObjects(t))
	actor, project := mediaStoreProject(t, db)
	repo, service := packageActualService(t, db, objects, packageTestAccess)
	scope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	m := mediaapp.LibraryPackageManifest{App: "lanverse-media-library", Version: 1, ExportedAt: time.Now().UTC(), LibraryKind: domain.LibraryProject, Folders: []mediaapp.LibraryPackageFolder{}, Items: []mediaapp.LibraryPackageItem{}, Files: []mediaapp.LibraryPackageFile{}}
	var parent *uuid.UUID
	for i := 0; i < 8; i++ {
		id := uuid.New()
		m.Folders = append(m.Folders, mediaapp.LibraryPackageFolder{ID: id, ParentID: parent, Name: "层级", Style: "cinema", Theme: "obsidian"})
		parent = &id
	}
	text := "完整嵌套正文"
	m.Items = append(m.Items, mediaapp.LibraryPackageItem{ID: "nested", Kind: "text", State: "active", Metadata: mediaapp.LibraryMetadata{Title: "正文", Category: "other", Tags: []string{}, FolderID: parent, PlainText: &text}})
	body, _ := json.Marshal(m)
	input := packageDownloaded(t, packageZIP(t, []string{"manifest.json"}, [][]byte{body}, false))
	job, err := service.Import(t.Context(), actor, mediaapp.PackageImportRequest{Scope: scope, ExpectedProjectRevision: 1, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}, input)
	if err != nil || job.Status != "succeeded" || job.FolderCount != 8 {
		t.Fatal("complete accepted depth", job, err)
	}
	packageCleanup(t, repo, objects, actor, scope)
	export, err := service.Export(t.Context(), actor, scope)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = export.Close() }()
	archive, err := mediaapp.ReadLibraryPackage(t.Context(), export.File, export.Size)
	if err != nil || len(archive.Manifest.Folders) != 8 || len(archive.Manifest.Items) != 1 {
		t.Fatal("complete tree export", err)
	}
	if _, err := service.Import(t.Context(), actor, mediaapp.PackageImportRequest{Scope: domain.LibraryScope{Kind: domain.LibraryPersonal}, LocalReviewConfirmed: true, Key: uuid.New(), RequestID: uuid.New()}, export); !errors.Is(err, mediaapp.ErrInvalidPackage) {
		t.Fatal("personal flattening discarded source tree", err)
	}
}
