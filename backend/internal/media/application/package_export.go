package application

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"time"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

type packageOutputWriter struct {
	writer io.Writer
	size   int64
}

func (w *packageOutputWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > MaxPackageBytes-w.size {
		return 0, ErrInvalidPackage
	}
	n, err := w.writer.Write(data)
	w.size += int64(n)
	return n, err
}

// Export reads the complete authorized snapshot and every original's actual SHA.
// Missing/private unreadable bytes reject the whole ZIP, never a partial export.
func (s *LibraryPackageService) Export(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope) (*Downloaded, error) {
	if s == nil || s.repo == nil || s.objects == nil {
		return nil, ErrUnavailable
	}
	f, err := os.CreateTemp("", "lanverse-package-export-*.zip")
	if err != nil {
		return nil, err
	}
	out := &Downloaded{File: f, MIMEType: "application/zip"}
	keep := false
	defer func() {
		if !keep {
			_ = out.Close()
		}
	}()
	h := sha256.New()
	bounded := &packageOutputWriter{writer: io.MultiWriter(f, h)}
	w := zip.NewWriter(bounded)
	err = s.repo.InspectPackageExport(ctx, actor, scope, func(export PackageExport) error {
		if err := export.Manifest.validate(); err != nil {
			return err
		}
		manifest, err := json.Marshal(export.Manifest)
		if err != nil || len(manifest) > maxPackageManifestBytes {
			return ErrInvalidPackage
		}
		header := &zip.FileHeader{Name: "manifest.json", Method: zip.Deflate}
		header.Modified = export.Manifest.ExportedAt.Truncate(time.Second)
		entry, err := w.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err = entry.Write(manifest); err != nil {
			return err
		}
		for _, file := range export.Manifest.Files {
			object, ok := export.Objects[file.Path]
			if !ok || object.ByteSize != file.ByteSize || object.SHA256 != file.SHA256 {
				return ErrPackageIncomplete
			}
			actual, err := s.readPackageObject(ctx, object)
			if err != nil {
				return err
			}
			header := &zip.FileHeader{Name: file.Path, Method: zip.Store}
			header.Modified = export.Manifest.ExportedAt.Truncate(time.Second)
			entry, err := w.CreateHeader(header)
			if err == nil {
				_, err = io.Copy(entry, uploadContextReader{ctx: ctx, reader: actual.File})
			}
			closeErr := actual.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
		return w.Close()
	})
	if err != nil {
		return nil, err
	}
	out.Size = bounded.size
	out.SHA256 = hex.EncodeToString(h.Sum(nil))
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return out, nil
}
