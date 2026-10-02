package application

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// LibraryPackageArchive retains the caller-owned bounded ZIP reader. Open
// creates a private temporary copy and rechecks its frozen digest and CRC.
// Reading is not publication, media probing or human review.
type LibraryPackageArchive struct {
	SourceFormat string
	Manifest     LibraryPackageManifest
	Warnings     []PackageWarning
	entries      map[string]*zip.File
}

func packagePath(name string, directory bool) bool {
	if directory {
		name = strings.TrimSuffix(name, "/")
	}
	if name == "" || len(name) > 512 || !utf8.ValidString(name) || strings.ContainsAny(name, "\\:") || strings.HasPrefix(name, "/") || path.Clean(name) != name {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// ReadLibraryPackage verifies every entry, complete references and actual CRC
// before returning a review draft. It never follows any embedded remote URL.
func ReadLibraryPackage(ctx context.Context, reader io.ReaderAt, size int64) (*LibraryPackageArchive, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if reader == nil || size < 1 || size > MaxPackageBytes {
		return nil, ErrInvalidPackage
	}
	r, err := zip.NewReader(reader, size)
	if err != nil || len(r.File) == 0 || len(r.File) > MaxPackageEntries {
		return nil, ErrInvalidPackage
	}
	archive := &LibraryPackageArchive{entries: make(map[string]*zip.File), Warnings: []PackageWarning{}}
	seen := make(map[string]bool, len(r.File))
	expanded := int64(0)
	for _, file := range r.File {
		directory := file.FileInfo().IsDir()
		key := strings.ToLower(strings.TrimSuffix(file.Name, "/"))
		if !packagePath(file.Name, directory) || seen[key] || file.Flags&1 != 0 || file.Method != zip.Store && file.Method != zip.Deflate || file.Mode()&os.ModeType != 0 && !directory || file.UncompressedSize64 > uint64(MaxPackageExpandedBytes-expanded) {
			return nil, ErrInvalidPackage
		}
		seen[key] = true
		expanded += int64(file.UncompressedSize64)
		if directory {
			if file.UncompressedSize64 != 0 {
				return nil, ErrInvalidPackage
			}
			continue
		}
		archive.entries[file.Name] = file
	}
	manifestName := "manifest.json"
	_, own := archive.entries[manifestName]
	_, source := archive.entries["assets.json"]
	if own == source {
		return nil, ErrInvalidPackage
	}
	if source {
		manifestName = "assets.json"
	}
	manifest, err := readPackageEntry(ctx, archive.entries[manifestName], maxPackageManifestBytes)
	if err != nil {
		return nil, err
	}
	if source {
		archive.SourceFormat = "beeftv-assets-v1"
		if err := archive.readSource(ctx, manifest); err != nil {
			return nil, err
		}
	} else {
		archive.SourceFormat = "lanverse-media-library-v1"
		if err := decodePackageJSON(manifest, &archive.Manifest); err != nil {
			return nil, err
		}
	}
	if err := archive.Manifest.validate(); err != nil {
		return nil, err
	}
	if len(archive.entries) != len(archive.Manifest.Files)+1 {
		return nil, ErrInvalidPackage
	}
	for _, expected := range archive.Manifest.Files {
		file, found := archive.entries[expected.Path]
		if !found {
			return nil, ErrPackageIncomplete
		}
		if file.UncompressedSize64 != uint64(expected.ByteSize) {
			return nil, ErrInvalidPackage
		}
		sha, size, err := hashPackageEntry(ctx, file, expected.ByteSize)
		if err != nil || size != expected.ByteSize || sha != expected.SHA256 {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrInvalidPackage
		}
	}
	return archive, nil
}

func readPackageEntry(ctx context.Context, file *zip.File, maxBytes int64) ([]byte, error) {
	if file == nil {
		return nil, ErrPackageIncomplete
	}
	if file.UncompressedSize64 > uint64(maxBytes) {
		return nil, ErrInvalidPackage
	}
	r, err := file.Open()
	if err != nil {
		return nil, ErrInvalidPackage
	}
	defer func() { _ = r.Close() }()
	data, err := io.ReadAll(io.LimitReader(uploadContextReader{ctx: ctx, reader: r}, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes || uint64(len(data)) != file.UncompressedSize64 {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrInvalidPackage
	}
	return data, nil
}

func hashPackageEntry(ctx context.Context, file *zip.File, maxBytes int64) (string, int64, error) {
	if file == nil || file.UncompressedSize64 > uint64(maxBytes) {
		return "", 0, ErrInvalidPackage
	}
	r, err := file.Open()
	if err != nil {
		return "", 0, ErrInvalidPackage
	}
	defer func() { _ = r.Close() }()
	h := sha256.New()
	size, err := io.Copy(h, io.LimitReader(uploadContextReader{ctx: ctx, reader: r}, maxBytes+1))
	if err != nil || size > maxBytes || uint64(size) != file.UncompressedSize64 {
		if ctx.Err() != nil {
			return "", 0, ctx.Err()
		}
		return "", 0, ErrInvalidPackage
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

// Open revalidates actual contained bytes into a new caller-owned local file.
// No path from the package is ever used as a local or private object pathname.
func (a *LibraryPackageArchive) Open(ctx context.Context, filePath string) (*Downloaded, error) {
	if a == nil {
		return nil, ErrInvalidPackage
	}
	var expected *LibraryPackageFile
	for _, file := range a.Manifest.Files {
		if file.Path == filePath {
			candidate := file
			expected = &candidate
			break
		}
	}
	entry, found := a.entries[filePath]
	if expected == nil || !found {
		return nil, ErrPackageIncomplete
	}
	input, err := entry.Open()
	if err != nil {
		return nil, ErrInvalidPackage
	}
	defer func() { _ = input.Close() }()
	file, err := os.CreateTemp("", "lanverse-package-*"+path.Ext(expected.FileName))
	if err != nil {
		return nil, err
	}
	out := &Downloaded{File: file, MIMEType: expected.MIMEType}
	keep := false
	defer func() {
		if !keep {
			_ = out.Close()
		}
	}()
	h := sha256.New()
	out.Size, err = io.Copy(io.MultiWriter(file, h), io.LimitReader(uploadContextReader{ctx: ctx, reader: input}, expected.ByteSize+1))
	out.SHA256 = hex.EncodeToString(h.Sum(nil))
	if err != nil || out.Size != expected.ByteSize || out.SHA256 != expected.SHA256 {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrInvalidPackage
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return out, nil
}

// decodePackageJSON rejects duplicate keys before closed typed decoding, and
// bounds aggregate JSON nodes/depth even for source-only ignored annotations.
func decodePackageJSON(data []byte, target any) error {
	if len(data) > maxPackageManifestBytes || !utf8.Valid(data) {
		return ErrInvalidPackage
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	nodes := 0
	var read func(int) error
	read = func(depth int) error {
		nodes++
		if depth > 64 || nodes > 200000 {
			return ErrInvalidPackage
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'):
			keys := make(map[string]bool)
			for d.More() {
				token, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				if !ok || keys[key] {
					return ErrInvalidPackage
				}
				keys[key] = true
				if err := read(depth + 1); err != nil {
					return err
				}
			}
			_, err := d.Token()
			return err
		case json.Delim('['):
			for d.More() {
				if err := read(depth + 1); err != nil {
					return err
				}
			}
			_, err := d.Token()
			return err
		default:
			return nil
		}
	}
	if read(0) != nil {
		return ErrInvalidPackage
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return ErrInvalidPackage
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return ErrInvalidPackage
	}
	return nil
}
