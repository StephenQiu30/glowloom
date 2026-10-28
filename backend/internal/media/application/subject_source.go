package application

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// SubjectSourceReader separates new adoption from an already authorized binding.
// ReadBoundSubjectSource is private server composition: its caller must first
// select an immutable binding under the subject owner's current authority.
type SubjectSourceReader interface {
	FindSubjectSource(context.Context, identity.Principal, domain.LibraryScope, uuid.UUID) (LibraryMediaFile, error)
	ReadBoundSubjectSource(context.Context, uuid.UUID) (LibraryMediaFile, error)
}

// SubjectSourceFact freezes the closed source owner as well as the original
// bytes. It is constructed from an authorized immutable subject binding.
type SubjectSourceFact struct {
	ReferenceFact
	ProjectID uuid.UUID
	Personal  *domain.PersonalOwnership
}

func (f SubjectSourceFact) matchesOwner(a domain.MediaAsset) bool {
	if f.Personal == nil {
		return f.ProjectID != uuid.Nil && a.Personal == nil && a.ProjectID == f.ProjectID
	}
	return f.ProjectID == uuid.Nil && f.Personal.OrgID != uuid.Nil && f.Personal.ActorID != uuid.Nil &&
		a.ProjectID == uuid.Nil && a.Personal != nil && *a.Personal == *f.Personal
}

// SubjectSourceQuery proves media without granting access to its source library.
type SubjectSourceQuery struct {
	reader       SubjectSourceReader
	objects      DocumentObjectGetter
	signer       PreviewSigner
	objectReader ObjectReader
	objectSigner ObjectSigner
}

// NewSubjectSourceQuery uses the existing single-profile object transport.
func NewSubjectSourceQuery(reader SubjectSourceReader, signer PreviewSigner, objects DocumentObjectGetter) *SubjectSourceQuery {
	return &SubjectSourceQuery{reader: reader, signer: signer, objects: objects}
}

// NewRoutedSubjectSourceQuery reads and signs the exact saved storage profile.
func NewRoutedSubjectSourceQuery(reader SubjectSourceReader, signer ObjectSigner, objects ObjectReader) *SubjectSourceQuery {
	return &SubjectSourceQuery{reader: reader, objectReader: objects, objectSigner: signer}
}

func subjectEligible(file LibraryMediaFile) bool {
	a := file.Asset
	return a.Validate() == nil && !a.IsDelete && a.Status == domain.StatusReady &&
		a.ModerationStatus == domain.ModerationPassed && !a.ContainsRealPerson && a.ConsentRecordID == nil &&
		(a.Kind == domain.KindImage || a.Kind == domain.KindVideo || a.Kind == domain.KindAudio) &&
		a.SHA256 != nil && copySHA(a.SHA256) && a.ByteSize > 0 && a.ByteSize <= libraryOriginalLimit(a.Kind)
}

// Reference verifies original bytes only after source-library authorization.
func (q *SubjectSourceQuery) Reference(ctx context.Context, actor identity.Principal, scope domain.LibraryScope, id uuid.UUID) (LibraryMediaFile, error) {
	if q == nil || q.reader == nil || scope.Validate() != nil || id == uuid.Nil {
		return LibraryMediaFile{}, ErrInvalidQuery
	}
	file, err := q.reader.FindSubjectSource(ctx, actor, scope, id)
	if err != nil {
		return LibraryMediaFile{}, err
	}
	a := file.Asset
	if a.ID != id || !subjectEligible(file) ||
		scope.Kind == domain.LibraryProject && (a.Personal != nil || a.ProjectID != *scope.ProjectID) ||
		scope.Kind == domain.LibraryPersonal && (a.ProjectID != uuid.Nil || a.Personal == nil || a.Personal.OrgID != actor.OrgID || a.Personal.ActorID != actor.ID) {
		return LibraryMediaFile{}, ErrNotFound
	}
	verify := &ReferenceFactQuery{objects: q.objects, objectReader: q.objectReader}
	if _, _, err := verify.verify(ctx, a.ObjectLocation, a.ObjectKey, &a.ByteSize, a.SHA256, libraryOriginalLimit(a.Kind)); err != nil {
		return LibraryMediaFile{}, errors.Join(ErrUnavailable, err)
	}
	return file, nil
}

