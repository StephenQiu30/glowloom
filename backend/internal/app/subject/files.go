package subject

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
	subjectdomain "github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

type files struct {
	db      *gorm.DB
	objects Objects
}

type verifiedStream struct {
	*os.File
	closeFile func() error
}

func (s *verifiedStream) Close() error { return s.closeFile() }

// NewFiles composes subject authorization with bounded media byte streams.
func NewFiles(db *gorm.DB, objects Objects) subject.Files { return files{db: db, objects: objects} }

func (f files) Media(ctx context.Context, a identity.Principal, id, version, binding uuid.UUID) (subject.File, error) {
	var out subject.File
	var downloaded *media.Downloaded
	err := f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bound, err := NewStore(tx, Objects{}).BoundMedia(ctx, a, binding)
		if err != nil {
			return err
		}
		if bound.SubjectID != id || bound.VersionID != version {
			return subject.ErrNotFound
		}
		file, summary, err := f.objects.query(tx, nil).DownloadBound(ctx, mediaFact(bound.Media))
		if err != nil {
			return err
		}
		downloaded = file
		out = subject.File{Reader: &verifiedStream{File: file.File, closeFile: file.Close}, Name: summary.FileName, MIMEType: summary.MIMEType, Size: file.Size}
		return nil
	})
	if err != nil && downloaded != nil {
		_ = downloaded.Close()
	}
	return out, err
}

type exportManifest struct {
	Format    string           `json:"format"`
	Version   int              `json:"version"`
	CreatedAt time.Time        `json:"created_at"`
	Subjects  []subject.Detail `json:"subjects"`
	Media     []exportMedia    `json:"media"`
}
type exportMedia struct {
	SubjectID uuid.UUID `json:"subject_id"`
	VersionID uuid.UUID `json:"version_id"`
	BindingID uuid.UUID `json:"binding_id"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
}

type removedTemp struct{ *os.File }

func (f *removedTemp) Close() error {
	name := f.Name()
	err := f.File.Close()
	remove := os.Remove(name)
	if err != nil {
		return err
	}
	return remove
}

// Export stages a bounded complete package before sending any response bytes.
func (f files) Export(ctx context.Context, a identity.Principal, in subject.ExportInput) (subject.File, error) {
	const maxBytes = int64(512 << 20)
	if len(in.Items) == 0 || len(in.Items) > 120 {
		return subject.File{}, subjectdomain.ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, item := range in.Items {
		if item.ID == uuid.Nil || item.VersionID == uuid.Nil || seen[item.ID] {
			return subject.File{}, subjectdomain.ErrInvalid
		}
		seen[item.ID] = true
	}
	tmp, err := os.CreateTemp("", "glowloom-subject-export-*.zip")
	if err != nil {
		return subject.File{}, err
	}
	staged := &removedTemp{File: tmp}
	keep := false
	defer func() {
		if !keep {
			_ = staged.Close()
		}
	}()
	archive := zip.NewWriter(tmp)
	manifest := exportManifest{Format: "glowloom.subjects", Version: 1, CreatedAt: time.Now().UTC(), Subjects: []subject.Detail{}, Media: []exportMedia{}}
	var budget int64
	for _, item := range in.Items {
		if err := ctx.Err(); err != nil {
			return subject.File{}, err
		}
		store := NewStore(f.db, Objects{})
		version, err := store.ReadVersion(ctx, a, item.ID, item.VersionID)
		if err != nil {
			return subject.File{}, err
		}
		current, err := store.Get(ctx, a, item.ID)
		if err != nil {
			return subject.File{}, err
		}
		head := subjectdomain.Head{ID: current.ID, OwnerScopeID: current.OwnerScopeID, Revision: current.Revision, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt}
		manifest.Subjects = append(manifest.Subjects, subject.Present(head, version, current.Favorite))
		bindings := []subjectdomain.Binding{version.Content.Primary}
		bindings = append(bindings, version.Content.References...)
		if version.Content.Voice != nil {
			bindings = append(bindings, version.Content.Voice.Binding)
		}
		for _, binding := range bindings {
			budget += binding.Media.ByteSize
			if budget > maxBytes {
				return subject.File{}, subjectdomain.ErrInvalid
			}
			file, err := f.Media(ctx, a, item.ID, item.VersionID, binding.ID)
			if err != nil {
				return subject.File{}, err
			}
			filename := binding.ID.String() + safeExtension(binding.Media.FileName)
			entryPath := path.Join("subjects", item.ID.String(), filename)
			writer, err := archive.Create(entryPath)
			if err == nil {
				var count int64
				count, err = io.Copy(writer, io.LimitReader(file.Reader, file.Size+1))
				if err == nil && count != file.Size {
					err = subject.ErrUnavailable
				}
			}
			closeErr := file.Reader.Close()
			if err != nil {
				return subject.File{}, err
			}
			if closeErr != nil {
				return subject.File{}, closeErr
			}
			manifest.Media = append(manifest.Media, exportMedia{SubjectID: item.ID, VersionID: item.VersionID, BindingID: binding.ID, Path: entryPath, SHA256: binding.Media.SHA256})
		}
	}
	entry, err := archive.Create("manifest.json")
	if err != nil {
		return subject.File{}, err
	}
	if err = json.NewEncoder(entry).Encode(manifest); err != nil {
		return subject.File{}, err
	}
	if err = archive.Close(); err != nil {
		return subject.File{}, err
	}
	// Reauthorize the complete selection after potentially slow object staging.
	for _, item := range in.Items {
		if _, err := NewStore(f.db, Objects{}).ReadVersion(ctx, a, item.ID, item.VersionID); err != nil {
			return subject.File{}, err
		}
	}
	stat, err := tmp.Stat()
	if err != nil {
		return subject.File{}, err
	}
	if _, err = tmp.Seek(0, io.SeekStart); err != nil {
		return subject.File{}, err
	}
	keep = true
	return subject.File{Reader: staged, Name: fmt.Sprintf("subjects-%s.zip", time.Now().UTC().Format("20060102-150405")), MIMEType: "application/zip", Size: stat.Size()}, nil
}

func safeExtension(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".mp4", ".mov", ".webm", ".mp3", ".wav", ".m4a", ".aac", ".ogg", ".flac":
		return ext
	default:
		return ".bin"
	}
}
