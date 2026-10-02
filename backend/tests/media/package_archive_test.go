package media_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func packageZIP(t *testing.T, names []string, bodies [][]byte, symlink bool) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for index, name := range names {
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		if symlink {
			h.SetMode(os.ModeSymlink | 0o777)
		}
		file, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(bodies[index]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func sourcePackageManifest(t *testing.T, binary []byte) []byte {
	t.Helper()
	data := map[string]any{"app": "infinite-canvas", "version": 1, "exportedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"assets": []any{map[string]any{"id": "source-image", "kind": "image", "title": "源角色", "coverUrl": "https://example.invalid/private-image", "tags": []string{"主角"}, "folderId": "local-folder-not-exported", "category": "character", "portraitCertified": true, "arkAssetId": "untrusted-certificate", "createdAt": time.Now().UTC().Format(time.RFC3339Nano), "updatedAt": time.Now().UTC().Format(time.RFC3339Nano), "data": map[string]any{"storageKey": "image:original", "dataUrl": "https://example.invalid/never-fetch", "width": 3, "height": 4, "bytes": len(binary), "mimeType": "image/png"}}},
		"files":  []any{map[string]any{"storageKey": "image:original", "path": "files/original.png", "mimeType": "image/png", "bytes": len(binary)}}}
	out, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMediaPackageSourceV1RequiresAllActualFilesAndDoesNotTrustURLsOrConsent(t *testing.T) {
	image := uploadPNG(t)
	manifest := sourcePackageManifest(t, image)
	data := packageZIP(t, []string{"assets.json", "files/original.png"}, [][]byte{manifest, image}, false)
	archive, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(data), int64(len(data)))
	if err != nil || archive.SourceFormat != "beeftv-assets-v1" || len(archive.Manifest.Items) != 1 || len(archive.Manifest.Files) != 1 {
		t.Fatal("source v1 complete actual package", archive, err)
	}
	item := archive.Manifest.Items[0]
	if item.Kind != "image" || item.Metadata.FolderID != nil || item.Metadata.Title != "源角色" || len(archive.Warnings) == 0 {
		t.Fatal("source-only annotations became owned authority or a fake directory", item, archive.Warnings)
	}
	file, err := archive.Open(t.Context(), "files/original.png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	actual, err := io.ReadAll(file.File)
	if err != nil || !bytes.Equal(actual, image) || file.SHA256 != archive.Manifest.Files[0].SHA256 {
		t.Fatal("archive returned anything other than actual contained bytes", err)
	}
	missing := packageZIP(t, []string{"assets.json"}, [][]byte{manifest}, false)
	if _, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(missing), int64(len(missing))); !errors.Is(err, mediaapp.ErrPackageIncomplete) {
		t.Fatal("source missing blob was silently skipped", err)
	}
}

func TestMediaPackageRejectsUnsafeDuplicateSymlinkCRCAndUnclaimedContent(t *testing.T) {
	image := uploadPNG(t)
	manifest := sourcePackageManifest(t, image)
	for _, name := range []string{"../original.png", "/original.png", "files/../original.png", `files\original.png`, "C:/original.png", "files/./original.png", "files/original.png\x00"} {
		t.Run(name, func(t *testing.T) {
			data := packageZIP(t, []string{"assets.json", name}, [][]byte{manifest, image}, false)
			if _, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(data), int64(len(data))); !errors.Is(err, mediaapp.ErrInvalidPackage) {
				t.Fatal("unsafe archive entry", name, err)
			}
		})
	}
	for _, test := range []struct {
		name    string
		names   []string
		bodies  [][]byte
		symlink bool
	}{
		{"duplicate", []string{"assets.json", "files/original.png", "files/original.png"}, [][]byte{manifest, image, image}, false},
		{"case ambiguity", []string{"assets.json", "files/original.png", "FILES/ORIGINAL.PNG"}, [][]byte{manifest, image, image}, false},
		{"symlink", []string{"assets.json", "files/original.png"}, [][]byte{manifest, image}, true},
		{"unclaimed", []string{"assets.json", "files/original.png", "files/hidden.png"}, [][]byte{manifest, image, image}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := packageZIP(t, test.names, test.bodies, test.symlink)
			if _, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(data), int64(len(data))); !errors.Is(err, mediaapp.ErrInvalidPackage) {
				t.Fatal("unsafe resource set was accepted", err)
			}
		})
	}
	data := packageZIP(t, []string{"assets.json", "files/original.png"}, [][]byte{manifest, image}, false)
	loc := bytes.Index(data, image)
	if loc < 0 {
		t.Fatal("fixture payload absent")
	}
	data[loc+len(image)/2] ^= 1
	if _, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(data), int64(len(data))); !errors.Is(err, mediaapp.ErrInvalidPackage) {
		t.Fatal("actual CRC error did not reject whole package", err)
	}
}

func TestMediaPackageOwnManifestPreservesFoldersAndRejectsDigestOrIdentityMismatch(t *testing.T) {
	folder := uuid.New()
	content := "包内完整文字"
	manifest := mediaapp.LibraryPackageManifest{App: "lanverse-media-library", Version: 1, ExportedAt: time.Now().UTC(), LibraryKind: domain.LibraryProject,
		Folders: []mediaapp.LibraryPackageFolder{{ID: folder, Name: "角色", Style: "cinema", Theme: "obsidian", Position: 0}},
		Items:   []mediaapp.LibraryPackageItem{{ID: uuid.NewString(), Kind: "text", Metadata: mediaapp.LibraryMetadata{FolderID: &folder, PlainText: &content, Title: "正文", Category: "character", Tags: []string{}}, State: "active"}}, Files: []mediaapp.LibraryPackageFile{}}
	binary, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data := packageZIP(t, []string{"manifest.json"}, [][]byte{binary}, false)
	archive, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.Manifest.Folders) != 1 || archive.Manifest.Items[0].Metadata.PlainText == nil || *archive.Manifest.Items[0].Metadata.PlainText != content {
		t.Fatal("owned full manifest lost editorial content", archive, err)
	}
	manifest.Items[0].Metadata.FolderID = ptrUUID(uuid.New())
	binary, _ = json.Marshal(manifest)
	data = packageZIP(t, []string{"manifest.json"}, [][]byte{binary}, false)
	if _, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(data), int64(len(data))); !errors.Is(err, mediaapp.ErrInvalidPackage) {
		t.Fatal("broken complete directory mapping was accepted", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := mediaapp.ReadLibraryPackage(ctx, bytes.NewReader(data), int64(len(data))); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled reader continued archive validation", err)
	}
}

func ptrUUID(id uuid.UUID) *uuid.UUID { return &id }
