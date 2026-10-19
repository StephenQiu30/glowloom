package media_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
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
	cases := []struct {
		names  []string
		bodies [][]byte
	}{
		{[]string{"assets.json", "files/original.png"}, [][]byte{manifest, image}},
		{[]string{"assets.json"}, [][]byte{manifest}},
		{[]string{"manifest.json", "assets.json"}, [][]byte{manifest, image}},
	}
	for _, item := range cases {
		names := item.names
		data := packageZIP(t, names, item.bodies, false)
		if _, err := mediaapp.ReadLibraryPackage(t.Context(), bytes.NewReader(data), int64(len(data))); !errors.Is(err, mediaapp.ErrInvalidPackage) {
			t.Fatal("upstream asset package was accepted", names, err)
		}
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
	data := ownPackageZIP(t)
	image = uploadPNG(t)
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
