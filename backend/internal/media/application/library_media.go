package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// LibraryMediaFile contains private owner facts only inside the process.
type LibraryMediaFile struct {
	Asset      domain.MediaAsset
	Renditions []domain.Rendition
}

// LibraryMediaReader rechecks the current closed scope, catalog visibility and
// original eligibility while holding the owning project/library/media locks.
type LibraryMediaReader interface {
	FindLibraryMedia(context.Context, identityapp.Principal, domain.LibraryScope, uuid.UUID) (LibraryMediaFile, error)
}

// LibraryRenditionPreview is an expiring lease, never durable catalog metadata.
type LibraryRenditionPreview struct {
	Kind      string    `json:"kind"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Width     *int32    `json:"width" extensions:"x-nullable"`
	Height    *int32    `json:"height" extensions:"x-nullable"`
}

// LibraryMediaPreview returns actual safe facts and short-lived private leases.
type LibraryMediaPreview struct {
	Asset      LibraryAssetSummary       `json:"asset"`
	URL        string                    `json:"url"`
	ExpiresAt  time.Time                 `json:"expires_at"`
	Renditions []LibraryRenditionPreview `json:"renditions"`
}

// LibraryMediaQuery shares the existing signer and original-byte object reader.
type LibraryMediaQuery struct {
	reader       LibraryMediaReader
	signer       PreviewSigner
	objects      DocumentObjectGetter
	objectSigner ObjectSigner
	objectReader ObjectReader
	previewTTL   time.Duration
}

// NewLibraryMediaQuery explicitly injects scope authorization and private I/O.
func NewLibraryMediaQuery(reader LibraryMediaReader, signer PreviewSigner, objects DocumentObjectGetter) *LibraryMediaQuery {
	return &LibraryMediaQuery{reader: reader, signer: signer, objects: objects, previewTTL: 10 * time.Minute}
}

// NewRoutedLibraryMediaQuery requires an explicit saved location for every object.
func NewRoutedLibraryMediaQuery(reader LibraryMediaReader, signer ObjectSigner, objects ObjectReader, ttl time.Duration) *LibraryMediaQuery {
	return &LibraryMediaQuery{reader: reader, objectSigner: signer, objectReader: objects, previewTTL: ttl}
}
func (q *LibraryMediaQuery) source(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, id uuid.UUID) (LibraryMediaFile, error) {
	if q == nil || q.reader == nil {
		return LibraryMediaFile{}, ErrUnavailable
	}
	if scope.Validate() != nil || id == uuid.Nil {
		return LibraryMediaFile{}, ErrInvalidQuery
	}
	file, err := q.reader.FindLibraryMedia(ctx, actor, scope, id)
	if err != nil {
		return LibraryMediaFile{}, err
	}
	a := file.Asset
	if a.ID != id || a.Validate() != nil || a.IsDelete || a.Status != domain.StatusReady || a.ModerationStatus != domain.ModerationPassed || a.ContainsRealPerson || a.ConsentRecordID != nil {
		return LibraryMediaFile{}, ErrNotFound
	}
	if scope.Kind == domain.LibraryProject && (a.Personal != nil || a.ProjectID != *scope.ProjectID) || scope.Kind == domain.LibraryPersonal && (a.ProjectID != uuid.Nil || a.Personal == nil || a.Personal.OrgID != actor.OrgID || a.Personal.ActorID != actor.ID) {
		return LibraryMediaFile{}, ErrNotFound
	}
	return file, nil
}

// Preview excludes documents and plaintext; the attachment route owns originals.
func (q *LibraryMediaQuery) Preview(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, id uuid.UUID) (LibraryMediaPreview, error) {
	file, err := q.source(ctx, actor, scope, id)
	if err != nil {
		return LibraryMediaPreview{}, err
	}
	if file.Asset.Kind == domain.KindDocument {
		return LibraryMediaPreview{}, ErrNotFound
	}
	grant, err := signMediaObject(ctx, file.Asset.ObjectLocation, file.Asset.ObjectKey, q.objectSigner, q.signer, q.previewTTL)
	if err != nil {
		return LibraryMediaPreview{}, err
	}
	result := LibraryMediaPreview{Asset: PersonalUploadSummary(file.Asset), URL: grant.URL, ExpiresAt: grant.ExpiresAt, Renditions: []LibraryRenditionPreview{}}
	for _, rend := range file.Renditions {
		grant, err := signMediaObject(ctx, rend.ObjectLocation, rend.ObjectKey, q.objectSigner, q.signer, q.previewTTL)
		if err != nil {
			return LibraryMediaPreview{}, err
		}
		result.Renditions = append(result.Renditions, LibraryRenditionPreview{Kind: string(rend.Kind), URL: grant.URL, ExpiresAt: grant.ExpiresAt, Width: rend.Width, Height: rend.Height})
	}
	if q.objectSigner != nil {
		current, err := q.source(ctx, actor, scope, id)
		if err != nil {
			return LibraryMediaPreview{}, err
		}
		if !sameLibraryLocations(current, file) {
			return LibraryMediaPreview{}, ErrNotFound
		}
	}
	if !result.ExpiresAt.After(time.Now()) {
		return LibraryMediaPreview{}, ErrUnavailable
	}
	for _, rendition := range result.Renditions {
		if !rendition.ExpiresAt.After(time.Now()) {
			return LibraryMediaPreview{}, ErrUnavailable
		}
	}
	return result, nil
}

// Download verifies the entire bounded original against its owning SHA before
// streaming an attachment. The caller owns and must close its temporary file.
func (q *LibraryMediaQuery) Download(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, id uuid.UUID) (*Downloaded, LibraryAssetSummary, error) {
	source, err := q.source(ctx, actor, scope, id)
	if err != nil {
		return nil, LibraryAssetSummary{}, err
	}
	a := source.Asset
	if q.objects == nil && q.objectReader == nil || a.SHA256 == nil || len(*a.SHA256) != 64 || a.ByteSize < 1 || a.ByteSize > libraryOriginalLimit(a.Kind) {
		return nil, LibraryAssetSummary{}, ErrUnavailable
	}
	name, err := SafeUploadFileName(a.FileName)
	if err != nil || name != a.FileName {
		return nil, LibraryAssetSummary{}, ErrUnavailable
	}
	reader, err := readMediaObject(ctx, a.ObjectLocation, a.ObjectKey, q.objectReader, q.objects)
	if err != nil || reader == nil {
		return nil, LibraryAssetSummary{}, fmt.Errorf("read library original: %w", errors.Join(ErrUnavailable, err))
	}
	defer func() { _ = reader.Close() }()
	file, err := os.CreateTemp("", "glowloom-library-original-*")
	if err != nil {
		return nil, LibraryAssetSummary{}, err
	}
	output := &Downloaded{File: file, MIMEType: a.MimeType}
	keep := false
	defer func() {
		if !keep {
			_ = output.Close()
		}
	}()
	size, sha, err := copyObjectBytes(ctx, reader, file, &a.ByteSize, a.SHA256)
	if err != nil {
		return nil, LibraryAssetSummary{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, LibraryAssetSummary{}, err
	}
	output.Size, output.SHA256 = size, sha
	keep = true
	return output, PersonalUploadSummary(a), nil
}
func libraryOriginalLimit(kind domain.Kind) int64 {
	switch kind {
	case domain.KindImage:
		return MaxUploadImageBytes
	case domain.KindVideo:
		return MaxUploadVideoBytes
	case domain.KindAudio:
		return MaxUploadAudioBytes
	case domain.KindModel:
		return domain.MaxModelBytes
	case domain.KindDocument:
		return domain.MaxDocumentBytes
	default:
		return 0
	}
}