// Bound rechecks only the exact asset previously selected by a subject binding.
// It does not expose a query or list of the original owner's private library.
func (q *SubjectSourceQuery) Bound(ctx context.Context, expected SubjectSourceFact) (LibraryMediaFile, error) {
	if q == nil || q.reader == nil || expected.AssetID == uuid.Nil || expected.Revision < 1 || expected.ByteSize < 1 || !copySHA(&expected.SHA256) {
		return LibraryMediaFile{}, ErrInvalidQuery
	}
	file, err := q.reader.ReadBoundSubjectSource(ctx, expected.AssetID)
	if err != nil {
		return LibraryMediaFile{}, err
	}
	a := file.Asset
	if !subjectEligible(file) || !expected.matchesOwner(a) || a.ID != expected.AssetID || a.Revision != expected.Revision || a.Kind != expected.Kind || a.SHA256 == nil || *a.SHA256 != expected.SHA256 || a.ByteSize != expected.ByteSize {
		return LibraryMediaFile{}, ErrNotFound
	}
	return file, nil
}

// VerifyBound proves original bytes before a saved binding is adopted into a new
// version. Metadata-only Bound remains available for repeated execution checks.
func (q *SubjectSourceQuery) VerifyBound(ctx context.Context, expected SubjectSourceFact) (LibraryMediaFile, error) {
	file, err := q.Bound(ctx, expected)
	if err != nil {
		return LibraryMediaFile{}, err
	}
	a := file.Asset
	verify := &ReferenceFactQuery{objects: q.objects, objectReader: q.objectReader}
	if _, _, err := verify.verify(ctx, a.ObjectLocation, a.ObjectKey, &a.ByteSize, a.SHA256, libraryOriginalLimit(a.Kind)); err != nil {
		return LibraryMediaFile{}, errors.Join(ErrUnavailable, err)
	}
	return file, nil
}

// SignBound returns a short-lived lease only after original byte verification.
func (q *SubjectSourceQuery) SignBound(ctx context.Context, expected SubjectSourceFact, ttl time.Duration) (AccessGrant, error) {
	file, err := q.VerifyBound(ctx, expected)
	if err != nil {
		return AccessGrant{}, err
	}
	return signMediaObject(ctx, file.Asset.ObjectLocation, file.Asset.ObjectKey, q.objectSigner, q.signer, ttl)
}

// DownloadBound verifies all bytes before exposing a preview or export stream.
// The caller owns the returned temporary file and must close it.
func (q *SubjectSourceQuery) DownloadBound(ctx context.Context, expected SubjectSourceFact) (*Downloaded, LibraryAssetSummary, error) {
	file, err := q.Bound(ctx, expected)
	if err != nil {
		return nil, LibraryAssetSummary{}, err
	}
	a := file.Asset
	reader, err := readMediaObject(ctx, a.ObjectLocation, a.ObjectKey, q.objectReader, q.objects)
	if err != nil || reader == nil {
		return nil, LibraryAssetSummary{}, errors.Join(ErrUnavailable, err)
	}
	defer func() { _ = reader.Close() }()
	tmp, err := os.CreateTemp("", "glowloom-subject-media-*")
	if err != nil {
		return nil, LibraryAssetSummary{}, err
	}
	out := &Downloaded{File: tmp, MIMEType: a.MimeType}
	keep := false
	defer func() {
		if !keep {
			_ = out.Close()
		}
	}()
	size, digest, err := copyObjectBytes(ctx, reader, tmp, &expected.ByteSize, &expected.SHA256)
	if err != nil {
		return nil, LibraryAssetSummary{}, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, LibraryAssetSummary{}, err
	}
	out.Size, out.SHA256 = size, digest
	keep = true
	return out, PersonalUploadSummary(a), nil
}
