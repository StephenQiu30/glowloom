package editproject

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// Renderer routes project packages without changing the existing media renderer.
type Renderer struct{ Media application.Renderer }

// NewRenderer wraps the established MP4/M4A implementation with exact project formats.
func NewRenderer(media application.Renderer) *Renderer { return &Renderer{Media: media} }
func document(frozen domain.FrozenExport) (string, []byte, error) {
	switch frozen.OutputKind {
	case domain.OutputFCPXML:
		data, e := fcpDocument(frozen)
		return "timeline.fcpxmld/Info.fcpxml", data, e
	case domain.OutputXMEML:
		data, e := xmemlDocument(frozen)
		return "timeline.xml", data, e
	default:
		return "", nil, nil
	}
}

// Render writes every verified original once and never serializes storage URLs.
func (r *Renderer) Render(ctx context.Context, frozen domain.FrozenExport, paths map[uuid.UUID]string, progress func(int, string) error) (*mediaapp.Downloaded, error) {
	if !frozen.OutputKind.IsArtifact() {
		if r == nil || r.Media == nil {
			return nil, application.ErrUnavailable
		}
		return r.Media.Render(ctx, frozen, paths, progress)
	}
	if err := Validate(frozen); err != nil {
		return nil, err
	}
	name, data, err := document(frozen)
	if err != nil {
		return nil, err
	}
	body, err := json.MarshalIndent(manifest(frozen), "", "  ")
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "glowloom-edit-project-*.zip")
	if err != nil {
		return nil, err
	}
	result := &mediaapp.Downloaded{File: file, MIMEType: "application/zip"}
	keep := false
	defer func() {
		if !keep {
			_ = result.Close()
		}
	}()
	hash := sha256.New()
	writer := &boundedWriter{Writer: io.MultiWriter(file, hash), remaining: MaxBytes}
	archive := zip.NewWriter(writer)
	add := func(name string, reader io.Reader) error {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(0600)
		header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
		entry, e := archive.CreateHeader(header)
		if e != nil {
			return e
		}
		_, e = io.Copy(entry, contextReader{ctx: ctx, reader: reader})
		return e
	}
	if err = add("manifest.json", bytes.NewReader(body)); err != nil {
		return nil, err
	}
	if name != "" {
		if err = add(name, bytes.NewReader(data)); err != nil {
			return nil, err
		}
	}
	for i, input := range frozen.Inputs {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if progress != nil {
			if err = progress(20+i*55/max(1, len(frozen.Inputs)), "packaging"); err != nil {
				return nil, err
			}
		}
		local, ok := paths[input.AssetID]
		if !ok || !filepath.IsAbs(local) {
			return nil, application.ErrInvalidExport
		}
		stat, e := os.Lstat(local)
		if e != nil || !stat.Mode().IsRegular() || stat.Size() != input.ByteSize {
			return nil, application.ErrInvalidExport
		}
		source, e := os.Open(local)
		if e != nil {
			return nil, e
		}
		checksum := sha256.New()
		err = add(mediaPath(frozen.OutputKind, sourceName(input)), io.TeeReader(io.LimitReader(source, input.ByteSize+1), checksum))
		closeErr := source.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if hex.EncodeToString(checksum.Sum(nil)) != input.SHA256 {
			return nil, fmt.Errorf("%w: original hash mismatch", application.ErrInvalidExport)
		}
	}
	if err = archive.Close(); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	result.Size = info.Size()
	result.SHA256 = hex.EncodeToString(hash.Sum(nil))
	if err = Verify(ctx, file, result.Size, frozen); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return result, nil
}

type boundedWriter struct {
	io.Writer
	remaining int64
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, fmt.Errorf("%w: package byte budget", application.ErrInvalidExport)
	}
	n, e := w.Writer.Write(p)
	w.remaining -= int64(n)
	return n, e
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Verify independently parses the container and XML, checks membership and exact hashes.
// It also validates a retried existing object against the original frozen input.
func Verify(ctx context.Context, file *os.File, size int64, frozen domain.FrozenExport) error {
	if err := Validate(frozen); err != nil {
		return err
	}
	if file == nil || size < 1 || size > MaxBytes {
		return application.ErrInvalidExport
	}
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return fmt.Errorf("%w: zip parse", application.ErrInvalidExport)
	}
	name, data, err := document(frozen)
	if err != nil {
		return err
	}
	manifestBytes, _ := json.MarshalIndent(manifest(frozen), "", "  ")
	expected := map[string][]byte{"manifest.json": manifestBytes}
	if name != "" {
		expected[name] = data
	}
	sources := make(map[string]domain.FrozenSource, len(frozen.Inputs))
	for _, input := range frozen.Inputs {
		sources[mediaPath(frozen.OutputKind, sourceName(input))] = input
	}
	if len(archive.File) != len(expected)+len(sources) {
		return fmt.Errorf("%w: archive membership", application.ErrInvalidExport)
	}
	seen := make(map[string]bool)
	for _, entry := range archive.File {
		if seen[entry.Name] || entry.Mode()&os.ModeSymlink != 0 || entry.Method != zip.Store || entry.UncompressedSize64 > uint64(MaxBytes) {
			return application.ErrInvalidExport
		}
		seen[entry.Name] = true
		reader, e := entry.Open()
		if e != nil {
			return e
		}
		if body, ok := expected[entry.Name]; ok {
			actual, e := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: reader}, maxDocumentBytes+1))
			_ = reader.Close()
			if e != nil {
				return e
			}
			if !bytes.Equal(actual, body) {
				return fmt.Errorf("%w: project document mismatch", application.ErrInvalidExport)
			}
			switch {
			case entry.Name == "manifest.json":
				var value Manifest
				if e = json.Unmarshal(actual, &value); e != nil || value.Source != *frozen.Source {
					return application.ErrInvalidExport
				}
			case frozen.OutputKind == domain.OutputFCPXML:
				var value fcpxml
				if e = xml.Unmarshal(actual, &value); e != nil || value.Version != "1.10" || len(value.Resources.Assets) != len(frozen.Inputs) {
					return application.ErrInvalidExport
				}
			default:
				var value xmeml
				if e = xml.Unmarshal(actual, &value); e != nil || value.Version != "5" {
					return application.ErrInvalidExport
				}
			}
		} else if input, ok := sources[entry.Name]; ok {
			checksum := sha256.New()
			count, e := io.Copy(checksum, io.LimitReader(contextReader{ctx: ctx, reader: reader}, input.ByteSize+1))
			_ = reader.Close()
			if e != nil {
				return e
			}
			if count != input.ByteSize || entry.UncompressedSize64 != uint64(input.ByteSize) || hex.EncodeToString(checksum.Sum(nil)) != input.SHA256 {
				return fmt.Errorf("%w: packaged original mismatch", application.ErrInvalidExport)
			}
		} else {
			_ = reader.Close()
			return application.ErrInvalidExport
		}
	}
	return ctx.Err()
}

// VerifyArtifact implements the worker's independent adoption gate.
func (r *Renderer) VerifyArtifact(ctx context.Context, file *os.File, size int64, frozen domain.FrozenExport) error {
	return Verify(ctx, file, size, frozen)
}
